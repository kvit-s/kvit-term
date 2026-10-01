# kvit-term

This repository is kvit-term, the terminal the Kvit apps embed: a
pseudo-terminal layer, an emulator with a scrollback, and a unison panel that
draws it. kvit-works is the one app that uses it. How the library works
inside, and why, is in `docs/design.md`.

## What is where

| Package | What it holds |
|---|---|
| `pty` | A child on a pseudo-terminal: `pty_unix.go` (creack/pty for the pair, os/exec for the child, the master made non-blocking), `pty_windows.go` (the pseudoconsole with CreateProcess) |
| `screen` | The emulator on the xterm-go copy: `screen.go` (feeding, rows, modes, text read back, mouse reports), `cell.go`, `palette.go`, `keys.go` (libvterm's key encoding), `export.go` (HTML); `vtdiff_test.go` is the comparison with libvterm |
| root (`kvitterm`) | `session.go` (a child and its screen, the locking, event delivery, activity, holding output back), `shellintegration.go`, `search.go`, `links.go`, the snippets in `shell/` |
| `view` | The unison panel: `view.go` (sizing, the font, scrolling), `draw.go`, `input.go` (keys, mouse, selection, clipboard, links), `access.go` (what a screen reader is told) |
| `cmd/kvit-term-demo` | The demonstration program: `main.go`, `check.go` (`--check`, the real-window check), `shots.go` (`--shots`) |
| `internal/termstub` | The program the tests run on a pseudo-terminal; `internal/stub` builds it for them |
| `third_party/xterm` | The copy of xterm-go; its changes are in `KVIT-PATCH.md` |
| `tools/vtdiff` | How the libvterm references in `screen/testdata/vtdiff` were made |
| `tools/win-check.ps1` | Reads what UI Automation reports about the check window, and pictures it |

## Rules that are easy to break

- **The screen is only touched under the session's lock.** Use the session's methods or `Session.View`. A hook (`screenHooks`) runs under the lock and must not call the session back. `Search` takes the lock itself, so never call it from inside `View`.
- **Events go through `Session.Dispatch`.** An application on unison sets it
  to `unison.InvokeTask`, and so do the view's tests; without it, a test
  that feeds output and then captures the window races the delivery.
- **The Unix master stays non-blocking.** Never call `Fd()` on it: that makes
  it blocking again, and a blocked read then stops `Hangup` from closing it.
- **The xterm-go copy changes only on purpose,** with the change marked in the source and listed in `KVIT-PATCH.md`, and `TestRecordedStreamsMatchLibvterm`
  run. A stream that starts or stops differing from libvterm changes its
  entry in `knownDifferences`.
- **Keys are encoded as libvterm encodes them** (`screen/keys.go`), so a program sees the same bytes for the same keys.
- **Tests never depend on a shell,** except the one that proves the bash
  snippet; everything else runs `termstub`.

## Building and checking

```sh
./build.sh               # build every package and build/kvit-term-demo
./build.sh --test        # also gofmt check, go vet on three systems, and the tests
./build.sh --race        # the tests under the race detector (cgo)
./build.sh --win-test    # the pty, session and screen tests on Windows, through the pseudoconsole
./build.sh --win-check   # the demo on Windows on PowerShell, driven and read through UI Automation
./build.sh --shots       # sample screens into build/shots
./build.sh --bench       # the emulator's throughput
./build.sh --cross       # the demo for Windows, macOS (both) and Linux
~/kvit-ui/tools/check-all.sh   # ./build.sh --test in every Kvit Go repository
```

The git hook in `.githooks/pre-commit` checks the Go files a commit changes:
`gofmt` on them and `go vet` on their packages, in a few seconds. It does
not run the tests, so run `./build.sh --test` before committing work
(`git config core.hooksPath .githooks` enables the hook in a fresh clone).

`--win-test` and `--win-check` run programs on the Windows desktop from WSL.
The check opens a window for about 40 seconds, twice, types into the shell
through the window's own key dispatch, and never sends input to the desktop.

## Conventions

- **Module path.** `github.com/kvit-s/kvit-term`. kvit-ui comes from
  `../kvit-ui` through a `replace` line, and so does kvit-ui's patched
  go-text; keep both lines.
- **unison stays unmodified.** Anything missing goes in this repository or in kvit-ui, never into unison.
- **cgo is off** except for `--race`, and every build must stay
  cross-compilable.
- **History.** Commit on `main` and keep it linear, with no branches and no
  merge commits.
