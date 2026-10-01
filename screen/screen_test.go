package screen

// The emulator, proved by feeding it recorded bytes and reading the screen
// back, with no window and no child process.

import (
	"bytes"
	"fmt"
	"os"
	"strings"
	"testing"
)

func rowText(s *Screen, row int) string { return s.Line(row).Text() }

func feed(s *Screen, text string) { s.Feed([]byte(text)) }

func TestPlainTextLandsOnTheScreen(t *testing.T) {
	s := New(20, 4)
	feed(s, "hello\r\nworld")
	if got := rowText(s, 0); got != "hello" {
		t.Errorf("row 0 = %q", got)
	}
	if got := rowText(s, 1); got != "world" {
		t.Errorf("row 1 = %q", got)
	}
	if c := s.Cursor(); c != (Point{5, 1}) {
		t.Errorf("cursor = %v", c)
	}
}

func TestACarriageReturnRedrawsTheLine(t *testing.T) {
	// What a progress bar does, and what a text view showing raw output gets
	// wrong: the later text replaces the earlier text.
	s := New(20, 2)
	feed(s, "working: 0%\rworking: 50%\rdone        ")
	if got := rowText(s, 0); got != "done" {
		t.Errorf("row 0 = %q", got)
	}
}

func TestColoursAndAttributesAreKept(t *testing.T) {
	s := New(40, 2)
	feed(s, "\x1b[31mR\x1b[0m\x1b[1;32mG\x1b[0m\x1b[38;5;208mI\x1b[0m"+
		"\x1b[38;2;10;20;30mT\x1b[0m\x1b[4mU\x1b[24m\x1b[7mV\x1b[27m")
	if c := s.Cell(0, 0).Style.Foreground; c != (Color{Kind: Indexed, Index: 1}) {
		t.Errorf("red = %+v", c)
	}
	g := s.Cell(0, 1).Style
	if !g.Bold || g.Foreground != (Color{Kind: Indexed, Index: 2}) {
		t.Errorf("bold green = %+v", g)
	}
	if c := s.Cell(0, 2).Style.Foreground; c != (Color{Kind: Indexed, Index: 208}) {
		t.Errorf("indexed = %+v", c)
	}
	if c := s.Cell(0, 3).Style.Foreground; c != (Color{Kind: RGB, R: 10, G: 20, B: 30}) {
		t.Errorf("true colour = %+v", c)
	}
	if u := s.Cell(0, 4).Style.Underline; u != SingleUnderline {
		t.Errorf("underline = %v", u)
	}
	if !s.Cell(0, 5).Style.Reverse {
		t.Error("reverse lost")
	}
	// The colour is kept as the program named it rather than resolved to
	// pixels, which is what lets a colour scheme change without the program
	// redrawing.
	if k := s.Cell(0, 6).Style.Foreground.Kind; k != Default {
		t.Errorf("after reset, kind = %v", k)
	}
}

func TestCursorMovementAndErasure(t *testing.T) {
	s := New(10, 3)
	feed(s, "abcdefghij")
	feed(s, "\x1b[1;1H")
	if c := s.Cursor(); c != (Point{0, 0}) {
		t.Errorf("home = %v", c)
	}
	feed(s, "\x1b[2;3H")
	if c := s.Cursor(); c != (Point{2, 1}) {
		t.Errorf("row 2 column 3 = %v", c)
	}
	feed(s, "\x1b[1;5H\x1b[K")
	if got := rowText(s, 0); got != "abcd" {
		t.Errorf("after erase to end of line: %q", got)
	}
	feed(s, "\x1b[2J")
	if !s.Line(0).IsBlank() {
		t.Error("the screen was not erased")
	}
}

