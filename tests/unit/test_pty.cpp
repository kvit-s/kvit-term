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

    void collect(Session &session)
    {
        QObject::connect(&session.pty, &Pty::dataAvailable, &session.pty,
                         [this, &session](const QByteArray &bytes) {
                             session.output += bytes;
                             m_lastOutput = session.output;
                         });
        QObject::connect(&session.pty, &Pty::finished, &session.pty, [&session](int code) {
            session.exitCode = code;
            session.finished = true;
        });
    }

    QByteArray m_lastOutput;

    // The text a child wrote, with the terminal's own escape sequences taken
    // out.
    //
    // What arrives on a pseudo-terminal is not the child's bytes alone.
    // Windows' console host in particular announces itself first — it hides
    // the cursor, clears the screen, sets the window title from the program's
    // path and shows the cursor again — and it emits those sequences
    // *between* the child's characters, so that a line the child printed in
    // one call arrives split around them. Interpreting the stream is the
    // emulator's job and it has a suite of its own; here it is enough to
    // remove what the child did not write.
    static QByteArray plainText(const QByteArray &stream)
    {
        QByteArray text;
        for (int index = 0; index < stream.size(); ++index) {
            const char byte = stream.at(index);
            if (byte != '\x1b') {
                text += byte;
                continue;
            }
            if (index + 1 >= stream.size())
                break;
            const char kind = stream.at(index + 1);
            if (kind == '[') {                       // a control sequence, ended by a letter
                index += 2;
                while (index < stream.size() && !QChar::isLetter(uchar(stream.at(index))))
                    ++index;
            } else if (kind == ']') {                // an operating-system command
                index += 2;
                while (index < stream.size() && stream.at(index) != '\a') {
                    if (stream.at(index) == '\x1b' && index + 1 < stream.size()
                        && stream.at(index + 1) == '\\') {
                        ++index;
                        break;
                    }
                    ++index;
                }
            } else {
                ++index;                             // a two-character escape
            }
        }
        return text;
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
    // The suite's own trace, on standard error, because standard output does
    // not survive the trip out of a Windows test runner. Only when asked for.
    void init()
    {
        if (qEnvironmentVariableIsSet("KVITTERM_PTY_DEBUG")) {
            std::fprintf(stderr, "[test_pty] begin %s\n", QTest::currentTestFunction());
            std::fflush(stderr);
        }
    }

    void cleanup()
    {
        if (qEnvironmentVariableIsSet("KVITTERM_PTY_DEBUG")) {
            std::fprintf(stderr, "[test_pty] end   %s: %s\n", QTest::currentTestFunction(),
                         QTest::currentTestFailed() ? "FAILED" : "passed");
            if (QTest::currentTestFailed()) {
                // What the child actually said, which is the one thing worth
                // knowing and the one thing the framework's own report cannot
                // deliver from here.
                std::fprintf(stderr, "[test_pty] the child wrote: %s\n",
                             m_lastOutput.trimmed().toPercentEncoding(" :=x").constData());
            }
            std::fflush(stderr);
        }
        m_lastOutput.clear();
    }

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
        QVERIFY2(plainText(session.output).contains("stdin 1 stdout 1 stderr 1"),
                 "the child did not see a terminal; " + plainText(session.output).trimmed());
        QCOMPARE(session.exitCode, 0);
    }

    void theChildIsToldTheTerminalSize()
    {
        Session session;
        collect(session);
        QVERIFY(session.pty.start(stubParams({QStringLiteral("size")}, 100, 30)));
        QTRY_VERIFY(session.finished);
        QVERIFY2(plainText(session.output).contains("size 100x30"),
                 plainText(session.output).trimmed());
    }

#ifndef Q_OS_WIN
    void aResizeReachesTheChild()
    {
        // The size is pushed with an ioctl rather than through the
        // environment, so a program that is already running learns about it.
        Session session;
        collect(session);
        QVERIFY(session.pty.start(stubParams({QStringLiteral("size-watch")}, 80, 24)));
        QTRY_VERIFY(plainText(session.output).contains("size 80x24"));
        session.pty.resize(120, 40);
        QTRY_VERIFY(plainText(session.output).contains("size 120x40"));
        QTRY_VERIFY(session.finished);
    }
#endif

    void whatIsWrittenReachesTheChild()
    {
        Session session;
        collect(session);
        QVERIFY(session.pty.start(stubParams({QStringLiteral("echo")})));
        // A terminal sends a carriage return for Enter, and it is the
        // terminal's line discipline that turns it into a newline for the
        // program. Sending a newline works on Unix, where the conversion runs
        // in both directions, and leaves a Windows console waiting.
        session.pty.write(QByteArray("hello\r"));
        QTRY_VERIFY(plainText(session.output).contains("echo:hello"));
        // The terminal's own line discipline echoed the input back as well,
        // which is what makes typing visible in a real terminal.
        QVERIFY(session.output.contains("hello\r\n"));
        session.pty.write(QByteArray("quit\r"));
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
        QVERIFY(plainText(session.output).contains("truecolour"));
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
        QTRY_VERIFY(plainText(session.output).contains("line 50"));
    }
};

QTEST_MAIN(TestPty)
#include "test_pty.moc"
