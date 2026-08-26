// How a line that has scrolled off the screen is kept.
//
// Storing scrollback as a grid of cells is the obvious thing and it is
// expensive: a cell carrying a code point, two colours and its attributes is
// about twenty bytes, so ten thousand lines of two hundred columns would be
// forty megabytes for one terminal, nearly all of it blanks in the default
// style.
//
// So a stored line keeps its text as code points and its styling as runs —
// one entry per stretch of identical attributes, which for ordinary output is
// a handful per line — and drops trailing blanks entirely. A one-word line in
// a wide terminal costs the word.
#pragma once

#include <QtCore/QHash>
#include <QtCore/QList>

#include <string>

#include "kvitterm/cell.h"

namespace kvitterm {

struct StyleRun
{
    int start = 0;
    int length = 0;
    Style style;
};

struct PackedLine
{
    // One entry per cell. A null character marks the right-hand half of a
    // double-width character, exactly as the emulator reports it.
    std::u32string chars;
    QList<StyleRun> runs;
    // Only for the rare cell holding combining marks, keyed by cell index.
    QHash<int, QString> combining;
    bool continuation = false;

    int cellCount() const { return int(chars.size()); }
    qsizetype approximateBytes() const;
};

PackedLine packLine(const QList<Cell> &cells, bool continuation);
Line unpackLine(const PackedLine &packed);

} // namespace kvitterm
