// The whole chain: a child process on a pseudo-terminal, its output
// interpreted onto a screen, and the answers going back. Each layer has its
// own suite; this one proves they are wired together.
#include <QtCore/QFileInfo>
#include <QtTest/QtTest>

#include "kvitterm/terminalsession.h"

using namespace kvitterm;

class TestSession : public QObject
{
    Q_OBJECT

private:
    static TerminalSession *stubSession(QObject *parent, const QStringList &arguments)
    {
        auto *session = new TerminalSession(parent);
        session->setAutoStart(false);
        session->setProgram(QString::fromLocal8Bit(KVITTERM_STUB_PATH));
        session->setArguments(arguments);
        return session;
    }

private Q_SLOTS:
    void aProgramsColoursSurviveTheWholeChain()
    {
        QObject owner;
        TerminalSession *session = stubSession(&owner, {QStringLiteral("scenario"),
                                                        QStringLiteral("colour")});
        QVERIFY(session->start());
        QTRY_VERIFY(session->screenText().contains(QStringLiteral("truecolour")));

        const Screen *screen = session->screen();
        const int column = session->lineText(0).indexOf(QStringLiteral("red"));
        QVERIFY(column >= 0);
        QCOMPARE(screen->cell(0, column).style.foreground.kind, Color::Indexed);
        QCOMPARE(int(screen->cell(0, column).style.foreground.index), 1);
    }

    void aProgressBarLeavesOneLineRatherThanFifty()
    {
        // Through a pipe this arrives as five lines with escape sequences in
        // them; through a terminal it is one line, redrawn.
        QObject owner;
        TerminalSession *session = stubSession(&owner, {QStringLiteral("scenario"),
                                                        QStringLiteral("progress")});
        QVERIFY(session->start());
        QTRY_VERIFY(session->screenText().contains(QStringLiteral("done")));
        QVERIFY(!session->screenText().contains(QStringLiteral("working: 25%")));
        QCOMPARE(session->lineText(0).trimmed(), QStringLiteral("done"));
    }

    void whatIsTypedReachesTheProgramAndComesBack()
    {
        QObject owner;
        TerminalSession *session = stubSession(&owner, {QStringLiteral("echo")});
        QVERIFY(session->start());
        session->sendText(QStringLiteral("hello\r"));
        QTRY_VERIFY(session->screenText().contains(QStringLiteral("echo:hello")));
    }

    void theSizeTheViewChoosesIsWhatTheProgramSees()
    {
        QObject owner;
        TerminalSession *session = stubSession(&owner, {QStringLiteral("size")});
        session->resize(100, 30);
        QVERIFY(session->start());
        QTRY_VERIFY(session->screenText().contains(QStringLiteral("size 100x30")));
    }

    void theExitStatusIsReported()
    {
        QObject owner;
        TerminalSession *session = stubSession(&owner, {QStringLiteral("exit"),
                                                        QStringLiteral("3")});
        QSignalSpy exits(session, &TerminalSession::exited);
        QVERIFY(session->start());
        QTRY_COMPARE(exits.count(), 1);
        QCOMPARE(exits.at(0).at(0).toInt(), 3);
        QVERIFY(!session->isRunning());
    }

    void aTitleSetByTheProgramIsReported()
    {
        QObject owner;
        TerminalSession *session = stubSession(&owner, {QStringLiteral("scenario"),
                                                        QStringLiteral("title")});
        QVERIFY(session->start());
        QTRY_COMPARE(session->title(), QStringLiteral("a title"));
    }

    void theSessionCanBeReadAsStyledHtml()
    {
        // The path an application takes to show a build log without having a
        // terminal in its interface at all.
        QObject owner;
        TerminalSession *session = stubSession(&owner, {QStringLiteral("scenario"),
                                                        QStringLiteral("colour")});
        QVERIFY(session->start());
        QTRY_VERIFY(session->screenText().contains(QStringLiteral("truecolour")));
        const QString html = session->toHtml();
        QVERIFY(html.contains(QStringLiteral("<pre")));
        QVERIFY(html.contains(QStringLiteral("#787878")) || html.contains(QStringLiteral("span")));
    }

    void closingTheSessionEndsTheChild()
    {
        QObject owner;
        TerminalSession *session = stubSession(&owner, {QStringLiteral("sleep")});
        QSignalSpy exits(session, &TerminalSession::exited);
        QVERIFY(session->start());
        QTRY_VERIFY(session->isRunning());
        session->close();
        QTRY_VERIFY_WITH_TIMEOUT(exits.count() == 1, 5000);
    }
};

QTEST_MAIN(TestSession)
#include "test_session.moc"
