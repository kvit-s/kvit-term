// The emulator, on top of libvterm.
//
// libvterm parses the escape sequences and keeps the visible screen; this
// class keeps everything libvterm deliberately leaves to its caller — the
// scrollback, the modes a view needs to know about, and the translation
// between Qt's idea of a key press and the bytes a terminal sends for it.
//
// One piece of bookkeeping here is less obvious than the rest, and §"wrapped
// lines" below explains it: which stored lines began as the overflow of the
// line above.
#include "kvitterm/screen.h"

#include "scrollback.h"

#include <QtCore/QVarLengthArray>

extern "C" {
#include <vterm.h>
}

namespace kvitterm {

namespace {

// libvterm marks the right-hand half of a double-width character with this
// value rather than with a separate flag.
constexpr uint32_t WideCharContinuation = uint32_t(-1);

Color colorFrom(const VTermColor &color)
{
    Color result;
    if (VTERM_COLOR_IS_DEFAULT_FG(&color) || VTERM_COLOR_IS_DEFAULT_BG(&color)) {
        result.kind = Color::Default;
    } else if (VTERM_COLOR_IS_INDEXED(&color)) {
        result.kind = Color::Indexed;
        result.index = color.indexed.idx;
    } else {
        result.kind = Color::Rgb;
        result.red = color.rgb.red;
        result.green = color.rgb.green;
        result.blue = color.rgb.blue;
    }
    return result;
}

Cell cellFrom(const VTermScreenCell &source)
{
    Cell cell;
    if (source.chars[0] == WideCharContinuation) {
        cell.ch = 0;
        cell.width = 0;
    } else if (source.chars[0] == 0) {
        cell.ch = U' ';
        cell.width = 1;
    } else {
        cell.ch = char32_t(source.chars[0]);
        cell.width = quint8(qMax(1, int(source.width)));
        if (source.chars[1] != 0) {
            // A base character with combining marks on it: one character
            // position made of several code points.
            QString text;
            for (int index = 0; index < VTERM_MAX_CHARS_PER_CELL && source.chars[index]; ++index)
                text += QString::fromUcs4(reinterpret_cast<const char32_t *>(&source.chars[index]), 1);
            cell.extra = text;
        }
    }

    cell.style.foreground = colorFrom(source.fg);
    cell.style.background = colorFrom(source.bg);
    cell.style.bold = source.attrs.bold;
    cell.style.italic = source.attrs.italic;
    cell.style.blink = source.attrs.blink;
    cell.style.reverse = source.attrs.reverse;
    cell.style.conceal = source.attrs.conceal;
    cell.style.strike = source.attrs.strike;
    switch (source.attrs.underline) {
    case VTERM_UNDERLINE_SINGLE: cell.style.underline = Underline::Single; break;
    case VTERM_UNDERLINE_DOUBLE: cell.style.underline = Underline::Double; break;
    case VTERM_UNDERLINE_CURLY:  cell.style.underline = Underline::Curly; break;
    default:                     cell.style.underline = Underline::None; break;
    }
    return cell;
}

VTermModifier modifiersFrom(Qt::KeyboardModifiers modifiers)
{
    int result = VTERM_MOD_NONE;
    if (modifiers & Qt::ShiftModifier)
        result |= VTERM_MOD_SHIFT;
    if (modifiers & Qt::AltModifier)
        result |= VTERM_MOD_ALT;
    if (modifiers & Qt::ControlModifier)
        result |= VTERM_MOD_CTRL;
    return VTermModifier(result);
}

// Qt names a key; a terminal sends a byte sequence for it, and which sequence
// depends on modes the program has selected. libvterm owns that decision, so
// all this does is name the same key in libvterm's terms.
bool vtermKeyFor(int qtKey, VTermKey *out)
{
    switch (qtKey) {
    case Qt::Key_Return:
    case Qt::Key_Enter:     *out = VTERM_KEY_ENTER; return true;
    case Qt::Key_Tab:       *out = VTERM_KEY_TAB; return true;
    case Qt::Key_Backspace: *out = VTERM_KEY_BACKSPACE; return true;
    case Qt::Key_Escape:    *out = VTERM_KEY_ESCAPE; return true;
    case Qt::Key_Up:        *out = VTERM_KEY_UP; return true;
    case Qt::Key_Down:      *out = VTERM_KEY_DOWN; return true;
    case Qt::Key_Left:      *out = VTERM_KEY_LEFT; return true;
    case Qt::Key_Right:     *out = VTERM_KEY_RIGHT; return true;
    case Qt::Key_Insert:    *out = VTERM_KEY_INS; return true;
    case Qt::Key_Delete:    *out = VTERM_KEY_DEL; return true;
    case Qt::Key_Home:      *out = VTERM_KEY_HOME; return true;
    case Qt::Key_End:       *out = VTERM_KEY_END; return true;
    case Qt::Key_PageUp:    *out = VTERM_KEY_PAGEUP; return true;
    case Qt::Key_PageDown:  *out = VTERM_KEY_PAGEDOWN; return true;
    default: break;
    }
    if (qtKey >= Qt::Key_F1 && qtKey <= Qt::Key_F12) {
        *out = VTermKey(VTERM_KEY_FUNCTION_0 + 1 + (qtKey - Qt::Key_F1));
        return true;
    }
    return false;
}

} // namespace

class ScreenPrivate
{
public:
    explicit ScreenPrivate(Screen *owner) : q(owner) {}

