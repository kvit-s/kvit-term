// The pseudo-terminal on Linux and macOS.
//
// openpty(3) hands back both ends of a fresh device pair. The parent keeps the
// master and reads it through a socket notifier; the child, between the fork
// and the exec, starts a session of its own, claims the slave as its
// controlling terminal and puts it on the three standard descriptors. From
// there the child is indistinguishable from one started by a terminal
// application.
//
// QProcess does the fork, the program lookup and the reaping. Its
// setChildProcessModifier callback runs in the child after the standard
// descriptors have been set up and before the exec, which is the one window in
// which the controlling terminal can be claimed.
#include "kvitterm/pseudoterminal.h"

#include <QtCore/QPointer>
#include <QtCore/QProcess>
#include <QtCore/QSocketNotifier>

#include <cerrno>
#include <csignal>
#include <cstring>

#include <fcntl.h>
#include <sys/ioctl.h>
#include <termios.h>
#include <unistd.h>

#ifdef Q_OS_MACOS
#  include <util.h>
#else
#  include <pty.h>
#endif

namespace kvitterm {

class PtyPrivate
{
public:
    explicit PtyPrivate(Pty *owner) : q(owner) {}

    void closeMaster()
    {
        delete readNotifier;
        readNotifier = nullptr;
        delete writeNotifier;
        writeNotifier = nullptr;
        if (master >= 0) {
            ::close(master);
            master = -1;
        }
    }

    void readFromMaster()
    {
        // A fixed budget per activation rather than a loop to EAGAIN: a
        // process writing faster than it can be drawn would otherwise keep
        // control here indefinitely and the interface would stop repainting.
        char buffer[65536];
        int reads = 0;
        while (reads++ < 4) {
            const ssize_t got = ::read(master, buffer, sizeof(buffer));
            if (got > 0) {
                Q_EMIT q->dataAvailable(QByteArray(buffer, int(got)));
                continue;
            }
            if (got == 0) {              // the child closed its end
                readNotifier->setEnabled(false);
                return;
            }
            if (errno == EINTR)
                continue;
            if (errno == EAGAIN || errno == EWOULDBLOCK)
                return;
            // EIO here is the ordinary end of a session on Linux: the last
            // descriptor on the slave side has closed because the child has
            // gone. The exit status arrives separately, through QProcess.
            readNotifier->setEnabled(false);
            return;
        }
    }

    void flushPendingWrites()
    {
        while (!pending.isEmpty()) {
            const ssize_t written = ::write(master, pending.constData(), size_t(pending.size()));
            if (written > 0) {
                pending.remove(0, int(written));
                continue;
            }
            if (written < 0 && errno == EINTR)
                continue;
            if (written < 0 && (errno == EAGAIN || errno == EWOULDBLOCK)) {
                if (!writeNotifier) {
                    writeNotifier = new QSocketNotifier(master, QSocketNotifier::Write, q);
                    QObject::connect(writeNotifier, &QSocketNotifier::activated, q,
                                     [this] { flushPendingWrites(); });
                }
                writeNotifier->setEnabled(true);
                return;
            }
            pending.clear();             // the child is gone; there is nowhere to put it
            return;
        }
        if (writeNotifier)
            writeNotifier->setEnabled(false);
    }

