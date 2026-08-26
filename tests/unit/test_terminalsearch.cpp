// Finding text in what a terminal has shown, screen and scrollback together.
#include <QtTest/QtTest>

#include "kvitterm/terminalsearch.h"
#include "kvitterm/terminalsession.h"

using namespace kvitterm;

class TestTerminalSearch : public QObject
{
    Q_OBJECT

private:
    static TerminalSession *sessionWithLines(QObject *parent, int count)
    {
        auto *session = new TerminalSession(parent);
        session->setAutoStart(false);
        session->resize(40, 5);
        for (int line = 1; line <= count; ++line) {
            session->screen()->feed(QStringLiteral("line %1 of output\r\n")
                                        .arg(line).toUtf8());
        }
        return session;
    }

private Q_SLOTS:
    void matchesAreFoundInTheScrollbackAsWellAsOnScreen()
    {
        QObject owner;
        TerminalSession *session = sessionWithLines(&owner, 20);
        QVERIFY(session->screen()->scrollbackCount() > 10);

        TerminalSearch search;
        search.setSession(session);
        search.setQuery(QStringLiteral("line 3 "));
        QCOMPARE(search.matchCount(), 1);
        // That line scrolled off long ago, so the match is in the history.
        QVERIFY(search.matches().at(0).row < 0);
    }

    void everyOccurrenceOnALineIsFound()
    {
        QObject owner;
        auto *session = new TerminalSession(&owner);
        session->setAutoStart(false);
        session->screen()->feed("aa bb aa bb aa\r\n");

        TerminalSearch search;
        search.setSession(session);
        search.setQuery(QStringLiteral("aa"));
        QCOMPARE(search.matchCount(), 3);
        QCOMPARE(search.matches().at(1).column, 6);
    }

    void caseIsIgnoredUnlessAskedFor()
    {
        QObject owner;
        auto *session = new TerminalSession(&owner);
        session->setAutoStart(false);
        session->screen()->feed("Error: ERROR error\r\n");

        TerminalSearch search;
        search.setSession(session);
        search.setQuery(QStringLiteral("error"));
        QCOMPARE(search.matchCount(), 3);
        search.setCaseSensitive(true);
        QCOMPARE(search.matchCount(), 1);
    }

    void aRegularExpressionCanBeUsed()
    {
        QObject owner;
        auto *session = new TerminalSession(&owner);
        session->setAutoStart(false);
        session->screen()->feed("test 1 passed\r\ntest 22 failed\r\n");

        TerminalSearch search;
        search.setSession(session);
        search.setRegularExpression(true);
        search.setQuery(QStringLiteral("test \\d+ failed"));
        QCOMPARE(search.matchCount(), 1);
        QCOMPARE(search.matches().at(0).length, int(QStringLiteral("test 22 failed").size()));
    }

    void anInvalidRegularExpressionFindsNothingRatherThanFailing()
    {
        QObject owner;
        auto *session = new TerminalSession(&owner);
        session->setAutoStart(false);
        session->screen()->feed("anything\r\n");

        TerminalSearch search;
        search.setSession(session);
        search.setRegularExpression(true);
        search.setQuery(QStringLiteral("("));
        QCOMPARE(search.matchCount(), 0);
    }

    void nextAndPreviousMoveThroughTheMatchesAndWrap()
    {
        QObject owner;
        TerminalSession *session = sessionWithLines(&owner, 6);

        TerminalSearch search;
        search.setSession(session);
        search.setQuery(QStringLiteral("output"));
        QCOMPARE(search.matchCount(), 6);
        QCOMPARE(search.currentIndex(), -1);

        // The first "next" goes to the most recent match, since that is the
        // one nearest what the user is looking at.
        search.findNext();
        QCOMPARE(search.currentIndex(), 5);
        search.findNext();
        QCOMPARE(search.currentIndex(), 0);
        search.findPrevious();
        QCOMPARE(search.currentIndex(), 5);
    }

    void newOutputIsSearchedToo()
    {
        QObject owner;
        auto *session = new TerminalSession(&owner);
        session->setAutoStart(false);
        session->screen()->feed("first needle\r\n");

        TerminalSearch search;
        search.setSession(session);
        search.setQuery(QStringLiteral("needle"));
        QCOMPARE(search.matchCount(), 1);

        session->screen()->feed("second needle\r\n");
        // Matches are held rather than recomputed on every line that arrives,
        // and asking again is what refreshes them.
        search.refresh();
        QCOMPARE(search.matchCount(), 2);
    }

    void anEmptyQueryMatchesNothing()
    {
        QObject owner;
        TerminalSession *session = sessionWithLines(&owner, 3);
        TerminalSearch search;
        search.setSession(session);
        search.setQuery(QStringLiteral("line"));
        QVERIFY(search.matchCount() > 0);
        search.clear();
        QCOMPARE(search.matchCount(), 0);
        QCOMPARE(search.currentIndex(), -1);
    }
};

QTEST_MAIN(TestTerminalSearch)
#include "test_terminalsearch.moc"