func TestTheAlternateScreenLeavesTheScrollbackAlone(t *testing.T) {
	// A full-screen program runs on a second screen and gives the first one
	// back untouched when it exits.
	s := New(20, 3)
	var changes []bool
	s.OnAlternateScreen = func(on bool) { changes = append(changes, on) }
	feed(s, "before\r\n")
	if s.AlternateScreen() {
		t.Fatal("started on the alternate screen")
	}
	feed(s, "\x1b[?1049h")
	if !s.AlternateScreen() {
		t.Fatal("did not switch")
	}
	feed(s, "\x1b[2J\x1b[Hinside")
	if got := rowText(s, 0); got != "inside" {
		t.Errorf("alternate row 0 = %q", got)
	}
	during := s.ScrollbackCount()
	feed(s, "\r\n\r\n\r\nmore\r\n\r\n")
	if s.ScrollbackCount() != during {
		t.Errorf("the alternate screen stored %d lines", s.ScrollbackCount()-during)
	}
	feed(s, "\x1b[?1049l")
	if s.AlternateScreen() {
		t.Fatal("did not switch back")
	}
	if got := rowText(s, 0); got != "before" {
		t.Errorf("normal row 0 = %q", got)
	}
	if fmt.Sprint(changes) != "[true false]" {
		t.Errorf("reported changes %v", changes)
	}
}

func TestLinesScrollIntoTheScrollback(t *testing.T) {
	s := New(20, 3)
	for i := 1; i <= 10; i++ {
		feed(s, fmt.Sprintf("line %d\r\n", i))
	}
	// The tenth newline left the cursor on an empty eleventh line, so the
	// screen holds lines 9 and 10 and a blank, and everything before them
	// is stored.
	if n := s.ScrollbackCount(); n != 8 {
		t.Fatalf("scrollback holds %d lines", n)
	}
	for row, want := range map[int]string{-1: "line 8", -8: "line 1", 0: "line 9", 1: "line 10"} {
		if got := rowText(s, row); got != want {
			t.Errorf("row %d = %q, want %q", row, got, want)
		}
	}
}

func TestTheScrollbackLimitIsHonoured(t *testing.T) {
	s := New(20, 3)
	s.SetScrollbackLimit(5)
	for i := 1; i <= 40; i++ {
		feed(s, fmt.Sprintf("line %d\r\n", i))
	}
	if n := s.ScrollbackCount(); n != 5 {
		t.Fatalf("scrollback holds %d lines", n)
	}
	if got := rowText(s, -1); got != "line 38" {
		t.Errorf("row -1 = %q", got)
	}
	if got := rowText(s, -5); got != "line 34" {
		t.Errorf("row -5 = %q", got)
	}
	// Lowering the limit drops the oldest at once.
	s.SetScrollbackLimit(2)
	if n := s.ScrollbackCount(); n != 2 || rowText(s, -2) != "line 37" {
		t.Errorf("after lowering the limit: %d lines, oldest %q", n, rowText(s, -2))
	}
}

func TestDoubleWidthCharactersOccupyTwoCells(t *testing.T) {
	s := New(10, 2)
	feed(s, "[日本]")
	if got := s.Cell(0, 1); got.Text() != "日" || got.Width != 2 {
		t.Errorf("cell 1 = %+v", got)
	}
	if got := s.Cell(0, 2); got.Width != 0 || got.Text() != "" {
		t.Errorf("the right half = %+v", got)
	}
	if got := s.Cell(0, 5).Text(); got != "]" {
		t.Errorf("cell 5 = %q", got)
	}
	if got := rowText(s, 0); got != "[日本]" {
		t.Errorf("row = %q", got)
	}
}

func TestEmojiOccupyTwoCells(t *testing.T) {
	// The width table is current Unicode's, where an emoji drawn as one is
	// wide; the xterm-go copy's original table made it one column.
	s := New(10, 2)
	feed(s, "a😀b")
	if c := s.Cursor(); c.X != 4 {
		t.Errorf("cursor after a, an emoji and b is at column %d", c.X)
	}
	if got := s.Cell(0, 1); got.Width != 2 || got.Text() != "😀" {
		t.Errorf("emoji cell = %+v", got)
	}
}

func TestCombiningMarksStayInOneCell(t *testing.T) {
	s := New(10, 2)
	feed(s, "éx")
	if got := s.Cell(0, 0).Text(); got != "é" {
		t.Errorf("cell 0 = %q", got)
	}
	if got := s.Cell(0, 1).Text(); got != "x" {
		t.Errorf("cell 1 = %q", got)
	}
}

func TestTheTitleIsReported(t *testing.T) {
	s := New(20, 2)
	var titles []string
	s.OnTitle = func(title string) { titles = append(titles, title) }
	feed(s, "\x1b]0;a title\x1b\\")
	if s.Title() != "a title" || len(titles) != 1 {
		t.Errorf("title %q, reported %v", s.Title(), titles)
	}
}

