# How kvit-term works

Written for somebody about to change it. The public interface is in
`docs/embedding.md`; this is the inside.

## The layers

Four, each usable without the one above it.

**`Pty`** (`src/pty/`) allocates a pseudo-terminal, starts a child on it, and
reads and writes the other end. It knows nothing about escape sequences.

**`Screen`** (`src/core/`) interprets the byte stream into a grid of cells with
a scrollback above it, and encodes input the other way. It has no child
process, no window and no drawing code, which is why the whole of its behaviour
can be proved by feeding it recorded bytes.

**`TerminalSession`** (`src/quick/`) holds one of each and connects them: output
is fed to the screen, and what the screen answers is written to the child.

**`TerminalView`** (`src/quick/`) draws a session and turns events into input.

`ShellIntegration`, `TerminalSearch` and the link detector (`src/integration/`)
sit beside the session rather than under it: each reads a screen and produces
something an application can use, and none of them is needed for a terminal to
work.

## The pseudo-terminal, twice

On Linux and macOS, `openpty` hands back both ends. `QProcess` does the fork,
the program lookup and the reaping; its `setChildProcessModifier` callback runs
in the child after the standard descriptors are set up and before the exec,
which is the one window in which the child can start a session of its own,
claim the slave as its controlling terminal and put it on all three
descriptors. The master is read through a `QSocketNotifier` with a fixed budget
per activation, so that a process writing faster than the interface can draw
cannot keep control of the event loop. Resizing pushes the size with
`TIOCSWINSZ`, and the kernel sends SIGWINCH to the foreground process group as
a consequence, which is how a full-screen program learns to redraw.

On Windows there is no fork, no controlling terminal and no SIGWINCH. There is
the pseudoconsole, from Windows 10 version 1809: a kernel object created with a
pair of ordinary pipes, attached to a child through a process-creation
attribute. `QProcess` cannot be used, because attaching it needs an extended
startup structure and `QProcess` builds a plain one, so that path calls
`CreateProcessW` directly. Reading is done on a thread, since a pipe read has
no equivalent of the socket notifier. Two behaviours differ and will show up in
tests: the console host repaints the whole screen on resize, and it rewrites
the child's console output into escape sequences, so both the timing and the
content differ from Unix.

Both paths share one form of flow control. Suspending reading fills the pipe,
which blocks the child in its own `write`, which is what stops a runaway
process from outrunning the interface. The view suspends when damage piles up
between repaints and resumes when it paints.

## Colours are not resolved until they are drawn

A cell keeps the colour the program named: the default, an index into the
256-colour table, or exact red-green-blue values. `Palette` turns that into a
`QColor` at drawing time. Changing the colour scheme therefore recolours what
is already on the screen, and the programs inside it never know.

## Scrollback storage

A cell carrying a code point, two colours and its attributes is about twenty
bytes, so ten thousand lines of two hundred columns would be forty megabytes
per terminal, nearly all of it blanks in the default style. A stored line
instead keeps its text as code points and its styling as runs — one entry per
stretch of identical attributes — and drops trailing blanks entirely, so a
one-word line costs the word. `src/core/scrollback.cpp` packs and unpacks;
`Screen::line()` returns the unpacked form either way, so callers never see the
difference.

## Which stored lines were wrapped, and why it was hard

Re-wrapping the history when the window is resized needs to know which stored
lines began as the overflow of the line above rather than at a newline of their
own. libvterm keeps exactly that flag, in `VTermLineInfo::continuation`, and
does not pass it to the scrollback callback.

Reading it from inside the callback does not work either, and the reason is
worth writing down. In `state.c`, `scroll()` shifts the whole array of line
flags up **before** it invokes the callbacks: by the time `sb_pushline` runs,
`lineinfo[0]` describes the line that will be at the top after the scroll, and
the flag for the line actually being stored has been overwritten. This is why
Neovim's `:terminal`, which is built on the same library, re-wraps the visible
screen and leaves the history at its old line breaks.

What survives is every other line's flag. A scroll of N lines destroys the
first N and shifts the rest down, so a mirror kept in `ScreenPrivate` can be
refreshed from what libvterm still holds once the scroll has finished — and the
one value that refresh cannot supply, the line about to be stored, was recorded
by the refresh before it. Hence: consume the front of the mirror once per
stored line, and re-read the whole mirror from the next callback that arrives
after the scroll. The invariant is that between a line being marked as wrapped
and that line being stored, at least one refresh happens, which holds because
a wrapped line has to travel to the top of the screen first and every scroll on
the way triggers one.

With the flag kept, `reflowScrollback()` joins each run of continuation lines
back into one logical line, re-splits it at the new width without cutting a
double-width character in half, and stores the pieces. `libvterm` re-wraps the
visible screen itself, once `vterm_screen_enable_reflow()` has been called.

`aWrappedLineIsMarkedAsOne` and `resizingRewrapsTheScrollbackToo` in
`tests/unit/test_screen.cpp` are the cases that hold this together.

## Drawing

`TerminalView` is a `QQuickPaintedItem`. Only the visible rows are drawn, from
the screen model rather than from a cache of pixels, and only regions the
emulator reported as damaged are repainted.

Cell backgrounds are merged into runs, so a line of one colour is one fill.
Text is drawn in runs of identical styling, broken wherever a character cannot
be trusted to advance by exactly one cell — code points from U+1100 upwards,
and any cell holding combining marks — because a monospaced font's advance is
only reliable for the range it was designed for, and letting an ideograph run
inside a text run shears the whole grid after it.

If the painter turns out to be too slow for output rates nobody has hit yet,
the escape hatch is a scene-graph node with a texture atlas rather than a
different item.

## Testing

`tests/unit/` is one executable per class, which is how Qt's test framework is
designed to be used: `QTEST_MAIN` generates a `main()` for a class, and a
process may hold exactly one `QCoreApplication`-derived object. `tests/shell/`
drives real QML through Qt Quick Test, offscreen, and asserts on the model
behind the item rather than on pixels; it runs serially, since Qt Quick tests
that want a surface interfere with each other in parallel.

Anything needing a child process runs `tests/stub/termstub`, which emits a
named scenario on request, so no test depends on which shell a machine has.
The single exception runs a real bash, because it is the only way to prove the
shell-integration snippet this library ships still does what the rest of the
suite assumes; it skips itself where bash is absent.
