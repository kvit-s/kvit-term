#include "kvitterm/terminalview.h"

#include "kvitterm/links.h"
#include "kvitterm/screen.h"

#include <QtCore/QTimer>
#include <QtGui/QClipboard>
#include <QtGui/QCursor>
#include <QtGui/QFontDatabase>
#include <QtGui/QFontMetricsF>
#include <QtGui/QGuiApplication>
#include <QtGui/QKeyEvent>
#include <QtGui/QPainter>
#include <QtGui/QTextLayout>

#include <cmath>

namespace kvitterm {

namespace {

// Characters a double-click treats as part of one word. Paths and URLs are
// what people actually double-click in a terminal, so the punctuation they
// contain counts as word material.
bool isWordCharacter(QChar character)
{
    return character.isLetterOrNumber() || QStringLiteral("_-./:@~+=%#?&").contains(character);
}

// A code point that a monospace font does not necessarily advance by one
// cell: the ideographs, the emoji, and everything else that is drawn double
// width. These are positioned cell by cell rather than let run.
bool needsIndividualPlacement(char32_t ch)
{
    return ch >= 0x1100;
}

} // namespace

class TerminalView::Private
{
public:
    explicit Private(TerminalView *owner) : q(owner) {}

    Screen *screen() const { return session ? session->screen() : nullptr; }
    const Palette &colours() const { return paletteObject->palette(); }

    void recomputeMetrics()
    {
        const QFontMetricsF metrics(font);
        // Integers, so that a column always lands on the same pixel: at
        // fractional widths the hundredth column is half a pixel off and the
        // grid visibly shears.
        cellWidth = qMax(1, int(std::ceil(metrics.horizontalAdvance(QLatin1Char('M')))));
        cellHeight = qMax(1, int(std::ceil(metrics.height())));
        baseline = metrics.ascent();

        boldFont = font;
        boldFont.setBold(true);
        italicFont = font;
        italicFont.setItalic(true);
        boldItalicFont = boldFont;
        boldItalicFont.setItalic(true);
    }

    void updateGrid()
    {
        if (cellWidth <= 0 || cellHeight <= 0)
            return;
        const int newColumns = qMax(1, int(q->width()) / cellWidth);
        const int newRows = qMax(1, int(q->height()) / cellHeight);
        if (newColumns == columns && newRows == rows)
            return;
        columns = newColumns;
        rows = newRows;
        if (session)
            session->resize(columns, rows);
        Q_EMIT q->gridChanged();
        q->update();
    }

    QPoint cellAt(const QPointF &position) const
    {
        const int column = qBound(0, int(position.x()) / cellWidth, qMax(0, columns - 1));
        const int viewRow = qBound(0, int(position.y()) / cellHeight, qMax(0, rows - 1));
        return QPoint(column, viewRow - scrollOffset);
    }

    bool selectionContains(int row, int column) const
    {
        if (!hasSelection)
            return false;
        QPoint from = selectionAnchor;
        QPoint to = selectionHead;
        if (to.y() < from.y() || (to.y() == from.y() && to.x() < from.x()))
            std::swap(from, to);
        if (row < from.y() || row > to.y())
            return false;
        if (row == from.y() && column < from.x())
            return false;
        if (row == to.y() && column > to.x())
            return false;
        return true;
    }

    void setSelection(const QPoint &anchor, const QPoint &head)
    {
        selectionAnchor = anchor;
        selectionHead = head;
        hasSelection = anchor != head;
        Q_EMIT q->selectionChanged();
        q->update();
    }