    // ── Callbacks. libvterm is C, so each one recovers `this` and forwards.
    static int onDamage(VTermRect rect, void *user)
    {
        auto *self = static_cast<ScreenPrivate *>(user);
        self->resyncWrapFlagsIfNeeded();
        Q_EMIT self->q->damaged(QRect(rect.start_col, rect.start_row,
                                      rect.end_col - rect.start_col,
                                      rect.end_row - rect.start_row));
        return 1;
    }

    static int onMoveRect(VTermRect dest, VTermRect src, void *user)
    {
        auto *self = static_cast<ScreenPrivate *>(user);
        self->resyncWrapFlagsIfNeeded();
        Q_EMIT self->q->damaged(QRect(dest.start_col, dest.start_row,
                                      dest.end_col - dest.start_col,
                                      dest.end_row - dest.start_row));
        return 1;
    }

    static int onMoveCursor(VTermPos pos, VTermPos, int visible, void *user)
    {
        auto *self = static_cast<ScreenPrivate *>(user);
        self->cursor = QPoint(pos.col, pos.row);
        self->cursorVisible = visible != 0;
        Q_EMIT self->q->cursorMoved(self->cursor);
        return 1;
    }

    static int onSetTermProp(VTermProp prop, VTermValue *value, void *user)
    {
        auto *self = static_cast<ScreenPrivate *>(user);
        switch (prop) {
        case VTERM_PROP_CURSORVISIBLE:
            self->cursorVisible = value->boolean;
            Q_EMIT self->q->cursorMoved(self->cursor);
            return 1;
        case VTERM_PROP_CURSORBLINK:
            self->cursorBlinks = value->boolean;
            return 1;
        case VTERM_PROP_ALTSCREEN:
            self->alternateScreen = value->boolean;
            Q_EMIT self->q->alternateScreenChanged(self->alternateScreen);
            return 1;
        case VTERM_PROP_REVERSE:
            self->reverseVideo = value->boolean;
            Q_EMIT self->q->damaged(QRect(0, 0, self->columns, self->rows));
            return 1;
        case VTERM_PROP_CURSORSHAPE:
            switch (value->number) {
            case VTERM_PROP_CURSORSHAPE_UNDERLINE: self->cursorShape = CursorShape::Underline; break;
            case VTERM_PROP_CURSORSHAPE_BAR_LEFT:  self->cursorShape = CursorShape::Bar; break;
            default:                               self->cursorShape = CursorShape::Block; break;
            }
            return 1;
        case VTERM_PROP_MOUSE:
            switch (value->number) {
            case VTERM_PROP_MOUSE_CLICK: self->mouseTracking = MouseTracking::Click; break;
            case VTERM_PROP_MOUSE_DRAG:  self->mouseTracking = MouseTracking::Drag; break;
            case VTERM_PROP_MOUSE_MOVE:  self->mouseTracking = MouseTracking::Motion; break;
            default:                     self->mouseTracking = MouseTracking::None; break;
            }
            Q_EMIT self->q->mouseTrackingChanged(self->mouseTracking);
            return 1;
        case VTERM_PROP_FOCUSREPORT:
            self->focusReporting = value->boolean;
            return 1;
        case VTERM_PROP_TITLE:
            self->collectFragment(self->titleFragment, value->string);
            if (value->string.final) {
                self->title = QString::fromUtf8(self->titleFragment);
                self->titleFragment.clear();
                Q_EMIT self->q->titleChanged(self->title);
            }
            return 1;
        default:
            return 0;
        }
    }

