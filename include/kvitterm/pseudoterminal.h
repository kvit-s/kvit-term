// The pseudo-terminal: a child process that believes it is talking to a
// person.
//
// A pseudo-terminal is a pair of kernel devices. The child opens one end as
// its controlling terminal and cannot tell it from the real thing, so it
// leaves its colours on, flushes a line at a time, draws progress bars, and
// has somewhere to ask a question. This class owns the other end: what the
// child writes arrives as `dataAvailable`, what is written here arrives at the
// child's standard input, and a resize is reported to the child as the window
// size change it expects.
//
// There is no emulation here. The bytes that arrive are a raw terminal stream,
// which `Screen` interprets.
//
// The file is named pseudoterminal.h rather than pty.h because the system's
// own <pty.h> would otherwise be shadowed wherever this directory is on the
// include path, which is a hard bug to read when it happens.
#pragma once

#include <QtCore/QObject>
#include <QtCore/QProcessEnvironment>
#include <QtCore/QStringList>

#include "kvitterm_global.h"

namespace kvitterm {

class PtyPrivate;

class KVITTERM_EXPORT Pty : public QObject
{
    Q_OBJECT

public:
    struct Params
    {
        QString program;                  // an absolute path, or a name found on PATH
        QStringList arguments;
        QString workingDirectory;         // empty means inherit
        QProcessEnvironment environment;  // empty means the current environment
        int columns = 80;
        int rows = 24;

        // Added to the environment unless it already sets them. TERM is what
        // a program reads to decide which escape sequences it may use, so a
        // terminal that claims more than it implements produces garbage; this
        // library implements what xterm-256color describes.
        QString term = QStringLiteral("xterm-256color");
        QString colorTerm = QStringLiteral("truecolor");
    };

    explicit Pty(QObject *parent = nullptr);
    ~Pty() override;

    // Allocates the pseudo-terminal and starts the child on it. Returns false
    // and fills `error` if either fails; a child that starts and then exits
    // immediately is a success here and arrives as `finished`.
    bool start(const Params &params, QString *error = nullptr);

    bool isRunning() const;
    qint64 processId() const;
    int columns() const;
    int rows() const;

    void write(const QByteArray &bytes);
    void resize(int columns, int rows);

    // Stop reading without stopping the child. The child fills the pipe, then
    // blocks in write(2), which is how a runaway process is prevented from
    // outrunning whatever is drawing its output.
    void setReadingSuspended(bool suspended);
    bool isReadingSuspended() const;

    // Close this end. The child's foreground process group gets SIGHUP, which
    // is what it would get if a person closed the window.
    void hangup();

    // Escalating, for a child that ignores a hangup.
    void terminateProcess();
    void killProcess();

    // The user's login shell, or a reasonable one if the system does not say.
    static QString defaultShell();

Q_SIGNALS:
    void dataAvailable(const QByteArray &bytes);
    void finished(int exitCode);
    void failed(const QString &message);

private:
    friend class PtyPrivate;
    PtyPrivate *d;
};

} // namespace kvitterm
