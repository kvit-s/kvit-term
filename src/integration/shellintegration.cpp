#include "kvitterm/shellintegration.h"

#include "kvitterm/screen.h"

#include <QtCore/QUrl>

// Generated from the files in shell/ at build time; see
// cmake/embed_shell_scripts.cmake.
#include "shellscripts_generated.cpp"  // NOLINT(bugprone-suspicious-include)

namespace kvitterm {

QVariantMap ShellCommand::toVariantMap() const
{
    return {
        {QStringLiteral("text"), text},
        {QStringLiteral("directory"), directory},
        {QStringLiteral("exitCode"), exitCode},
        {QStringLiteral("finished"), finished},
        {QStringLiteral("outputFirstRow"), outputFirstRow},
        {QStringLiteral("outputLastRow"), outputLastRow},
    };
}

QString shellIntegrationFileName(Shell shell)
{
    switch (shell) {
    case Shell::Bash:       return QStringLiteral("kvitterm.bash");
    case Shell::Zsh:        return QStringLiteral("kvitterm.zsh");
    case Shell::Fish:       return QStringLiteral("kvitterm.fish");
    case Shell::PowerShell: return QStringLiteral("kvitterm.ps1");
    }
    return {};
}

QString shellIntegrationScript(Shell shell)
{
    const QString wanted = shellIntegrationFileName(shell);
    for (const generated::ShellScript *script = generated::shellScripts; script->name; ++script) {
        if (wanted == QLatin1StringView(script->name))
            return QString::fromUtf8(script->text);
    }
    return {};
}

class ShellIntegration::Private
{
public:
    // Rows are counted from the first line ever written rather than from the
    // top of the screen, because the top of the screen moves. The screen
    // reports each line that scrolls away, so the count of those plus a screen
    // row is a number that stays put.
    int absoluteRow(int screenRow) const { return linesScrolledAway + screenRow; }

    // The last row a command's output actually reached. When a command's
    // output ends with a newline — as nearly all of it does — the cursor has
    // already moved to the row below, and that row belongs to the next prompt
    // rather than to this command.
    int lastOutputRow() const
    {
        if (!session)
            return 0;
        const QPoint cursor = session->screen()->cursor();
        const int row = (cursor.x() == 0 && cursor.y() > 0) ? cursor.y() - 1 : cursor.y();
        return absoluteRow(row);
    }

    void handle(int command, const QByteArray &payload);
    void handleMark(char mark, const QByteArray &rest);
    void setDirectory(const QString &directory);
    void finishCommand(int exitCode);

