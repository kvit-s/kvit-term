// The pseudo-terminal on Windows, which is a different mechanism with the
// same purpose.
//
// Windows has no fork, no controlling terminal and no SIGWINCH. What it has,
// from Windows 10 version 1809 onwards, is the pseudoconsole: a kernel object
// created with a pair of ordinary pipes, which a child process is attached to
// through a process-creation attribute. The console host on the other side
// translates the child's console output into the same escape sequences a Unix
// terminal would see, so everything above this file is unchanged.
//
// Two consequences are worth knowing before reading the code. QProcess cannot
// be used: attaching a pseudoconsole needs an extended startup structure and
// QProcess builds a plain one, so this calls CreateProcessW directly. And
// reading is done on a thread, because a pipe read here has no equivalent of
// the socket notifier the Unix side uses.
// The pseudoconsole appeared in Windows 10 version 1809, and the SDK hides
// its process-creation attribute behind these version macros. The build
// system sets them too; this is here so the file also compiles on its own.
#if defined(NTDDI_VERSION) && NTDDI_VERSION < 0x0A000006
#  undef NTDDI_VERSION
#endif
#ifndef NTDDI_VERSION
#  define NTDDI_VERSION 0x0A000006
#endif
#if defined(_WIN32_WINNT) && _WIN32_WINNT < 0x0A00
#  undef _WIN32_WINNT
#endif
#ifndef _WIN32_WINNT
#  define _WIN32_WINNT 0x0A00
#endif

#include "kvitterm/pseudoterminal.h"

#include <QtCore/QDir>
#include <QtCore/QMutex>
#include <QtCore/QThread>
#include <QtCore/QWaitCondition>
#include <QtCore/QWinEventNotifier>

#include <atomic>
#include <cstdarg>
#include <cstdio>

#ifndef WIN32_LEAN_AND_MEAN
#  define WIN32_LEAN_AND_MEAN
#endif
#ifndef NOMINMAX
#  define NOMINMAX
#endif
#include <windows.h>

namespace kvitterm {

namespace {

// The quoting rules CommandLineToArgvW undoes at the other end. Windows
// passes a command line rather than an argument vector, so every child has to
// take it apart again, and only this exact encoding survives the trip.
QString quoteArgument(const QString &argument)
{
    if (!argument.isEmpty()
        && !argument.contains(QLatin1Char(' '))
        && !argument.contains(QLatin1Char('\t'))
        && !argument.contains(QLatin1Char('"'))) {
        return argument;
    }

    QString quoted = QStringLiteral("\"");
    int backslashes = 0;
    for (const QChar character : argument) {
        if (character == QLatin1Char('\\')) {
            ++backslashes;
            continue;
        }
        if (character == QLatin1Char('"')) {
            quoted += QString(backslashes * 2 + 1, QLatin1Char('\\'));
            quoted += QLatin1Char('"');
        } else {
            quoted += QString(backslashes, QLatin1Char('\\'));
            quoted += character;
        }
        backslashes = 0;
    }
    quoted += QString(backslashes * 2, QLatin1Char('\\'));
    quoted += QLatin1Char('"');
    return quoted;
}

// Set KVITTERM_PTY_DEBUG to have every step of the attachment report itself.
// Written straight to the unbuffered standard error rather than through
// qWarning, so that it survives a process that dies before it flushes.
bool debugging()
{
    static const bool on = qEnvironmentVariableIsSet("KVITTERM_PTY_DEBUG");
    return on;
}

void report(const char *format, ...)
{
    if (!debugging())
        return;
    va_list arguments;
    va_start(arguments, format);
    std::fprintf(stderr, "[kvitterm pty] ");
    std::vfprintf(stderr, format, arguments);
    std::fprintf(stderr, "\n");
    std::fflush(stderr);
    va_end(arguments);
}

QString formatLastError(DWORD code)
{
    LPWSTR buffer = nullptr;
    const DWORD length = FormatMessageW(
        FORMAT_MESSAGE_ALLOCATE_BUFFER | FORMAT_MESSAGE_FROM_SYSTEM
            | FORMAT_MESSAGE_IGNORE_INSERTS,
        nullptr, code, 0, reinterpret_cast<LPWSTR>(&buffer), 0, nullptr);
    QString message = length ? QString::fromWCharArray(buffer, int(length)).trimmed()
                             : QStringLiteral("error %1").arg(code);
    if (buffer)
        LocalFree(buffer);
    return message;
}

} // namespace

// Reads the pseudoconsole's output pipe. A blocking read on a thread is the
// only shape available: suspending it lets the pipe fill, which blocks the
// child in its own write, which is the flow control the Unix side gets by
// switching off a notifier.
class PtyReader : public QThread
{
public:
    PtyReader(Pty *owner, HANDLE pipe) : m_owner(owner), m_pipe(pipe) {}

