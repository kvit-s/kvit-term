#include "kvitterm/cell.h"

namespace kvitterm {

QString Cell::text() const
{
    if (!extra.isEmpty())
        return extra;
    if (ch == 0)             // the right-hand half of a double-width character
        return {};
    return QString::fromUcs4(&ch, 1);
}

QString Line::text(int fromColumn, int toColumn, TrailingBlanks trailing) const
{
    const int last = cells.size() - 1;
    const bool toEndOfLine = toColumn < 0 || toColumn >= last;
    if (toEndOfLine)
        toColumn = last;

    QString result;
    for (int column = qMax(0, fromColumn); column <= toColumn; ++column)
        result += cells.at(column).text();

    // Trailing blanks are the width of the window rather than anything the
    // program wrote, so a copied line does not carry them — but only when the
    // range asked for reaches the end of the line, since spaces in the middle
    // of a selection are the user's. A caller joining the rows of a wrapped
    // line asks to keep them for that same reason: on a row the line carries
    // on past, they are spaces inside it.
    if (toEndOfLine && trailing == TrailingBlanks::Drop) {
        while (result.endsWith(QLatin1Char(' ')))
            result.chop(1);
    }
    return result;
}

bool Line::isBlank() const
{
    for (const Cell &cell : cells) {
        if (!cell.isBlank())
            return false;
    }
    return true;
}

} // namespace kvitterm
