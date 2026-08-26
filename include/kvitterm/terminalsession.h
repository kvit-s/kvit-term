// One terminal: a child process on a pseudo-terminal, and the screen its
// output is interpreted onto.
//
// This is the object an application creates. It holds the two layers below it
// together — what the child writes is fed to the screen, what the screen
// produces in answer is written back to the child — and it exists
// independently of any view, which is what lets an application run a command
// and read its screen without drawing anything.
#pragma once

#include <QtCore/QObject>
#include <QtCore/QVariant>
#include <QtCore/QVariantMap>
#include <QtQml/qqmlregistration.h>
#include <QtQml/QQmlParserStatus>

#include "kvitterm_global.h"
#include "screen.h"

namespace kvitterm {

class KVITTERM_EXPORT TerminalSession : public QObject, public QQmlParserStatus
{
    Q_OBJECT
    QML_ELEMENT
    Q_INTERFACES(QQmlParserStatus)

    Q_PROPERTY(QString program READ program WRITE setProgram NOTIFY programChanged)
    Q_PROPERTY(QStringList arguments READ arguments WRITE setArguments NOTIFY argumentsChanged)
    Q_PROPERTY(QString workingDirectory READ workingDirectory WRITE setWorkingDirectory
                       NOTIFY workingDirectoryChanged)
    Q_PROPERTY(QVariantMap environment READ environment WRITE setEnvironment
                       NOTIFY environmentChanged)
    Q_PROPERTY(int scrollbackLimit READ scrollbackLimit WRITE setScrollbackLimit
                       NOTIFY scrollbackLimitChanged)
    // Start as soon as the object is complete. An application that wants to
    // decide later sets this false and calls start().
    Q_PROPERTY(bool autoStart READ autoStart WRITE setAutoStart NOTIFY autoStartChanged)
    Q_PROPERTY(bool running READ isRunning NOTIFY runningChanged)
    Q_PROPERTY(QString title READ title NOTIFY titleChanged)
    Q_PROPERTY(int columns READ columns NOTIFY sizeChanged)
    Q_PROPERTY(int rows READ rows NOTIFY sizeChanged)
    Q_PROPERTY(int scrollbackCount READ scrollbackCount NOTIFY contentChanged)

public:
    explicit TerminalSession(QObject *parent = nullptr);
    ~TerminalSession() override;

    QString program() const;
    void setProgram(const QString &program);
    QStringList arguments() const;
    void setArguments(const QStringList &arguments);
    QString workingDirectory() const;
    void setWorkingDirectory(const QString &directory);
    QVariantMap environment() const;
    void setEnvironment(const QVariantMap &environment);
    int scrollbackLimit() const;
    void setScrollbackLimit(int lines);
    bool autoStart() const;
    void setAutoStart(bool autoStart);

    bool isRunning() const;
    QString title() const;
    int columns() const;
    int rows() const;
    int scrollbackCount() const;

    // The screen this session is drawing onto. A view reads it; most
    // applications never need it.
    Screen *screen() const;

    Q_INVOKABLE bool start();
    Q_INVOKABLE void resize(int columns, int rows);
    Q_INVOKABLE void sendText(const QString &text);
    Q_INVOKABLE void paste(const QString &text);
    // Ends the session the way closing a window would: the child's process
    // group is hung up, and killed if it ignores that.
    Q_INVOKABLE void close();

    // What is on the screen, as text. An application that wants the output of
    // a command it ran — to show it, to search it, or to hand it to something
    // else — needs no view and no renderer to get it.
    // Wipe the screen and the scrollback, as the `clear` command does.
    Q_INVOKABLE void clear();
    Q_INVOKABLE QString screenText() const;
    Q_INVOKABLE QString lineText(int row) const;
    // Everything the session holds, scrollback included, as styled HTML.
    Q_INVOKABLE QString toHtml() const;

    // Stop reading the child until the view has caught up. The pipe fills,
    // the child blocks in its own write, and the interface keeps repainting.
    void setReadingSuspended(bool suspended);
    bool isReadingSuspended() const;

    void keyPress(int qtKey, Qt::KeyboardModifiers modifiers, const QString &text);

    // QQmlParserStatus
    void classBegin() override;
    void componentComplete() override;

Q_SIGNALS:
    void programChanged();
    void argumentsChanged();
    void workingDirectoryChanged();
    void environmentChanged();
    void scrollbackLimitChanged();
    void autoStartChanged();
    void runningChanged();
    void titleChanged();
    void sizeChanged();
    void contentChanged();
    void started();
    void exited(int exitCode);
    void failed(const QString &message);
    void bell();

private:
    class Private;
    Private *d;
};

} // namespace kvitterm