    Pty *q;
    QProcess *process = nullptr;
    QSocketNotifier *readNotifier = nullptr;
    QSocketNotifier *writeNotifier = nullptr;
    QByteArray pending;
    int master = -1;
    int columns = 80;
    int rows = 24;
    bool suspended = false;
    bool reportedFinished = false;
};

Pty::Pty(QObject *parent) : QObject(parent), d(new PtyPrivate(this)) {}

Pty::~Pty()
{
    d->closeMaster();
    if (d->process && d->process->state() != QProcess::NotRunning) {
        d->process->kill();
        d->process->waitForFinished(1000);
    }
    delete d;
}

bool Pty::start(const Pty::Params &params, QString *error)
{
    const auto fail = [error](const QString &message) {
        if (error)
            *error = message;
        return false;
    };

    if (isRunning())
        return fail(QStringLiteral("This pseudo-terminal already has a child on it."));
    if (params.program.isEmpty())
        return fail(QStringLiteral("No program was named."));

    d->columns = qMax(1, params.columns);
    d->rows = qMax(1, params.rows);

    struct winsize size = {};
    size.ws_row = ushort(d->rows);
    size.ws_col = ushort(d->columns);

    int master = -1;
    int slave = -1;
    if (::openpty(&master, &slave, nullptr, nullptr, &size) == -1)
        return fail(QStringLiteral("Could not allocate a pseudo-terminal: %1")
                        .arg(QString::fromLocal8Bit(strerror(errno))));

    ::fcntl(master, F_SETFD, FD_CLOEXEC);
    ::fcntl(master, F_SETFL, ::fcntl(master, F_GETFL, 0) | O_NONBLOCK);

    QProcessEnvironment environment = params.environment.isEmpty()
            ? QProcessEnvironment::systemEnvironment()
            : params.environment;
    if (!environment.contains(QStringLiteral("TERM")) && !params.term.isEmpty())
        environment.insert(QStringLiteral("TERM"), params.term);
    if (!environment.contains(QStringLiteral("COLORTERM")) && !params.colorTerm.isEmpty())
        environment.insert(QStringLiteral("COLORTERM"), params.colorTerm);
    // Some programs read these instead of asking the terminal. They go stale
    // on the first resize, which is why the ioctl is what actually matters.
    environment.insert(QStringLiteral("COLUMNS"), QString::number(d->columns));
    environment.insert(QStringLiteral("LINES"), QString::number(d->rows));

    d->reportedFinished = false;
    d->process = new QProcess(this);
    d->process->setProgram(params.program);
    d->process->setArguments(params.arguments);
    if (!params.workingDirectory.isEmpty())
        d->process->setWorkingDirectory(params.workingDirectory);
    d->process->setProcessEnvironment(environment);
    d->process->setChildProcessModifier([slave] {
        // Everything here runs between the fork and the exec, so it must be
        // safe to call in that state: no allocation, no Qt.
        ::setsid();
#ifdef TIOCSCTTY
        ::ioctl(slave, TIOCSCTTY, 0);
#endif
        ::dup2(slave, STDIN_FILENO);
        ::dup2(slave, STDOUT_FILENO);
        ::dup2(slave, STDERR_FILENO);
        if (slave > STDERR_FILENO)
            ::close(slave);
        // A shell expects to be started with these at their defaults rather
        // than inherited from whatever spawned the application.
        ::signal(SIGPIPE, SIG_DFL);
        ::signal(SIGINT, SIG_DFL);
        ::signal(SIGQUIT, SIG_DFL);
        ::signal(SIGHUP, SIG_DFL);
        ::signal(SIGTERM, SIG_DFL);
    });

    connect(d->process, &QProcess::finished, this, [this](int exitCode, QProcess::ExitStatus status) {
        if (d->reportedFinished)
            return;
        d->reportedFinished = true;
        // Drain whatever the child wrote before it exited: the notifier will
        // not fire again once the process is gone.
        if (d->master >= 0 && d->readNotifier && d->readNotifier->isEnabled())
            d->readFromMaster();
        Q_EMIT finished(status == QProcess::CrashExit ? 128 + 9 : exitCode);
    });

    d->process->start();
    // The fork must have happened before the parent lets go of the slave,
    // and a program that does not exist should be reported as a failure to
    // start rather than as an immediate exit.
    if (!d->process->waitForStarted(30000)) {
        const QString message = d->process->errorString();
        ::close(master);
        ::close(slave);
        delete d->process;
        d->process = nullptr;
        return fail(QStringLiteral("Could not start %1: %2").arg(params.program, message));
    }
    ::close(slave);

    d->master = master;
    d->readNotifier = new QSocketNotifier(master, QSocketNotifier::Read, this);
    connect(d->readNotifier, &QSocketNotifier::activated, this, [this] { d->readFromMaster(); });
    d->readNotifier->setEnabled(!d->suspended);
    return true;
}

bool Pty::isRunning() const
{
    return d->process && d->process->state() != QProcess::NotRunning;
}

qint64 Pty::processId() const
{
    return d->process ? d->process->processId() : 0;
}

int Pty::columns() const { return d->columns; }
int Pty::rows() const { return d->rows; }

void Pty::write(const QByteArray &bytes)
{
    if (bytes.isEmpty() || d->master < 0)
        return;
    d->pending.append(bytes);
    d->flushPendingWrites();
}

void Pty::resize(int columns, int rows)
{
    columns = qMax(1, columns);
    rows = qMax(1, rows);
    if (columns == d->columns && rows == d->rows)
        return;
    d->columns = columns;
    d->rows = rows;
    if (d->master < 0)
        return;
    struct winsize size = {};
    size.ws_row = ushort(rows);
    size.ws_col = ushort(columns);
    ::ioctl(d->master, TIOCSWINSZ, &size);
    // The kernel sends SIGWINCH to the foreground process group as a
    // consequence, which is how a full-screen program learns to redraw.
}

void Pty::setReadingSuspended(bool suspended)
{
    d->suspended = suspended;
    if (d->readNotifier)
        d->readNotifier->setEnabled(!suspended);
}

bool Pty::isReadingSuspended() const { return d->suspended; }

void Pty::hangup()
{
    d->closeMaster();
}

void Pty::terminateProcess()
{
    if (d->process)
        d->process->terminate();
}

void Pty::killProcess()
{
    if (d->process)
        d->process->kill();
}

} // namespace kvitterm
