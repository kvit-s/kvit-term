// Uses an installed kvit-term the way an application would: register the QML
// types, run a screen without a view, and read back what it holds. Prints one
// line and exits non-zero if anything is wrong, so that continuous integration
// can tell a broken package from a working one.
#include <QtCore/QDebug>
#include <QtGui/QGuiApplication>

#include <kvitterm/qmlregistration.h>
#include <kvitterm/screenexport.h>
#include <kvitterm/shellintegration.h>
#include <kvitterm/terminalsession.h>

int main(int argc, char **argv)
{
    QGuiApplication application(argc, argv);
    kvitterm::ensureQmlTypesRegistered();

    kvitterm::TerminalSession session;
    session.setAutoStart(false);
    session.screen()->feed("\x1b[32mhello\x1b[0m from an installed kvit-term\r\n");

    const QString text = session.screenText().trimmed();
    if (text != QStringLiteral("hello from an installed kvit-term")) {
        qWarning() << "unexpected screen contents:" << text;
        return 1;
    }
    if (!session.toHtml().contains(QStringLiteral("<span"))) {
        qWarning() << "the styled export produced no styling";
        return 1;
    }
    if (kvitterm::shellIntegrationScript(kvitterm::Shell::Bash).isEmpty()) {
        qWarning() << "the shell integration snippet is missing from the library";
        return 1;
    }

    qInfo().noquote() << text;
    return 0;
}
