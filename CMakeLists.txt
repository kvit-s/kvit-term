# kvit-term — a terminal emulator for Qt Quick applications, built as a library.
#
# Three build decisions are made here rather than left to CMake's defaults,
# because each one is expensive to change once a tree exists. They are
# explained where they are made.
cmake_minimum_required(VERSION 3.21)
project(kvit-term VERSION 0.1.0 LANGUAGES C CXX)

set(CMAKE_CXX_STANDARD 20)
set(CMAKE_CXX_STANDARD_REQUIRED ON)
set(CMAKE_C_STANDARD 99)
set(CMAKE_AUTOMOC ON)

if(PROJECT_IS_TOP_LEVEL)
    set(kvitterm_default_tests ON)
else()
    set(kvitterm_default_tests OFF)
endif()

option(KVITTERM_FULL_DEBUG_INFO "Emit full (-g) debug information rather than line tables only" OFF)
option(KVITTERM_SHARED_LIBS "Build the library shared (development builds)" OFF)
option(KVITTERM_USE_SYSTEM_VTERM "Link the system libvterm instead of the vendored copy" OFF)
option(KVITTERM_BUILD_TESTS "Build the test suites" ${kvitterm_default_tests})
option(KVITTERM_BUILD_EXAMPLES "Build the demonstration application" ${kvitterm_default_tests})

# ── Debug information level.
#
# RelWithDebInfo and Debug compile at -g, which is full DWARF: line tables plus
# a description of every type and the location of every variable at every
# program counter, so that a person at a debugger prompt can print a variable.
# Qt makes that enormous and none of it is shared between translation units.
# -g1 keeps the line tables and subprogram entries, which is what a stack trace
# and a sanitizer report need, and drops the rest. Declared before
# add_subdirectory(third_party) so the vendored libvterm inherits it.
if(NOT MSVC AND NOT KVITTERM_FULL_DEBUG_INFO)
    add_compile_options($<$<COMPILE_LANGUAGE:CXX,C>:-g1>)
endif()

# ── Shared for development, static by default.
#
# Every test executable linking a static library copies what it pulls in,
# debug sections included, and with one executable per test class that
# multiplies. build.sh turns this on. It stays OFF by default because
# packaging paths usually carry no flag identifying themselves as packaging,
# and a shipped binary must stand alone.
if(KVITTERM_SHARED_LIBS)
    set(KVITTERM_LIB_TYPE SHARED)
else()
    set(KVITTERM_LIB_TYPE STATIC)
endif()

find_package(Qt6 6.10 REQUIRED COMPONENTS Core Gui Qml Quick)
if(KVITTERM_BUILD_EXAMPLES)
    find_package(Qt6 6.10 REQUIRED COMPONENTS QuickControls2)
endif()
if(KVITTERM_BUILD_TESTS)
    find_package(Qt6 6.10 REQUIRED COMPONENTS Test QuickTest)
    enable_testing()
endif()

# Where the KvitTerm QML module's generated files live in the resource system:
# `:/qt/qml/KvitTerm`, which is on the engine's default import path rather than
# the pre-6.5 `:/KvitTerm`. Unset, every configure prints a QTP0001 warning.
qt_policy(SET QTP0001 NEW)

qt_standard_project_setup(REQUIRES 6.10)
# qt_standard_project_setup turns CMAKE_RUNTIME_OUTPUT_DIRECTORY into a
# per-QML-module layout; tests locate their helper programs through generated
# paths instead, so leave it alone.

add_subdirectory(third_party)
add_subdirectory(src)

if(KVITTERM_BUILD_EXAMPLES AND EXISTS ${CMAKE_CURRENT_SOURCE_DIR}/examples/kvitterm-demo/CMakeLists.txt)
    add_subdirectory(examples/kvitterm-demo)
endif()
if(KVITTERM_BUILD_TESTS)
    add_subdirectory(tests)
endif()
