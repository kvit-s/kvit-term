#include "scrollback.h"

namespace kvitterm {

qsizetype PackedLine::approximateBytes() const
{
    return qsizetype(chars.size() * sizeof(char32_t))
         + runs.size() * qsizetype(sizeof(StyleRun))
         + combining.size() * 64;
}

PackedLine packLine(const QList<Cell> &cells, bool continuation)
{
    PackedLine packed;
    packed.continuation = continuation;
    packed.width = int(cells.size());

    // Trailing blanks in the default style are not stored: they are what the
    // renderer draws for anything past the end of a line anyway, and on a row
    // a longer line wrapped through `unpackLine` puts them back.
    int end = cells.size();
    const Style blankStyle;
    while (end > 0) {
        const Cell &last = cells.at(end - 1);
        if (!last.isBlank() || last.style != blankStyle)
            break;
        --end;
    }

    packed.chars.reserve(size_t(end));
    for (int index = 0; index < end; ++index) {
        const Cell &cell = cells.at(index);
        packed.chars.push_back(cell.ch);
        if (!cell.extra.isEmpty())
            packed.combining.insert(index, cell.extra);
        if (!packed.runs.isEmpty() && packed.runs.last().style == cell.style) {
            packed.runs.last().length += 1;
        } else {
            packed.runs.append(StyleRun{index, 1, cell.style});
        }
    }
    return packed;
}

Line unpackLine(const PackedLine &packed)
{
    Line line;
    line.continuation = packed.continuation;
    // The width it was stored at, not just the text that survived: the blanks
    // dropped from the end are cells of the line wherever a longer one wrapped
    // through this row.
    const int count = packed.cellCount();
    const int stored = int(packed.chars.size());
    line.cells.reserve(count);

    int runIndex = 0;
    for (int index = 0; index < stored; ++index) {
        while (runIndex + 1 < packed.runs.size()
               && index >= packed.runs.at(runIndex).start + packed.runs.at(runIndex).length) {
            ++runIndex;
        }
        Cell cell;
        cell.ch = packed.chars[size_t(index)];
        cell.width = cell.ch == 0 ? 0 : 1;
        if (runIndex < packed.runs.size()) {
            const StyleRun &run = packed.runs.at(runIndex);
            if (index >= run.start && index < run.start + run.length)
                cell.style = run.style;
        }
        const auto combining = packed.combining.constFind(index);
        if (combining != packed.combining.constEnd())
            cell.extra = *combining;
        line.cells.append(cell);
    }
    for (int index = stored; index < count; ++index)
        line.cells.append(Cell{});

    // A double-width character is stored as its code point followed by a null:
    // restore the width the renderer needs from that pairing.
    for (int index = 0; index + 1 < line.cells.size(); ++index) {
        if (line.cells.at(index + 1).ch == 0 && line.cells.at(index).ch != 0)
            line.cells[index].width = 2;
    }
    return line;
}

} // namespace kvitterm
