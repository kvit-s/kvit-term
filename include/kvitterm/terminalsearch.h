// Finding text in what a terminal has shown.
//
// Scanning is done over the screen and the scrollback together, on demand
// rather than as an index kept up to date: a search runs when somebody asks
// for one, and output arrives thousands of times more often than that.
#pragma once

#include <QtCore/QObject>
#include <QtQml/qqmlregistration.h>

#include "kvitterm_global.h"
#include "terminalsession.h"

namespace kvitterm {

struct KVITTERM_EXPORT SearchMatch
{
    int row = 0;       // as Screen numbers rows: negative is the scrollback
    int column = 0;
    int length = 0;
};

class KVITTERM_EXPORT TerminalSearch : public QObject
{
    Q_OBJECT
    QML_ELEMENT

    Q_PROPERTY(kvitterm::TerminalSession *session READ session WRITE setSession NOTIFY sessionChanged)
    Q_PROPERTY(QString query READ query WRITE setQuery NOTIFY queryChanged)
    Q_PROPERTY(bool caseSensitive READ isCaseSensitive WRITE setCaseSensitive
                       NOTIFY caseSensitiveChanged)
    Q_PROPERTY(bool regularExpression READ isRegularExpression WRITE setRegularExpression
                       NOTIFY regularExpressionChanged)
    Q_PROPERTY(int matchCount READ matchCount NOTIFY matchesChanged)
    // Which match is the current one, counting from zero; -1 when there is
    // none. `findNext` and `findPrevious` move it, wrapping at either end.
    Q_PROPERTY(int currentIndex READ currentIndex WRITE setCurrentIndex NOTIFY currentIndexChanged)
    Q_PROPERTY(int currentRow READ currentRow NOTIFY currentIndexChanged)

public:
    explicit TerminalSearch(QObject *parent = nullptr);
    ~TerminalSearch() override;

    TerminalSession *session() const;
    void setSession(TerminalSession *session);
    QString query() const;
    void setQuery(const QString &query);
    bool isCaseSensitive() const;
    void setCaseSensitive(bool caseSensitive);
    bool isRegularExpression() const;
    void setRegularExpression(bool regularExpression);

    int matchCount() const;
    int currentIndex() const;
    void setCurrentIndex(int index);
    int currentRow() const;
    QList<SearchMatch> matches() const;
    QList<SearchMatch> matchesOnRow(int row) const;

    // Run the search again. Matches are found once and then held, so a view
    // can draw them without rescanning; new output invalidates them.
    Q_INVOKABLE void refresh();
    Q_INVOKABLE void findNext();
    Q_INVOKABLE void findPrevious();
    Q_INVOKABLE void clear();

Q_SIGNALS:
    void sessionChanged();
    void queryChanged();
    void caseSensitiveChanged();
    void regularExpressionChanged();
    void matchesChanged();
    void currentIndexChanged();

private:
    class Private;
    Private *d;
};

} // namespace kvitterm