func TestTheBellIsReported(t *testing.T) {
	s := New(20, 2)
	bells := 0
	s.OnBell = func() { bells++ }
	feed(s, "\a")
	if bells != 1 {
		t.Errorf("%d bells", bells)
	}
}

func TestWholeScreenReverseVideoIsTracked(t *testing.T) {
	// DECSCNM, mode 5: it is tracked and never drawn, so it does not change
	// what is shown.
	s := New(20, 2)
	if s.ReverseVideo() {
		t.Fatal("started reversed")
	}
	feed(s, "plain")
	feed(s, "\x1b[?5h")
	if !s.ReverseVideo() {
		t.Fatal("CSI ? 5 h did not set reverse video")
	}
	if got := rowText(s, 0); got != "plain" {
		t.Errorf("setting it changed row 0 to %q", got)
	}
	out := written(s)
	feed(s, "\x1b[?5$p")
	if got := out.String(); got != "\x1b[?5;1$y" {
		t.Errorf("DECRQM after set = %q", got)
	}
	out.Reset()
	feed(s, "\x1b[?5l")
	if s.ReverseVideo() {
		t.Fatal("CSI ? 5 l did not reset reverse video")
	}
	feed(s, "\x1b[?5$p")
	if got := out.String(); got != "\x1b[?5;2$y" {
		t.Errorf("DECRQM after reset = %q", got)
	}
}

func TestUnhandledOperatingSystemCommandsReachTheCaller(t *testing.T) {
	// The seam the shell-integration layer is built on: the emulator acts
	// on the commands it knows and hands the rest here.
	s := New(20, 2)
	var got []string
	s.OnOSC = func(cmd int, payload string) { got = append(got, fmt.Sprintf("%d:%s", cmd, payload)) }
	feed(s, "\x1b]133;A\x1b\\\x1b]7;file://host/tmp\x1b\\\x1b]633;E;ls -l\a")
	want := []string{"133:A", "7:file://host/tmp", "633:E;ls -l"}
	if fmt.Sprint(got) != fmt.Sprint(want) {
		t.Errorf("got %q, want %q", got, want)
	}
}

// written collects what the screen asks to have sent to the child.
func written(s *Screen) *bytes.Buffer {
	var b bytes.Buffer
	s.OnWrite = func(p []byte) { b.Write(p) }
	return &b
}

func TestKeysBecomeTheBytesATerminalSends(t *testing.T) {
	s := New(20, 4)
	out := written(s)
	check := func(what string, want string, act func()) {
		t.Helper()
		out.Reset()
		act()
		if out.String() != want {
			t.Errorf("%s sent %q, want %q", what, out.String(), want)
		}
	}
	check("a", "a", func() { s.TypeChar('a', 0) })
	check("Ctrl+C", "\x03", func() { s.TypeChar('c', Ctrl) })
	check("Return", "\r", func() { s.PressKey(KeyEnter, 0) })
	check("Tab", "\t", func() { s.PressKey(KeyTab, 0) })
	check("Shift+Tab", "\x1b[Z", func() { s.PressKey(KeyTab, Shift) })
	check("Up", "\x1b[A", func() { s.PressKey(KeyUp, 0) })
	// The same key sends something different once a program has asked for
	// application cursor keys, which is why an editor's arrow keys keep
	// working when the shell's history does too.
	feed(s, "\x1b[?1h")
	check("Up in application mode", "\x1bOA", func() { s.PressKey(KeyUp, 0) })
	feed(s, "\x1b[?1l")
	check("F1", "\x1bOP", func() { s.PressKey(KeyF1, 0) })
	check("F5", "\x1b[15~", func() { s.PressKey(KeyF5, 0) })
	check("Backspace", "\x7f", func() { s.PressKey(KeyBackspace, 0) })
	check("Ctrl+Right", "\x1b[1;5C", func() { s.PressKey(KeyRight, Ctrl) })
	check("Alt+x", "\x1bx", func() { s.TypeChar('x', Alt) })
	check("Ctrl+I, which is not Tab", "\x1b[105;5u", func() { s.TypeChar('i', Ctrl) })
	check("Shift+Enter", "\x1b[13;2u", func() { s.PressKey(KeyEnter, Shift) })
	check("Delete", "\x1b[3~", func() { s.PressKey(KeyDelete, 0) })
}

