# kvit-term

[![build and test](https://github.com/kvit-s/kvit-term/actions/workflows/ci.yml/badge.svg?branch=main)](https://github.com/kvit-s/kvit-term/actions/workflows/ci.yml)

A terminal emulator for Qt Quick applications, delivered as an embeddable
library: a pseudo-terminal layer for Linux, macOS and Windows, a screen and
scrollback model, and a `QQuickItem` that draws it and handles keyboard,
mouse, selection, clipboard and input methods.

```qml
import KvitTerm

TerminalView {
    anchors.fill: parent
    session: TerminalSession { workingDirectory: project.path }
}
```

That is a working terminal running the user's shell.

![A terminal drawn by kvit-term, showing colours, text attributes and a
progress line redrawn in place](docs/terminal.png)

## Why this exists

An application that runs developer tools in a panel — a build, a test suite, a
one-off shell command — normally connects those child processes to pipes, and a
pipe is not a terminal. Every program that checks reacts to it: colour turns
itself off, the C library switches from line buffering to block buffering so a
long build appears to stall and then dump everything at once, progress bars
arrive as escape sequences a text view draws as garbage, and anything that
wants to ask a question has nowhere to ask it.

Qt 6 ships no pseudo-terminal API and no terminal widget. The mature Qt
options — QTermWidget and `qmltermwidget`, both derived from KDE's Konsole —
are GPL-licensed, which rules them out of a closed-source binary. `xterm.js` is
excellent, and reaching it from Qt means embedding Chromium through
`QtWebEngine`: hundreds of megabytes, slower startup, and packaging work on
three platforms.

This library is the middle: the escape-sequence parsing handled by
[libvterm](https://www.leonerd.org.uk/code/libvterm/), the rendering and input
written against Qt Quick directly, and a licence that permits embedding in
proprietary applications.

## What it does

- **Runs a program on a real terminal.** `openpty` on Linux and macOS, ConPTY
  on Windows. Resize reaches a running program, output written just before a
  process exits is not lost, and a process that outruns the interface has its
  reading suspended until the next repaint rather than queueing unbounded work.
- **Interprets the stream.** The visible grid with per-cell attributes, the
  alternate screen, colour in all three encodings, the cursor's shape and
  visibility, the title, the bell, mouse reporting, bracketed paste, and the
  answers a program expects when it asks the terminal about itself.
- **Keeps a scrollback that re-wraps.** Stored lines keep their styling as runs
  rather than as a grid, and widening the window re-wraps the history as well
  as the visible screen — which libvterm alone does not do, and which
  `docs/design.md` explains.
- **Draws and accepts input.** Selection by drag, word and line; clipboard on
  three platforms with the middle button pasting the X11 selection; the wheel
  scrolling the history or becoming arrow keys where a program has none;
  composed text through the input method; and a shortcut policy the embedding
  application controls.
- **Reads what the shell says about itself.** With the shipped snippet
  installed, an application gets each command's text, its exit status, its
  output and the shell's directory. Both the standard OSC 133 marks and the
  OSC 633 variants Visual Studio Code's snippets emit are understood.
- **Finds things.** Search across screen and scrollback, plain or by regular
  expression; detection of web addresses and of file paths with a line and
  column after them, which is the shape every compiler prints.
- **Reports screen contents as text or styled HTML,** so an application can
  show a coloured build log without having a terminal in its interface at all.

## What it does not do

Panels, tabs and splits; profile storage; sessions that outlive the process;
writing to a user's shell startup files; sandboxing. Those belong to the
application, and keeping them out is what makes this a library rather than a
feature of one program.

Not in this version: image protocols (Sixel, iTerm2, Kitty), a `QWidget`
wrapper, multiplexing, and terminals over SSH.

## Requirements

- **Qt 6.10** with `Core`, `Gui`, `Qml` and `Quick`; `Test` and `QuickTest` for
  the suites, `QuickControls2` for the demonstration application. Older
  versions are not claimed, because none is built or tested.
- **CMake 3.21+** and a **C++20** compiler. On Windows, **MSVC**; MinGW is
  neither built nor tested.
- **libvterm 0.3.3**, vendored under `third_party/`. Nothing else is needed:
  the release tarball it came from ships its generated character tables, so no
  Perl is involved. `-DKVITTERM_USE_SYSTEM_VTERM=ON` links a system copy
  instead.

## Building

```sh
./build.sh --test          # configure, build, and run the suites
```

The script builds the library shared, which is a development setting: every
test executable linking a static library copies what it pulls in, debug
sections included, and one executable per test class multiplies that. A
packaging build should take the default, which is static and self-contained.
Debug information is emitted at `-g1` — line tables and subprogram entries,
which is what a stack trace or a sanitizer report needs — rather than the full
DWARF the standard build types ask for; `-DKVITTERM_FULL_DEBUG_INFO=ON`
restores it for a debugger session.

## Embedding it

As a subdirectory, pinned to a commit:

```cmake
add_subdirectory(third_party/kvit-term)
target_link_libraries(myapp PRIVATE kvit-term)
```

With `FetchContent`:

```cmake
include(FetchContent)
FetchContent_Declare(kvit-term
    GIT_REPOSITORY https://github.com/kvit-s/kvit-term.git
    GIT_TAG main)
FetchContent_MakeAvailable(kvit-term)
target_link_libraries(myapp PRIVATE kvit-term)
```

Or installed:

```cmake
find_package(KvitTerm REQUIRED)
target_link_libraries(myapp PRIVATE KvitTerm::kvit-term)
```

In a **static** build, call `kvitterm::ensureQmlTypesRegistered()` once before
any QML is loaded. The QML type registration runs by itself in a shared build;
in a static one the linker discards it, and the symptom is `import KvitTerm`
failing with "TerminalView is not a type".

`docs/embedding.md` covers the interface in full: the palette, the shortcut
policy, shell integration, search, links, and reading a session's output
without a view. `docs/design.md` covers how it works inside.

## The demonstration application

```sh
./build/examples/kvitterm-demo/kvitterm-demo        # or name a program to run
```

A window with one terminal, a find bar on Ctrl+Shift+F, a heading showing which
command produced what is at the top of the window, Ctrl+click on paths and
addresses, and one shortcut the application deliberately keeps for itself.
Where the shell is bash, it installs the shell-integration snippet into a
directory of its own and starts bash with an init file that sources the user's
configuration first — the user's own files are read and never written, which is
the pattern an application should copy.

## Testing

```sh
ctest --test-dir build --output-on-failure
```

Seven suites. The emulator's is the one that grows: it feeds recorded bytes and
asserts the resulting grid, cursor, attributes and scrollback, with no window
and no child process. Everything that needs a child process runs `termstub`, a
small program that emits a named scenario on request, so no test depends on
which shell a machine has or which version it is. One case is the exception and
runs a real bash, because it is the only way to prove the snippet this library
ships still works; it skips itself where bash is absent.

## Licence

Mozilla Public License 2.0: changes to this library's own files are published,
and linking it into an application, including a closed-source one, is
explicitly allowed.

The vendored libvterm is MIT and keeps its own notice under
`third_party/libvterm/`.
