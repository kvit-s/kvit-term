# Embedding kvit-term

What an application has to know to put a terminal in its interface: the three
objects it deals with, who owns which keystroke, and the things that are
deliberately left to the application rather than decided here.

Everything below assumes the library is linked and, in a static build, that
`kvitterm::ensureQmlTypesRegistered()` has been called once before any QML is
loaded. The README covers the build side.

## The three objects

**`TerminalSession`** is one terminal: a child process on a pseudo-terminal,
and the screen its output is interpreted onto. It exists independently of any
view, so an application can run a command and read the result without drawing
anything.

**`TerminalView`** draws a session and turns keyboard, mouse, clipboard and
input-method events into what the child expects. Several views can show the
same session; a view without a session draws an empty screen.

**`TerminalPalette`** is the colour scheme. The emulator keeps colours as the
program named them — "the default foreground", "colour 4", or exact
red-green-blue values — and resolves them only when drawing, so changing the
palette recolours what is already on screen without the program redrawing.

```qml
import KvitTerm

TerminalView {
    anchors.fill: parent
    font.family: "JetBrains Mono"
    font.pointSize: 11

    session: TerminalSession {
        program: "/bin/bash"                    // empty means the user's shell
        arguments: ["-i"]
        workingDirectory: project.path
        environment: ({ "GIT_PAGER": "cat" })   // added to the current environment
        scrollbackLimit: 10000
        onExited: (code) => panel.close()
        onFailed: (message) => banner.show(message)
    }

    palette: TerminalPalette {
        background: theme.base
        foreground: theme.text
        ansiColors: [theme.black, theme.red /* … up to sixteen */]
    }
}
```

A session starts as soon as its object is complete. Set `autoStart: false` and
call `start()` to decide the moment yourself — worth doing when the working
directory or the size is not known until later, since the child is told the
size at startup and a program that draws a full screen uses it immediately.

## Sizing

The view computes columns and rows from its own geometry and the font's cell
size, and tells the session, which tells the child. An application therefore
sizes the item and nothing else.

## The font

The default is the platform's own fixed-width font, so a view nobody
configures is already a terminal. A family named in QML is used as asked for
where the machine has it, and falls back to another fixed-width font where it
does not — which is the usual case rather than the odd one, since an
application cannot know that "JetBrains Mono" is installed, and `monospace` is
a fontconfig alias that exists on Linux and nowhere else. Without that
fallback Qt would substitute the proportional interface font.

A proportional font an application asks for by name is drawn as asked. Each
cell is then positioned individually rather than a run at a time, so the grid
still holds and a space still occupies a column; the text is simply spread out,
since the columns are as wide as the widest character the font draws.

## Who owns which keystroke

There is no neutral answer, so the application decides:

```qml
TerminalView {
    reservedShortcuts: ["Ctrl+Shift+T", "Ctrl+Shift+W", "F11"]
}
```

Anything named there is ignored by the view and reaches the application's own
shortcut handling. Everything else goes to the child, including sequences an
application might expect to keep, because inside a terminal they usually belong
to the program running there.

The view has a few shortcuts of its own, and a reserved sequence overrides any
of them: Ctrl+Shift+C copies, Ctrl+Shift+V pastes, Ctrl+Shift+A selects
everything, and Shift+PageUp and Shift+PageDown scroll the history. Typing
anything else returns the view to the bottom, which is what makes scrolling
back safe.

Ctrl+click activates a link. Holding Ctrl underlines the one under the pointer
and sets `hoveredLink`.

## Reading a session without a view

```cpp
session->screenText();                    // the visible screen, as text
session->lineText(0);                     // one line
session->toHtml();                        // scrollback included, styled
```

and from C++, the whole model through `session->screen()`: `line(row)` with
negative rows for the scrollback, `cell(row, column)`, `textInRange()` for a
selection, and `exportHtml()` for any range.

This is how an application shows a build log with its colours and its
carriage-return redraws applied without having a terminal in its interface: run
the command on a session, and put the exported HTML in an ordinary text view.

## Telling whether a terminal is doing something

`session.activity` is true while the screen is changing and false once it has
been still for `session.activityPeriod` milliseconds — one second by default.
Anything that alters the display counts: output from the child, a line
scrolling off, the cursor moving, the program setting a new title. A terminal
sitting at a prompt reports nothing.

```qml
TerminalSession {
    id: session
    activityPeriod: 2000                      // milliseconds of stillness
    onActivityStarted: tab.indicator.show()
    onActivityEnded: if (!tab.visible) notify("build finished")
}

Rectangle { visible: session.activity }       // or bind to the property
```

The two signals are the edges of that same property, for an application that
would rather act on the change than bind to the state.

This is a different question from which command is running, which needs the
shell's own marks; see below. Use `activity` for "is anything happening in
there", which is what an unfocused tab or a background terminal needs, and use
the shell integration when the answer has to be a command and an exit code.

## Shell integration

A terminal on its own cannot say where one command ended and the next began,
what it exited with, or which directory the shell is in, because none of that
is in the bytes. A shell can be made to say so.

```qml
ShellIntegration {
    id: shell
    session: session
    onCommandFinished: (command, exitCode) => {
        if (exitCode !== 0)
            failures.add(command, shell.outputOf(shell.commandCount - 1))
    }
}
```

`active` stays false until a mark actually arrives, so an application can tell
a shell without the snippet from a shell that has run nothing yet.
`currentDirectory` follows the shell, which is what "open the next terminal
where this one is" needs. `commandAt(index)` gives a command's text, directory,
exit status and the rows its output occupied, and `outputOf(index)` gives the
output itself.

Installing the snippet changes a user's shell configuration, so the library
does not do it. It hands over the text:

```cpp
kvitterm::shellIntegrationScript(kvitterm::Shell::Bash);
```

The pattern to copy is the demonstration application's: write the snippet to a
directory of your own, write an init file that sources the user's own
configuration and then the snippet, and start bash with `--rcfile` pointing at
it. The user's files are read and never written. For zsh the equivalent is a
`ZDOTDIR` of your own whose `.zshrc` sources theirs; for fish and PowerShell,
the snippets are `kvitterm.fish` and `kvitterm.ps1`.

## Searching, and a sticky heading

```qml
TerminalSearch { id: search; session: session; query: findField.text }
TerminalView { search: search; shellIntegration: shell }
```

Giving the view a search highlights every match and scrolls to the current one
as `findNext()` and `findPrevious()` move through them. Matches are found when
asked for rather than kept up to date as output arrives; `refresh()` after new
output.

Giving the view a `shellIntegration` populates `stickyCommand`, the command
that produced whatever is at the top of the window — bind a label to it for a
heading that stays put while its output scrolls.

## Links

The view finds web addresses and file paths with an optional line and column,
and reports what the user activated:

```qml
onLinkActivated: (link, line, character) => {
    if (link.startsWith("http")) Qt.openUrlExternally(link)
    else editor.open(link, line)
}
```

Deciding that a path should open in an editor, and which editor, is not a
terminal library's business, which is why nothing happens until an application
says so.

## Accessibility

`accessibleText` is the visible screen as plain text and changes as the screen
does. An application binds it, and sets the role and name it wants:

```qml
TerminalView {
    Accessible.role: Accessible.Terminal
    Accessible.name: qsTr("terminal")
    Accessible.description: accessibleText
}
```

## Ending a session

`close()` hangs up, which delivers SIGHUP to the child's process group on Unix
and closes the pseudoconsole on Windows — what a program sees when a window is
closed on it — and kills anything that ignores it. Sessions do not outlive the
process, and nothing is restored on the next run.
