#include "kvitterm/links.h"

#include <QtCore/QRegularExpression>

namespace kvitterm {

namespace {

// A web address, ended by whitespace or by one of the characters that
// commonly surrounds one rather than belonging to it.
const QRegularExpression &urlPattern()
{
    static const QRegularExpression pattern(
        QStringLiteral(R"((?:https?|ftp|file)://[^\s<>"'`\[\]{}|\\^]+)"));
    return pattern;
}

// A path with at least one separator in it, optionally followed by a line and
// a column: src/core/screen.cpp:42:7, ./build/log.txt, ~/notes/todo.md. A bare
// word is deliberately not a path, since most words in output are not.
const QRegularExpression &pathPattern()
{
    static const QRegularExpression pattern(
        QStringLiteral(R"((?<![\w/~.])((?:~|\.{1,2})?(?:/[\w.+-]+)+/?|(?:[\w.+-]+/)+[\w.+-]+)"
                       R"((?::(\d+))?(?::(\d+))?))"));
    return pattern;
}

// Trailing punctuation is nearly always the sentence's rather than the link's.
int trimTrailingPunctuation(const QString &text, int length)
{
    static const QString trailing = QStringLiteral(".,;:!?'\")]}");
    while (length > 0 && trailing.contains(text.at(length - 1))) {
        // A closing bracket that has an opening one inside the match belongs
        // to the link: en.wikipedia.org/wiki/Terminal_(disambiguation)
        const QChar last = text.at(length - 1);
        if (last == QLatin1Char(')') && text.left(length).contains(QLatin1Char('(')))
            break;
        --length;
    }
    return length;
}

} // namespace

QList<Link> findLinks(const QString &text)
{
    QList<Link> links;
    QList<QPair<int, int>> claimed;   // ranges a web address already covers

    auto urls = urlPattern().globalMatch(text);
    while (urls.hasNext()) {
        const QRegularExpressionMatch match = urls.next();
        Link link;
        link.kind = Link::Url;
        link.column = int(match.capturedStart());
        link.length = trimTrailingPunctuation(match.captured(), int(match.capturedLength()));
        link.text = text.mid(link.column, link.length);
        links.append(link);
        claimed.append({link.column, link.column + link.length});
    }

    auto paths = pathPattern().globalMatch(text);
    while (paths.hasNext()) {
        const QRegularExpressionMatch match = paths.next();
        const int start = int(match.capturedStart());

        // A run of numbers separated by slashes is a count rather than a
        // path: "Compiling 12/34", "3/10 tests passed". Every real path has
        // at least one segment that is not a bare number.
        const QString pathPart = match.captured(1);
        bool allNumeric = true;
        const QStringList segments = pathPart.split(QLatin1Char('/'), Qt::SkipEmptyParts);
        for (const QString &segment : segments) {
            bool numeric = false;
            segment.toLongLong(&numeric);
            if (!numeric) {
                allNumeric = false;
                break;
            }
        }
        if (allNumeric || segments.isEmpty())
            continue;
        bool insideUrl = false;
        for (const auto &range : std::as_const(claimed)) {
            if (start >= range.first && start < range.second) {
                insideUrl = true;
                break;
            }
        }
        if (insideUrl)
            continue;

        Link link;
        link.kind = Link::Path;
        link.column = start;
        link.length = int(match.capturedLength());
        link.text = match.captured();
        if (!match.captured(2).isEmpty())
            link.line = match.captured(2).toInt();
        if (!match.captured(3).isEmpty())
            link.character = match.captured(3).toInt();
        links.append(link);
    }

    std::sort(links.begin(), links.end(),
              [](const Link &a, const Link &b) { return a.column < b.column; });
    return links;
}

QList<Link> findLinks(const Line &line)
{
    return findLinks(line.text());
}

} // namespace kvitterm
