// Turning the colours a program asks for into colours that can be drawn.
//
// A terminal program names colours in three ways: one of sixteen names that
// every terminal renders differently, an index into a 256-entry table, or
// exact red-green-blue values. Only the last is unambiguous. This is where the
// other two are resolved, and it is why changing the colour scheme of a
// terminal does not require the programs inside it to redraw.
#pragma once

#include <QtGui/QColor>

#include "cell.h"
#include "kvitterm_global.h"

namespace kvitterm {

struct KVITTERM_EXPORT Palette
{
    QColor background{QStringLiteral("#12141a")};
    QColor foreground{QStringLiteral("#d5d8de")};
    QColor cursor{QStringLiteral("#d5d8de")};
    QColor cursorText{QStringLiteral("#12141a")};
    QColor selectionBackground{QStringLiteral("#2f4f7f")};
    QColor selectionForeground{};   // invalid means "keep the cell's own colour"

    // The sixteen named colours, in the order every terminal numbers them:
    // black, red, green, yellow, blue, magenta, cyan, white, then the same
    // eight again as the bright variants.
    QColor ansi[16] = {
        QColor(QStringLiteral("#22242c")), QColor(QStringLiteral("#e05561")),
        QColor(QStringLiteral("#8cc265")), QColor(QStringLiteral("#d18f52")),
        QColor(QStringLiteral("#4aa5f0")), QColor(QStringLiteral("#c162de")),
        QColor(QStringLiteral("#42b3c2")), QColor(QStringLiteral("#c7ccd6")),
        QColor(QStringLiteral("#4d4f57")), QColor(QStringLiteral("#ff616e")),
        QColor(QStringLiteral("#a5e075")), QColor(QStringLiteral("#f0a45d")),
        QColor(QStringLiteral("#4dc4ff")), QColor(QStringLiteral("#de73ff")),
        QColor(QStringLiteral("#4cd1e0")), QColor(QStringLiteral("#e6e6e6")),
    };

    // Resolve a cell colour. `asBackground` decides what `Color::Default`
    // means, and nothing else.
    QColor resolve(const Color &color, bool asBackground) const;

    // The 256-colour table: 0-15 are the named colours above, 16-231 are a
    // 6x6x6 colour cube, and 232-255 are a 24-step grey ramp.
    QColor indexed(int index) const;
};

} // namespace kvitterm