    void stop()
    {
        m_stopping.store(true);
        resume();
    }

    void suspend()
    {
        QMutexLocker locker(&m_mutex);
        m_suspended = true;
    }

    void resume()
    {
        QMutexLocker locker(&m_mutex);
        m_suspended = false;
        m_wakeUp.wakeAll();
    }

protected:
    void run() override
    {
        // Waiting for data with PeekNamedPipe rather than blocking in
        // ReadFile. A blocked read cannot be cancelled from another thread
        // without a handle to this one, so ending a session would mean either
        // hanging until the child wrote something or killing a thread in the
        // middle of a read; polling every few milliseconds costs nothing
        // measurable and makes shutdown ordinary.
        QVarLengthArray<char, 65536> buffer(65536);
        for (;;) {
            {
                QMutexLocker locker(&m_mutex);
                while (m_suspended && !m_stopping.load())
                    m_wakeUp.wait(&m_mutex);
            }
            if (m_stopping.load())
                return;

            DWORD available = 0;
            if (!PeekNamedPipe(m_pipe, nullptr, 0, nullptr, &available, nullptr))
                return;                       // the pseudoconsole has gone
            if (available == 0) {
                QThread::msleep(4);
                continue;
            }

            DWORD got = 0;
            const DWORD wanted = qMin<DWORD>(available, DWORD(buffer.size()));
            if (!ReadFile(m_pipe, buffer.data(), wanted, &got, nullptr) || got == 0)
                return;

            const QByteArray bytes(buffer.constData(), qsizetype(got));
            Pty *owner = m_owner;
            QMetaObject::invokeMethod(owner, [owner, bytes] {
                Q_EMIT owner->dataAvailable(bytes);
            }, Qt::QueuedConnection);
        }
    }

private:
    Pty *m_owner;
    HANDLE m_pipe;
    QMutex m_mutex;
    QWaitCondition m_wakeUp;
    bool m_suspended = false;
    std::atomic_bool m_stopping{false};
};

class PtyPrivate
{
public:
    explicit PtyPrivate(Pty *owner) : q(owner) {}

    void closeHandles()
    {
        // The reader goes first and is joined before anything it touches is
        // closed: it polls, so it notices the stop within a few milliseconds.
        bool readerStopped = true;
        if (reader) {
            reader->stop();
            readerStopped = reader->wait(3000);
            if (readerStopped) {
                delete reader;
            } else {
                // It cannot be deleted while it runs, and killing a thread is
                // worse than leaking one on a path this rare. The pipe it is
                // reading is left open below for the same reason: closing a
                // handle out from under a thread is how a rare hang becomes a
                // rare crash.
                report("the reader thread did not stop; leaving it and its pipe behind");
            }
            reader = nullptr;
        }
        if (pseudoConsole) {
            ClosePseudoConsole(pseudoConsole);
            pseudoConsole = nullptr;
        }
        if (inputWrite != INVALID_HANDLE_VALUE) {
            CloseHandle(inputWrite);
            inputWrite = INVALID_HANDLE_VALUE;
        }
        if (outputRead != INVALID_HANDLE_VALUE && readerStopped) {
            CloseHandle(outputRead);
            outputRead = INVALID_HANDLE_VALUE;
        }
        if (attributeList) {
            DeleteProcThreadAttributeList(attributeList);
            free(attributeList);
            attributeList = nullptr;
        }
    }

    void closeProcessHandles()
    {
        delete exitNotifier;
        exitNotifier = nullptr;
        if (processInfo.hThread) {
            CloseHandle(processInfo.hThread);
            processInfo.hThread = nullptr;
        }
        if (processInfo.hProcess) {
            CloseHandle(processInfo.hProcess);
            processInfo.hProcess = nullptr;
        }
    }

