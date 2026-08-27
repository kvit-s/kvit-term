#include "kvitterm/terminalsession.h"

#include "kvitterm/pseudoterminal.h"
#include "kvitterm/screenexport.h"

#include <QtCore/QProcessEnvironment>
#include <QtCore/QTimer>

namespace kvitterm {

class TerminalSession::Private
{
public:
    Pty pty;
    Screen *screen = nullptr;
    QString program;
    QStringList arguments;
    QString workingDirectory;
    QVariantMap environment;
    bool autoStart = true;
    bool complete = false;
    bool startedOnce = false;
    // Nothing has changed for this long, and the terminal is quiet again.
    QTimer activityTimer;
    bool activity = false;
};

TerminalSession::TerminalSession(QObject *parent) : QObject(parent), d(new Private)
{
    d->screen = new Screen(80, 24, this);

    // What the child writes is interpreted; what the screen answers — a key
    // press, a mouse report, a reply to a question the program asked — goes
    // back to the child.
    connect(&d->pty, &Pty::dataAvailable, this, [this](const QByteArray &bytes) {
        d->screen->feed(bytes);
        Q_EMIT contentChanged();
    });
    connect(d->screen, &Screen::writeRequested, &d->pty, &Pty::write);
    connect(d->screen, &Screen::titleChanged, this, &TerminalSession::titleChanged);
    connect(d->screen, &Screen::bell, this, &TerminalSession::bell);
    connect(d->screen, &Screen::sizeChanged, this, &TerminalSession::sizeChanged);
    connect(&d->pty, &Pty::finished, this, [this](int exitCode) {
        Q_EMIT runningChanged();
        Q_EMIT exited(exitCode);
    });
    connect(&d->pty, &Pty::failed, this, &TerminalSession::failed);

    // Activity means the screen changed, so it is taken from the screen rather
    // than from the child's bytes: some of what a child writes — a query, a
    // mode change — alters nothing on the display, and a resize or a clear
    // alters it with the child writing nothing at all.
    d->activityTimer.setSingleShot(true);
    d->activityTimer.setInterval(1000);
    connect(&d->activityTimer, &QTimer::timeout, this, [this] {
        d->activity = false;
        Q_EMIT activityChanged();
        Q_EMIT activityEnded();
    });
    const auto noteActivity = [this] {
        d->activityTimer.start();
        if (d->activity)
            return;
        d->activity = true;
        Q_EMIT activityChanged();
        Q_EMIT activityStarted();
    };
    connect(d->screen, &Screen::damaged, this, noteActivity);
    connect(d->screen, &Screen::scrolled, this, noteActivity);
    connect(d->screen, &Screen::cursorMoved, this, noteActivity);
    connect(d->screen, &Screen::titleChanged, this, noteActivity);
    connect(d->screen, &Screen::scrollbackCleared, this, noteActivity);
}

TerminalSession::~TerminalSession()
{
    delete d;
}

QString TerminalSession::program() const { return d->program; }

void TerminalSession::setProgram(const QString &program)
{
    if (d->program == program)
        return;
    d->program = program;
    Q_EMIT programChanged();
}

QStringList TerminalSession::arguments() const { return d->arguments; }

void TerminalSession::setArguments(const QStringList &arguments)
{
    if (d->arguments == arguments)
        return;
    d->arguments = arguments;
    Q_EMIT argumentsChanged();
}

QString TerminalSession::workingDirectory() const { return d->workingDirectory; }

void TerminalSession::setWorkingDirectory(const QString &directory)
{
    if (d->workingDirectory == directory)
        return;
    d->workingDirectory = directory;
    Q_EMIT workingDirectoryChanged();
}

QVariantMap TerminalSession::environment() const { return d->environment; }

void TerminalSession::setEnvironment(const QVariantMap &environment)
{
    if (d->environment == environment)
        return;
    d->environment = environment;
    Q_EMIT environmentChanged();
}

int TerminalSession::scrollbackLimit() const { return d->screen->scrollbackLimit(); }

void TerminalSession::setScrollbackLimit(int lines)
{
    if (d->screen->scrollbackLimit() == lines)
        return;
    d->screen->setScrollbackLimit(lines);
    Q_EMIT scrollbackLimitChanged();
}

bool TerminalSession::autoStart() const { return d->autoStart; }

void TerminalSession::setAutoStart(bool autoStart)
{
    if (d->autoStart == autoStart)
        return;
    d->autoStart = autoStart;
    Q_EMIT autoStartChanged();
}

bool TerminalSession::isRunning() const { return d->pty.isRunning(); }
QString TerminalSession::title() const { return d->screen->title(); }
int TerminalSession::columns() const { return d->screen->columns(); }
int TerminalSession::rows() const { return d->screen->rows(); }
int TerminalSession::scrollbackCount() const { return d->screen->scrollbackCount(); }
bool TerminalSession::hasActivity() const { return d->activity; }
int TerminalSession::activityPeriod() const { return d->activityTimer.interval(); }

void TerminalSession::setActivityPeriod(int milliseconds)
{
    milliseconds = qMax(0, milliseconds);
    if (d->activityTimer.interval() == milliseconds)
        return;
    // A running timer takes the new period from where it is now, which is what
    // an application changing the period mid-command would expect.
    d->activityTimer.setInterval(milliseconds);
    Q_EMIT activityPeriodChanged();
}

Screen *TerminalSession::screen() const { return d->screen; }

bool TerminalSession::start()
{
    if (d->pty.isRunning())
        return true;

    Pty::Params params;
    params.program = d->program.isEmpty() ? Pty::defaultShell() : d->program;
    params.arguments = d->arguments;
    params.workingDirectory = d->workingDirectory;
    params.columns = d->screen->columns();
    params.rows = d->screen->rows();
    if (!d->environment.isEmpty()) {
        QProcessEnvironment environment = QProcessEnvironment::systemEnvironment();
        for (auto entry = d->environment.constBegin(); entry != d->environment.constEnd(); ++entry)
            environment.insert(entry.key(), entry.value().toString());
        params.environment = environment;
    }

    QString error;
    d->startedOnce = true;
    if (!d->pty.start(params, &error)) {
        Q_EMIT failed(error);
        return false;
    }
    Q_EMIT runningChanged();
    Q_EMIT started();
    return true;
}

void TerminalSession::resize(int columns, int rows)
{
    if (columns < 1 || rows < 1)
        return;
    if (columns == d->screen->columns() && rows == d->screen->rows())
        return;
    d->screen->setSize(columns, rows);
    d->pty.resize(columns, rows);
    Q_EMIT sizeChanged();
    Q_EMIT contentChanged();
}

void TerminalSession::sendText(const QString &text)
{
    d->screen->sendText(text);
}

void TerminalSession::paste(const QString &text)
{
    d->screen->paste(text);
}

void TerminalSession::close()
{
    d->pty.hangup();
    if (d->pty.isRunning())
        d->pty.terminateProcess();
}

void TerminalSession::clear()
{
    d->screen->reset(true);
    d->screen->clearScrollback();
    Q_EMIT contentChanged();
}

QString TerminalSession::screenText() const
{
    return d->screen->text(0, d->screen->rows() - 1);
}

QString TerminalSession::lineText(int row) const
{
    return d->screen->line(row).text();
}

QString TerminalSession::toHtml() const
{
    return exportHtml(*d->screen, -d->screen->scrollbackCount(), d->screen->rows() - 1);
}

void TerminalSession::setReadingSuspended(bool suspended)
{
    d->pty.setReadingSuspended(suspended);
}

bool TerminalSession::isReadingSuspended() const { return d->pty.isReadingSuspended(); }

void TerminalSession::keyPress(int qtKey, Qt::KeyboardModifiers modifiers, const QString &text)
{
    d->screen->keyPress(qtKey, modifiers, text);
}

void TerminalSession::classBegin() {}

void TerminalSession::componentComplete()
{
    d->complete = true;
    // The view sets the size from its own geometry before this runs when it
    // owns the session, so starting here gives the child the right size from
    // its first line of output.
    if (d->autoStart && !d->startedOnce)
        start();
}

} // namespace kvitterm
