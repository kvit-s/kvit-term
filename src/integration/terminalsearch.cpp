#include "kvitterm/terminalsearch.h"

#include "kvitterm/screen.h"

#include <QtCore/QRegularExpression>

namespace kvitterm {

class TerminalSearch::Private
{
public:
    void scan();

    TerminalSearch *q = nullptr;
    TerminalSession *session = nullptr;
    QString query;
    bool caseSensitive = false;
    bool regularExpression = false;
    bool stale = true;
    QList<SearchMatch> matches;
    int currentIndex = -1;
};

void TerminalSearch::Private::scan()
{
    stale = false;
    const int previousCount = int(matches.size());
    matches.clear();
    if (!session || query.isEmpty()) {
        if (previousCount)
            Q_EMIT q->matchesChanged();
        return;
    }

    const Screen *screen = session->screen();
    const Qt::CaseSensitivity sensitivity = caseSensitive ? Qt::CaseSensitive
                                                          : Qt::CaseInsensitive;
    QRegularExpression pattern;
    if (regularExpression) {
        pattern.setPattern(query);
        if (!caseSensitive)
            pattern.setPatternOptions(QRegularExpression::CaseInsensitiveOption);
        if (!pattern.isValid()) {
            if (previousCount)
                Q_EMIT q->matchesChanged();
            return;
        }
    }

    // Oldest first, so that "next" moves the way reading does.
    for (int row = -screen->scrollbackCount(); row < screen->rows(); ++row) {
        const QString text = screen->line(row).text();
        if (text.isEmpty())
            continue;
        if (regularExpression) {
            auto found = pattern.globalMatch(text);
            while (found.hasNext()) {
                const QRegularExpressionMatch match = found.next();
                if (match.capturedLength() == 0)
                    continue;
                matches.append({row, int(match.capturedStart()), int(match.capturedLength())});
            }
        } else {
            int from = 0;
            for (;;) {
                const int at = text.indexOf(query, from, sensitivity);
                if (at < 0)
                    break;
                matches.append({row, at, int(query.size())});
                from = at + int(query.size());
            }
        }
    }
    Q_EMIT q->matchesChanged();
}

TerminalSearch::TerminalSearch(QObject *parent) : QObject(parent), d(new Private)
{
    d->q = this;
}

TerminalSearch::~TerminalSearch()
{
    delete d;
}

TerminalSession *TerminalSearch::session() const { return d->session; }

void TerminalSearch::setSession(TerminalSession *session)
{
    if (d->session == session)
        return;
    if (d->session)
        d->session->disconnect(this);
    d->session = session;
    if (d->session) {
        // New output makes the held matches wrong; they are found again when
        // somebody next asks for them rather than on every line that arrives.
        connect(d->session, &TerminalSession::contentChanged, this, [this] { d->stale = true; });
    }
    d->stale = true;
    Q_EMIT sessionChanged();
}

QString TerminalSearch::query() const { return d->query; }

void TerminalSearch::setQuery(const QString &query)
{
    if (d->query == query)
        return;
    d->query = query;
    d->stale = true;
    d->currentIndex = -1;
    Q_EMIT queryChanged();
    refresh();
}

bool TerminalSearch::isCaseSensitive() const { return d->caseSensitive; }

void TerminalSearch::setCaseSensitive(bool caseSensitive)
{
    if (d->caseSensitive == caseSensitive)
        return;
    d->caseSensitive = caseSensitive;
    d->stale = true;
    Q_EMIT caseSensitiveChanged();
    refresh();
}

bool TerminalSearch::isRegularExpression() const { return d->regularExpression; }

void TerminalSearch::setRegularExpression(bool regularExpression)
{
    if (d->regularExpression == regularExpression)
        return;
    d->regularExpression = regularExpression;
    d->stale = true;
    Q_EMIT regularExpressionChanged();
    refresh();
}

int TerminalSearch::matchCount() const
{
    if (d->stale)
        const_cast<Private *>(d)->scan();
    return int(d->matches.size());
}

int TerminalSearch::currentIndex() const { return d->currentIndex; }

void TerminalSearch::setCurrentIndex(int index)
{
    const int count = matchCount();
    index = count == 0 ? -1 : qBound(0, index, count - 1);
    if (index == d->currentIndex)
        return;
    d->currentIndex = index;
    Q_EMIT currentIndexChanged();
}

int TerminalSearch::currentRow() const
{
    if (d->currentIndex < 0 || d->currentIndex >= d->matches.size())
        return 0;
    return d->matches.at(d->currentIndex).row;
}

QList<SearchMatch> TerminalSearch::matches() const
{
    if (d->stale)
        const_cast<Private *>(d)->scan();
    return d->matches;
}

QList<SearchMatch> TerminalSearch::matchesOnRow(int row) const
{
    QList<SearchMatch> found;
    for (const SearchMatch &match : matches()) {
        if (match.row == row)
            found.append(match);
    }
    return found;
}

void TerminalSearch::refresh()
{
    d->scan();
    if (d->currentIndex >= d->matches.size())
        setCurrentIndex(int(d->matches.size()) - 1);
}

void TerminalSearch::findNext()
{
    const int count = matchCount();
    if (count == 0)
        return;
    setCurrentIndex(d->currentIndex < 0 ? count - 1 : (d->currentIndex + 1) % count);
}

void TerminalSearch::findPrevious()
{
    const int count = matchCount();
    if (count == 0)
        return;
    setCurrentIndex(d->currentIndex <= 0 ? count - 1 : d->currentIndex - 1);
}

void TerminalSearch::clear()
{
    setQuery(QString());
}

} // namespace kvitterm
