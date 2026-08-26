#!/bin/sh
# Configure and build, with the development settings: the library shared, so
# that test executables stop copying the whole stack into themselves, and
# tests and the demonstration application built.
#
#   ./build.sh              configure and build
#   ./build.sh --test       build, then run the suites
#   ./build.sh --clean      start the build tree again
set -e

BUILD_DIR=${BUILD_DIR:-build}
QT_PREFIX=${QT_PREFIX:-$HOME/Qt/6.10.1/gcc_64}

run_tests=0
for argument in "$@"; do
    case "$argument" in
        --test) run_tests=1 ;;
        --clean) rm -rf "$BUILD_DIR" ;;
        *) echo "unknown option: $argument" >&2; exit 2 ;;
    esac
done

cmake -S . -B "$BUILD_DIR" \
    -DCMAKE_BUILD_TYPE=RelWithDebInfo \
    -DCMAKE_PREFIX_PATH="$QT_PREFIX" \
    -DKVITTERM_SHARED_LIBS=ON
cmake --build "$BUILD_DIR" --parallel "$(nproc 2>/dev/null || echo 4)"

if [ "$run_tests" = 1 ]; then
    # Serially, and offscreen: Qt Quick tests that want a surface interfere
    # with each other in parallel, and a test that opens a real window takes
    # the keyboard focus from whatever the user is doing.
    QT_QPA_PLATFORM=offscreen ctest --test-dir "$BUILD_DIR" --output-on-failure
fi
