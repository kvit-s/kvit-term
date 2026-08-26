// The Qt Quick test runner for the .qml files beside this one.
//
// The setup object exists to hand the tests the path of the stub program, so
// that they spawn something whose behaviour is fixed rather than a shell whose
// version varies from machine to machine.
#include <QtCore/QObject>
#include <QtQml/QQmlContext>
#include <QtQml/QQmlEngine>
#include <QtQuickTest/quicktest.h>

#include "kvitterm/qmlregistration.h"

class Setup : public QObject
{
    Q_OBJECT

public:
    Setup() { kvitterm::ensureQmlTypesRegistered(); }

public Q_SLOTS:
    void qmlEngineAvailable(QQmlEngine *engine)
    {
        engine->rootContext()->setContextProperty(QStringLiteral("testStubPath"),
                                                  QString::fromLocal8Bit(KVITTERM_STUB_PATH));
    }
};

QUICK_TEST_MAIN_WITH_SETUP(shell, Setup)
#include "shellmain.moc"
