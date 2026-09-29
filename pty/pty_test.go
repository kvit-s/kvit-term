package pty

// The pseudo-terminal, proved against termstub rather than a shell: a
// shell's version, startup files and prompt all vary between machines, and
// none of them is what these cases are about. These are the library's
// tests (tests/unit/test_pty.cpp), case for case.

import (
	"bytes"
	"errors"
	"io"
	"regexp"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/kvit-s/kvit-term/internal/stub"
)

// session collects everything the child writes, so a case can wait for a
// substring rather than a number of reads: how a stream is cut into reads is
// the kernel's business and varies from run to run.
type session struct {
	*Pty
	mu     sync.Mutex
	output bytes.Buffer
	eof    chan struct{}
}

func start(t *testing.T, p Params) *session {
	t.Helper()
	pt, err := Start(p)
	if err != nil {
		t.Fatal(err)
	}
	s := &session{Pty: pt, eof: make(chan struct{})}
	go func() {
		defer close(s.eof)
		buf := make([]byte, 4096)
		for {
			n, err := pt.Read(buf)
			s.mu.Lock()
			s.output.Write(buf[:n])
			s.mu.Unlock()
			if err != nil {
				return
			}
		}
	}()
	t.Cleanup(func() {
		pt.Hangup()
		pt.Kill()
		pt.Release()
	})
	return s
}

func stubParams(t *testing.T, args ...string) Params {
	return Params{Program: stub.Path(t), Args: args, Columns: 80, Rows: 24}
}

// escapes matches the sequences a terminal adds to what the child wrote.
// Windows' console host in particular announces itself — it hides the
// cursor, clears the screen, sets the title and shows the cursor again —
// and can emit those between the child's characters.
var escapes = regexp.MustCompile(`\x1b\[[0-9;?]*[ -/]*[@-~]|\x1b\][^\x07\x1b]*(\x07|\x1b\\)|\x1b[^\[\]]`)

// text is what the child wrote, with the terminal's own escape sequences
// taken out.
func (s *session) text() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return escapes.ReplaceAllString(s.output.String(), "")
}

func (s *session) raw() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.output.String()
}

func (s *session) waitFor(t *testing.T, want string) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		if strings.Contains(s.text(), want) {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("waited for %q; the child wrote %q", want, s.text())
}

// finish waits for the child to exit and for its output to be read to the
// end.
func (s *session) finish(t *testing.T, timeout time.Duration) int {
	t.Helper()
	select {
	case <-s.Done():
	case <-time.After(timeout):
		t.Fatalf("the child did not exit; it wrote %q", s.text())
	}
	select {
	case <-s.eof:
	case <-time.After(2 * time.Second):
	}
	return s.ExitCode()
}

func TestTheChildSeesATerminal(t *testing.T) {
	// The whole point of the layer: a child on a pipe reports 0 here and
	// turns its colours off accordingly.
	s := start(t, stubParams(t, "isatty"))
	if code := s.finish(t, 10*time.Second); code != 0 {
		t.Errorf("exit code %d", code)
	}
	if !strings.Contains(s.text(), "stdin 1 stdout 1 stderr 1") {
		t.Errorf("the child did not see a terminal: %q", s.text())
	}
}

func TestTheChildIsToldTheTerminalSize(t *testing.T) {
	p := stubParams(t, "size")
	p.Columns, p.Rows = 100, 30
	s := start(t, p)
	s.finish(t, 10*time.Second)
	if !strings.Contains(s.text(), "size 100x30") {
		t.Errorf("the child said %q", s.text())
	}
}

func TestAResizeReachesTheChild(t *testing.T) {
	// The size is pushed to the terminal rather than through the
	// environment, so a program already running learns about it.
	s := start(t, stubParams(t, "size-watch"))
	s.waitFor(t, "size 80x24")
	if err := s.Resize(120, 40); err != nil {
		t.Fatal(err)
	}
	s.waitFor(t, "size 120x40")
	s.finish(t, 10*time.Second)
}

