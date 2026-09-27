package main

// --check: a terminal in a real window on the user's own shell. The program
// opens its window on the desktop, drives it through the same key and
// character dispatch unison gives platform input (the window's own callback
// first, then the focused panel and its parents), waits for the shell to
// answer each step, and stays open long enough for a screen capture and a
// screen reader's view of the window to be taken from outside. It sends
// nothing to the desktop, so no keystroke can land in another program.

import (
	"fmt"
	"runtime"
	"strings"
	"time"

	"github.com/kvit-s/kvit-term/screen"
	"github.com/richardwilkes/toolbox/v2/geom"
	"github.com/richardwilkes/unison"
	"github.com/richardwilkes/unison/enums/mod"
)

// checkTitle is the window's title, which the outside check finds it by.
const checkTitle = "kvit-term check"

// checkWord is what the check types, so it can be found on the screen.
const checkWord = "kvit-term-check"

type checkStep struct {
	what    string
	do      func()
	done    func() bool
	timeout time.Duration
}

func (d *demo) press(key unison.KeyCode, mods mod.Modifiers) {
	uw := d.win.Window
	if uw.KeyDownCallback != nil && uw.KeyDownCallback(key, mods, false) {
		return
	}
	for p := uw.CurrentFocus(); p != nil; p = p.Parent() {
		if p.Enabled() && p.KeyDownCallback != nil && p.KeyDownCallback(key, mods, false) {
			return
		}
	}
}

func (d *demo) typ(s string) {
	uw := d.win.Window
	for _, ch := range s {
		for p := uw.CurrentFocus(); p != nil; p = p.Parent() {
			if p.Enabled() && p.RuneTypedCallback != nil && p.RuneTypedCallback(ch) {
				break
			}
		}
	}
}

// shellKind is which of the shells the check knows the session runs.
func (d *demo) shellKind() string {
	p := strings.ToLower(d.session.Program)
	switch {
	case strings.Contains(p, "pwsh"), strings.Contains(p, "powershell"):
		return "powershell"
	case strings.HasSuffix(p, "cmd.exe"), strings.HasSuffix(p, "cmd"):
		return "cmd"
	}
	return "posix"
}

func (d *demo) checkSteps() []checkStep {
	text := func() string { return d.session.ScreenText() }
	var redCmd string
	switch d.shellKind() {
	case "powershell":
		redCmd = "Write-Host kvit-red -ForegroundColor Red"
	case "posix":
		redCmd = `printf '\033[31mkvit-red\033[0m\n'`
	}
	grid0 := [2]int{}
	steps := []checkStep{
		{"the shell draws its prompt", nil, func() bool { return strings.TrimSpace(text()) != "" }, 15 * time.Second},
		{"a typed command runs and its output appears", func() {
			d.typ("echo " + checkWord)
			d.press(unison.KeyReturn, 0)
		}, func() bool { return strings.Count(text(), checkWord) >= 2 }, 10 * time.Second},
		{"Up recalls the command from the shell's history", func() {
			d.press(unison.KeyUp, 0)
		}, func() bool { return strings.Count(text(), checkWord) >= 3 }, 5 * time.Second},
		{"the recalled line is cleared", func() {
			if runtime.GOOS == "windows" {
				d.press(unison.KeyEscape, 0)
			} else {
				d.press(unison.KeyU, mod.Control)
			}
		}, func() bool { return strings.Count(text(), checkWord) == 2 }, 5 * time.Second},
		{"resizing the window reaches the shell's terminal", func() {
			grid0[0], grid0[1] = d.term.Grid()
			r := d.win.ContentRect()
			d.win.SetContentRect(geom.NewRect(r.X, r.Y, r.Width+160, r.Height+80))
		}, func() bool {
			c, rows := d.term.Grid()
			sc, sr := d.session.Size()
			return (c != grid0[0] || rows != grid0[1]) && c == sc && rows == sr
		}, 5 * time.Second},
		{"selecting everything and copying puts the output on the clipboard", func() {
			d.term.SelectAll()
			d.term.Copy()
		}, func() bool { return strings.Contains(unison.ClipboardGetText(), checkWord) }, 3 * time.Second},
	}
	if d.shellKind() != "cmd" {
		steps = append(steps,
			checkStep{"the shell integration reports the directory", nil, func() bool { return d.shell.Directory() != "" }, 5 * time.Second},
			checkStep{"the command is recorded with its text and status", nil, func() bool {
				c, ok := d.shell.Command(0)
				return ok && c.Finished && c.ExitCode == 0 && c.Text == "echo "+checkWord &&
					strings.Contains(d.shell.Output(0), checkWord)
			}, 5 * time.Second})
	}
	if redCmd != "" {
		steps = append(steps, checkStep{"a colour a program asks for arrives as a palette colour", func() {
			d.term.ClearSelection()
			d.typ(redCmd)
			d.press(unison.KeyReturn, 0)
		}, func() bool {
			found := false
			d.session.View(func(scr *screen.Screen) {
				for row := 0; row < scr.Rows() && !found; row++ {
					l := scr.Line(row)
					if i := strings.Index(l.Text(), "kvit-red"); i >= 0 && !strings.Contains(l.Text(), "Write-Host") && !strings.Contains(l.Text(), "printf") {
						fg := l.CellAt(i).Style.Foreground
						found = fg.Kind == screen.Indexed && (fg.Index == 1 || fg.Index == 9) || fg.Kind == screen.RGB && fg.R > 150 && fg.G < 100
					}
				}
			})
			return found
		}, 10 * time.Second})
	}
	return steps
}

