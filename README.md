# kvit-term

A terminal emulator for Qt Quick applications, delivered as an embeddable library: a
pseudo-terminal layer for Linux, macOS and Windows, a screen and scrollback model, and a
`QQuickItem` that draws the screen and handles keyboard, mouse, selection and clipboard.

**Status: nothing is implemented yet.** This repository currently holds its licence and this
description. The first release is the library described below; the sections marked *later* are
planned but not scheduled.

## Why this exists

An application that runs developer tools in a panel — a build, a test suite, a one-off shell
command — normally connects those child processes to pipes, and a pipe is not a terminal. Every
program that checks whether its output is a terminal reacts to that. Colour turns itself off. The C
library switches from line buffering to block buffering, so a long build appears to stall and then
dump everything at once. Progress bars and spinners arrive as raw escape sequences that a plain text
view draws as garbage. Anything that wants to ask a question has nowhere to ask it, and either fails
or hangs.

Fixing that needs a pseudo-terminal, which is the kernel device pair that makes a child process
believe a person is on the other end, followed by something that turns the resulting byte stream
into a screen. That stream is not text: it is printable characters interleaved with escape sequences
that move the cursor, set colours, switch to an alternate screen, define scroll regions, report the
mouse and set the window title. Interpreting them is what a terminal emulator does, and the reason
to use an existing one is that the sequence set is large and the edge cases never end.

Qt 6 ships no pseudo-terminal API and no terminal widget. The mature Qt options — QTermWidget and
`qmltermwidget`, both derived from KDE's Konsole — are GPL-licensed, which rules them out of any
closed-source binary. `xterm.js` is excellent, and reaching it from Qt means embedding Chromium
through `QtWebEngine`: hundreds of megabytes, slower startup, and packaging work on three platforms.

This library is the missing middle: the emulation handled by
[libvterm](https://www.leonerd.org.uk/code/libvterm/), the rendering and input written against Qt
Quick directly, and a licence that permits embedding in proprietary applications.

## What it will do

- Create a pseudo-terminal and run a child on it — `openpty` on Linux and macOS, ConPTY on Windows —
  with resize, signalling and exit reporting.
- Interpret the byte stream into a screen: the visible grid with per-cell attributes, the alternate
  screen, scrollback stored as runs rather than as a grid, cursor state, title and bell, and the
  input modes a program can select.
- Draw it as a `TerminalView` item, repainting damaged regions from a glyph cache, with selection,
  mouse reporting, clipboard, input-method support and flow control that pauses reading when a
  runaway process outruns the interface.
- Report screen contents as styled text, so an application with no terminal in its interface can
  still show a build log with its colours and its carriage-return redraws applied.
- *Later:* interpret the shell-integration markers (OSC 133, OSC 7) that give an application command
  boundaries, exit codes and the shell's current directory; search the scrollback; detect file paths
  and URLs.

## What it will not do

Panels, tabs and splits; profile storage; sessions that outlive the process; writing to a user's
shell startup files; and sandboxing. Those are the embedding application's, and keeping them out is
what makes this a library rather than a feature of one program.

Also out of scope for the first release: image protocols (Sixel, iTerm2, Kitty), a `QWidget`
wrapper, multiplexing, and remote terminals over SSH.

## Intended use

```qml
import KvitTerm

TerminalView {
    anchors.fill: parent

    session: TerminalSession {
        program: "/bin/bash"
        workingDirectory: project.path
        onExited: (code) => panel.close()
    }

    font.family: "JetBrains Mono"
    font.pixelSize: 13
    palette: TerminalPalette { background: theme.base; foreground: theme.text }

    // Keys the application keeps; everything else goes to the child.
    reservedShortcuts: ["Ctrl+Shift+C", "Ctrl+Shift+V", "Ctrl+Shift+F"]
}
```

## Requirements

- **Qt 6.10** with `Core`, `Gui`, `Qml` and `Quick`, plus `Test` and `QuickTest` for the suites.
  6.10 is what this is developed and tested against; older versions are not claimed until a
  continuous-integration job builds them.
- **CMake 3.21+** and a **C++20** compiler. On Windows, **MSVC**; MinGW is neither built nor tested.
- **libvterm 0.3.3**, vendored from its release tarball. Nothing else is needed to build it: the
  tarball ships its generated character-encoding tables, so no Perl is involved.

## Licence

Mozilla Public License 2.0. Changes to this library's own files are published; linking it into an
application, including a closed-source one, is explicitly allowed.

The vendored libvterm is MIT and keeps its own notice.
