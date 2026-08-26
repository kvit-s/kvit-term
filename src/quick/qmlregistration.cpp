#include "kvitterm/qmlregistration.h"

#include <QtCore/QtGlobal>

// Qt generates this in kvit-term_qmltyperegistrations.cpp, with this same
// export decoration; taking its address from here is what makes the linker
// keep that file, and with it the registration that runs from it.
#if !defined(QT_STATIC)
#  define KVITTERM_QMLTYPE_EXPORT Q_DECL_EXPORT
#else
#  define KVITTERM_QMLTYPE_EXPORT
#endif

extern KVITTERM_QMLTYPE_EXPORT void qml_register_types_KvitTerm();

namespace kvitterm {

void ensureQmlTypesRegistered()
{
    // Volatile on purpose. Taking the address into an ordinary variable has
    // no observable effect, so the compiler removes it, the object file ends
    // up referring to nothing, and the linker discards the registration after
    // all — which shows up as "TerminalView is not a type" in a static build
    // and nowhere else. A volatile store has to be emitted.
    static void *volatile reference = reinterpret_cast<void *>(&qml_register_types_KvitTerm);
    Q_UNUSED(reference);
}

} // namespace kvitterm
