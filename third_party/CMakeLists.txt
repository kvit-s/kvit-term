# The vendored libvterm.
#
# Vendored rather than found, because it is packaged for Linux and for nothing
# else. KVITTERM_USE_SYSTEM_VTERM exists for distributions that would rather
# link their own copy.
#
# The sources below are the whole library: the three programs under bin/ and
# the test corpus under t/ are upstream's own and are not vendored. Upstream's
# Makefile can generate src/encoding/*.inc and src/fullwidth.inc from .tbl
# files with two Perl scripts, but the 0.3.3 release tarball this copy came
# from ships those files already generated and ships neither the inputs nor the
# scripts, so nothing here needs Perl. A copy taken from upstream's git
# repository would have the inputs instead of the outputs, and would.
if(KVITTERM_USE_SYSTEM_VTERM)
    find_package(PkgConfig REQUIRED)
    pkg_check_modules(VTERM REQUIRED IMPORTED_TARGET vterm)
    add_library(kvitterm_vterm INTERFACE)
    target_link_libraries(kvitterm_vterm INTERFACE PkgConfig::VTERM)
    return()
endif()

add_library(kvitterm_vterm STATIC
    libvterm/src/encoding.c
    libvterm/src/keyboard.c
    libvterm/src/mouse.c
    libvterm/src/parser.c
    libvterm/src/pen.c
    libvterm/src/screen.c
    libvterm/src/state.c
    libvterm/src/unicode.c
    libvterm/src/vterm.c
)
target_include_directories(kvitterm_vterm
    PUBLIC ${CMAKE_CURRENT_SOURCE_DIR}/libvterm/include
    PRIVATE ${CMAKE_CURRENT_SOURCE_DIR}/libvterm/src
)
set_target_properties(kvitterm_vterm PROPERTIES
    C_STANDARD 99
    POSITION_INDEPENDENT_CODE ON
)
if(MSVC)
    # libvterm is C99 written for POSIX toolchains: snprintf and friends are
    # standard C99, and MSVC's "unsafe function" warnings are about its own
    # alternatives rather than about a defect.
    target_compile_definitions(kvitterm_vterm PRIVATE _CRT_SECURE_NO_WARNINGS)
    target_compile_options(kvitterm_vterm PRIVATE /wd4244 /wd4245 /wd4267)
else()
    target_compile_options(kvitterm_vterm PRIVATE -Wno-unused-parameter)
endif()
