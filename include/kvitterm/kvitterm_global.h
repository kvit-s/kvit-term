// kvit-term — shared declaration macros.
#pragma once

#include <QtCore/QtGlobal>

#if defined(KVITTERM_STATIC_BUILD)
#  define KVITTERM_EXPORT
#elif defined(KVITTERM_BUILDING_LIBRARY)
#  define KVITTERM_EXPORT Q_DECL_EXPORT
#else
#  define KVITTERM_EXPORT Q_DECL_IMPORT
#endif
