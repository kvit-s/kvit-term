# How kvit-term works

Written for somebody about to change it. `README.md` says what the library
does; this is the inside, and the reasons for the parts that are not obvious.

## The layers

Four, each usable without the one above it.

**`pty`** starts a child on a pseudo-terminal and reads and writes the other
end. It knows nothing about escape sequences.

**`screen`** interprets the byte stream into a grid of cells with a
scrollback above it, and encodes keys and mouse reports the other way. It
has no child process, no window and no drawing code, which is why its whole
behaviour can be proved by feeding it recorded bytes.

**`Session`** (the root package) holds one of each: what the child writes is
fed to the screen, and what the screen produces in answer — a key, a mouse
report, a reply to a question — is written to the child.

**`view`** draws a session on a unison panel and turns events into input.

`ShellIntegration`, `Search` and `FindLinks` sit beside the session rather
than under it: each reads the screen and produces something an application
can use, and none is needed for a terminal to work.

## Threads, and who holds the lock

A session has three goroutines of its own while its child runs: one reads
the child's output, one writes the child's input, and one waits for the
child to exit. The application calls the session from its own goroutine,
usually unison's interface thread.

- **The screen is guarded by the session's mutex.** The reader feeds each
  read (up to 64 KB) to the screen with the lock held. Every public method
  takes the lock; `Session.View` runs a function with the screen under it.
  The view copies the rows it draws out of the screen under the lock and
  draws without it, so shaping text never holds up the reader.
- **Events are delivered after the lock is let go**, through
  `Session.Dispatch`, one at a time and in order. An application on unison
  sets it to `unison.InvokeTask`; left nil, events are delivered on a
  goroutine of the session's. Content changes between two deliveries arrive
  as one event.
- **Hooks run under the lock, at the point in the stream where they
  happen.** `ShellIntegration` is built on them, because where the cursor is
  when a mark arrives is part of what the mark means: the command line runs
  from the cursor at mark B to the cursor at mark C. A hook must not call the
  session back.
- **Writes to the child never block the caller.** Key presses and replies
  are queued and written by the session's writer goroutine, so a child that
  is not reading its input cannot freeze the interface.

## Holding a fast program back

A program writing faster than the interface can draw would otherwise have
its output read and interpreted as fast as it arrives, with nothing shown in
between. A view calls `Session.HoldForDraw(true)` and then `Session.Drawn`
after each frame; while more than eight reads have been interpreted since the
last frame, the reader stops reading. The pseudo-terminal's buffer fills, and
the child blocks in its own write. A view that is not drawing — its window is
hidden, say — holds reading back for at most a quarter of a second at a time,
so a terminal nobody is looking at still makes progress. `SetReadingSuspended`
is the application's own switch for the same thing.

## The pseudo-terminal, twice

**Linux and macOS.** `creack/pty` opens the pair; the child is started with
`os/exec` in a session of its own (`Setsid`) with the slave as its
controlling terminal (`Setctty`) on all three standard descriptors. Two
details only showed up by running it:

- *The master must be non-blocking.* The library opens it in blocking mode,
  and a read blocked in the kernel cannot be interrupted: closing the file
  from another goroutine waits for that read to return, so the terminal was
  never actually hung up and the child never got SIGHUP. `pollable` duplicates
  the descriptor, makes it non-blocking and hands it to Go's poller, whose
  `Close` interrupts a pending read. Asking the file for its descriptor
  (`Fd()`) would make it blocking again, which is why `Resize` goes through
  `SyscallConn`.
- *Ignored signals are inherited.* A signal ignored in a process stays
  ignored across `exec`, so a child of an application started with SIGHUP
  ignored (by `nohup`, or by a tool that runs commands that way) would
  outlive its terminal. Go resets the signals it handles to their defaults in
  a forked child, but not the ones it found ignored, so the first `Start`
  takes over each of SIGHUP, SIGINT, SIGQUIT, SIGTERM and SIGPIPE that is
  ignored and discards it: this process keeps ignoring them, and its
  children start with the defaults. Resetting them in the child between
  fork and exec is not possible in Go, which runs no code of the caller's
  there.

A child exits before its last output is necessarily read. The session waits
for the reader to reach the end of the output, or for it to have been quiet
for 50 ms (a process the child left in the background can keep the terminal
open indefinitely), for at most a second, before it reports the exit.

**Windows** has no fork, no controlling terminal and no SIGWINCH. It has the
pseudoconsole: made with a pair of pipes, and attached to a child through a
process-creation attribute, so the child is started with `CreateProcess`
directly. Two details of starting it matter:

- *This process's standard handles are cleared while the child starts.*
  Windows copies them into the child's process parameters, where they take
  precedence over the pseudoconsole's; under a test runner they are pipes,
  and the child then writes to the runner's pipe, decides it is not on a
  terminal, and turns its colours off.
- *The console host writes sequences of its own* — hiding the cursor,
  clearing the screen, setting the title — sometimes between a child's
  characters, and repaints the whole screen on a resize. The emulator does
  not care; a test asserting on raw bytes has to strip them.

When the child exits, the pseudoconsole is closed, which makes the console
host write what it still holds and then close its pipe, so the reader sees
everything and then the end. Hanging up closes the pseudoconsole too; a child
still running three seconds later is terminated.

## The emulator, and the copy of xterm-go