    // A double-click takes the word under the pointer; what counts as a word
    // is above.
    void selectWordAt(const QPoint &cell)
    {
        Screen *s = screen();
        if (!s)
            return;
        const Line line = s->line(cell.y());
        int first = cell.x();
        int last = cell.x();
        const auto characterAt = [&line](int column) {
            const QString text = line.cellAt(column).text();
            return text.isEmpty() ? QChar(u' ') : text.at(0);
        };
        if (!isWordCharacter(characterAt(cell.x())))
            return;
        while (first > 0 && isWordCharacter(characterAt(first - 1)))
            --first;
        while (last + 1 < line.cells.size() && isWordCharacter(characterAt(last + 1)))
            ++last;
        setSelection(QPoint(first, cell.y()), QPoint(last, cell.y()));
    }

    // The link under a cell, if any. Wrapped lines are joined first, or an
    // address split across the wrap is missed.
    bool linkAt(const QPoint &cell, Link *found) const
    {
        Screen *s = screen();
        if (!s)
            return false;
        int row = cell.y();
        int offset = 0;
        QString text = s->line(row).text();
        // Walk back over continuations so that the whole logical line is
        // searched, remembering how far into it this row starts.
        int first = row;
        while (first > -s->scrollbackCount() && s->line(first).continuation)
            --first;
        if (first != row) {
            QString joined;
            for (int r = first; r <= row; ++r) {
                if (r == row)
                    offset = int(joined.size());
                joined += s->line(r).text();
            }
            text = joined;
        }
        const int column = offset + cell.x();
        for (const Link &link : findLinks(text)) {
            if (column >= link.column && column < link.column + link.length) {
                *found = link;
                return true;
            }
        }
        return false;
    }

    void updateStickyCommand()
    {
        if (!shellIntegration) {
            if (!stickyCommand.isEmpty()) {
                stickyCommand.clear();
                Q_EMIT q->stickyCommandChanged();
            }
            return;
        }
        // The command whose output is at the top of the window, but only when
        // its own line is above it: while the command line itself is visible
        // there is nothing to repeat.
        const int topRow = -scrollOffset;
        const int index = shellIntegration->commandAtScreenRow(topRow);
        QString text;
        if (index >= 0) {
            const QVariantMap command = shellIntegration->commandAt(index);
            const int firstRow = command.value(QStringLiteral("outputFirstRow")).toInt();
            Q_UNUSED(firstRow);
            text = command.value(QStringLiteral("text")).toString();
        }
        if (text == stickyCommand)
            return;
        stickyCommand = text;
        Q_EMIT q->stickyCommandChanged();
    }

    void noteDamage(const QRect &region)
    {
        // Backpressure: a process writing faster than the interface can draw
        // would otherwise queue an unbounded amount of work. Reading stops
        // until the next repaint, which fills the pipe and blocks the child.
        ++damageSinceLastPaint;
        Q_EMIT q->accessibleTextChanged();
        updateStickyCommand();
        if (session && damageSinceLastPaint > 400 && !session->isReadingSuspended())
            session->setReadingSuspended(true);

        if (scrollOffset != 0)
            return;   // what changed is not on screen
        const QRect pixels(region.x() * cellWidth, (region.y() + scrollOffset) * cellHeight,
                           region.width() * cellWidth, region.height() * cellHeight);
        q->update(pixels.adjusted(-1, -1, 1, 1));
    }

    TerminalView *q;
    TerminalSession *session = nullptr;
    TerminalPalette *paletteObject = nullptr;
    TerminalPalette defaultPalette;

    QFont font;
    QFont boldFont;
    QFont italicFont;
    QFont boldItalicFont;
    int cellWidth = 8;
    int cellHeight = 16;
    qreal baseline = 12;

    int columns = 80;
    int rows = 24;
    int scrollOffset = 0;
    int damageSinceLastPaint = 0;

    QStringList reservedShortcuts;
    QPoint selectionAnchor;
    QPoint selectionHead;
    bool hasSelection = false;
    bool selecting = false;

    TerminalSearch *search = nullptr;
    ShellIntegration *shellIntegration = nullptr;
    QString stickyCommand;
    QString hoveredLink;
    int hoveredLinkLine = -1;
    int hoveredLinkCharacter = -1;
    QRect hoveredLinkCells;