    Pty *q;
    HPCON pseudoConsole = nullptr;
    HANDLE inputWrite = INVALID_HANDLE_VALUE;
    HANDLE outputRead = INVALID_HANDLE_VALUE;
    PROCESS_INFORMATION processInfo = {};
    LPPROC_THREAD_ATTRIBUTE_LIST attributeList = nullptr;
    PtyReader *reader = nullptr;
    QWinEventNotifier *exitNotifier = nullptr;
    int columns = 80;
    int rows = 24;
    bool suspended = false;
    bool running = false;
};

Pty::Pty(QObject *parent) : QObject(parent), d(new PtyPrivate(this)) {}

Pty::~Pty()
{
    if (d->running && d->processInfo.hProcess)
        TerminateProcess(d->processInfo.hProcess, 1);
    d->closeHandles();
    d->closeProcessHandles();
    delete d;
}

bool Pty::start(const Pty::Params &params, QString *error)
{
    const auto fail = [this, error](const QString &message) {
        if (error)
            *error = message;
        d->closeHandles();
        d->closeProcessHandles();
        return false;
    };

    if (isRunning())
        return fail(QStringLiteral("This pseudo-terminal already has a child on it."));
    if (params.program.isEmpty())
        return fail(QStringLiteral("No program was named."));

    d->columns = qMax(1, params.columns);
    d->rows = qMax(1, params.rows);

    HANDLE inputRead = INVALID_HANDLE_VALUE;
    HANDLE outputWrite = INVALID_HANDLE_VALUE;
    if (!CreatePipe(&inputRead, &d->inputWrite, nullptr, 0)
        || !CreatePipe(&d->outputRead, &outputWrite, nullptr, 0)) {
        return fail(QStringLiteral("Could not create the pipes for a pseudoconsole: %1")
                        .arg(formatLastError(GetLastError())));
    }

    report("parent stdout is file type %lu", (unsigned long) GetFileType(GetStdHandle(STD_OUTPUT_HANDLE)));

    const COORD size = {SHORT(d->columns), SHORT(d->rows)};
    HRESULT created = CreatePseudoConsole(size, inputRead, outputWrite, 0, &d->pseudoConsole);
    report("CreatePseudoConsole(%d x %d) = 0x%08lx, handle %p", int(size.X), int(size.Y),
           (unsigned long) created, (void *) d->pseudoConsole);
    // The console host duplicates both handles, so this end has no further
    // use for them whether it succeeded or not.
    CloseHandle(inputRead);
    CloseHandle(outputWrite);
    if (FAILED(created)) {
        return fail(QStringLiteral("Could not create a pseudoconsole: %1")
                        .arg(formatLastError(DWORD(created))));
    }

    SIZE_T attributeSize = 0;
    InitializeProcThreadAttributeList(nullptr, 1, 0, &attributeSize);
    report("attribute list wants %llu bytes", (unsigned long long) attributeSize);
    d->attributeList = static_cast<LPPROC_THREAD_ATTRIBUTE_LIST>(malloc(attributeSize));
    if (!d->attributeList
        || !InitializeProcThreadAttributeList(d->attributeList, 1, 0, &attributeSize)) {
        return fail(QStringLiteral("Could not prepare the process attributes: %1")
                        .arg(formatLastError(GetLastError())));
    }
    report("attaching pseudoconsole %p as attribute 0x%llx", (void *) d->pseudoConsole,
           (unsigned long long) PROC_THREAD_ATTRIBUTE_PSEUDOCONSOLE);
    if (!UpdateProcThreadAttribute(d->attributeList, 0, PROC_THREAD_ATTRIBUTE_PSEUDOCONSOLE,
                                   d->pseudoConsole, sizeof(d->pseudoConsole), nullptr, nullptr)) {
        return fail(QStringLiteral("Could not attach the pseudoconsole to the child: %1")
                        .arg(formatLastError(GetLastError())));
    }

    QString commandLine = quoteArgument(QDir::toNativeSeparators(params.program));
    for (const QString &argument : params.arguments)
        commandLine += QLatin1Char(' ') + quoteArgument(argument);

    QProcessEnvironment environment = params.environment.isEmpty()
            ? QProcessEnvironment::systemEnvironment()
            : params.environment;
    if (!environment.contains(QStringLiteral("TERM")) && !params.term.isEmpty())
        environment.insert(QStringLiteral("TERM"), params.term);
    if (!environment.contains(QStringLiteral("COLORTERM")) && !params.colorTerm.isEmpty())
        environment.insert(QStringLiteral("COLORTERM"), params.colorTerm);

    QString environmentBlock;
    const QStringList keys = environment.keys();
    for (const QString &key : keys) {
        environmentBlock += key + QLatin1Char('=') + environment.value(key);
        environmentBlock += QChar(u'\0');
    }
    environmentBlock += QChar(u'\0');

    STARTUPINFOEXW startup = {};
    startup.StartupInfo.cb = sizeof(STARTUPINFOEXW);
    startup.lpAttributeList = d->attributeList;

    // This process's own standard handles are kept away from the child.
    //
    // Windows passes them into a child's process parameters, where they take
    // precedence over the ones the pseudoconsole installs. Where this process
    // has a console that is invisible; where its handles are pipes — any
    // redirected parent, a test runner included — the child ends up writing
    // to the parent's pipe rather than to the terminal, reports that it is
    // not on a terminal, and turns its colours off.
    //
    // Clearing them for the duration of the call, rather than declaring them
    // empty with STARTF_USESTDHANDLES, leaves console initialisation free to
    // fill the child's in from the pseudoconsole: the flag would mean "these
    // are the handles, take them", and empty ones would be taken literally.
    const HANDLE savedInput = GetStdHandle(STD_INPUT_HANDLE);
    const HANDLE savedOutput = GetStdHandle(STD_OUTPUT_HANDLE);
    const HANDLE savedError = GetStdHandle(STD_ERROR_HANDLE);
    report("parent standard handles in %p out %p err %p", (void *) savedInput,
           (void *) savedOutput, (void *) savedError);
    SetStdHandle(STD_INPUT_HANDLE, nullptr);
    SetStdHandle(STD_OUTPUT_HANDLE, nullptr);
    SetStdHandle(STD_ERROR_HANDLE, nullptr);

    QVarLengthArray<wchar_t, 512> commandLineBuffer(commandLine.size() + 1);
    commandLine.toWCharArray(commandLineBuffer.data());
    commandLineBuffer[commandLine.size()] = L'\0';

    QVarLengthArray<wchar_t, 2048> environmentBuffer(environmentBlock.size());
    environmentBlock.toWCharArray(environmentBuffer.data());

    const QString workingDirectory = params.workingDirectory.isEmpty()
            ? QString()
            : QDir::toNativeSeparators(params.workingDirectory);

    const BOOL startedChild =
        CreateProcessW(nullptr, commandLineBuffer.data(), nullptr, nullptr, FALSE,
                       EXTENDED_STARTUPINFO_PRESENT | CREATE_UNICODE_ENVIRONMENT,
                       environmentBuffer.data(),
                       workingDirectory.isEmpty()
                           ? nullptr
                           : reinterpret_cast<LPCWSTR>(workingDirectory.utf16()),
                       &startup.StartupInfo, &d->processInfo);
    const DWORD startError = GetLastError();

    SetStdHandle(STD_INPUT_HANDLE, savedInput);
    SetStdHandle(STD_OUTPUT_HANDLE, savedOutput);
    SetStdHandle(STD_ERROR_HANDLE, savedError);

    if (!startedChild) {
        return fail(QStringLiteral("Could not start %1: %2")
                        .arg(params.program, formatLastError(startError)));
    }

    report("started %s as process %lu, startup size %llu, attribute list %p",
           qPrintable(params.program), (unsigned long) d->processInfo.dwProcessId,
           (unsigned long long) startup.StartupInfo.cb, (void *) startup.lpAttributeList);

    d->running = true;
    d->exitNotifier = new QWinEventNotifier(d->processInfo.hProcess, this);
    connect(d->exitNotifier, &QWinEventNotifier::activated, this, [this] {
        if (!d->running)
            return;
        d->running = false;
        d->exitNotifier->setEnabled(false);
        DWORD exitCode = 0;
        GetExitCodeProcess(d->processInfo.hProcess, &exitCode);
        // The console host may still have buffered output; giving the reader
        // a moment to deliver it costs nothing on a process that has already
        // exited and saves the last line of a short-lived command.
        QMetaObject::invokeMethod(this, [this, exitCode] {
            Q_EMIT finished(int(exitCode));
        }, Qt::QueuedConnection);
    });

    d->reader = new PtyReader(this, d->outputRead);
    if (d->suspended)
        d->reader->suspend();
    d->reader->start();
    return true;
}

bool Pty::isRunning() const { return d->running; }

qint64 Pty::processId() const
{
    return d->processInfo.dwProcessId ? qint64(d->processInfo.dwProcessId) : 0;
}

int Pty::columns() const { return d->columns; }
int Pty::rows() const { return d->rows; }

void Pty::write(const QByteArray &bytes)
{
    if (bytes.isEmpty() || d->inputWrite == INVALID_HANDLE_VALUE)
        return;
    DWORD written = 0;
    WriteFile(d->inputWrite, bytes.constData(), DWORD(bytes.size()), &written, nullptr);
}

void Pty::resize(int columns, int rows)
{
    columns = qMax(1, columns);
    rows = qMax(1, rows);
    if (columns == d->columns && rows == d->rows)
        return;
    d->columns = columns;
    d->rows = rows;
    if (d->pseudoConsole) {
        const COORD size = {SHORT(columns), SHORT(rows)};
        ResizePseudoConsole(d->pseudoConsole, size);
        // The console host repaints the whole screen in response, which is
        // one of the ways Windows output differs from Unix output.
    }
}

void Pty::setReadingSuspended(bool suspended)
{
    d->suspended = suspended;
    if (!d->reader)
        return;
    if (suspended)
        d->reader->suspend();
    else
        d->reader->resume();
}

bool Pty::isReadingSuspended() const { return d->suspended; }

void Pty::hangup()
{
    // Closing the pseudoconsole is what a closed window looks like to the
    // child: the console host tells it the console is gone.
    d->closeHandles();
}

void Pty::terminateProcess()
{
    if (d->processInfo.hProcess && d->running)
        TerminateProcess(d->processInfo.hProcess, 1);
}

void Pty::killProcess() { terminateProcess(); }

} // namespace kvitterm
