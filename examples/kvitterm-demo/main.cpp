// The demonstration application: a window, a terminal, a shell.
#include <QtGui/QGuiApplication>
#include <QtQml/QQmlApplicationEngine>
#include <QtQuickControls2/QQuickStyle>

int main(int argc, char **argv)
{
    QGuiApplication application(argc, argv);
    application.setApplicationName(QStringLiteral("kvitterm-demo"));
    application.setOrganizationName(QStringLiteral("kvit-term"));
    QQuickStyle::setStyle(QStringLiteral("Basic"));

    QQmlApplicationEngine engine;
    // A shell named on the command line, so that the demonstration can be
    // pointed at a particular program: kvitterm-demo /bin/sh
    const QStringList arguments = application.arguments();
    if (arguments.size() > 1)
        engine.setInitialProperties({{QStringLiteral("program"), arguments.at(1)}});
    engine.loadFromModule("KvitTermDemo", "Main");
    if (engine.rootObjects().isEmpty())
        return 1;
    return application.exec();
}
