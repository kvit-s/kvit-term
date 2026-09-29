package kvitterm

// The whole chain: a child on a pseudo-terminal, its output interpreted
// onto a screen, and the answers going back. These are the library's
// session tests (tests/unit/test_session.cpp), case for case.

import (
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/kvit-s/kvit-term/screen"
)

const wait = 10 * time.Second

func TestAProgramsColoursSurviveTheWholeChain(t *testing.T) {
	s := stubSession(t, "scenario", "colour")
	if err := s.Start(); err != nil {
		t.Fatal(err)
	}
	eventually(t, "the colour scenario", wait, func() bool { return strings.Contains(s.ScreenText(), "truecolour") })
	col := strings.Index(s.LineText(0), "red")
	if col < 0 {
		t.Fatalf("no red in %q", s.LineText(0))
	}
	s.View(func(scr *screen.Screen) {
		if c := scr.Cell(0, col).Style.Foreground; c != (screen.Color{Kind: screen.Indexed, Index: 1}) {
			t.Errorf("red arrived as %+v", c)
		}
	})
}

func TestAProgressBarLeavesOneLineRatherThanFive(t *testing.T) {
	// Through a pipe this arrives as five lines with escape sequences in
	// them; through a terminal it is one line, redrawn.
	s := stubSession(t, "scenario", "progress")
	s.Start()
	eventually(t, "done", wait, func() bool { return strings.Contains(s.ScreenText(), "done") })
	if strings.Contains(s.ScreenText(), "working: 25%") {
		t.Errorf("the progress line was not redrawn: %q", s.ScreenText())
	}
	if got := strings.TrimSpace(s.LineText(0)); got != "done" {
		t.Errorf("row 0 = %q", got)
	}
}

func TestWhatIsTypedReachesTheProgramAndComesBack(t *testing.T) {
	s := stubSession(t, "echo")
	s.Start()
	s.SendText("hello\r")
	eventually(t, "the echo", wait, func() bool { return strings.Contains(s.ScreenText(), "echo:hello") })
}

func TestTheSizeTheViewChoosesIsWhatTheProgramSees(t *testing.T) {
	s := stubSession(t, "size")
	s.Resize(100, 30)
	s.Start()
	eventually(t, "the size", wait, func() bool { return strings.Contains(s.ScreenText(), "size 100x30") })
}

func TestTheExitStatusIsReported(t *testing.T) {
	s := stubSession(t, "exit", "3")
	ev := record(s)
	s.Start()
	eventually(t, "the exit", wait, func() bool { return ev.count(Exited) == 1 })
	if e, _ := ev.last(Exited); e.ExitCode != 3 {
		t.Errorf("exit code %d", e.ExitCode)
	}
	if s.Running() {
		t.Error("still running after the exit")
	}
	if ev.count(Started) != 1 {
		t.Error("the start was not reported")
	}
}

func TestAProgramThatCannotStartFails(t *testing.T) {
	s := NewSession()
	s.Program = "/nonexistent/kvitterm-no-such-program"
	ev := record(s)
	if err := s.Start(); err == nil {
		t.Fatal("started a program that does not exist")
	}
	eventually(t, "the failure", wait, func() bool { return ev.count(Failed) == 1 })
	if e, _ := ev.last(Failed); e.Message == "" {
		t.Error("the failure has no message")
	}
}

func TestAnExitedSessionStartsAgainOnTheSameScreen(t *testing.T) {
	// kvit-works restarts a track's shell in place, so the view keeps
	// pointing at the same session and what came before is still there.
	s := stubSession(t, "scenario", "title")
	ev := record(s)
	s.Start()
	eventually(t, "the first exit", wait, func() bool { return ev.count(Exited) == 1 })
	s.Args = []string{"scenario", "links"}
	if err := s.Start(); err != nil {
		t.Fatal(err)
	}
	eventually(t, "the second exit", wait, func() bool { return ev.count(Exited) == 2 })
	if !strings.Contains(s.ScreenText(), "example.invalid") {
		t.Errorf("screen = %q", s.ScreenText())
	}
}

func TestATitleSetByTheProgramIsReported(t *testing.T) {
	s := stubSession(t, "scenario", "title")
	ev := record(s)
	s.Start()
	eventually(t, "the title", wait, func() bool { return s.Title() == "a title" })
	eventually(t, "the title event", wait, func() bool { return ev.count(TitleChanged) == 1 })
}

