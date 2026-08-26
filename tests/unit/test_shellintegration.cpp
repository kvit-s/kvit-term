// What a shell tells the terminal about itself, and what can be built on it.
//
// The marks are fed straight into the screen here rather than produced by a
// real shell: which shell is installed, which version it is and what its
// startup files do all vary, and none of that is what these cases are about.
// One case at the end runs the whole path through a pseudo-terminal.
#include <QtCore/QFileInfo>
#include <QtTest/QtTest>

#include "kvitterm/shellintegration.h"
#include "kvitterm/terminalsession.h"

using namespace kvitterm;

class TestShellIntegration : public QObject
{
    Q_OBJECT

private:
    // The sequence a shell with the integration installed actually emits
    // around one command: prompt, end of prompt, the command as typed, the
    // start of its output, and the status it exited with.
    static QByteArray promptAndCommand(const QByteArray &command, const QByteArray &output,
                                       int exitCode)
    {
        QByteArray bytes;
        bytes += "\x1b]133;A\x1b\\";
        bytes += "$ ";
        bytes += "\x1b]133;B\x1b\\";
        bytes += command;
        bytes += "\r\n";
        bytes += "\x1b]133;C\x1b\\";
        bytes += output;
        bytes += "\x1b]133;D;" + QByteArray::number(exitCode) + "\x1b\\";
        return bytes;
    }

private Q_SLOTS:
    void nothingIsClaimedUntilAMarkArrives()
    {
        // A shell without the snippet installed says nothing, and an
        // application should be able to tell that from a shell that has run
        // no commands yet.
        QObject owner;
        auto *session = new TerminalSession(&owner);
        session->setAutoStart(false);
        ShellIntegration integration;
        integration.setSession(session);

        QVERIFY(!integration.isActive());
        session->screen()->feed("ordinary output with no marks in it\r\n");
        QVERIFY(!integration.isActive());
        QCOMPARE(integration.commandCount(), 0);
    }

    void aCommandIsRecordedWithItsTextAndItsStatus()
    {
        QObject owner;
        auto *session = new TerminalSession(&owner);
        session->setAutoStart(false);
        ShellIntegration integration;
        integration.setSession(session);
        QSignalSpy started(&integration, &ShellIntegration::commandStarted);
        QSignalSpy finished(&integration, &ShellIntegration::commandFinished);

        session->screen()->feed(promptAndCommand("ls -l", "total 0\r\nfile.txt\r\n", 0));

        QVERIFY(integration.isActive());
        QCOMPARE(integration.commandCount(), 1);
        QCOMPARE(started.count(), 1);
        QCOMPARE(finished.count(), 1);

        const QVariantMap command = integration.commandAt(0);
        QCOMPARE(command.value(QStringLiteral("text")).toString(), QStringLiteral("ls -l"));
        QCOMPARE(command.value(QStringLiteral("exitCode")).toInt(), 0);
        QVERIFY(command.value(QStringLiteral("finished")).toBool());
        QVERIFY(!integration.isCommandRunning());
    }

    void aFailingCommandKeepsItsStatus()
    {
        QObject owner;
        auto *session = new TerminalSession(&owner);
        session->setAutoStart(false);
        ShellIntegration integration;
        integration.setSession(session);

        session->screen()->feed(promptAndCommand("false", QByteArray(), 1));
        QCOMPARE(integration.commandAt(0).value(QStringLiteral("exitCode")).toInt(), 1);
    }

    void theOutputOfACommandCanBeReadBack()
    {
        // This is the thing a scrollback buffer alone cannot do: say which
        // bytes belonged to which command.
        QObject owner;
        auto *session = new TerminalSession(&owner);
        session->setAutoStart(false);
        ShellIntegration integration;
        integration.setSession(session);

        session->screen()->feed(promptAndCommand("echo one", "one\r\n", 0));
        session->screen()->feed(promptAndCommand("echo two", "two\r\n", 0));

        QCOMPARE(integration.commandCount(), 2);
        const QString first = integration.outputOf(0);
        const QString second = integration.outputOf(1);
        QVERIFY(first.contains(QStringLiteral("one")));
        QVERIFY(!first.contains(QStringLiteral("two")));
        QVERIFY(second.contains(QStringLiteral("two")));
    }

