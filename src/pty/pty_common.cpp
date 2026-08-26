// The parts of Pty that are the same everywhere.
#include "kvitterm/pty.h"

#include <QtCore/QFileInfo>
#include <QtCore/QStandardPaths>

namespace kvitterm {

QString Pty::defaultShell()
{
#ifdef Q_OS_WIN
    // PowerShell if it is there, the command prompt if it is not. cmd.exe is
    // always present, which is the only reason it is the fallback.
    for (const QString &candidate : {QStringLiteral("pwsh.exe"), QStringLiteral("powershell.exe")}) {
        const QString found = QStandardPaths::findExecutable(candidate);
        if (!found.isEmpty())
            return found;
    }
    const QString comspec = qEnvironmentVariable("COMSPEC");
    return comspec.isEmpty() ? QStringLiteral("cmd.exe") : comspec;
#else
    const QString fromEnvironment = qEnvironmentVariable("SHELL");
    if (!fromEnvironment.isEmpty() && QFileInfo(fromEnvironment).isExecutable())
        return fromEnvironment;
    for (const QString &candidate : {QStringLiteral("/bin/bash"), QStringLiteral("/bin/sh")}) {
        if (QFileInfo(candidate).isExecutable())
            return candidate;
    }
    return QStringLiteral("/bin/sh");
#endif
}

} // namespace kvitterm
