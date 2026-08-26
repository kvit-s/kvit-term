# The public headers are listed as sources so that Qt's meta-object compiler
# sees them: it scans a target's own files, and a header included from a
# source of a different name is not one of them.
set(KVITTERM_HEADERS
    ${PROJECT_SOURCE_DIR}/include/kvitterm/kvitterm_global.h
    ${PROJECT_SOURCE_DIR}/include/kvitterm/cell.h
    ${PROJECT_SOURCE_DIR}/include/kvitterm/palette.h
    ${PROJECT_SOURCE_DIR}/include/kvitterm/pty.h
    ${PROJECT_SOURCE_DIR}/include/kvitterm/screen.h
    ${PROJECT_SOURCE_DIR}/include/kvitterm/screenexport.h
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
)
if(WIN32)
    list(APPEND KVITTERM_SOURCES pty/pty_windows.cpp)
else()
    list(APPEND KVITTERM_SOURCES pty/pty_unix.cpp)
endif()

qt_add_library(kvit-term ${KVITTERM_LIB_TYPE} ${KVITTERM_SOURCES})
add_library(KvitTerm::kvit-term ALIAS kvit-term)

target_include_directories(kvit-term
    PUBLIC
        $<BUILD_INTERFACE:${PROJECT_SOURCE_DIR}/include>
        $<INSTALL_INTERFACE:include>
    PRIVATE
        ${CMAKE_CURRENT_SOURCE_DIR}
)
target_link_libraries(kvit-term
    PUBLIC Qt6::Core Qt6::Gui
    PRIVATE kvitterm_vterm
)
target_compile_definitions(kvit-term
    PRIVATE KVITTERM_BUILDING_LIBRARY
    PUBLIC $<$<NOT:$<BOOL:${KVITTERM_SHARED_LIBS}>>:KVITTERM_STATIC_BUILD>
)
set_target_properties(kvit-term PROPERTIES
    VERSION ${PROJECT_VERSION}
    SOVERSION ${PROJECT_VERSION_MAJOR}
)

# openpty(3) lives in libutil on Linux and the BSDs; on macOS it is part of
# the system library and there is nothing to link.
if(UNIX AND NOT APPLE)
    target_link_libraries(kvit-term PRIVATE util)
endif()