// runCheck runs the steps one after another, each waiting for the shell's
// answer, reports, and closes the window when stay has passed.
func runCheck(d *demo, started time.Time, stay time.Duration) {
	steps := d.checkSteps()
	var fails []string
	var run func(i int)
	run = func(i int) {
		if i == len(steps) {
			d.report(fails, len(steps), started, stay)
			return
		}
		s := steps[i]
		if s.do != nil {
			s.do()
		}
		deadline := time.Now().Add(s.timeout)
		var wait func()
		wait = func() {
			if s.done() {
				fmt.Println("ok  ", s.what)
				unison.InvokeTaskAfter(func() { run(i + 1) }, 150*time.Millisecond)
				return
			}
			if time.Now().After(deadline) {
				fmt.Println("FAIL", s.what)
				fails = append(fails, s.what)
				unison.InvokeTaskAfter(func() { run(i + 1) }, 150*time.Millisecond)
				return
			}
			unison.InvokeTaskAfter(wait, 50*time.Millisecond)
		}
		unison.InvokeTaskAfter(wait, 50*time.Millisecond)
	}
	unison.InvokeTaskAfter(func() { run(0) }, 300*time.Millisecond)
}

func (d *demo) report(fails []string, steps int, started time.Time, stay time.Duration) {
	r := d.win.ContentRect()
	cols, rows := d.session.Size()
	fmt.Printf("window %.0f x %.0f at scale %.2f, terminal %d x %d cells, %s/%s, shell %s\n",
		r.Width, r.Height, d.win.BackingScale().X, cols, rows, runtime.GOOS, runtime.GOARCH, d.session.Program)
	if len(fails) == 0 {
		fmt.Printf("check: all %d steps passed\n", steps)
	} else {
		fmt.Printf("check: %d of %d steps failed\n", len(fails), steps)
	}
	fmt.Println("the screen now:")
	for _, line := range strings.Split(strings.TrimRight(d.session.ScreenText(), "\n"), "\n") {
		fmt.Println("  |" + line)
	}
	var ms runtime.MemStats
	runtime.GC()
	runtime.ReadMemStats(&ms)
	fmt.Printf("shell integration: active %v, directory %q, %d commands\n", d.shell.Active(), d.shell.Directory(), d.shell.CommandCount())
	for i, c := range d.shell.Commands() {
		fmt.Printf("  command %d: %q exit %d, output %q\n", i, c.Text, c.ExitCode, d.shell.Output(i))
	}
	fmt.Printf("Go heap in use %d MB, Go memory from the system %d MB\n", ms.HeapInuse>>20, ms.Sys>>20)
	unison.InvokeTaskAfter(func() {
		d.session.Close()
		d.win.Dispose()
	}, max(0, stay-time.Since(started)))
}
