# Turns the shell snippets into a C++ source file, so that they travel inside
# the library rather than as files a packager has to place somewhere and an
# application has to find.
#
# A Qt resource would do the same job, at the cost of an initialiser that a
# static build discards unless something refers to it — the failure mode being
# an empty script at runtime and nothing at all at build time.
#
#   cmake -DOUTPUT=<file> -DINPUTS=<file>;<file> -P embed_shell_scripts.cmake

file(WRITE "${OUTPUT}" "// Generated from shell/ by cmake/embed_shell_scripts.cmake. Do not edit.\n\n")
file(APPEND "${OUTPUT}" "namespace kvitterm {\nnamespace generated {\n\n")
file(APPEND "${OUTPUT}" "struct ShellScript { const char *name; const char *text; };\n\n")
file(APPEND "${OUTPUT}" "static const ShellScript shellScripts[] = {\n")

foreach(input IN LISTS INPUTS)
    get_filename_component(name "${input}" NAME)
    file(READ "${input}" text)
    if(text MATCHES "\\)KVITTERM\"")
        message(FATAL_ERROR "${input} contains the raw-string delimiter used to embed it")
    endif()
    file(APPEND "${OUTPUT}" "    { \"${name}\", R\"KVITTERM(${text})KVITTERM\" },\n")
endforeach()

file(APPEND "${OUTPUT}" "    { nullptr, nullptr },\n};\n\n} // namespace generated\n} // namespace kvitterm\n")