func TestPasteIsMarkedOnlyWhenTheProgramAsksForIt(t *testing.T) {
	s := New(20, 4)
	out := written(s)
	s.Paste("one\ntwo")
	if out.String() != "one\rtwo" {
		t.Errorf("plain paste sent %q", out.String())
	}
	out.Reset()
	feed(s, "\x1b[?2004h")
	s.Paste("one")
	if out.String() != "\x1b[200~one\x1b[201~" {
		t.Errorf("bracketed paste sent %q", out.String())
	}
}

func TestTheMouseIsReportedOnlyWhenTheProgramAsksForIt(t *testing.T) {
	s := New(20, 4)
	out := written(s)
	if s.MouseTracking() != NoMouse {
		t.Fatal("tracking the mouse from the start")
	}
	s.MouseButton(LeftButton, true, 0)
	s.MouseButton(LeftButton, false, 0)
	if out.Len() != 0 {
		t.Fatalf("reported %q without being asked", out.String())
	}
	feed(s, "\x1b[?1000h")
	if s.MouseTracking() != MouseClick {
		t.Fatal("tracking not reported")
	}
	s.MouseMove(2, 3, 0)
	s.MouseButton(LeftButton, true, 0)
	// The X10 form: the button, then the column and row biased by 33 so
	// that they are printable.
	if out.String() != "\x1b[M\x20\x24\x23" {
		t.Errorf("press sent %q", out.String())
	}
	out.Reset()
	feed(s, "\x1b[?1006h") // the SGR form
	s.MouseButton(LeftButton, false, 0)
	if out.String() != "\x1b[<0;4;3m" {
		t.Errorf("release sent %q", out.String())
	}
	out.Reset()
	s.MouseButton(WheelUp, true, 0)
	if out.String() != "\x1b[<64;4;3M" {
		t.Errorf("wheel sent %q", out.String())
	}
}

func TestFocusIsReportedOnlyWhenAskedFor(t *testing.T) {
	s := New(20, 4)
	out := written(s)
	s.SetFocused(true)
	if out.Len() != 0 {
		t.Fatalf("reported focus unasked: %q", out.String())
	}
	feed(s, "\x1b[?1004h")
	out.Reset()
	s.SetFocused(false)
	if out.String() != "\x1b[O" {
		t.Errorf("focus out sent %q", out.String())
	}
}

func TestTheTerminalAnswersQuestionsAboutItself(t *testing.T) {
	// A program asks where the cursor is; the terminal replies. Without
	// this, tools that measure the window before drawing hang.
	s := New(20, 4)
	out := written(s)
	feed(s, "\x1b[3;7H\x1b[6n")
	if out.String() != "\x1b[3;7R" {
		t.Errorf("replied %q", out.String())
	}
}

func TestAWrappedLineIsMarkedAsOne(t *testing.T) {
	s := New(10, 4)
	feed(s, "0123456789ABCDE\r\n")
	if s.Line(0).Continuation || !s.Line(1).Continuation {
		t.Fatal("the visible halves are not marked")
	}
	for i := 0; i < 12; i++ {
		feed(s, "x\r\n")
	}
	first := 0
	for row := -s.ScrollbackCount(); row < 0; row++ {
		if rowText(s, row) == "0123456789" {
			first = row
		}
	}
	if first == 0 {
		t.Fatal("the wrapped line was not found in the scrollback")
	}
	if s.Line(first).Continuation {
		t.Error("the first half claims to continue something")
	}
	if l := s.Line(first + 1); l.Text() != "ABCDE" || !l.Continuation {
		t.Errorf("the second half = %q, continuation %v", l.Text(), l.Continuation)
	}
}

func TestResizingRewrapsTheVisibleScreen(t *testing.T) {
	s := New(10, 4)
	feed(s, "0123456789ABCDEFGHIJ\r\n")
	if rowText(s, 0) != "0123456789" || rowText(s, 1) != "ABCDEFGHIJ" {
		t.Fatalf("before: %q %q", rowText(s, 0), rowText(s, 1))
	}
	s.SetSize(20, 4)
	if got := rowText(s, 0); got != "0123456789ABCDEFGHIJ" {
		t.Errorf("after widening: %q", got)
	}
}

