# Parity with the Qt kvit-term

This is the checklist for step 7 of the migration plan
(`~/kvit-shirei/go-ui-plan.md`): everything the Qt library in `~/kvit-term`
does, taken from its README, `docs/embedding.md`, `docs/design.md`, its
demonstration program and its 81 test cases, with the Go evidence for each
item. An item is ticked only with a test name or a check that shows it.

"Windows" after a test name means it also passes on Windows through the
pseudoconsole (`./build.sh --win-test`). "Check" means the real-window check
on Windows with PowerShell 7 (`./build.sh --win-check`).

The plan's measure for this step is parity inside kvit-works-go, which is
step 8 and does not exist yet. The last section lists what kvit-works uses of
the Qt library and the Go equivalent of each, so that step can start from it.

## The pseudo-terminal

- [x] A child on a real terminal: openpty on Linux and macOS, the
  pseudoconsole on Windows. Evidence: `TestTheChildSeesATerminal` (Windows)
- [x] The child is told the size at start. Evidence: `TestTheChildIsToldTheTerminalSize` (Windows)
- [x] A resize reaches a running program. Evidence: `TestAResizeReachesTheChild` (Windows; the Qt test was Unix only)
- [x] What is written reaches the child through its line discipline. Evidence: `TestWhatIsWrittenReachesTheChild` (Windows)
- [x] The exit code. Evidence: `TestTheExitCodeIsReported` (Windows)
- [x] Output written just before exit is not lost. Evidence: `TestOutputWrittenJustBeforeExitIsNotLost` (Windows)
- [x] A program that does not exist fails to start. Evidence: `TestAProgramThatDoesNotExistFailsToStart` (Windows)
- [x] The working directory. Evidence: `TestTheWorkingDirectoryIsHonoured` (Windows)
- [x] Hanging up ends the child; on Windows it is ended three seconds later if it has not gone. Evidence: `TestHangingUpEndsTheChild` (Windows)
- [x] Not reading holds the child back. Evidence: `TestNotReadingHoldsTheChildBack` (Windows)
- [x] TERM, COLORTERM, COLUMNS and LINES in the environment. Evidence: `TestTheEnvironmentNamesTheTerminal`
- [x] The user's shell as the default: $SHELL, else bash, else sh; PowerShell 7, else Windows PowerShell, else %COMSPEC%. Evidence: `pty.DefaultShell`; Check runs pwsh.exe
- [x] This process's standard handles kept away from a Windows child. Evidence: the Windows test run itself, whose standard handles are pipes
- [x] Children start with SIGHUP, SIGINT, SIGQUIT, SIGTERM and SIGPIPE at their defaults, whatever this process ignores. Evidence: `resetIgnoredSignals` (the Qt library did it in the child)

## The emulator

- [x] Text, carriage returns, cursor movement and erasing. Evidence: `TestPlainTextLandsOnTheScreen`, `TestACarriageReturnRedrawsTheLine`, `TestCursorMovementAndErasure`
- [x] Colour in all three encodings and the attributes. Evidence: `TestColoursAndAttributesAreKept`
- [x] Colours kept as the program named them and resolved when drawn. Evidence: `TestColoursAndAttributesAreKept`, `Palette.Resolve`
- [x] The alternate screen leaves the scrollback alone. Evidence: `TestTheAlternateScreenLeavesTheScrollbackAlone`
- [x] Lines scroll into the scrollback, within a limit that can change. Evidence: `TestLinesScrollIntoTheScrollback`, `TestTheScrollbackLimitIsHonoured`
- [x] Double-width characters take two cells. Evidence: `TestDoubleWidthCharactersOccupyTwoCells`
- [x] Emoji take two cells, as in libvterm. Evidence: `TestEmojiOccupyTwoCells` (xterm-go as found gave them one)
- [x] Combining marks stay in one cell. Evidence: `TestCombiningMarksStayInOneCell`
- [x] Title and bell. Evidence: `TestTheTitleIsReported`, `TestTheBellIsReported`
- [x] OSC commands the emulator does not act on reach the caller. Evidence: `TestUnhandledOperatingSystemCommandsReachTheCaller`
- [x] Answers to questions about the terminal. Evidence: `TestTheTerminalAnswersQuestionsAboutItself`
- [x] Cursor shape, blinking and visibility. Evidence: `TestTheCursorsShapeAndVisibilityFollowTheProgram`
- [x] The same screens as libvterm on 62 of 73 recorded streams, with the other eleven explained. Evidence: `TestRecordedStreamsMatchLibvterm`
- [ ] Whole-screen reverse video (DECSCNM, mode 5): xterm-go does not act on it. The Qt library tracked it and never drew it, so neither version shows it.

