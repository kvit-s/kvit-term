package kvitterm

// What a shell tells the terminal about itself. The marks are fed straight
// into the screen rather than produced by a real shell, whose version and
// startup files vary; two cases at the end run the whole path through a
// pseudo-terminal, one of them with a real bash. These are the library's
// tests (tests/unit/test_shellintegration.cpp), case for case.

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
)

// promptAndCommand is what a shell with the snippet writes around one
// command: prompt, end of prompt, the command as typed, the start of its
// output, and the status it exited with.
func promptAndCommand(command, output string, code int) []byte {
	return []byte("\x1b]133;A\x1b\\$ \x1b]133;B\x1b\\" + command + "\r\n\x1b]133;C\x1b\\" + output +
		fmt.Sprintf("\x1b]133;D;%d\x1b\\", code))
}

type shellEvents struct {
	mu   sync.Mutex
	list []ShellEvent
}

func recordShell(si *ShellIntegration) *shellEvents {
	e := &shellEvents{}
	si.Observe(func(ev ShellEvent) {
		e.mu.Lock()
		e.list = append(e.list, ev)
		e.mu.Unlock()
	})
	return e
}

func (e *shellEvents) count(k ShellEventKind) int {
	e.mu.Lock()
	defer e.mu.Unlock()
	n := 0
	for _, ev := range e.list {
		if ev.Kind == k {
			n++
		}
	}
	return n
}

func TestNothingIsClaimedUntilAMarkArrives(t *testing.T) {
	s := NewSession()
	si := NewShellIntegration(s)
	if si.Active() {
		t.Fatal("active before anything was said")
	}
	s.Feed([]byte("ordinary output with no marks in it\r\n"))
	if si.Active() || si.CommandCount() != 0 {
		t.Error("ordinary output was taken for a mark")
	}
}

func TestACommandIsRecordedWithItsTextAndItsStatus(t *testing.T) {
	s := NewSession()
	si := NewShellIntegration(s)
	ev := recordShell(si)
	s.Feed(promptAndCommand("ls -l", "total 0\r\nfile.txt\r\n", 0))
	if !si.Active() || si.CommandCount() != 1 {
		t.Fatalf("active %v, %d commands", si.Active(), si.CommandCount())
	}
	c, _ := si.Command(0)
	if c.Text != "ls -l" || c.ExitCode != 0 || !c.Finished {
		t.Errorf("command = %+v", c)
	}
	if si.CommandRunning() {
		t.Error("still running")
	}
	eventually(t, "the events", wait, func() bool { return ev.count(CommandStarted) == 1 && ev.count(CommandFinished) == 1 })
}

func TestAFailingCommandKeepsItsStatus(t *testing.T) {
	s := NewSession()
	si := NewShellIntegration(s)
	s.Feed(promptAndCommand("false", "", 1))
	if c, _ := si.Command(0); c.ExitCode != 1 {
		t.Errorf("exit code %d", c.ExitCode)
	}
}

func TestTheOutputOfACommandCanBeReadBack(t *testing.T) {
	// The thing a scrollback alone cannot do: say which bytes belonged to
	// which command.
	s := NewSession()
	si := NewShellIntegration(s)
	s.Feed(promptAndCommand("echo one", "one\r\n", 0))
	s.Feed(promptAndCommand("echo two", "two\r\n", 0))
	if si.CommandCount() != 2 {
		t.Fatalf("%d commands", si.CommandCount())
	}
	first, second := si.Output(0), si.Output(1)
	if !strings.Contains(first, "one") || strings.Contains(first, "two") || !strings.Contains(second, "two") {
		t.Errorf("outputs %q and %q", first, second)
	}
}

func TestOutputIsFoundAfterItScrolls(t *testing.T) {
	s := NewSession()
	si := NewShellIntegration(s)
	s.Feed(promptAndCommand("seq 1 5", "1\r\n2\r\n3\r\n4\r\n5\r\n", 0))
	for i := 0; i < 50; i++ {
		s.Feed([]byte("filler\r\n"))
	}
	if got := si.Output(0); got != "1\n2\n3\n4\n5" {
		t.Errorf("output after scrolling = %q", got)
	}
	if row := si.CommandAtRow(-si.s.scr.ScrolledAway() + 1); row != 0 {
		t.Errorf("the command at its first output row is %d", row)
	}
}

func TestACommandThatPrintedNothingHasNoOutput(t *testing.T) {
	// The row where its output would have started holds the next prompt by
	// the time anybody asks.
	s := NewSession()
	si := NewShellIntegration(s)
	s.Feed(promptAndCommand("true", "", 0))
	s.Feed([]byte("\x1b]133;A\x1b\\$ "))
	if got := si.Output(0); got != "" {
		t.Errorf("output = %q", got)
	}
}