    static int onBell(void *user)
    {
        Q_EMIT static_cast<ScreenPrivate *>(user)->q->bell();
        return 1;
    }

    static int onResize(int rows, int cols, void *user)
    {
        auto *self = static_cast<ScreenPrivate *>(user);
        self->rows = rows;
        self->columns = cols;
        self->wrapFlags.resize(rows, false);
        Q_EMIT self->q->sizeChanged(cols, rows);
        return 1;
    }

    // ── Wrapped lines
    //
    // A stored line needs to remember whether it began as the overflow of the
    // line above, because that is what a re-wrap on resize has to join back
    // together. libvterm knows — it keeps a `continuation` flag per row — but
    // it does not pass it here, and by the time this runs the flag for the
    // line being stored has already been overwritten: a scroll shifts the
    // whole array of flags up before it calls back.
    //
    // What survives is everything else. A scroll of N lines destroys the first
    // N flags and shifts the rest down, so a mirror kept here can be refreshed
    // from what libvterm still holds after every scroll, and the one value
    // that refresh cannot supply — the line about to be stored — was recorded
    // by the refresh before it. Hence: consume the front of the mirror per
    // stored line, and re-read the whole mirror once the scroll has finished,
    // which is what `resyncWrapFlagsIfNeeded` does from the next callback.
    static int onPushLine(int cols, const VTermScreenCell *cells, void *user)
    {
        auto *self = static_cast<ScreenPrivate *>(user);
        QList<Cell> line;
        line.reserve(cols);
        for (int column = 0; column < cols; ++column)
            line.append(cellFrom(cells[column]));

        bool continuation = false;
        if (!self->wrapFlags.isEmpty()) {
            continuation = self->wrapFlags.takeFirst();
            self->wrapFlags.append(false);
        }
        self->wrapFlagsStale = true;

        self->scrollback.append(packLine(line, continuation));
        while (self->scrollback.size() > self->scrollbackLimit)
            self->scrollback.removeFirst();
        Q_EMIT self->q->scrolled(1);
        return 1;
    }

    static int onPopLine(int cols, VTermScreenCell *cells, void *user)
    {
        auto *self = static_cast<ScreenPrivate *>(user);
        if (self->scrollback.isEmpty())
            return 0;
        const Line line = unpackLine(self->scrollback.takeLast());
        for (int column = 0; column < cols; ++column) {
            const Cell cell = line.cellAt(column);
            VTermScreenCell &target = cells[column];
            target = {};
            target.width = char(cell.width == 0 ? 1 : cell.width);
            if (cell.ch == 0) {
                target.chars[0] = WideCharContinuation;
            } else if (!cell.extra.isEmpty()) {
                const QList<uint> points = cell.extra.toUcs4();
                int index = 0;
                for (; index < points.size() && index < VTERM_MAX_CHARS_PER_CELL; ++index)
                    target.chars[index] = points.at(index);
                if (index < VTERM_MAX_CHARS_PER_CELL)
                    target.chars[index] = 0;
            } else {
                target.chars[0] = uint32_t(cell.ch);
                target.chars[1] = 0;
            }
            applyStyle(cell.style, target);
        }
        Q_EMIT self->q->scrolled(-1);
        return 1;
    }

    static int onClearScrollback(void *user)
    {
        auto *self = static_cast<ScreenPrivate *>(user);
        self->scrollback.clear();
        Q_EMIT self->q->scrollbackCleared();
        return 1;
    }

    // Operating-system commands libvterm does not act on itself. The
    // shell-integration markers are all of this kind.
    static int onOsc(int command, VTermStringFragment fragment, void *user)
    {
        auto *self = static_cast<ScreenPrivate *>(user);
        if (fragment.initial)
            self->oscFragment.clear();
        self->collectFragment(self->oscFragment, fragment);
        if (fragment.final) {
            Q_EMIT self->q->osc(command, self->oscFragment);
            self->oscFragment.clear();
        }
        return 1;
    }

    static void onOutput(const char *bytes, size_t length, void *user)
    {
        auto *self = static_cast<ScreenPrivate *>(user);
        Q_EMIT self->q->writeRequested(QByteArray(bytes, qsizetype(length)));
    }

