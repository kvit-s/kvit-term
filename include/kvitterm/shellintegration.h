// What a shell can tell the terminal about itself.
//
// A terminal on its own sees a stream of characters. It cannot say where one
// command ended and the next began, what any of them exited with, or which
// directory the shell is in, because none of that is in the bytes. A shell can
// be made to say so, by emitting marker escape sequences around each prompt
// and command — the standard OSC 133 marks, OSC 7 for the directory, and the
// OSC 633 variants that Visual Studio Code's own snippets emit.
//
// Everything that makes an integrated terminal feel different from a plain one
// is built on those marks: a pass or fail mark beside each command, re-running
// a recent one, jumping between them, opening the next terminal where the last
// one was, and reading a command's output without the user selecting it.
//
// This class turns the marks into a list of commands. Installing the snippet
// that emits them is the application's decision, and `shellIntegrationScript`
// hands over the text to install.
#pragma once

#include <QtCore/QObject>
#include <QtCore/QVariant>
#include <QtCore/QVariantMap>
#include <QtQml/qqmlregistration.h>

#include "kvitterm_global.h"
#include "terminalsession.h"

namespace kvitterm {

// One command, from the moment the shell said it was about to run to the
// moment it said what it exited with.
struct KVITTERM_EXPORT ShellCommand
{
    QString text;             // the command line, as typed
    QString directory;        // where the shell was when it ran
    int outputFirstRow = 0;   // counted from the first line ever written
    int outputLastRow = 0;
    int exitCode = -1;        // -1 until it finishes
    bool finished = false;

    QVariantMap toVariantMap() const;
};

enum class Shell { Bash, Zsh, Fish, PowerShell };

// The snippet a shell must run to emit the marks, as text. Writing it
// anywhere — an init file, a directory the shell is pointed at — is the
// application's business, because it means changing a user's configuration.
KVITTERM_EXPORT QString shellIntegrationScript(Shell shell);
KVITTERM_EXPORT QString shellIntegrationFileName(Shell shell);

class KVITTERM_EXPORT ShellIntegration : public QObject
{
    Q_OBJECT
    QML_ELEMENT

    Q_PROPERTY(kvitterm::TerminalSession *session READ session WRITE setSession NOTIFY sessionChanged)
    // False until a mark actually arrives: a shell without the snippet
    // installed never says anything, and an application should be able to see
    // the difference rather than show an empty command list.
    Q_PROPERTY(bool active READ isActive NOTIFY activeChanged)
    Q_PROPERTY(QString currentDirectory READ currentDirectory NOTIFY currentDirectoryChanged)
    Q_PROPERTY(int commandCount READ commandCount NOTIFY commandsChanged)
    Q_PROPERTY(bool commandRunning READ isCommandRunning NOTIFY commandsChanged)

public:
    explicit ShellIntegration(QObject *parent = nullptr);
    ~ShellIntegration() override;

    TerminalSession *session() const;
    void setSession(TerminalSession *session);

    bool isActive() const;
    QString currentDirectory() const;
    int commandCount() const;
    bool isCommandRunning() const;

    QList<ShellCommand> commands() const;
    Q_INVOKABLE QVariantMap commandAt(int index) const;
    // The output of a command that has finished, as text. This is the thing a
    // scrollback buffer alone cannot give: the bytes between two marks.
    Q_INVOKABLE QString outputOf(int index) const;
    Q_INVOKABLE void clear();

Q_SIGNALS:
    void sessionChanged();
    void activeChanged();
    void currentDirectoryChanged();
    void commandsChanged();
    void commandStarted(const QString &command);
    void commandFinished(const QString &command, int exitCode);

private:
    class Private;
    Private *d;
};

} // namespace kvitterm