## Input the child receives

- [x] Keys become the bytes libvterm sent: Return, Tab, Shift+Tab, arrows in both cursor modes, F1–F12, Backspace, Ctrl and Alt with characters, the CSI u forms. Evidence: `TestKeysBecomeTheBytesATerminalSends`; end to end through a raw-mode child, `TestKeysReachTheProgramAsATerminalSendsThem`
- [x] Paste marked only when the program asks. Evidence: `TestPasteIsMarkedOnlyWhenTheProgramAsksForIt`
- [x] The mouse reported only when the program asks, in the X10 and SGR forms. Evidence: `TestTheMouseIsReportedOnlyWhenTheProgramAsksForIt`; from a click in the view, `TestTheMouseIsReportedWhenTheProgramAsks`
- [x] Focus in and out reported when asked for. Evidence: `TestFocusIsReportedOnlyWhenAskedFor`

## Scrollback and reading text back

- [x] A wrapped line is marked as one, in the scrollback too. Evidence: `TestAWrappedLineIsMarkedAsOne`
- [x] Resizing re-wraps the screen and the scrollback. Evidence: `TestResizingRewrapsTheVisibleScreen`, `TestResizingRewrapsTheScrollbackToo`
- [x] Narrowing to the smallest width and back keeps the spaces. Evidence: `TestNarrowingToOneColumnKeepsTheSpaces` (two columns: the emulator's smallest)
- [x] Text read back across lines and in blocks. Evidence: `TestTextIsReadBackAcrossLinesAndInBlocks`
- [x] A wrapped line copied as one, keeping the spaces it broke at. Evidence: `TestAWrappedLineIsCopiedAsOneLine`, `TestAWrappedLineKeepsTheSpacesItBrokeAt`
- [x] Plain text and styled HTML. Evidence: `TestWhatIsOnTheScreenComesOutAsStyledText`, `TestTheSessionCanBeReadAsStyledHTML`
- [ ] **Different:** the line holding the cursor is not re-wrapped on a resize; the shell redraws it. xterm.js does the same; libvterm re-wraps it. Evidence: `TestTheLineWithTheCursorIsLeftToTheProgram`
- [x] A scrollback line is stored without the cells nothing was written to, so its cost follows what it holds rather than the width: 10,000 lines of recorded output take 7.1 MB at 80 columns and 7.9 MB at 200. The Qt library packed each stored line into runs of one style for the same purpose. Evidence: `TestCompactedLinesReadBackAsTheFullLines` (the same cells as uncompacted storage, through resizes), KVIT-PATCH.md item 7
- [x] Narrowing a full scrollback to a few columns and back. Evidence: `TestNarrowingAFullScrollbackDoesNotLoseTheScreen` (xterm-go as found panicked)

## The session

- [x] Output interpreted, answers written back: colours survive the chain. Evidence: `TestAProgramsColoursSurviveTheWholeChain` (Windows)
- [x] A progress line is one line. Evidence: `TestAProgressBarLeavesOneLineRatherThanFive` (Windows)
- [x] Typing reaches the program. Evidence: `TestWhatIsTypedReachesTheProgramAndComesBack` (Windows)
- [x] The size a view chooses is what the program sees. Evidence: `TestTheSizeTheViewChoosesIsWhatTheProgramSees` (Windows)
- [x] Started, exited with its status, failed with a message. Evidence: `TestTheExitStatusIsReported`, `TestAProgramThatCannotStartFails` (Windows)
- [x] The title. Evidence: `TestATitleSetByTheProgramIsReported` (Windows)
- [x] Activity follows the screen and ends after a quiet period; an idle terminal reports none. Evidence: `TestActivityFollowsTheScreenAndEndsAfterTheQuietPeriod`, `TestATerminalNobodyIsUsingReportsNoActivity` (Windows)
- [x] Closing ends the child. Evidence: `TestClosingTheSessionEndsTheChild` (Windows)
- [x] Clear wipes the screen and the scrollback. Evidence: `TestClearWipesTheScreenAndTheScrollback`
- [x] Screen text, a line's text, HTML. Evidence: `ScreenText`, `LineText`, `HTML`
- [x] A session that exited starts again on the same screen, which kvit-works relies on. Evidence: `TestAnExitedSessionStartsAgainOnTheSameScreen` (Windows)
- [x] Output held back until a view has drawn it. Evidence: `TestOutputIsHeldBackUntilItIsDrawn` (Windows)
- [ ] **Different:** no `autoStart`. A QML session started itself once its object was complete; a Go session starts when `Start` is called, which is what kvit-works did anyway (`autoStart: false`).

## Shell integration

- [x] Nothing claimed until a mark arrives. Evidence: `TestNothingIsClaimedUntilAMarkArrives`
- [x] A command with its text and status; a failing one. Evidence: `TestACommandIsRecordedWithItsTextAndItsStatus`, `TestAFailingCommandKeepsItsStatus`
- [x] A command's output read back, and after it has scrolled. Evidence: `TestTheOutputOfACommandCanBeReadBack`, `TestOutputIsFoundAfterItScrolls`
- [x] A command that printed nothing has no output. Evidence: `TestACommandThatPrintedNothingHasNoOutput`
- [x] The directory, percent-encoded, and Windows drive and network paths. Evidence: `TestTheDirectoryTheShellIsInIsReported`
- [x] Visual Studio Code's OSC 633 marks, with its escaping. Evidence: `TestVisualStudioCodesOwnMarksAreUnderstoodToo`
- [x] A command that never reports is closed by the next prompt. Evidence: `TestACommandThatNeverReportsIsClosedByTheNextPrompt`
- [x] The four snippets inside the library. Evidence: `TestTheSnippetsShipInsideTheLibrary`
- [x] The bash snippet in a real bash. Evidence: `TestTheBashSnippetWorksInARealShell`
- [x] The marks through a real pseudo-terminal. Evidence: `TestTheMarksSurviveARealPseudoTerminal` (Windows)
- [x] **Better:** the PowerShell snippet reports its directory and marks where each command starts, so commands are recorded with their text, status and output. Evidence: Check. The Qt snippet wrote the directory as `file://HOSTC:\path`, which no URL parser reads, and had no start mark, so PowerShell commands were never recorded.

## Search and links

- [x] Search over the screen and the scrollback, every occurrence, case, regular expressions, an invalid one, next and previous with wrapping, new output, the empty query. Evidence: the eight Qt cases, `TestMatchesAreFound…` to `TestAnEmptyQueryMatchesNothing`
- [x] Matches placed in cells after wide characters. Evidence: `TestMatchesAreInCellsAfterWideCharacters`
- [x] Addresses and paths with line and column, trailing punctuation, brackets, no double reports, prose left alone, order, a line of cells. Evidence: the nine Qt cases, `TestWebAddressesAreFound` to `TestALineOfCellsCanBeSearchedDirectly`
- [ ] **Different:** regular expressions use Go's syntax (RE2) rather than Qt's (PCRE): no look-around and no back-references.

## The view

- [x] The grid follows the view's size and reaches the child. Evidence: `TestTheGridFollowsTheViewSize`
- [x] Output reaches the screen; typing reaches the child. Evidence: `TestAProgramsOutputReachesTheScreen`, `TestWhatIsTypedReachesTheChild`; Check
- [x] Selection read back; drag, double click for a word, triple click for a line. Evidence: `TestSelectionCanBeReadBack`, `TestDraggingAndDoubleClickingSelect`
- [x] Copy and paste through the clipboard. Evidence: `TestCopyAndPasteGoThroughTheClipboard`; Check (Windows clipboard)
- [x] Scrolling back, and typing returns to the bottom. Evidence: `TestScrollingBackAndReturning`
- [x] The wheel scrolls the history, holds still while output arrives, becomes arrow keys for a full-screen program, and reports to a program that asked. Evidence: `TestTheWheelScrollsTheHistoryOrBecomesArrowKeys`; the reports through `mouseWheel`
- [x] A blinking cursor is not activity. Evidence: `TestABlinkingCursorIsNotActivity`
- [x] A reserved shortcut is not consumed; key sequences read as Qt writes them. Evidence: `TestAReservedShortcutIsNotConsumed`, `TestShortcutsAreReadAsQtWritesThem`
- [x] Ctrl+click on a link, the underline and hovered link while Ctrl is held, a link broken by the window found whole. Evidence: `TestCtrlClickActivatesALink`, `TestALinkBrokenByTheWindowIsFoundWhole`
- [x] The sticky command. Evidence: `TestTheStickyCommandNamesTheOutputAtTheTop`
- [x] Search matches drawn, the current one scrolled into view. Evidence: `TestTheCurrentMatchIsScrolledIntoView`
- [x] A family the machine lacks falls back to a fixed-width face. Evidence: `TestAnUnknownFamilyFallsBackToAFixedWidthFont`
- [x] Every cell keeps its column, in a fixed-width and a proportional font. Evidence: `TestEveryCellKeepsItsColumn`
- [x] Grounds, the palette and the cursor drawn. Evidence: `TestPaletteAndCursorAreDrawn`; the Qt sample drawn beside the Qt image, `./build.sh --shots`
- [x] What a screen reader is told: the visible rows as a read-only text area, with lines, caret and selection. Evidence: `TestAScreenReaderIsToldTheVisibleText`; Check (UI Automation reads a read-only edit control holding the keyboard focus). The Qt view gave the text as a description for the application to attach.
- [x] Drawn in a real window on Windows, with OpenGL and with the software renderer. Evidence: Check
- [ ] **Different:** the middle button pastes the clipboard. unison has no access to the X11 primary selection, which the Qt view pasted on Linux.
- [ ] **Not built:** input methods for Chinese, Japanese and Korean. The owner dropped them as a requirement on 2026-09-26; characters from dead keys and other composed input arrive as typing.
- [ ] **Different:** a change redraws the whole view rather than the damaged rows. A frame of 45 rows draws in a few milliseconds; nothing has needed less.

## The demonstration program

- [x] One terminal on the user's shell, with bash and PowerShell given the snippet from a directory of the program's own, the user's files read and never written. Evidence: `shellIntegrationArguments`; Check
- [x] A find bar on Ctrl+Shift+F with a count, Previous, Next and Close; a heading with the sticky command and the directory; "N lines back"; a status line for notes. Evidence: `demo`, `./build.sh --shots`
- [x] One shortcut the application keeps (Ctrl+Shift+N), and Ctrl+click opening addresses and naming paths. Evidence: `demo.keyDown`, `demo.openLink`
- [x] **Addition:** the terminal takes the Kvit theme's ground, text, accent and selection, with kvit-works' two sets of sixteen colours. Evidence: `demo.applyTheme`

## What kvit-works uses, and the Go equivalent

kvit-works-go (step 8) will need these; each exists.

| kvit-works (Qt) | kvit-term-go |
|---|---|
| `TerminalSession` made in C++ with `setProgram`, `setArguments`, `setWorkingDirectory`, `setScrollbackLimit(10000)`, `setAutoStart(false)`, then `start()` | `NewSession`, the `Program`, `Args` and `Dir` fields, `SetScrollbackLimit`, `Start` |
| `exited` and `failed` signals, `isRunning()`, `title()`; `start()` again on an exited session | `Observe` with `Exited` and `Failed`, `Running`, `Title`; `Start` again (`TestAnExitedSessionStartsAgainOnTheSameScreen`) |
| `Pty::defaultShell()` | `pty.DefaultShell` |
| `TerminalPalette` from the theme, with dark and light ANSI tables | `screen.Palette`, `View.SetPalette` (the demo's `applyTheme` is the same code) |
| `TerminalView` with `font` from the interface's monospace family and body size | `View.SetFont(family, size)` |
| `reservedShortcuts: ["F6", "Shift+F6"]` | `View.SetReservedShortcuts("F6", "Shift+F6")` |
| `Keys.onShortcutOverride`: the terminal takes every key before the application's shortcuts while running | `View.Claims`, asked from the window's `OnKeyDown` |
| `Accessible.role`, `name`, `description: accessibleText` | `View.Accessibility.Name`; the view describes itself (`ProvideAccessibility`), and `AccessibleText` remains |
| `linkActivated(link, line, character)`, a relative path resolved against the track's folder | `View.OnLinkActivated` |
| `forceActiveFocus` | `View.RequestFocus` |