    static void applyStyle(const Style &style, VTermScreenCell &cell)
    {
        cell.attrs.bold = style.bold;
        cell.attrs.italic = style.italic;
        cell.attrs.blink = style.blink;
        cell.attrs.reverse = style.reverse;
        cell.attrs.conceal = style.conceal;
        cell.attrs.strike = style.strike;
        switch (style.underline) {
        case Underline::Single: cell.attrs.underline = VTERM_UNDERLINE_SINGLE; break;
        case Underline::Double: cell.attrs.underline = VTERM_UNDERLINE_DOUBLE; break;
        case Underline::Curly:  cell.attrs.underline = VTERM_UNDERLINE_CURLY; break;
        case Underline::None:   cell.attrs.underline = VTERM_UNDERLINE_OFF; break;
        }
        const auto assign = [](const Color &color, VTermColor &target, bool background) {
            switch (color.kind) {
            case Color::Default:
                target.type = uint8_t(background ? VTERM_COLOR_DEFAULT_BG : VTERM_COLOR_DEFAULT_FG);
                break;
            case Color::Indexed:
                vterm_color_indexed(&target, color.index);
                break;
            case Color::Rgb:
                vterm_color_rgb(&target, color.red, color.green, color.blue);
                break;
            }
        };
        assign(style.foreground, cell.fg, false);
        assign(style.background, cell.bg, true);
    }

    void collectFragment(QByteArray &target, const VTermStringFragment &fragment)
    {
        if (fragment.initial)
            target.clear();
        target.append(fragment.str, qsizetype(fragment.len));
    }

    void resyncWrapFlagsIfNeeded()
    {
        if (!wrapFlagsStale)
            return;
        wrapFlagsStale = false;
        syncWrapFlags();
    }

    void syncWrapFlags()
    {
        wrapFlags.resize(rows);
        for (int row = 0; row < rows; ++row) {
            const VTermLineInfo *info = vterm_state_get_lineinfo(state, row);
            wrapFlags[row] = info && info->continuation;
        }
    }

    Line visibleLine(int row) const
    {
        Line line;
        line.cells.reserve(columns);
        VTermScreenCell source;
        for (int column = 0; column < columns; ++column) {
            const VTermPos position = {row, column};
            if (!vterm_screen_get_cell(screen, position, &source))
                break;
            line.cells.append(cellFrom(source));
        }
        if (row >= 0 && row < wrapFlags.size())
            line.continuation = wrapFlags.at(row);
        return line;
    }

    // Re-wrap the stored lines for a new width. libvterm re-wraps the visible
    // screen itself; this is the history above it, which it leaves alone and
    // which would otherwise keep the line breaks of whatever width it was
    // written at.
    void reflowScrollback(int newColumns)
    {
        if (scrollback.isEmpty() || newColumns < 1)
            return;

        QList<PackedLine> rewrapped;
        rewrapped.reserve(scrollback.size());

        int index = 0;
        while (index < scrollback.size()) {
            // Gather one logical line: a line plus every continuation of it.
            QList<Cell> logical = unpackLine(scrollback.at(index)).cells;
            const bool startsAsContinuation = scrollback.at(index).continuation;
            int next = index + 1;
            while (next < scrollback.size() && scrollback.at(next).continuation) {
                logical += unpackLine(scrollback.at(next)).cells;
                ++next;
            }
            index = next;

            // Trailing blanks are an artefact of the width it was wrapped at.
            while (!logical.isEmpty() && logical.last().isBlank()
                   && logical.last().style == Style{}) {
                logical.removeLast();
            }

            if (logical.isEmpty()) {
                rewrapped.append(packLine({}, startsAsContinuation));
                continue;
            }

            bool continuation = startsAsContinuation;
            int position = 0;
            while (position < logical.size()) {
                int take = qMin(newColumns, int(logical.size()) - position);
                // Never split a double-width character down the middle.
                if (position + take < logical.size() && logical.at(position + take).ch == 0)
                    --take;
                if (take <= 0)
                    break;
                rewrapped.append(packLine(logical.mid(position, take), continuation));
                position += take;
                continuation = true;
            }
        }

        while (rewrapped.size() > scrollbackLimit)
            rewrapped.removeFirst();
        scrollback = rewrapped;
    }

    Screen *q;
    VTerm *vt = nullptr;
    VTermScreen *screen = nullptr;
    VTermState *state = nullptr;

    QList<PackedLine> scrollback;
    int scrollbackLimit = 10000;
    QList<bool> wrapFlags;
    bool wrapFlagsStale = false;