xterm-go is a Go port of xterm.js's headless core, chosen in `go-ui.md`
section 8 after comparing three Go emulators with libvterm. It is young
(created 2026-03, no releases), so it is kept as a copy that changes only
deliberately. Its changes are listed in `third_party/xterm/KVIT-PATCH.md`: a
current character width table, a fix for switching autowrap off and on, a
count of lines trimmed from the top of the buffer, a way to change the
scrollback size, access to its parser, scrollback lines stored compactly,
and a fix for a panic when a full scrollback is narrowed to a few columns.

`screen/vtdiff_test.go` is the regression test: 73 recorded streams, from
real programs (vim, less, top, tmux, nano, git log) to one feature at a time,
each compared cell by cell with what libvterm made of the same bytes. 62
match exactly. The other eleven are listed with their reasons: five where
xterm-go does what xterm does and libvterm does not, one cell of `top`, and
five streams of random operations, where the counts only show that something
changed.

## Colours are resolved when drawn

A cell keeps the colour the program named: the default, an index into the
256-colour table, or exact red, green and blue. `Palette.Resolve` turns it
into pixels when drawing, so changing the palette recolours what is already
on the screen, and the program never knows.

## Rows, and numbering them so they stay put

Screen rows are numbered from the top of the visible screen: 0 is the first
visible row, negative rows are the scrollback, and `-ScrollbackCount()` is
the oldest kept. The scrollback belongs to the normal screen; a full-screen
program's alternate screen has none.

A command's output has to be found again after it has scrolled, so the shell
integration numbers rows from the first line the screen ever showed:
`ScrolledAway()` plus a screen row. `ScrolledAway` is the lines trimmed from
the top of the normal buffer (the copy's `Buffer.Trimmed`) plus the lines
above the screen; erasing the scrollback (CSI 3 J, which `clear` sends) and a
hard reset start the count again, and the screen does the erasing itself so
that the count restarts at the same point in the stream.

xterm-go re-wraps the scrollback as well as the screen on a resize, keeping a
per-line flag saying a line continues the one above (`Line.Continuation`).
Like xterm.js, xterm-go does not re-wrap the line the cursor is on: the shell
redraws the line being edited when told the new width, and re-wrapping it as
well can leave the prompt drawn twice. libvterm re-wraps it.

Reading rows back as text joins a wrapped line to the one above, and keeps
the blanks at the end of a row the line continues past, since those are
spaces the program wrote: without that, `hello     world` broken across a
wrap copies back as `helloworld`. `Screen.Text`, `TextInRange`, `HTML`, the
link finder and the search all follow the row below's flag for this.

A line in the scrollback is stored only up to its last written cell,
twelve bytes a cell, so a one-word line in a wide terminal costs the word:
10,000 lines of recorded output take 7.1 MB at 80 columns and 7.9 MB at 200.
Blanks a program wrote, and cells with a colour of their own, are kept, which
matters for the same reason as above: at the end of a row a longer line
wraps through, the blanks are spaces inside that line. The copy of xterm-go
does this (`BufferLine.Compact`, its KVIT-PATCH.md item 7).

## Drawing

Only the visible rows are drawn, from rows copied out of the screen. Grounds
are merged into runs of one colour, so a line of one colour is one fill.
Text is drawn in runs of one style and colour through kvit-ui's text layer,
with its `Grid` option: each character is placed at the start of its own
cell, whatever the font's advances. A fixed-width font's advance is rarely a
whole number of pixels, and letting the font decide drifts a run off the grid
by a pixel or two over ten characters, leaving a gap wherever the colour
changes. A character two cells wide, or one with combining marks, is drawn on
its own, centred in its cells.

The cell size is measured rather than assumed: every printable ASCII
character is laid out, and a font whose characters advance alike gets cells
of its advance rounded to a whole pixel, so every column lands on the same
pixel. A proportional font asked for by name gets cells as wide as its widest
character; on the grid, a space still takes a column. A family the machine
does not have falls back to the platform's fixed-width face, not to the
interface's proportional one. `TestEveryCellKeepsItsColumn` reads the pixels
to hold this, in both kinds of font.

## Input

Keys that send sequences of their own (Return, Tab, arrows, F1–F12 and the
rest) and characters typed with Ctrl or Alt are encoded by `screen.PressKey`
and `TypeChar`, a port of libvterm's `keyboard.c`, so a program gets the
bytes a terminal built on libvterm sends it. Plain typing arrives as
characters. Windows also delivers Ctrl+letter as a control character, which
is dropped since the key already sent it; Ctrl with Alt on Windows is AltGr
and is left to type its character. On macOS, Option types characters of its
own and Command belongs to the application.

Which keys belong to the application is the application's to say:
`SetReservedShortcuts` takes key sequences written as `Ctrl+Shift+T` or `F6`,
which the view then leaves alone. Its own are Ctrl+Shift+C, V and A (Command
on macOS, where Command+C and V work too), and Shift+Page Up and Down.

## What a screen reader is told

The view is a read-only text area holding the visible rows, one line per row,
each with the position of every character, the cursor as the caret, and the
selection. On Windows, UI Automation reads it as a read-only edit control
(`./build.sh --win-check`).

## Testing

`screen` feeds recorded bytes and asserts on the grid. Everything that needs
a child runs `internal/termstub`, which does one named thing per mode —
report whether it is on a terminal, its size, echo lines, print escape
sequences, or print every byte it is sent in hex (`raw`) — so no test depends
on a shell. One test runs a real bash, because only that proves the snippet
works; it skips itself without bash. `view`'s tests run on unison's headless
screen and read pixels where the question is where things are drawn.
`./build.sh --win-test` runs the pty, session and screen tests on Windows,
through the pseudoconsole.
