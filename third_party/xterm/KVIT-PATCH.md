# Kvit's copy of xterm-go

This directory is a copy of github.com/gitpod-io/xterm-go at commit
`dae5128cb6b3` (2026-09-07, module version
`v0.0.0-20260907130418-dae5128cb6b3`), a Go port of xterm.js's headless core
under the MIT licence (`LICENSE`). It is kept here rather than used as a
dependency because the project is young — created in 2026-03, mostly
machine-written, with no releases — so kvit-term changes only when this copy
is changed on purpose, as the Qt kvit-term kept libvterm.

Its tests run with the rest (`go test ./third_party/xterm`), and
`screen/vtdiff_test.go` compares the emulator with libvterm on 73 recorded
streams. Upstream's own formatting is kept, so `build.sh` leaves this
directory out of its gofmt check.

## Changes from upstream

Each is marked in the source with "Kvit's change" or "Kvit's addition".

1. **Current character widths** (`unicode.go`). Upstream uses xterm.js's
   Unicode 6 table, where every emoji is one column wide. The widths now come
   from the current Unicode data in `golang.org/x/text/width`: two columns for
   East Asian Wide and Fullwidth characters, which includes the emoji drawn
   as emoji by default; zero for combining and formatting marks and the Hangul
   vowel and final jamo; one otherwise. That is what wcwidth(3) answers in a
   current C library, so a shell measuring its own prompt agrees with the
   terminal. `unicode_test.go` expects the new width for its emoji case. With
   it, the emoji stream in the vtdiff set matches libvterm.
2. **Switching autowrap off and on** (`inputhandler.go`, `Print`). With
   autowrap off, upstream left the cursor waiting to wrap after the last
   column, so the first character written after autowrap came back on started
   a new line; xterm keeps the cursor on the last column and overwrites it.
   The vtdiff stream `wrap-off` found it and now matches libvterm.
3. **`Buffer.Trimmed`** (`buffer.go`): how many lines have been removed from
   the top of a buffer's lines since it was made. With `YBase` it numbers a
   line from the first one ever written, which the shell integration needs to
   find a command's output after it has scrolled.
4. **`Terminal.SetScrollback`** (`terminal.go`, `bufferset.go`): upstream
   fixes the scrollback size when the buffers are made.
5. **`Terminal.Parser`** (`terminal.go`): access to the escape-sequence
   parser, for the fallback that hands every OSC command no handler claims to
   the application.
6. **The conformance test's import path** (`conformance_test.go`), which
   names this copy.

## Updating

Compare upstream's history since `dae5128cb6b3` with the list above, copy the
new files over this directory, reapply the changes, and run
`go test ./third_party/xterm ./screen`. A change in the vtdiff results is
either an improvement, whose entry in `knownDifferences` is removed, or a
regression.