    ShellIntegration *q = nullptr;
    TerminalSession *session = nullptr;
    QList<ShellCommand> commands;
    QString currentDirectory;
    QPoint commandStart;        // where the command line begins, in screen coordinates
    bool active = false;
    bool commandRunning = false;
    bool haveCommandStart = false;
    int linesScrolledAway = 0;
};

void ShellIntegration::Private::setDirectory(const QString &directory)
{
    if (directory.isEmpty() || directory == currentDirectory)
        return;
    currentDirectory = directory;
    Q_EMIT q->currentDirectoryChanged();
}

void ShellIntegration::Private::finishCommand(int exitCode)
{
    if (commands.isEmpty() || commands.last().finished)
        return;
    ShellCommand &command = commands.last();
    command.exitCode = exitCode;
    command.finished = true;
    // May be one row above the first: a command that printed nothing has no
    // output rows at all, and the row where its output would have started now
    // holds the next prompt.
    command.outputLastRow = lastOutputRow();
    commandRunning = false;
    Q_EMIT q->commandsChanged();
    Q_EMIT q->commandFinished(command.text, exitCode);
}

void ShellIntegration::Private::handleMark(char mark, const QByteArray &rest)
{
    if (!active) {
        active = true;
        Q_EMIT q->activeChanged();
    }
    Screen *screen = session ? session->screen() : nullptr;

    switch (mark) {
    case 'A':
        // A prompt is about to be drawn, which means whatever ran before it
        // has finished even if no explicit status arrived.
        if (!commands.isEmpty() && !commands.last().finished)
            finishCommand(-1);
        haveCommandStart = false;
        break;
    case 'B':
        // The prompt has ended: what the user types starts here.
        if (screen) {
            commandStart = screen->cursor();
            haveCommandStart = true;
        }
        break;
    case 'C': {
        // The command has been read and is about to run.
        if (!screen)
            break;
        ShellCommand command;
        command.directory = currentDirectory;
        if (haveCommandStart) {
            const QPoint end = screen->cursor();
            command.text = screen->textInRange(commandStart,
                                               QPoint(qMax(0, end.x() - 1), end.y())).trimmed();
        }
        command.outputFirstRow = absoluteRow(screen->cursor().y());
        command.outputLastRow = command.outputFirstRow;
        commands.append(command);
        commandRunning = true;
        Q_EMIT q->commandsChanged();
        Q_EMIT q->commandStarted(command.text);
        break;
    }
    case 'D': {
        // The command finished; the status follows if the shell sent one.
        int exitCode = -1;
        const QList<QByteArray> parts = rest.split(';');
        for (const QByteArray &part : parts) {
            bool converted = false;
            const int value = part.trimmed().toInt(&converted);
            if (converted) {
                exitCode = value;
                break;
            }
        }
        finishCommand(exitCode);
        break;
    }
    case 'E':
        // Visual Studio Code's snippets report the command line explicitly
        // rather than leaving it to be read off the screen.
        if (!rest.isEmpty()) {
            QString text = QString::fromUtf8(rest);
            if (text.startsWith(QLatin1Char(';')))
                text.remove(0, 1);
            if (!commands.isEmpty() && !commands.last().finished)
                commands.last().text = text;
        }
        break;
    case 'P': {
        // 633;P;Cwd=/some/path
        const int marker = rest.indexOf("Cwd=");
        if (marker >= 0)
            setDirectory(QString::fromUtf8(rest.mid(marker + 4)));
        break;
    }
    default:
        break;
    }
}

void ShellIntegration::Private::handle(int command, const QByteArray &payload)
{
    switch (command) {
    case 7: {
        // OSC 7 reports the directory as a file URL, host included.
        const QUrl url = QUrl::fromEncoded(payload);
        if (url.isValid() && !url.path().isEmpty())
            setDirectory(QUrl::fromPercentEncoding(url.path().toUtf8()));
        break;
    }
    case 133:
    case 633: {
        if (payload.isEmpty())
            break;
        const char mark = payload.at(0);
        QByteArray rest = payload.mid(1);
        if (rest.startsWith(';'))
            rest.remove(0, 1);
        handleMark(mark, rest);
        break;
    }
    default:
        break;
    }
}

ShellIntegration::ShellIntegration(QObject *parent) : QObject(parent), d(new Private)
{
    d->q = this;
}

ShellIntegration::~ShellIntegration()
{
    delete d;
}

TerminalSession *ShellIntegration::session() const { return d->session; }

void ShellIntegration::setSession(TerminalSession *session)
{
    if (d->session == session)
        return;
    if (d->session)
        d->session->screen()->disconnect(this);

    d->session = session;
    if (d->session) {
        Screen *screen = d->session->screen();
        connect(screen, &Screen::osc, this, [this](int command, const QByteArray &payload) {
            d->handle(command, payload);
        });
        connect(screen, &Screen::scrolled, this, [this](int lines) {
            d->linesScrolledAway += lines;
        });
        connect(screen, &Screen::scrollbackCleared, this, [this] { d->linesScrolledAway = 0; });
    }
    Q_EMIT sessionChanged();
}

bool ShellIntegration::isActive() const { return d->active; }
QString ShellIntegration::currentDirectory() const { return d->currentDirectory; }
int ShellIntegration::commandCount() const { return int(d->commands.size()); }
bool ShellIntegration::isCommandRunning() const { return d->commandRunning; }
QList<ShellCommand> ShellIntegration::commands() const { return d->commands; }

QVariantMap ShellIntegration::commandAt(int index) const
{
    if (index < 0 || index >= d->commands.size())
        return {};
    return d->commands.at(index).toVariantMap();
}

QString ShellIntegration::outputOf(int index) const
{
    if (!d->session || index < 0 || index >= d->commands.size())
        return {};
    const ShellCommand &command = d->commands.at(index);
    const Screen *screen = d->session->screen();
    // Back from absolute rows to where those lines are now, which may be off
    // the top of the stored history if it has been running long enough.
    const int firstRow = command.outputFirstRow - d->linesScrolledAway;
    const int lastRow = (command.finished ? command.outputLastRow : d->lastOutputRow())
                        - d->linesScrolledAway;
    if (lastRow < firstRow)
        return {};                       // the command printed nothing
    if (lastRow < -screen->scrollbackCount())
        return {};                       // it has scrolled out of the history
    return screen->text(qMax(firstRow, -screen->scrollbackCount()), qMin(lastRow, screen->rows() - 1));
}

int ShellIntegration::commandAtScreenRow(int screenRow) const
{
    const int absolute = d->absoluteRow(screenRow);
    for (int index = int(d->commands.size()) - 1; index >= 0; --index) {
        const ShellCommand &command = d->commands.at(index);
        const int last = command.finished ? command.outputLastRow : d->lastOutputRow();
        if (absolute >= command.outputFirstRow && absolute <= last)
            return index;
    }
    return -1;
}

void ShellIntegration::clear()
{
    d->commands.clear();
    d->commandRunning = false;
    Q_EMIT commandsChanged();
}

} // namespace kvitterm
