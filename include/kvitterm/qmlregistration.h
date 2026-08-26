// Making the QML types available in a static build.
//
// The library registers TerminalView, TerminalSession and TerminalPalette
// with the QML engine from a translation unit Qt generates. In a shared build
// that unit is loaded with the library and there is nothing to do. In a static
// build the linker discards object files nothing refers to, and a registration
// that runs by itself is exactly such a file, so `import KvitTerm` fails with
// "module not installed".
//
// Calling this once before an engine loads any QML prevents that. It is a
// no-op in a shared build.
#pragma once

#include "kvitterm_global.h"

namespace kvitterm {

KVITTERM_EXPORT void ensureQmlTypesRegistered();

}
