# The public headers are listed as sources so that Qt's meta-object compiler
# sees them: it scans a target's own files, and a header included from a
# source of a different name is not one of them.
set(KVITTERM_HEADERS
    ${PROJECT_SOURCE_DIR}/include/kvitterm/kvitterm_global.h
    ${PROJECT_SOURCE_DIR}/include/kvitterm/cell.h
    ${PROJECT_SOURCE_DIR}/include/kvitterm/palette.h
    ${PROJECT_SOURCE_DIR}/include/kvitterm/pseudoterminal.h
    ${PROJECT_SOURCE_DIR}/include/kvitterm/screen.h
    ${PROJECT_SOURCE_DIR}/include/kvitterm/screenexport.h
    ${PROJECT_SOURCE_DIR}/include/kvitterm/terminalpalette.h
    ${PROJECT_SOURCE_DIR}/include/kvitterm/terminalsession.h
    ${PROJECT_SOURCE_DIR}/include/kvitterm/terminalview.h
    ${PROJECT_SOURCE_DIR}/include/kvitterm/qmlregistration.h
    ${PROJECT_SOURCE_DIR}/include/kvitterm/shellintegration.h
)

set(KVITTERM_SOURCES
    ${KVITTERM_HEADERS}
    core/cell.cpp
    core/palette.cpp
    core/scrollback.h
    core/scrollback.cpp
    core/screen.cpp
    core/screenexport.cpp
    pty/pty_common.cpp
    integration/shellintegration.cpp
    quick/qmlregistration.cpp
    quick/terminalpalette.cpp
    quick/terminalsession.cpp
    quick/terminalview.cpp
)
if(WIN32)
    list(APPEND KVITTERM_SOURCES pty/pty_windows.cpp)
else()
    list(APPEND KVITTERM_SOURCES pty/pty_unix.cpp)
endif()

# The shell snippets are compiled into the library: an application installing
# shell integration should not have to find a file on disk that a packager may
# have put anywhere.
set(KVITTERM_SHELL_SCRIPTS
    ${PROJECT_SOURCE_DIR}/shell/kvitterm.bash
    ${PROJECT_SOURCE_DIR}/shell/kvitterm.zsh
    ${PROJECT_SOURCE_DIR}/shell/kvitterm.fish
    ${PROJECT_SOURCE_DIR}/shell/kvitterm.ps1
)
set(KVITTERM_GENERATED_SCRIPTS ${CMAKE_CURRENT_BINARY_DIR}/shellscripts_generated.cpp)
add_custom_command(
    OUTPUT ${KVITTERM_GENERATED_SCRIPTS}
    COMMAND ${CMAKE_COMMAND}
            -DOUTPUT=${KVITTERM_GENERATED_SCRIPTS}
            "-DINPUTS=${KVITTERM_SHELL_SCRIPTS}"
            -P ${PROJECT_SOURCE_DIR}/cmake/embed_shell_scripts.cmake
    DEPENDS ${KVITTERM_SHELL_SCRIPTS} ${PROJECT_SOURCE_DIR}/cmake/embed_shell_scripts.cmake
    COMMENT "Embedding the shell integration snippets"
    VERBATIM
)
set_source_files_properties(${KVITTERM_GENERATED_SCRIPTS} PROPERTIES GENERATED TRUE SKIP_AUTOMOC ON)

qt_add_library(kvit-term ${KVITTERM_LIB_TYPE} ${KVITTERM_SOURCES})
target_sources(kvit-term PRIVATE ${KVITTERM_GENERATED_SCRIPTS})
add_library(KvitTerm::kvit-term ALIAS kvit-term)

target_include_directories(kvit-term
    PUBLIC
        $<BUILD_INTERFACE:${PROJECT_SOURCE_DIR}/include>
        $<INSTALL_INTERFACE:include>
    PRIVATE
        ${CMAKE_CURRENT_SOURCE_DIR}
        # Qt generates the QML type registration with `#include <header.h>`
        # by bare file name, so the directory the headers actually live in has
        # to be reachable that way as well as by `kvitterm/header.h`.
        ${CMAKE_CURRENT_BINARY_DIR}
        ${PROJECT_SOURCE_DIR}/include/kvitterm
)
target_link_libraries(kvit-term
    PUBLIC Qt6::Core Qt6::Gui Qt6::Qml Qt6::Quick
    PRIVATE kvitterm_vterm
)

# The QML module. NO_PLUGIN because every type is registered from C++ through
# QML_ELEMENT and the library is linked into the application directly; there
# is no plugin for an engine to load and no .qml file to ship.
qt_add_qml_module(kvit-term
    URI KvitTerm
    VERSION 1.0
    NO_PLUGIN
    DEPENDENCIES QtQuick
    # The generated qmldir and type description have to sit in a directory
    # whose path ends in the module's own name, or qmllint cannot find them.
    OUTPUT_DIRECTORY ${CMAKE_BINARY_DIR}/qmlmodules/KvitTerm
)
target_compile_definitions(kvit-term
    PRIVATE KVITTERM_BUILDING_LIBRARY
    PUBLIC $<$<NOT:$<BOOL:${KVITTERM_SHARED_LIBS}>>:KVITTERM_STATIC_BUILD>
)
set_target_properties(kvit-term PROPERTIES
    VERSION ${PROJECT_VERSION}
    SOVERSION ${PROJECT_VERSION_MAJOR}
)

# The pseudoconsole appeared in Windows 10 version 1809, and the SDK hides its
# process-creation attribute behind the version macros: without these, the
# header compiles but PROC_THREAD_ATTRIBUTE_PSEUDOCONSOLE_HANDLE is undefined.
if(WIN32)
    target_compile_definitions(kvit-term PRIVATE
        _WIN32_WINNT=0x0A00
        NTDDI_VERSION=0x0A000006      # NTDDI_WIN10_RS5
        WIN32_LEAN_AND_MEAN
        NOMINMAX
    )
endif()

# openpty(3) lives in libutil on Linux and the BSDs; on macOS it is part of
# the system library and there is nothing to link.
if(UNIX AND NOT APPLE)
    target_link_libraries(kvit-term PRIVATE util)
endif()