func TestWhatIsWrittenReachesTheChild(t *testing.T) {
	s := start(t, stubParams(t, "echo"))
	// A terminal sends a carriage return for Enter, and the terminal's line
	// discipline turns it into a newline for the program.
	io.WriteString(s, "hello\r")
	s.waitFor(t, "echo:hello")
	// The line discipline echoed the input as well, which is what makes
	// typing visible in a real terminal.
	if !strings.Contains(s.text(), "hello\r\n") {
		t.Errorf("no echo of the typing in %q", s.raw())
	}
	io.WriteString(s, "quit\r")
	s.finish(t, 10*time.Second)
}

func TestTheExitCodeIsReported(t *testing.T) {
	s := start(t, stubParams(t, "exit", "7"))
	if code := s.finish(t, 10*time.Second); code != 7 {
		t.Errorf("exit code %d", code)
	}
}

func TestOutputWrittenJustBeforeExitIsNotLost(t *testing.T) {
	// A short-lived program is the ordinary case — a build command that
	// fails in a tenth of a second — and losing its last line because the
	// process died before it was read would be the obvious bug.
	s := start(t, stubParams(t, "scenario", "colour"))
	s.finish(t, 10*time.Second)
	if !strings.Contains(s.text(), "truecolour") {
		t.Errorf("the last output was lost: %q", s.text())
	}
}

func TestAProgramThatDoesNotExistFailsToStart(t *testing.T) {
	_, err := Start(Params{Program: "/nonexistent/kvitterm-no-such-program"})
	if err == nil {
		t.Fatal("started a program that does not exist")
	}
	if _, err := Start(Params{}); !errors.Is(err, ErrNoProgram) {
		t.Errorf("no program: %v", err)
	}
}

func TestTheWorkingDirectoryIsHonoured(t *testing.T) {
	p := stubParams(t, "isatty")
	p.Dir = t.TempDir()
	s := start(t, p)
	if code := s.finish(t, 10*time.Second); code != 0 {
		t.Errorf("exit code %d", code)
	}
}

func TestHangingUpEndsTheChild(t *testing.T) {
	// Closing this end hangs the child's session up, which is what a program
	// sees when a window is closed on it. Windows asks the child to end and
	// ends it three seconds later if it has not.
	s := start(t, stubParams(t, "sleep"))
	time.Sleep(100 * time.Millisecond)
	s.Hangup()
	s.finish(t, 15*time.Second)
}

func TestNotReadingHoldsTheChildBack(t *testing.T) {
	// What stops a runaway process outrunning whatever draws it: while
	// nothing reads, the child blocks in its own write, and nothing is
	// lost.
	pt, err := Start(stubParams(t, "scenario", "flood"))
	if err != nil {
		t.Fatal(err)
	}
	defer pt.Release()
	select {
	case <-pt.Done():
		t.Fatal("the child finished 1.6 MB of output with nobody reading it")
	case <-time.After(300 * time.Millisecond):
	}
	var all bytes.Buffer
	buf := make([]byte, 64<<10)
	for !strings.Contains(all.String(), "flood done") {
		n, err := pt.Read(buf)
		all.Write(buf[:n])
		if err != nil {
			break
		}
	}
	if got := strings.Count(all.String(), "xxxxxxxxxx\r\n") + strings.Count(all.String(), "xxxxxxxxxx\n"); got < 20000 && runtime.GOOS != "windows" {
		t.Errorf("read %d of 20000 lines", got)
	}
	if !strings.Contains(all.String(), "flood done") {
		t.Error("the end of the output was lost")
	}
}

func TestTheEnvironmentNamesTheTerminal(t *testing.T) {
	env := Params{Env: []string{"PATH=/bin", "TERM=dumb"}, Columns: 90, Rows: 20}.environment()
	joined := strings.Join(env, "\n")
	for _, want := range []string{"TERM=dumb", "COLORTERM=truecolor", "COLUMNS=90", "LINES=20"} {
		if !strings.Contains(joined, want) {
			t.Errorf("environment lacks %s: %v", want, env)
		}
	}
	if strings.Contains(joined, "TERM=xterm-256color") {
		t.Error("TERM set by the caller was replaced")
	}
}