    void theDirectoryTheShellIsInIsReported()
    {
        QObject owner;
        auto *session = new TerminalSession(&owner);
        session->setAutoStart(false);
        ShellIntegration integration;
        integration.setSession(session);
        QSignalSpy directories(&integration, &ShellIntegration::currentDirectoryChanged);

        session->screen()->feed("\x1b]7;file://somehost/home/someone/work\x1b\\");
        QCOMPARE(integration.currentDirectory(), QStringLiteral("/home/someone/work"));
        QCOMPARE(directories.count(), 1);

        // Percent-encoding, which is how a path with a space arrives.
        session->screen()->feed("\x1b]7;file://somehost/home/some%20one\x1b\\");
        QCOMPARE(integration.currentDirectory(), QStringLiteral("/home/some one"));
    }

    void visualStudioCodesOwnMarksAreUnderstoodToo()
    {
        // Its snippets emit OSC 633 with the same letters, and give the
        // command line explicitly rather than leaving it to be read off the
        // screen.
        QObject owner;
        auto *session = new TerminalSession(&owner);
        session->setAutoStart(false);
        ShellIntegration integration;
        integration.setSession(session);

        session->screen()->feed("\x1b]633;A\x1b\\$ \x1b]633;B\x1b\\");
        session->screen()->feed("\x1b]633;C\x1b\\");
        session->screen()->feed("\x1b]633;E;git status --short\x1b\\");
        session->screen()->feed("\x1b]633;P;Cwd=/tmp/project\x1b\\");
        session->screen()->feed("\x1b]633;D;0\x1b\\");

        QCOMPARE(integration.commandCount(), 1);
        QCOMPARE(integration.commandAt(0).value(QStringLiteral("text")).toString(),
                 QStringLiteral("git status --short"));
        QCOMPARE(integration.currentDirectory(), QStringLiteral("/tmp/project"));
    }

    void aCommandThatNeverReportsIsClosedByTheNextPrompt()
    {
        // Not every shell reports a status for every command — a signal, or an
        // interrupted line, can end one silently — and a prompt appearing is
        // proof that whatever was running is not running any more.
        QObject owner;
        auto *session = new TerminalSession(&owner);
        session->setAutoStart(false);
        ShellIntegration integration;
        integration.setSession(session);

        session->screen()->feed("\x1b]133;A\x1b\\$ \x1b]133;B\x1b\\sleep 100\r\n\x1b]133;C\x1b\\");
        QVERIFY(integration.isCommandRunning());
        session->screen()->feed("\x1b]133;A\x1b\\$ ");
        QVERIFY(!integration.isCommandRunning());
        QVERIFY(integration.commandAt(0).value(QStringLiteral("finished")).toBool());
        QCOMPARE(integration.commandAt(0).value(QStringLiteral("exitCode")).toInt(), -1);
    }

    void theSnippetsAreCarriedInsideTheLibrary()
    {
        // An application installing shell integration should not have to find
        // a file on disk that a packager may have put anywhere.
        for (Shell shell : {Shell::Bash, Shell::Zsh, Shell::Fish, Shell::PowerShell}) {
            const QString script = shellIntegrationScript(shell);
            QVERIFY2(!script.isEmpty(), qPrintable(shellIntegrationFileName(shell)));
            QVERIFY(script.contains(QStringLiteral("133;A")));
            QVERIFY(script.contains(QStringLiteral("133;D")));
        }
    }

    void theMarksSurviveARealPseudoTerminal()
    {
        QObject owner;
        auto *session = new TerminalSession(&owner);
        session->setAutoStart(false);
        session->setProgram(QString::fromLocal8Bit(KVITTERM_STUB_PATH));
        session->setArguments({QStringLiteral("scenario"), QStringLiteral("marks")});
        ShellIntegration integration;
        integration.setSession(session);

        QVERIFY(session->start());
        QTRY_COMPARE(integration.commandCount(), 1);
        QTRY_VERIFY(integration.commandAt(0).value(QStringLiteral("finished")).toBool());
        QCOMPARE(integration.commandAt(0).value(QStringLiteral("text")).toString(),
                 QStringLiteral("ls -l"));
        QCOMPARE(integration.currentDirectory(), QStringLiteral("/tmp"));
        QVERIFY(integration.outputOf(0).contains(QStringLiteral("file.txt")));
    }
};

QTEST_MAIN(TestShellIntegration)
#include "test_shellintegration.moc"