    QTimer *blinkTimer = nullptr;
    bool cursorPhase = true;
    QString preedit;
};

TerminalView::TerminalView(QQuickItem *parent) : QQuickPaintedItem(parent), d(new Private(this))
{
    setFlag(ItemHasContents, true);
    setFlag(ItemAcceptsInputMethod, true);
    setFlag(ItemIsFocusScope, true);
    setAcceptedMouseButtons(Qt::LeftButton | Qt::MiddleButton | Qt::RightButton);
    setAcceptHoverEvents(true);
    setActiveFocusOnTab(true);

    // The system's own fixed-width font, so that a terminal looks like the
    // platform's terminal before anybody configures anything.
    d->font = QFontDatabase::systemFont(QFontDatabase::FixedFont);
    d->font.setStyleHint(QFont::Monospace);
    d->font.setFixedPitch(true);
    d->paletteObject = &d->defaultPalette;
    d->recomputeMetrics();

    d->blinkTimer = new QTimer(this);
    d->blinkTimer->setInterval(600);
    connect(d->blinkTimer, &QTimer::timeout, this, [this] {
        d->cursorPhase = !d->cursorPhase;
        Screen *screen = d->screen();
        if (screen)
            update(cellRect(screen->cursor().x(), screen->cursor().y()).toAlignedRect());
    });
}

TerminalView::~TerminalView()
{
    delete d;
}

TerminalSession *TerminalView::session() const { return d->session; }

void TerminalView::setSession(TerminalSession *session)
{
    if (d->session == session)
        return;
    if (d->session)
        d->session->disconnect(this);
    d->session = session;
    if (d->session) {
        Screen *screen = d->session->screen();
        connect(screen, &Screen::damaged, this, [this](const QRect &region) { d->noteDamage(region); });
        connect(screen, &Screen::scrolled, this, [this] {
            if (d->scrollOffset > 0) {
                // Hold the view still while the user is reading back through
                // the history rather than dragging them to the bottom.
                d->scrollOffset = qMin(d->scrollOffset + 1, d->screen()->scrollbackCount());
                Q_EMIT scrollOffsetChanged();
            }
            update();
        });
        connect(screen, &Screen::cursorMoved, this, [this] { update(); });
        connect(screen, &Screen::scrollbackCleared, this, [this] {
            d->scrollOffset = 0;
            update();
        });
        d->session->resize(d->columns, d->rows);
    }
    Q_EMIT sessionChanged();
    update();
}

TerminalPalette *TerminalView::palette() const { return d->paletteObject; }

void TerminalView::setPalette(TerminalPalette *palette)
{
    if (d->paletteObject == palette)
        return;
    if (d->paletteObject)
        d->paletteObject->disconnect(this);
    d->paletteObject = palette ? palette : &d->defaultPalette;
    connect(d->paletteObject, &TerminalPalette::changed, this, [this] { update(); });
    Q_EMIT paletteChanged();
    update();
}

QFont TerminalView::font() const { return d->font; }

void TerminalView::setFont(const QFont &font)
{
    if (d->font == font)
        return;
    d->font = font;
    d->recomputeMetrics();
    d->updateGrid();
    Q_EMIT fontChanged();
    update();
}

int TerminalView::scrollOffset() const { return d->scrollOffset; }

void TerminalView::setScrollOffset(int offset)
{
    Screen *screen = d->screen();
    const int maximum = screen ? screen->scrollbackCount() : 0;
    offset = qBound(0, offset, maximum);
    if (offset == d->scrollOffset)
        return;
    d->scrollOffset = offset;
    Q_EMIT scrollOffsetChanged();
    update();
}

int TerminalView::columns() const { return d->columns; }
int TerminalView::rows() const { return d->rows; }

int TerminalView::scrollbackCount() const
{
    Screen *screen = d->screen();
    return screen ? screen->scrollbackCount() : 0;
}

QStringList TerminalView::reservedShortcuts() const { return d->reservedShortcuts; }

void TerminalView::setReservedShortcuts(const QStringList &shortcuts)
{
    if (d->reservedShortcuts == shortcuts)
        return;
    d->reservedShortcuts = shortcuts;
    Q_EMIT reservedShortcutsChanged();
}

bool TerminalView::hasSelection() const { return d->hasSelection; }

QString TerminalView::selectedText() const
{
    Screen *screen = d->screen();
    if (!screen || !d->hasSelection)
        return {};
    return screen->textInRange(d->selectionAnchor, d->selectionHead);
}

TerminalSearch *TerminalView::search() const { return d->search; }

void TerminalView::setSearch(TerminalSearch *search)
{
    if (d->search == search)
        return;
    if (d->search)
        d->search->disconnect(this);
    d->search = search;
    if (d->search) {
        connect(d->search, &TerminalSearch::matchesChanged, this, [this] { update(); });
        connect(d->search, &TerminalSearch::currentIndexChanged, this, [this] {
            // Bring the current match into view, leaving a few lines of
            // context above it rather than pinning it to the top edge.
            if (d->search->currentIndex() < 0) {
                update();
                return;
            }
            const int row = d->search->currentRow();
            if (row < -d->scrollOffset || row >= d->rows - d->scrollOffset)
                setScrollOffset(-row + qMin(3, d->rows / 4));
            update();
        });
    }
    Q_EMIT searchChanged();
    update();
}

ShellIntegration *TerminalView::shellIntegration() const { return d->shellIntegration; }

void TerminalView::setShellIntegration(ShellIntegration *integration)
{
    if (d->shellIntegration == integration)
        return;
    if (d->shellIntegration)
        d->shellIntegration->disconnect(this);
    d->shellIntegration = integration;
    if (d->shellIntegration) {
        connect(d->shellIntegration, &ShellIntegration::commandsChanged, this,
                [this] { d->updateStickyCommand(); });
    }
    Q_EMIT shellIntegrationChanged();
    d->updateStickyCommand();
}

QString TerminalView::stickyCommand() const { return d->stickyCommand; }
QString TerminalView::hoveredLink() const { return d->hoveredLink; }

QString TerminalView::accessibleText() const
{
    Screen *screen = d->screen();
    return screen ? screen->text(-d->scrollOffset, d->rows - 1 - d->scrollOffset) : QString();
}

void TerminalView::copy()
{
    const QString text = selectedText();
    if (!text.isEmpty())
        QGuiApplication::clipboard()->setText(text);
}

void TerminalView::paste()
{
    if (d->session)
        d->session->paste(QGuiApplication::clipboard()->text());
}

void TerminalView::selectAll()
{
    Screen *screen = d->screen();
    if (!screen)
        return;
    d->setSelection(QPoint(0, -screen->scrollbackCount()),
                    QPoint(screen->columns() - 1, screen->rows() - 1));
}

void TerminalView::clearSelection()
{
    if (!d->hasSelection)
        return;
    d->hasSelection = false;
    Q_EMIT selectionChanged();
    update();
}

void TerminalView::scrollBy(int lines)
{
    setScrollOffset(d->scrollOffset + lines);
}

void TerminalView::scrollToBottom()
{
    setScrollOffset(0);
}

QRectF TerminalView::cellRect(int column, int row) const
{
    return QRectF((row + d->scrollOffset) >= 0 ? column * d->cellWidth : 0,
                  (row + d->scrollOffset) * d->cellHeight, d->cellWidth, d->cellHeight);
}

void TerminalView::paint(QPainter *painter)
{
    const Palette &colours = d->colours();
    painter->fillRect(boundingRect(), colours.background);

    Screen *screen = d->screen();
    if (!screen) {
        d->damageSinceLastPaint = 0;
        return;
    }

    painter->setRenderHint(QPainter::TextAntialiasing, true);

    for (int viewRow = 0; viewRow < d->rows; ++viewRow) {
        const int screenRow = viewRow - d->scrollOffset;
        const Line line = screen->line(screenRow);
        const int top = viewRow * d->cellHeight;

        // Backgrounds first, merged into runs, so that a line of one colour
        // is one fill rather than eighty.
        int column = 0;
        while (column < d->columns) {
            const Cell cell = line.cellAt(column);
            const bool selected = d->selectionContains(screenRow, column);
            QColor background = colours.resolve(cell.style.background, true);
            if (cell.style.reverse)
                background = colours.resolve(cell.style.foreground, false);
            if (selected)
                background = colours.selectionBackground;

            int end = column + 1;
            while (end < d->columns) {
                const Cell next = line.cellAt(end);
                const bool nextSelected = d->selectionContains(screenRow, end);
                QColor nextBackground = colours.resolve(next.style.background, true);
                if (next.style.reverse)
                    nextBackground = colours.resolve(next.style.foreground, false);
                if (nextSelected)
                    nextBackground = colours.selectionBackground;
                if (nextBackground != background)
                    break;
                ++end;
            }
            if (background != colours.background) {
                painter->fillRect(QRect(column * d->cellWidth, top,
                                        (end - column) * d->cellWidth, d->cellHeight),
                                  background);
            }
            column = end;
        }

        // Search matches sit above the cell backgrounds and below the text,
        // with the current one brighter than the rest.
        if (d->search && d->search->matchCount() > 0) {
            const QList<SearchMatch> found = d->search->matchesOnRow(screenRow);
            const int currentIndex = d->search->currentIndex();
            const int currentRow = d->search->currentRow();
            for (const SearchMatch &match : found) {
                const bool isCurrent = currentIndex >= 0 && match.row == currentRow
                                       && match.column == d->search->matches()
                                                              .at(currentIndex).column;
                QColor highlight = colours.ansi[isCurrent ? 11 : 3];
                highlight.setAlpha(isCurrent ? 220 : 120);
                painter->fillRect(QRect(match.column * d->cellWidth, top,
                                        match.length * d->cellWidth, d->cellHeight),
                                  highlight);
            }
        }

        // Then the text, in runs of identical styling, breaking wherever a
        // character cannot be trusted to advance by exactly one cell.
        column = 0;
        while (column < d->columns) {
            const Cell cell = line.cellAt(column);
            const QString text = cell.text();
            if (text.isEmpty() || cell.style.conceal) {
                ++column;
                continue;
            }

            const bool individual = needsIndividualPlacement(cell.ch) || !cell.extra.isEmpty();
            QString run = text;
            int end = column + 1;
            if (!individual) {
                while (end < d->columns) {
                    const Cell next = line.cellAt(end);
                    if (next.style != cell.style || next.text().isEmpty()
                        || needsIndividualPlacement(next.ch) || !next.extra.isEmpty()) {
                        break;
                    }
                    run += next.text();
                    ++end;
                }
            }

            QColor foreground = colours.resolve(cell.style.foreground, false);
            if (cell.style.reverse)
                foreground = colours.resolve(cell.style.background, true);
            if (d->selectionContains(screenRow, column)
                && colours.selectionForeground.isValid()) {
                foreground = colours.selectionForeground;
            }

            const QFont &runFont = cell.style.bold && cell.style.italic ? d->boldItalicFont
                                 : cell.style.bold                     ? d->boldFont
                                 : cell.style.italic                   ? d->italicFont
                                                                       : d->font;
            painter->setFont(runFont);
            painter->setPen(foreground);
            painter->drawText(QPointF(column * d->cellWidth, top + d->baseline), run);

            if (d->hoveredLinkCells.height() > 0 && screenRow == d->hoveredLinkCells.y()
                && column < d->hoveredLinkCells.right() && end > d->hoveredLinkCells.left()) {
                const int from = qMax(column, d->hoveredLinkCells.left());
                const int to = qMin(end, d->hoveredLinkCells.right());
                const int y = top + int(d->baseline) + 2;
                painter->drawLine(from * d->cellWidth, y, to * d->cellWidth, y);
            }
            if (cell.style.underline != Underline::None) {
                const int y = top + int(d->baseline) + 2;
                painter->drawLine(column * d->cellWidth, y, end * d->cellWidth, y);
                if (cell.style.underline == Underline::Double)
                    painter->drawLine(column * d->cellWidth, y + 2, end * d->cellWidth, y + 2);
            }
            if (cell.style.strike) {
                const int y = top + int(d->baseline * 0.65);
                painter->drawLine(column * d->cellWidth, y, end * d->cellWidth, y);
            }
            column = end;
        }
    }

    // The cursor, last, and only where it is actually visible: scrolled back
    // through the history there is nothing to point at.
    if (screen->cursorVisible() && d->scrollOffset == 0 && (d->cursorPhase || !hasActiveFocus())) {
        const QPoint position = screen->cursor();
        const QRect box(position.x() * d->cellWidth, position.y() * d->cellHeight,
                        d->cellWidth, d->cellHeight);
        if (!hasActiveFocus()) {
            // An unfocused terminal shows an outline, so that it is obvious
            // which one keystrokes are going to.
            painter->setPen(colours.cursor);
            painter->drawRect(box.adjusted(0, 0, -1, -1));
        } else {
            switch (screen->cursorShape()) {
            case CursorShape::Underline:
                painter->fillRect(QRect(box.x(), box.bottom() - 1, box.width(), 2), colours.cursor);
                break;
            case CursorShape::Bar:
                painter->fillRect(QRect(box.x(), box.y(), 2, box.height()), colours.cursor);
                break;
            case CursorShape::Block: {
                painter->fillRect(box, colours.cursor);
                const QString text = screen->cell(position.y(), position.x()).text();
                if (!text.isEmpty()) {
                    painter->setPen(colours.cursorText);
                    painter->setFont(d->font);
                    painter->drawText(QPointF(box.x(), box.y() + d->baseline), text);
                }
                break;
            }
            }
        }
    }

    d->damageSinceLastPaint = 0;
    if (d->session && d->session->isReadingSuspended())
        d->session->setReadingSuspended(false);
}

void TerminalView::geometryChange(const QRectF &newGeometry, const QRectF &oldGeometry)
{
    QQuickPaintedItem::geometryChange(newGeometry, oldGeometry);
    d->updateGrid();
}

void TerminalView::keyPressEvent(QKeyEvent *event)
{
    const QKeySequence sequence(event->keyCombination());
    for (const QString &reserved : std::as_const(d->reservedShortcuts)) {
        if (QKeySequence::fromString(reserved) == sequence) {
            // Not this item's key: the application asked for it.
            event->ignore();
            return;
        }
    }

    // The item's own shortcuts, which a reserved sequence above overrides.
    if (event->modifiers().testFlag(Qt::ControlModifier)
        && event->modifiers().testFlag(Qt::ShiftModifier)) {
        switch (event->key()) {
        case Qt::Key_C: copy(); event->accept(); return;
        case Qt::Key_V: paste(); event->accept(); return;
        case Qt::Key_A: selectAll(); event->accept(); return;
        default: break;
        }
    }
    if (event->modifiers().testFlag(Qt::ShiftModifier)) {
        switch (event->key()) {
        case Qt::Key_PageUp:   scrollBy(d->rows - 1); event->accept(); return;
        case Qt::Key_PageDown: scrollBy(-(d->rows - 1)); event->accept(); return;
        default: break;
        }
    }

    if (!d->session) {
        event->ignore();
        return;
    }
    // Typing brings the view back to where new output appears, which is what
    // every terminal does and what makes scrolling back safe.
    scrollToBottom();
    d->session->keyPress(event->key(), event->modifiers(), event->text());
    event->accept();
}

void TerminalView::mousePressEvent(QMouseEvent *event)
{
    forceActiveFocus();
    Screen *screen = d->screen();
    if (!screen) {
        event->ignore();
        return;
    }

    const QPoint cell = d->cellAt(event->position());

    if (event->button() == Qt::MiddleButton) {
        // The X11 convention: the middle button pastes the current selection
        // rather than the clipboard.
        const QClipboard *clipboard = QGuiApplication::clipboard();
        const QString text = clipboard->supportsSelection()
                ? clipboard->text(QClipboard::Selection)
                : clipboard->text();
        if (d->session && !text.isEmpty())
            d->session->paste(text);
        event->accept();
        return;
    }

    // A program that asked to be told about the mouse gets told, unless the
    // user holds shift, which is the escape hatch every terminal offers for
    // selecting text inside a full-screen program.
    if (screen->mouseTracking() != MouseTracking::None
        && !event->modifiers().testFlag(Qt::ShiftModifier)) {
        screen->mouseMove(cell.y(), cell.x(), event->modifiers());
        screen->mouseButton(event->button() == Qt::RightButton ? MouseButton::Right
                                                               : MouseButton::Left,
                            true, event->modifiers());
        event->accept();
        return;
    }

    if (event->button() == Qt::LeftButton && event->modifiers().testFlag(Qt::ControlModifier)) {
        Link found;
        if (d->linkAt(cell, &found)) {
            Q_EMIT linkActivated(found.text, found.line, found.character);
            event->accept();
            return;
        }
    }

    clearSelection();
    d->selecting = true;
    d->setSelection(cell, cell);
    event->accept();
}

void TerminalView::mouseMoveEvent(QMouseEvent *event)
{
    Screen *screen = d->screen();
    if (!screen) {
        event->ignore();
        return;
    }
    const QPoint cell = d->cellAt(event->position());
    if (d->selecting) {
        d->setSelection(d->selectionAnchor, cell);
        // Dragging past the top or bottom edge scrolls, so that a selection
        // can reach further than the window.
        if (event->position().y() < 0)
            scrollBy(1);
        else if (event->position().y() > height())
            scrollBy(-1);
        event->accept();
        return;
    }
    if (screen->mouseTracking() == MouseTracking::Drag
        || screen->mouseTracking() == MouseTracking::Motion) {
        screen->mouseMove(cell.y(), cell.x(), event->modifiers());
        event->accept();
        return;
    }
    event->ignore();
}

void TerminalView::mouseReleaseEvent(QMouseEvent *event)
{
    Screen *screen = d->screen();
    if (screen && !d->selecting && screen->mouseTracking() != MouseTracking::None
        && !event->modifiers().testFlag(Qt::ShiftModifier)) {
        const QPoint cell = d->cellAt(event->position());
        screen->mouseMove(cell.y(), cell.x(), event->modifiers());
        screen->mouseButton(event->button() == Qt::RightButton ? MouseButton::Right
                                                               : MouseButton::Left,
                            false, event->modifiers());
        event->accept();
        return;
    }

    d->selecting = false;
    if (d->hasSelection) {
        // On X11 a selection is immediately available to the middle button
        // without any copy step, which is what people expect there.
        QClipboard *clipboard = QGuiApplication::clipboard();
        if (clipboard->supportsSelection())
            clipboard->setText(selectedText(), QClipboard::Selection);
    }
    event->accept();
}

void TerminalView::mouseDoubleClickEvent(QMouseEvent *event)
{
    d->selectWordAt(d->cellAt(event->position()));
    event->accept();
}

void TerminalView::hoverMoveEvent(QHoverEvent *event)
{
    Screen *screen = d->screen();
    if (!screen) {
        event->ignore();
        return;
    }
    if (screen->mouseTracking() == MouseTracking::Motion) {
        const QPoint cell = d->cellAt(event->position());
        screen->mouseMove(cell.y(), cell.x(), event->modifiers());
    }

    // Links light up only while the modifier that activates them is held, so
    // that ordinary output does not turn into a field of underlines.
    QString link;
    QRect cells;
    int line = -1;
    int character = -1;
    if (event->modifiers().testFlag(Qt::ControlModifier)) {
        const QPoint cell = d->cellAt(event->position());
        Link found;
        if (d->linkAt(cell, &found)) {
            link = found.text;
            line = found.line;
            character = found.character;
            // The match is in the joined logical line; place the underline on
            // the row under the pointer, which is the row the user sees.
            cells = QRect(cell.x() - 1, cell.y(), found.length + 2, 1);
        }
    }
    if (link != d->hoveredLink) {
        d->hoveredLink = link;
        d->hoveredLinkLine = line;
        d->hoveredLinkCharacter = character;
        d->hoveredLinkCells = cells;
        setCursor(QCursor(link.isEmpty() ? Qt::IBeamCursor : Qt::PointingHandCursor));
        Q_EMIT hoveredLinkChanged();
        update();
    }
    event->ignore();
}

void TerminalView::wheelEvent(QWheelEvent *event)
{
    Screen *screen = d->screen();
    if (!screen) {
        event->ignore();
        return;
    }
    const int notches = event->angleDelta().y() / 120;
    if (notches == 0) {
        event->ignore();
        return;
    }

    if (screen->mouseTracking() != MouseTracking::None
        && !event->modifiers().testFlag(Qt::ShiftModifier)) {
        const MouseButton button = notches > 0 ? MouseButton::WheelUp : MouseButton::WheelDown;
        for (int step = 0; step < qAbs(notches); ++step) {
            screen->mouseButton(button, true, event->modifiers());
            screen->mouseButton(button, false, event->modifiers());
        }
        event->accept();
        return;
    }

    if (screen->alternateScreen()) {
        // A full-screen program has no scrollback of its own, so the wheel
        // becomes the arrow keys, which is what makes a pager scroll.
        const int key = notches > 0 ? Qt::Key_Up : Qt::Key_Down;
        for (int step = 0; step < qAbs(notches) * 3; ++step)
            screen->keyPress(key, Qt::NoModifier, QString());
        event->accept();
        return;
    }

    scrollBy(notches * 3);
    event->accept();
}

void TerminalView::focusInEvent(QFocusEvent *event)
{
    QQuickPaintedItem::focusInEvent(event);
    if (Screen *screen = d->screen())
        screen->setFocused(true);
    d->cursorPhase = true;
    d->blinkTimer->start();
    update();
}

void TerminalView::focusOutEvent(QFocusEvent *event)
{
    QQuickPaintedItem::focusOutEvent(event);
    if (Screen *screen = d->screen())
        screen->setFocused(false);
    d->blinkTimer->stop();
    d->cursorPhase = true;
    update();
}

void TerminalView::inputMethodEvent(QInputMethodEvent *event)
{
    // Composed text — an input method for Chinese, Japanese or Korean, or a
    // dead key on a European layout — arrives here rather than as key presses.
    if (!event->commitString().isEmpty() && d->session)
        d->session->sendText(event->commitString());
    d->preedit = event->preeditString();
    event->accept();
    update();
}

QVariant TerminalView::inputMethodQuery(Qt::InputMethodQuery query) const
{
    Screen *screen = d->screen();
    switch (query) {
    case Qt::ImEnabled:
        return true;
    case Qt::ImFont:
        return d->font;
    case Qt::ImCursorRectangle:
        if (screen)
            return cellRect(screen->cursor().x(), screen->cursor().y());
        return QRectF();
    case Qt::ImHints:
        return int(Qt::ImhMultiLine | Qt::ImhNoPredictiveText);
    default:
        return QQuickPaintedItem::inputMethodQuery(query);
    }
}

} // namespace kvitterm