func TestTheLineWithTheCursorIsLeftToTheProgram(t *testing.T) {
	// xterm.js, and so xterm-go, does not re-wrap the line the cursor is on:
	// a shell redraws the line being edited when it is told the new width,
	// and re-wrapping it as well can leave the prompt drawn twice.
	// libvterm, which the terminal used, re-wraps it too.
	s := New(10, 4)
	feed(s, "0123456789ABCDEFGHIJ")
	s.SetSize(20, 4)
	if rowText(s, 0) != "0123456789" || rowText(s, 1) != "ABCDEFGHIJ" {
		t.Errorf("after widening: %q %q", rowText(s, 0), rowText(s, 1))
	}
}

func TestResizingRewrapsTheScrollbackToo(t *testing.T) {
	// libvterm re-wraps only the visible screen; xterm-go re-wraps the
	// scrollback as well.
	s := New(10, 4)
	feed(s, "0123456789ABCDEFGHIJ\r\n")
	for i := 0; i < 12; i++ {
		feed(s, "x\r\n")
	}
	s.SetSize(20, 4)
	joined := 0
	for row := -s.ScrollbackCount(); row < 0; row++ {
		if rowText(s, row) == "0123456789ABCDEFGHIJ" {
			joined++
		}
	}
	if joined != 1 {
		t.Errorf("found the joined line %d times", joined)
	}
	s.SetSize(5, 4)
	var pieces []string
	for row := -s.ScrollbackCount(); row < 0; row++ {
		text := rowText(s, row)
		if text != "" && strings.ContainsRune("05AF", rune(text[0])) {
			pieces = append(pieces, text)
		}
	}
	if fmt.Sprint(pieces) != "[01234 56789 ABCDE FGHIJ]" {
		t.Errorf("after narrowing: %q", pieces)
	}
}

func TestNarrowingToOneColumnKeepsTheSpaces(t *testing.T) {
	// A row that a longer line wrapped through is full to its last cell, so
	// blanks at the end of it are spaces the program wrote. An application
	// that puts the terminal in a layout can resize it to its smallest
	// before the window settles, and at that width every space in the
	// output is wrapped onto a row of its own. (The emulator's smallest
	// width is two columns.)
	written := "alpha beta gamma delta epsilon zeta eta theta"
	s := New(44, 10)
	for i := 0; i < 20; i++ {
		feed(s, written+"\r\n")
	}
	s.SetSize(1, 10)
	s.SetSize(80, 10)
	if s.ScrollbackCount() == 0 {
		t.Fatal("nothing was stored, so the round trip did not happen")
	}
	if got := rowText(s, -s.ScrollbackCount()); got != written {
		t.Errorf("oldest line = %q", got)
	}
}

func TestTextIsReadBackAcrossLinesAndInBlocks(t *testing.T) {
	s := New(20, 4)
	feed(s, "alpha beta\r\ngamma delta\r\n")
	if got := s.TextInRange(Point{6, 0}, Point{4, 1}, false); got != "beta\ngamma" {
		t.Errorf("linear = %q", got)
	}
	if got := s.TextInRange(Point{0, 0}, Point{4, 1}, true); got != "alpha\ngamma" {
		t.Errorf("block = %q", got)
	}
	if got := s.TextInRange(Point{4, 1}, Point{6, 0}, false); got != "beta\ngamma" {
		t.Errorf("backwards = %q", got)
	}
}

func TestWhatIsOnTheScreenComesOutAsStyledText(t *testing.T) {
	s := New(20, 3)
	feed(s, "plain \x1b[31mred\x1b[0m \x1b[1mbold\x1b[0m\r\n<escaped>")
	if got := PlainText(s, 0, 1); got != "plain red bold\n<escaped>" {
		t.Errorf("plain = %q", got)
	}
	h := HTML(s, 0, 1, DefaultPalette())
	for _, want := range []string{"&lt;escaped&gt;", "font-weight:bold", Hex(DefaultPalette().ANSI[1])} {
		if !strings.Contains(h, want) {
			t.Errorf("HTML lacks %q: %s", want, h)
		}
	}
	if !strings.HasPrefix(h, "<pre") {
		t.Errorf("HTML = %s", h)
	}
}

func TestAWrappedLineIsCopiedAsOneLine(t *testing.T) {
	s := New(10, 4)
	feed(s, "0123456789ABCDE\r\n")
	if got := s.Text(0, 1); got != "0123456789ABCDE" {
		t.Errorf("got %q", got)
	}
}