    int columns = 80;
    int rows = 24;
    QPoint cursor;
    bool cursorVisible = true;
    bool cursorBlinks = true;
    CursorShape cursorShape = CursorShape::Block;
    QString title;
    bool alternateScreen = false;
    bool reverseVideo = false;
    MouseTracking mouseTracking = MouseTracking::None;
    bool focusReporting = false;

    QByteArray titleFragment;
    QByteArray oscFragment;
};

namespace {

VTermScreenCallbacks screenCallbacks = {
    ScreenPrivate::onDamage,
    ScreenPrivate::onMoveRect,
    ScreenPrivate::onMoveCursor,
    ScreenPrivate::onSetTermProp,
    ScreenPrivate::onBell,
    ScreenPrivate::onResize,
    ScreenPrivate::onPushLine,
    ScreenPrivate::onPopLine,
    ScreenPrivate::onClearScrollback,
};

VTermStateFallbacks stateFallbacks = {
    nullptr,                  // control
    nullptr,                  // csi
    ScreenPrivate::onOsc,     // osc
    nullptr,                  // dcs
    nullptr,                  // apc
    nullptr,                  // pm
    nullptr,                  // sos
};

} // namespace

Screen::Screen(int columns, int rows, QObject *parent)
    : QObject(parent), d(new ScreenPrivate(this))
{
    d->columns = qMax(1, columns);
    d->rows = qMax(1, rows);

    d->vt = vterm_new(d->rows, d->columns);
    vterm_set_utf8(d->vt, 1);
    d->state = vterm_obtain_state(d->vt);
    d->screen = vterm_obtain_screen(d->vt);

    vterm_screen_set_callbacks(d->screen, &screenCallbacks, d);
    vterm_screen_set_unrecognised_fallbacks(d->screen, &stateFallbacks, d);
    vterm_output_set_callback(d->vt, ScreenPrivate::onOutput, d);
    vterm_screen_enable_altscreen(d->screen, 1);
    // Re-wrap the visible screen on resize rather than truncating it.
    vterm_screen_enable_reflow(d->screen, true);
    vterm_screen_reset(d->screen, 1);
    d->syncWrapFlags();
}

Screen::~Screen()
{
    vterm_free(d->vt);
    delete d;
}

void Screen::feed(const QByteArray &bytes)
{
    if (bytes.isEmpty())
        return;
    vterm_input_write(d->vt, bytes.constData(), size_t(bytes.size()));
    vterm_screen_flush_damage(d->screen);
    d->syncWrapFlags();
    d->wrapFlagsStale = false;
}

void Screen::reset(bool hard)
{
    vterm_screen_reset(d->screen, hard ? 1 : 0);
    d->syncWrapFlags();
    Q_EMIT damaged(QRect(0, 0, d->columns, d->rows));
}

int Screen::columns() const { return d->columns; }
int Screen::rows() const { return d->rows; }

void Screen::setSize(int columns, int rows)
{
    columns = qMax(1, columns);
    rows = qMax(1, rows);
    if (columns == d->columns && rows == d->rows)
        return;

    const int previousColumns = d->columns;
    // libvterm re-wraps the visible screen here, pushing what overflows the
    // top into the scrollback and pulling lines back when it grows.
    vterm_set_size(d->vt, rows, columns);
    vterm_screen_flush_damage(d->screen);
    d->columns = columns;
    d->rows = rows;
    if (columns != previousColumns)
        d->reflowScrollback(columns);
    d->syncWrapFlags();
    Q_EMIT damaged(QRect(0, 0, columns, rows));
}

int Screen::scrollbackCount() const { return int(d->scrollback.size()); }
int Screen::scrollbackLimit() const { return d->scrollbackLimit; }

void Screen::setScrollbackLimit(int lines)
{
    d->scrollbackLimit = qMax(0, lines);
    while (d->scrollback.size() > d->scrollbackLimit)
        d->scrollback.removeFirst();
}

void Screen::clearScrollback()
{
    d->scrollback.clear();
    Q_EMIT scrollbackCleared();
}

Line Screen::line(int row) const
{
    if (row >= 0)
        return d->visibleLine(row);
    const int index = int(d->scrollback.size()) + row;
    if (index < 0 || index >= d->scrollback.size())
        return {};
    return unpackLine(d->scrollback.at(index));
}

Cell Screen::cell(int row, int column) const
{
    return line(row).cellAt(column);
}

QString Screen::text(int fromRow, int toRow) const
{
    QString result;
    for (int row = fromRow; row <= toRow; ++row) {
        const Line current = line(row);
        // A line that was wrapped from the one above is joined back onto it,
        // so that copying a long command line gives the command rather than
        // the shape the window happened to have.
        if (row != fromRow && !current.continuation)
            result += QLatin1Char('\n');
        result += current.text();
    }
    return result;
}

QString Screen::textInRange(const QPoint &start, const QPoint &end, bool block) const
{
    QPoint from = start;
    QPoint to = end;
    if (to.y() < from.y() || (to.y() == from.y() && to.x() < from.x()))
        std::swap(from, to);

    QString result;
    for (int row = from.y(); row <= to.y(); ++row) {
        const Line current = line(row);
        int firstColumn = 0;
        int lastColumn = -1;
        if (block) {
            firstColumn = qMin(from.x(), to.x());
            lastColumn = qMax(from.x(), to.x());
        } else {
            if (row == from.y())
                firstColumn = from.x();
            if (row == to.y())
                lastColumn = to.x();
        }
        if (row != from.y())
            result += (!block && current.continuation) ? QString() : QStringLiteral("\n");
        result += current.text(firstColumn, lastColumn);
    }
    return result;
}

QPoint Screen::cursor() const { return d->cursor; }
bool Screen::cursorVisible() const { return d->cursorVisible; }
bool Screen::cursorBlinks() const { return d->cursorBlinks; }
CursorShape Screen::cursorShape() const { return d->cursorShape; }
QString Screen::title() const { return d->title; }
bool Screen::alternateScreen() const { return d->alternateScreen; }
bool Screen::reverseVideo() const { return d->reverseVideo; }
MouseTracking Screen::mouseTracking() const { return d->mouseTracking; }
bool Screen::focusReporting() const { return d->focusReporting; }

void Screen::keyPress(int qtKey, Qt::KeyboardModifiers modifiers, const QString &text)
{
    const VTermModifier mods = modifiersFrom(modifiers);
    VTermKey key = VTERM_KEY_NONE;
    if (vtermKeyFor(qtKey, &key)) {
        vterm_keyboard_key(d->vt, key, mods);
        return;
    }

    // Control combinations arrive from Qt with the control character already
    // in `text`; libvterm wants the letter and the modifier, and encodes the
    // rest itself, including the cases that need the modern CSI-u form.
    if ((modifiers & Qt::ControlModifier) && qtKey >= Qt::Key_A && qtKey <= Qt::Key_Z) {
        vterm_keyboard_unichar(d->vt, uint32_t(QChar(qtKey).toLower().unicode()), mods);
        return;
    }
    if (!text.isEmpty()) {
        const QList<uint> points = text.toUcs4();
        for (uint point : points) {
            if (point < 0x20 && !(modifiers & Qt::ControlModifier)) {
                // A control character typed directly, such as Return arriving
                // as a carriage return: pass it through untouched.
                const char byte = char(point);
                Q_EMIT writeRequested(QByteArray(&byte, 1));
                continue;
            }
            vterm_keyboard_unichar(d->vt, point, mods);
        }
        return;
    }
    if (qtKey == Qt::Key_Space)
        vterm_keyboard_unichar(d->vt, ' ', mods);
}

void Screen::sendText(const QString &text)
{
    for (uint point : text.toUcs4())
        vterm_keyboard_unichar(d->vt, point, VTERM_MOD_NONE);
}

void Screen::paste(const QString &text)
{
    // The markers are emitted only if the program asked for them, which is
    // what lets an editor tell pasted text from typed text and stop
    // auto-indenting it.
    vterm_keyboard_start_paste(d->vt);
    QString normalised = text;
    normalised.replace(QStringLiteral("\r\n"), QStringLiteral("\r"));
    normalised.replace(QLatin1Char('\n'), QLatin1Char('\r'));
    for (uint point : normalised.toUcs4())
        vterm_keyboard_unichar(d->vt, point, VTERM_MOD_NONE);
    vterm_keyboard_end_paste(d->vt);
}

void Screen::mouseMove(int row, int column, Qt::KeyboardModifiers modifiers)
{
    vterm_mouse_move(d->vt, row, column, modifiersFrom(modifiers));
}

void Screen::mouseButton(MouseButton button, bool pressed, Qt::KeyboardModifiers modifiers)
{
    vterm_mouse_button(d->vt, int(button), pressed, modifiersFrom(modifiers));
}

void Screen::setFocused(bool focused)
{
    if (focused)
        vterm_state_focus_in(d->state);
    else
        vterm_state_focus_out(d->state);
}

} // namespace kvitterm
