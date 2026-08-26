# One executable per test class, which is how Qt's test framework is meant to
# be used: QTEST_MAIN generates a main() for a class, and a process may hold
# exactly one QCoreApplication-derived object, so a suite needing both a
# console application and a GUI one cannot share a process.

qt_add_executable(termstub stub/termstub.cpp)
set_target_properties(termstub PROPERTIES WIN32_EXECUTABLE OFF MACOSX_BUNDLE OFF)

function(kvitterm_unit_test name)
    qt_add_executable(${name} unit/${name}.cpp)
    target_link_libraries(${name} PRIVATE kvit-term Qt6::Test ${ARGN})
    target_include_directories(${name} PRIVATE ${PROJECT_SOURCE_DIR}/src)
    target_compile_definitions(${name} PRIVATE
        KVITTERM_STUB_PATH="$<TARGET_FILE:termstub>"
        KVITTERM_TEST_DATA_DIR="${CMAKE_CURRENT_SOURCE_DIR}/data"
    )
    add_dependencies(${name} termstub)
    add_test(NAME ${name} COMMAND ${name})
    set_tests_properties(${name} PROPERTIES ENVIRONMENT "QT_QPA_PLATFORM=offscreen")
endfunction()

kvitterm_unit_test(test_pty)
kvitterm_unit_test(test_screen)
kvitterm_unit_test(test_session)

# The view's suite: Qt Quick tests loading real QML with the module imported,
# offscreen, and asserting on the model behind the item rather than on pixels.
qt_add_executable(test_terminalview shell/shellmain.cpp)
target_link_libraries(test_terminalview PRIVATE kvit-term Qt6::QuickTest Qt6::Qml)
target_compile_definitions(test_terminalview PRIVATE
    KVITTERM_STUB_PATH="$<TARGET_FILE:termstub>"
    QUICK_TEST_SOURCE_DIR="${CMAKE_CURRENT_SOURCE_DIR}/shell"
)
add_dependencies(test_terminalview termstub)
add_test(NAME test_terminalview COMMAND test_terminalview)
set_tests_properties(test_terminalview PROPERTIES
    ENVIRONMENT "QT_QPA_PLATFORM=offscreen"
    RUN_SERIAL TRUE
)
kvitterm_unit_test(test_shellintegration)
