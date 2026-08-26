// The screen's contents: what one cell holds, and how a line of them is
// handed out.
//
// A terminal's screen is a grid of cells, and a cell is one character
// position: the text drawn there and how it is drawn. Two details are less
// obvious than they look and shape everything below.
//
// A character can occupy two cells. East Asian ideographs and most emoji are
// drawn double-width, so they live in a cell whose `width` is 2, and the cell
// to their right is a placeholder holding no character of its own. This class
// marks the placeholder with `ch == 0`.
//
// A cell can hold more than one code point. A base letter followed by
// combining marks — an accent, or one of the modifiers that make up an emoji
// sequence — is one character position made of several code points, so the
// rare cell that needs them keeps the whole sequence in `extra` and reports
// the base in `ch`.
#pragma once

#include <QtCore/QList>
#include <QtCore/QString>

#include "kvitterm_global.h"

namespace kvitterm {

// One of the three ways a terminal names a colour. `Default` means the
// program asked for no colour and the palette's own foreground or background
// applies, which is what lets a colour scheme be changed without redrawing.
struct Color
{
    enum Kind : quint8 { Default, Indexed, Rgb };

    Kind kind = Default;
    quint8 index = 0;                  // 0-255, when kind == Indexed
    quint8 red = 0, green = 0, blue = 0;  // when kind == Rgb

    friend bool operator==(const Color &a, const Color &b)
    {
        if (a.kind != b.kind)
            return false;
        switch (a.kind) {
        case Default: return true;
        case Indexed: return a.index == b.index;
        case Rgb:     return a.red == b.red && a.green == b.green && a.blue == b.blue;
        }
        return false;
    }
    friend bool operator!=(const Color &a, const Color &b) { return !(a == b); }
};

enum class Underline : quint8 { None = 0, Single, Double, Curly };

// How a cell is drawn, apart from which character it holds. Two cells with
// equal styles can be drawn in one operation, which is what scrollback storage
// and the renderer both rely on.
struct Style
{
    Color foreground;
    Color background;
    Underline underline = Underline::None;
    bool bold = false;
    bool italic = false;
    bool blink = false;
    bool reverse = false;
    bool conceal = false;
    bool strike = false;

    friend bool operator==(const Style &a, const Style &b)
    {
        return a.foreground == b.foreground && a.background == b.background
            && a.underline == b.underline && a.bold == b.bold && a.italic == b.italic
            && a.blink == b.blink && a.reverse == b.reverse && a.conceal == b.conceal
            && a.strike == b.strike;
    }
    friend bool operator!=(const Style &a, const Style &b) { return !(a == b); }
};

struct KVITTERM_EXPORT Cell
{
    char32_t ch = U' ';   // 0 marks the right-hand half of a double-width character
    quint8 width = 1;     // 1 or 2; the right-hand half reports 0
    Style style;
    QString extra;        // the whole sequence when the cell holds combining marks

    QString text() const;
    bool isBlank() const { return extra.isEmpty() && (ch == U' ' || ch == 0); }

    friend bool operator==(const Cell &a, const Cell &b)
    {
        return a.ch == b.ch && a.width == b.width && a.style == b.style && a.extra == b.extra;
    }
    friend bool operator!=(const Cell &a, const Cell &b) { return !(a == b); }
};

// One line of the screen or of the scrollback.
//
// `cells` can be shorter than the terminal is wide: a scrolled-off line is
// stored with its trailing blanks removed, since a one-word line in a
// two-hundred-column terminal is otherwise a hundred and ninety-odd cells of
// nothing. Anything past the end is a blank cell in the default style.
//
// `continuation` says the line began as the overflow of the line above rather
// than at a newline of its own. It is what a re-wrap would need, and what
// `Screen::exportPlainText` uses to join a wrapped line back together.
struct KVITTERM_EXPORT Line
{
    QList<Cell> cells;
    bool continuation = false;

    Cell cellAt(int column) const { return column < cells.size() ? cells.at(column) : Cell{}; }
    QString text(int fromColumn = 0, int toColumn = -1) const;
    bool isBlank() const;
};

} // namespace kvitterm