func TestTheSessionCanBeReadAsStyledHTML(t *testing.T) {
	// The path an application takes to show a build log without having a
	// terminal in its interface at all.
	s := stubSession(t, "scenario", "colour")
	s.Start()
	eventually(t, "the colour scenario", wait, func() bool { return strings.Contains(s.ScreenText(), "truecolour") })
	h := s.HTML(screen.DefaultPalette())
	if !strings.Contains(h, "<pre") || !strings.Contains(h, "#78b4f0") {
		t.Errorf("HTML = %s", h)
	}
}

func TestActivityFollowsTheScreenAndEndsAfterTheQuietPeriod(t *testing.T) {
	s := stubSession(t, "echo")
	s.SetActivityPeriod(200 * time.Millisecond)
	ev := record(s)
	s.Start()
	time.Sleep(300 * time.Millisecond)
	if s.Activity() {
		// The console host on Windows draws on start; on Unix nothing
		// happens until the program writes.
		eventually(t, "the start's activity to end", wait, func() bool { return !s.Activity() })
	}
	before := ev.count(ActivityStarted)
	s.SendText("hello\r")
	eventually(t, "activity", wait, s.Activity)
	eventually(t, "the start of activity", wait, func() bool { return ev.count(ActivityStarted) == before+1 })
	eventually(t, "the end of activity", wait, func() bool { return !s.Activity() })
	eventually(t, "the end event", wait, func() bool { return ev.count(ActivityEnded) == before+1 })
	if !strings.Contains(s.ScreenText(), "echo:hello") {
		t.Errorf("screen = %q", s.ScreenText())
	}
	// A second command is a second activity, not a continuation.
	s.SendText("again\r")
	eventually(t, "a second activity", wait, func() bool { return ev.count(ActivityStarted) == before+2 })
}

func TestATerminalNobodyIsUsingReportsNoActivity(t *testing.T) {
	s := stubSession(t, "sleep")
	s.SetActivityPeriod(200 * time.Millisecond)
	ev := record(s)
	s.Start()
	// Windows' console host draws when it starts — it clears the screen
	// and sets the title — which is a change like any other; on Unix nothing
	// happens until the program writes. After that, nothing.
	time.Sleep(300 * time.Millisecond)
	eventually(t, "the start's activity to end", wait, func() bool { return !s.Activity() })
	starts := ev.count(ActivityStarted)
	time.Sleep(400 * time.Millisecond)
	if s.Activity() || ev.count(ActivityStarted) != starts {
		t.Errorf("activity %v, %d starts after the first %d", s.Activity(), ev.count(ActivityStarted)-starts, starts)
	}
	if runtime.GOOS != "windows" && starts != 0 {
		t.Errorf("%d activities before anything was written", starts)
	}
}

func TestClosingTheSessionEndsTheChild(t *testing.T) {
	s := stubSession(t, "sleep")
	ev := record(s)
	s.Start()
	eventually(t, "running", wait, s.Running)
	s.Close()
	eventually(t, "the exit", 15*time.Second, func() bool { return ev.count(Exited) == 1 })
}

func TestOutputIsHeldBackUntilItIsDrawn(t *testing.T) {
	// A view holds reading back while it has output it has not drawn, so a
	// runaway program is slowed to what can be shown rather than queueing
	// unbounded work; one that stops drawing holds it back only briefly.
	s := stubSession(t, "scenario", "flood")
	s.HoldForDraw(true)
	ev := record(s)
	s.Start()
	time.Sleep(200 * time.Millisecond)
	if ev.count(Exited) != 0 {
		t.Fatal("1.6 MB arrived without a single draw")
	}
	stop := make(chan struct{})
	go func() {
		for {
			select {
			case <-stop:
				return
			case <-time.After(5 * time.Millisecond):
				s.Drawn()
			}
		}
	}()
	defer close(stop)
	eventually(t, "the end of the flood", 30*time.Second, func() bool { return ev.count(Exited) == 1 })
	if !strings.Contains(s.ScreenText(), "flood done") {
		t.Errorf("the end of the output was lost: %q", s.ScreenText())
	}
}

func TestClearWipesTheScreenAndTheScrollback(t *testing.T) {
	s := NewSession()
	for i := 0; i < 40; i++ {
		s.Feed([]byte("line\r\n"))
	}
	if s.ScrollbackCount() == 0 {
		t.Fatal("nothing scrolled")
	}
	s.Clear()
	if s.ScrollbackCount() != 0 || strings.TrimSpace(s.ScreenText()) != "" {
		t.Errorf("after clear: %d lines kept, screen %q", s.ScrollbackCount(), s.ScreenText())
	}
}