func TestTheDirectoryTheShellIsInIsReported(t *testing.T) {
	s := NewSession()
	si := NewShellIntegration(s)
	ev := recordShell(si)
	s.Feed([]byte("\x1b]7;file://somehost/home/someone/work\x1b\\"))
	if si.Directory() != "/home/someone/work" {
		t.Errorf("directory %q", si.Directory())
	}
	// Percent-encoding, which is how a path with a space arrives.
	s.Feed([]byte("\x1b]7;file://somehost/home/some%20one\x1b\\"))
	if si.Directory() != "/home/some one" {
		t.Errorf("directory %q", si.Directory())
	}
	eventually(t, "two directory events", wait, func() bool { return ev.count(DirectoryChanged) == 2 })
	// A Windows drive and a network share, as the PowerShell snippet sends
	// them.
	for payload, want := range map[string]string{
		"file://pc/C:/Program%20Files/x":          `C:\Program Files\x`,
		"file://pc///wsl.localhost/ubuntu/home/a": `\\wsl.localhost\ubuntu\home\a`,
	} {
		s.Feed([]byte("\x1b]7;" + payload + "\x1b\\"))
		if runtime.GOOS != "windows" {
			want = strings.ReplaceAll(want, `\`, "/")
		}
		if si.Directory() != want {
			t.Errorf("%s gave %q, want %q", payload, si.Directory(), want)
		}
	}
}

func TestVisualStudioCodesOwnMarksAreUnderstoodToo(t *testing.T) {
	// Its snippets write OSC 633 with the same letters, and give the command
	// line explicitly rather than leaving it to be read off the screen.
	s := NewSession()
	si := NewShellIntegration(s)
	s.Feed([]byte("\x1b]633;A\x1b\\$ \x1b]633;B\x1b\\"))
	s.Feed([]byte("\x1b]633;C\x1b\\"))
	s.Feed([]byte("\x1b]633;E;git status --short\x1b\\"))
	s.Feed([]byte("\x1b]633;P;Cwd=/tmp/project\x1b\\"))
	s.Feed([]byte("\x1b]633;D;0\x1b\\"))
	if si.CommandCount() != 1 {
		t.Fatalf("%d commands", si.CommandCount())
	}
	if c, _ := si.Command(0); c.Text != "git status --short" {
		t.Errorf("text %q", c.Text)
	}
	if si.Directory() != "/tmp/project" {
		t.Errorf("directory %q", si.Directory())
	}
	// A semicolon in the command line arrives escaped.
	s.Feed([]byte("\x1b]633;A\x1b\\$ \x1b]633;B\x1b\\\x1b]633;C\x1b\\\x1b]633;E;a \\x3b b\x1b\\\x1b]633;D;0\x1b\\"))
	if c, _ := si.Command(1); c.Text != "a ; b" {
		t.Errorf("escaped text %q", c.Text)
	}
}

func TestACommandThatNeverReportsIsClosedByTheNextPrompt(t *testing.T) {
	s := NewSession()
	si := NewShellIntegration(s)
	s.Feed([]byte("\x1b]133;A\x1b\\$ \x1b]133;B\x1b\\sleep 100\r\n\x1b]133;C\x1b\\"))
	if !si.CommandRunning() {
		t.Fatal("not running")
	}
	s.Feed([]byte("\x1b]133;A\x1b\\$ "))
	c, _ := si.Command(0)
	if si.CommandRunning() || !c.Finished || c.ExitCode != -1 {
		t.Errorf("running %v, command %+v", si.CommandRunning(), c)
	}
}

func TestTheSnippetsShipInsideTheLibrary(t *testing.T) {
	for _, sh := range []Shell{Bash, Zsh, Fish, PowerShell} {
		script := ShellScript(sh)
		if !strings.Contains(script, "133;A") || !strings.Contains(script, "133;D") {
			t.Errorf("%s lacks the marks", ShellScriptName(sh))
		}
	}
}

func TestTheBashSnippetWorksInARealShell(t *testing.T) {
	// The one case that depends on a program the machine may not have, and
	// the only way to prove the snippet this library ships does what the
	// rest assumes. Bash starts with an init file of the test's own, so the
	// person running the tests keeps their configuration.
	if runtime.GOOS == "windows" {
		t.Skip("bash is not the shell here")
	}
	bash, err := exec.LookPath("bash")
	if err != nil {
		t.Skip("bash is not installed")
	}
	dir := t.TempDir()
	snippet := filepath.Join(dir, "kvitterm.bash")
	os.WriteFile(snippet, []byte(ShellScript(Bash)), 0o644)
	init := filepath.Join(dir, "init.bash")
	os.WriteFile(init, []byte("PS1='$ '\n. "+snippet+"\n"), 0o644)

	s := NewSession()
	s.Program = bash
	s.Args = []string{"--rcfile", init, "-i"}
	s.Dir = dir
	t.Cleanup(s.Close)
	si := NewShellIntegration(s)
	if err := s.Start(); err != nil {
		t.Fatal(err)
	}
	eventually(t, "the first mark", wait, si.Active)
	s.SendText("echo one\r")
	eventually(t, "the first command to finish", wait, func() bool { c, ok := si.Command(0); return ok && c.Finished })
	if c, _ := si.Command(0); c.Text != "echo one" || c.ExitCode != 0 {
		t.Errorf("command = %+v", c)
	}
	if !strings.Contains(si.Output(0), "one") {
		t.Errorf("output = %q", si.Output(0))
	}
	s.SendText("false\r")
	eventually(t, "the second command to finish", wait, func() bool { c, ok := si.Command(1); return ok && c.Finished })
	if c, _ := si.Command(1); c.ExitCode != 1 {
		t.Errorf("command = %+v", c)
	}
	if si.Output(1) != "" || si.Directory() == "" {
		t.Errorf("output %q, directory %q", si.Output(1), si.Directory())
	}
}

func TestTheMarksSurviveARealPseudoTerminal(t *testing.T) {
	s := stubSession(t, "scenario", "marks")
	si := NewShellIntegration(s)
	s.Start()
	eventually(t, "the command to finish", wait, func() bool { c, ok := si.Command(0); return ok && c.Finished })
	if c, _ := si.Command(0); c.Text != "ls -l" {
		t.Errorf("text %q", c.Text)
	}
	if si.Directory() != "/tmp" || !strings.Contains(si.Output(0), "file.txt") {
		t.Errorf("directory %q, output %q", si.Directory(), si.Output(0))
	}
}
