# A window with one terminal in it: the smallest thing that shows what the
# library does, and the bench the shell test suite drives.
qt_add_executable(kvitterm-demo main.cpp)

qt_add_qml_module(kvitterm-demo
    URI KvitTermDemo
    VERSION 1.0
    QML_FILES Main.qml
)

target_link_libraries(kvitterm-demo PRIVATE kvit-term Qt6::Quick Qt6::QuickControls2)
set_target_properties(kvitterm-demo PROPERTIES WIN32_EXECUTABLE OFF MACOSX_BUNDLE OFF)
