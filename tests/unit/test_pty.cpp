// The pseudo-terminal layer, proved against a stub program rather than
// against a shell: a shell's version, its startup files and its prompt all
// vary between machines, and none of them is what these cases are about.
#include <cstdio>

#include <QtCore/QCoreApplication>
#include <QtCore/QFileInfo>
#include <QtCore/QTemporaryDir>
#include <QtTest/QtTest>

#include "kvitterm/pseudoterminal.h"

using namespace kvitterm;

class TestPty : public QObject
{
    Q_OBJECT

private:
    // Collects everything the child writes, so a case can wait for a
    // substring rather than for a particular number of reads: how a stream is
    // cut into reads is the kernel's business and varies run to run.
    struct Session
    {
        Pty pty;
        QByteArray output;
        int exitCode = -1;
        bool finished = false;
    };

    static void collect(Session &session)
    {
        QObject::connect(&session.pty, &Pty::dataAvailable, &session.pty,
                         [&session](const QByteArray &bytes) { session.output += bytes; });
        QObject::connect(&session.pty, &Pty::finished, &session.pty, [&session](int code) {
            session.exitCode = code;
            session.finished = true;
        });
    }

    static Pty::Params stubParams(const QStringList &arguments, int columns = 80, int rows = 24)
    {
        Pty::Params params;
        params.program = QString::fromLocal8Bit(KVITTERM_STUB_PATH);
        params.arguments = arguments;
        params.columns = columns;
        params.rows = rows;
        return params;
    }

private Q_SLOTS:
    void initTestCase()
    {
        // Unbuffered, so that a case which takes the process down with it
        // still leaves a record of how far the suite got. Standard output is
        // a pipe under a test runner, and the C runtime buffers a pipe until
        // it is flushed or the process exits cleanly.
        setvbuf(stdout, nullptr, _IONBF, 0);
        QVERIFY2(QFileInfo::exists(QString::fromLocal8Bit(KVITTERM_STUB_PATH)),
                 "the stub program was not built");
    }

    void theChildSeesATerminal()
    {
        // The whole point of the layer: a child on a pipe reports 0 here and
        // turns its colours off accordingly.
        Session session;
        collect(session);
        QString error;
        QVERIFY2(session.pty.start(stubParams({QStringLiteral("isatty")}), &error),
                 qPrintable(error));
        QTRY_VERIFY(session.finished);
        QVERIFY2(session.output.contains("stdin 1 stdout 1 stderr 1"),
                 "the child did not see a terminal; " + session.output.trimmed());
        QCOMPARE(session.exitCode, 0);
    }

    void theChildIsToldTheTerminalSize()
    {
        Session session;
        collect(session);
        QVERIFY(session.pty.start(stubParams({QStringLiteral("size")}, 100, 30)));
        QTRY_VERIFY(session.finished);
        QCOMPARE(session.output.trimmed(), QByteArray("size 100x30"));
    }

#ifndef Q_OS_WIN
    void aResizeReachesTheChild()
    {
        // The size is pushed with an ioctl rather than through the
        // environment, so a program that is already running learns about it.
        Session session;
        collect(session);
        QVERIFY(session.pty.start(stubParams({QStringLiteral("size-watch")}, 80, 24)));
        QTRY_VERIFY(session.output.contains("size 80x24"));
        session.pty.resize(120, 40);
        QTRY_VERIFY(session.output.contains("size 120x40"));
        QTRY_VERIFY(session.finished);
    }
#endif

    void whatIsWrittenReachesTheChild()
    {
        Session session;
        collect(session);
        QVERIFY(session.pty.start(stubParams({QStringLiteral("echo")})));
        session.pty.write(QByteArray("hello\n"));
        QTRY_VERIFY(session.output.contains("echo:hello"));
        // The terminal's own line discipline echoed the input back as well,
        // which is what makes typing visible in a real terminal.
        QVERIFY(session.output.contains("hello\r\n"));
        session.pty.write(QByteArray("quit\n"));
        QTRY_VERIFY(session.finished);
    }

    void theExitCodeIsReported()
    {
        Session session;
        collect(session);
        QVERIFY(session.pty.start(stubParams({QStringLiteral("exit"), QStringLiteral("7")})));
        QTRY_VERIFY(session.finished);
        QCOMPARE(session.exitCode, 7);
    }

    void outputWrittenJustBeforeExitIsNotLost()
    {
        // A short-lived program is the ordinary case — a build command that
        // fails in a tenth of a second — and losing its last line because the
        // process died before the reader ran would be the obvious bug.
        Session session;
        collect(session);
        QVERIFY(session.pty.start(stubParams({QStringLiteral("scenario"), QStringLiteral("colour")})));
        QTRY_VERIFY(session.finished);
        QVERIFY(session.output.contains("truecolour"));
    }

    void aProgramThatDoesNotExistFailsToStart()
    {
        Pty pty;
        Pty::Params params;
        params.program = QStringLiteral("/nonexistent/kvitterm-no-such-program");
        QString error;
        QVERIFY(!pty.start(params, &error));
        QVERIFY(!error.isEmpty());
        QVERIFY(!pty.isRunning());
    }

    void theWorkingDirectoryIsHonoured()
    {
        QTemporaryDir directory;
        QVERIFY(directory.isValid());
        Session session;
        collect(session);
        Pty::Params params = stubParams({QStringLiteral("isatty")});
        params.workingDirectory = directory.path();
        QVERIFY(session.pty.start(params));
        QTRY_VERIFY(session.finished);
        QCOMPARE(session.exitCode, 0);
    }

    void hangingUpEndsTheChild()
    {
        // Closing this end delivers SIGHUP to the child's process group,
        // which is what a program sees when a window is closed on it.
        Session session;
        collect(session);
        QVERIFY(session.pty.start(stubParams({QStringLiteral("sleep")})));
        QTRY_VERIFY(session.pty.isRunning());
        session.pty.hangup();
        // Windows takes longer over this: closing the pseudoconsole asks the
        // child to end, and the console host waits before insisting.
        QTRY_VERIFY_WITH_TIMEOUT(session.finished, 15000);
    }

    void suspendingReadingHoldsOutputBack()
    {
        // What stops a runaway process from outrunning whatever draws it.
        Session session;
        collect(session);
        session.pty.setReadingSuspended(true);
        QVERIFY(session.pty.start(stubParams({QStringLiteral("scenario"), QStringLiteral("scroll")})));
        QTest::qWait(200);
        QVERIFY(session.output.isEmpty());
        session.pty.setReadingSuspended(false);
        QTRY_VERIFY(session.output.contains("line 50"));
    }
};

QTEST_MAIN(TestPty)
#include "test_pty.moc"
