# A separate project that finds kvit-term the way somebody else's application
# would, rather than building it as part of itself.
#
# It exists to be built in continuous integration after an install, because
# nothing in the library's own build exercises the installed package: the
# export set, the config file, and the include and link paths are only wrong
# for other people.
cmake_minimum_required(VERSION 3.21)
project(kvitterm-embedding-check LANGUAGES CXX)

set(CMAKE_CXX_STANDARD 20)
set(CMAKE_CXX_STANDARD_REQUIRED ON)

find_package(Qt6 6.10 REQUIRED COMPONENTS Core Gui Quick)
find_package(KvitTerm REQUIRED)
qt_standard_project_setup()

qt_add_executable(kvitterm-embedding-check main.cpp)
set_target_properties(kvitterm-embedding-check PROPERTIES WIN32_EXECUTABLE OFF MACOSX_BUNDLE OFF)
target_link_libraries(kvitterm-embedding-check PRIVATE KvitTerm::kvit-term Qt6::Quick)
