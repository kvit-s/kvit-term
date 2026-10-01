# kvit-term

A terminal for Go applications drawn with [unison](https://github.com/richardwilkes/unison):
a pseudo-terminal layer for Linux, macOS and Windows, a screen and scrollback
model, and a unison panel that draws it and handles the keyboard, the mouse,
selection and the clipboard.

```go
s := kvitterm.NewSession()          // the user's shell, 80 × 24 until a view sizes it
s.Dispatch = unison.InvokeTask      // events on the interface's thread
v := view.New(ui.Fonts)             // ui is a kvit-ui UI
v.SetSession(s)
s.Start()
```

That is a working terminal running the user's shell.

## Why it exists

A program started with its output connected to a pipe behaves differently
from one on a terminal: colour turns itself off, the C library buffers output
in large blocks so a build looks stalled and then dumps everything at once,
progress bars arrive as escape sequences, and a program that asks a question
has nowhere to ask it. An application that runs developer tools — a build, a
test suite, a one-off command — needs a pseudo-terminal under them, and an
emulator to turn what they write into a screen.

## What it does

- **Runs a program on a real terminal** (`pty`): `openpty` on Linux and
  macOS, the pseudoconsole on Windows. A resize reaches a running program,
  output written just before a process exits is not lost, and a program that
  writes faster than the interface draws is held back rather than queueing
  unbounded work.
- **Interprets the stream** (`screen`): the visible grid with per-cell
  colours and attributes, the alternate screen, colour in all three
  encodings, the cursor's shape and visibility, the title, the bell, mouse
  reporting, bracketed paste, focus reports, and the answers a program
  expects when it asks the terminal about itself. The escape sequences are
  handled by xterm-go, a Go port of xterm.js kept as a copy in
  `third_party/xterm`.
- **Keeps a scrollback that re-wraps**: widening the window re-wraps the
  history as well as the visible screen.
- **Draws and takes input** (`view`): every character on its cell, whatever
  the font's own advances; selection by drag, double click (word) and triple
  click (line); the clipboard; the wheel scrolling the history, or sending
  arrow keys to a full-screen program, or reports to one that asked; and
  shortcuts the application reserves for itself.
- **Reads what the shell says about itself** (`ShellIntegration`): with the
  shipped snippet, each command's text, its exit status, its output and the
  shell's directory, from the standard OSC 133 marks and Visual Studio Code's
  OSC 633 ones. Snippets for bash, zsh, fish and PowerShell are in `shell/`
  and inside the library (`ShellScript`).
- **Finds things** (`Search`, `FindLinks`): search across the screen and the
  scrollback, plain or by regular expression; web addresses, and file paths
  with a line and column after them, which is the shape compilers print.
- **Reports its contents as text or styled HTML** (`screen.HTML`), so an
  application can show a coloured build log without a terminal in its
  interface.
- **Tells a screen reader what is on the screen**: the visible rows as a
  read-only text area, with the cursor as its caret.

Not provided, and left to the application: tabs and splits, profiles,
sessions that outlive the process, and writing to a user's shell startup
files. Not in this version: image protocols (Sixel, iTerm2, Kitty), input
methods for Chinese, Japanese and Korean, and the X11 primary selection.

## Packages

| Package | What it holds |
|---|---|
| `pty` | A child process on a pseudo-terminal: `Start`, `Read`, `Write`, `Resize`, `Hangup`; `DefaultShell` |
| `screen` | The emulator: `Screen` (feed bytes, read lines and cells, encode keys and the mouse), `Cell`, `Line`, `Palette`, `HTML` |
| `kvitterm` (the root) | `Session` (a child and its screen, safe from any goroutine), `ShellIntegration`, `Search`, `FindLinks`, the shell snippets |
| `view` | `View`, the unison panel that draws a session and turns input into bytes for it |
| `cmd/kvit-term-demo` | A window with one terminal; `--shots` draws sample screens, `--check` drives a real window |
| `third_party/xterm` | xterm-go, with the changes listed in its `KVIT-PATCH.md` |

`docs/design.md` explains how it works inside.

## Building and testing

```sh
./build.sh --test        # gofmt, go vet on three systems, and the tests
./build.sh --win-test    # the pty, session and screen tests on Windows, through its pseudoconsole
./build.sh --win-check   # the demonstration program on Windows, driven through a scripted check
./build.sh --shots       # sample screens into build/shots
```

Go 1.27 (fetched by the toolchain line in `go.mod`), with cgo off. The
library draws with kvit-ui's text layer (`../kvit-ui`), which a `replace`
line in `go.mod` points at.

## Licence

Mozilla Public License 2.0. The copy of xterm-go is MIT and keeps its own
notice in `third_party/xterm/LICENSE`.