func TestAWrappedLineKeepsTheSpacesItBrokeAt(t *testing.T) {
	s := New(10, 4)
	feed(s, "hello     world")
	if got := s.Line(0).Text(); got != "hello" {
		t.Errorf("the row on its own = %q", got)
	}
	if !s.Line(1).Continuation {
		t.Fatal("the second row is not a continuation")
	}
	if got := s.Text(0, 1); got != "hello     world" {
		t.Errorf("joined = %q", got)
	}
	if got := s.TextInRange(Point{0, 0}, Point{4, 1}, false); got != "hello     world" {
		t.Errorf("selected = %q", got)
	}
	if h := HTML(s, 0, 1, DefaultPalette()); !strings.Contains(h, ">hello     <") {
		t.Errorf("HTML lost the spaces: %s", h)
	}
}

func TestClearingTheScrollbackNumbersRowsFromZero(t *testing.T) {
	s := New(20, 3)
	cleared := 0
	s.OnScrollbackCleared = func() { cleared++ }
	for i := 0; i < 10; i++ {
		feed(s, "x\r\n")
	}
	if s.ScrolledAway() != 8 {
		t.Fatalf("scrolled away %d", s.ScrolledAway())
	}
	feed(s, "\x1b[3J") // what `clear` sends
	if s.ScrollbackCount() != 0 || s.ScrolledAway() != 0 || cleared != 1 {
		t.Errorf("after CSI 3 J: %d kept, %d away, %d reports", s.ScrollbackCount(), s.ScrolledAway(), cleared)
	}
	for i := 0; i < 4; i++ {
		feed(s, "y\r\n")
	}
	if s.ScrolledAway() != 4 {
		t.Errorf("then scrolled away %d", s.ScrolledAway())
	}
}

func TestScrolledAwayKeepsCountingPastTheLimit(t *testing.T) {
	s := New(20, 3)
	s.SetScrollbackLimit(5)
	for i := 0; i < 40; i++ {
		feed(s, "x\r\n")
	}
	if s.ScrollbackCount() != 5 || s.ScrolledAway() != 38 {
		t.Errorf("%d kept, %d scrolled away", s.ScrollbackCount(), s.ScrolledAway())
	}
}

func TestTheCursorsShapeAndVisibilityFollowTheProgram(t *testing.T) {
	s := New(20, 4)
	if s.CursorShape() != BlockCursor || !s.CursorVisible() || !s.CursorBlinks() {
		t.Fatal("the cursor does not start as a visible blinking block")
	}
	feed(s, "\x1b[6 q") // a steady bar
	if s.CursorShape() != BarCursor || s.CursorBlinks() {
		t.Errorf("shape %v, blinks %v", s.CursorShape(), s.CursorBlinks())
	}
	feed(s, "\x1b[3 q") // a blinking underline
	if s.CursorShape() != UnderlineCursor || !s.CursorBlinks() {
		t.Errorf("shape %v, blinks %v", s.CursorShape(), s.CursorBlinks())
	}
	feed(s, "\x1b[?25l")
	if s.CursorVisible() {
		t.Error("hidden cursor still visible")
	}
}

func TestNarrowingAFullScrollbackDoesNotLoseTheScreen(t *testing.T) {
	// A layout can give a terminal a few columns for a moment while a
	// window settles. With a scrollback full of real output, narrowing adds
	// more lines than it holds, and xterm-go as found wrote the surplus
	// above its first line and panicked; xterm.js, which it ports, ignores
	// those writes.
	data, err := os.ReadFile("testdata/vtdiff/streams/gitlog.bin")
	if err != nil {
		t.Fatal(err)
	}
	s := New(80, 24)
	s.SetScrollbackLimit(500)
	for i := 0; i < 4; i++ {
		s.Feed(data)
	}
	last := s.Text(0, s.Rows()-1)
	s.SetSize(2, 5)
	s.SetSize(80, 24)
	if s.ScrollbackCount() > 500 {
		t.Errorf("the scrollback holds %d lines", s.ScrollbackCount())
	}
	if got := s.Text(0, s.Rows()-1); !strings.Contains(last, strings.TrimSpace(got[strings.LastIndex(strings.TrimSpace(got), "\n")+1:])) {
		t.Errorf("after narrowing and widening, the screen ends with %q", got)
	}
}
