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
    static const auto reference = &qml_register_types_KvitTerm;
    Q_UNUSED(reference);
}

} // namespace kvitterm
