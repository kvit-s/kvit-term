#include "kvitterm/screenexport.h"

#include "kvitterm/screen.h"

#include <QtCore/QTextStream>

namespace kvitterm {

namespace {

QString escaped(const QString &text)
{
    QString result;
    result.reserve(text.size());
    for (const QChar character : text) {
        switch (character.unicode()) {
        case u'&': result += QStringLiteral("&amp;"); break;
        case u'<': result += QStringLiteral("&lt;"); break;
        case u'>': result += QStringLiteral("&gt;"); break;
        default:   result += character; break;
        }
    }
    return result;
}

QString styleAttribute(const Style &style, const Palette &palette)
{
    // Reverse video swaps the two colours rather than setting a third one,
    // which is how a terminal draws a selected or highlighted region.
    QColor foreground = palette.resolve(style.foreground, false);
    QColor background = palette.resolve(style.background, true);
    if (style.reverse)
        std::swap(foreground, background);
    if (style.conceal)
        foreground = background;

    QStringList parts;
    parts << QStringLiteral("color:%1").arg(foreground.name());
    if (background != palette.background)
        parts << QStringLiteral("background-color:%1").arg(background.name());
    if (style.bold)
        parts << QStringLiteral("font-weight:bold");
    if (style.italic)
        parts << QStringLiteral("font-style:italic");
    if (style.underline != Underline::None && style.strike)
        parts << QStringLiteral("text-decoration:underline line-through");
    else if (style.underline != Underline::None)
        parts << QStringLiteral("text-decoration:underline");
    else if (style.strike)
        parts << QStringLiteral("text-decoration:line-through");
    return parts.join(QLatin1Char(';'));
}

} // namespace

QString exportPlainText(const Screen &screen, int firstRow, int lastRow)
{
    return screen.text(firstRow, lastRow);
}

QString exportHtml(const Screen &screen, int firstRow, int lastRow, const Palette &palette)
{
    QString html;
    QTextStream out(&html);
    out << QStringLiteral("<pre style=\"color:%1;background-color:%2;white-space:pre-wrap\">")
               .arg(palette.foreground.name(), palette.background.name());

    Line current;
    for (int row = firstRow; row <= lastRow; ++row) {
        if (row == firstRow)
            current = screen.line(row);
        // Kept for the next turn, since whether this row's last blanks belong
        // to the window or to a line running past it is the row below's to say.
        const Line below = row < lastRow ? screen.line(row + 1) : Line{};
        if (row != firstRow && !current.continuation)
            out << '\n';

        // One span per run of identical styling, which for ordinary output is
        // a handful per line rather than one per character.
        int column = 0;
        const int count = int(current.cells.size());
        while (column < count) {
            const Style &style = current.cells.at(column).style;
            int end = column;
            QString text;
            while (end < count && current.cells.at(end).style == style) {
                text += current.cells.at(end).text();
                ++end;
            }
            // The blanks closing a row that the line carries on past are
            // spaces inside it rather than the empty part of the window.
            if (end == count && !below.continuation) {
                while (text.endsWith(QLatin1Char(' ')))
                    text.chop(1);
            }
            if (!text.isEmpty()) {
                const QString attribute = styleAttribute(style, palette);
                if (attribute.isEmpty())
                    out << escaped(text);
                else
                    out << QStringLiteral("<span style=\"%1\">").arg(attribute)
                        << escaped(text) << QStringLiteral("</span>");
            }
            column = end;
        }
        current = below;
    }
    out << QStringLiteral("</pre>");
    return html;
}

} // namespace kvitterm
