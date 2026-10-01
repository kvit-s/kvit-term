package view

// The view driven as an application drives it, on unison's headless
// screen: what it draws, and the bytes each input path sends, which
// termstub's raw mode shows byte for byte.

import (
	"fmt"
	"image"
	"strings"
	"testing"
	"time"

	"github.com/kvit-s/kvit-term"
	"github.com/kvit-s/kvit-term/internal/stub"
	"github.com/kvit-s/kvit-term/screen"
	kvitui "github.com/kvit-s/kvit-ui"
	"github.com/kvit-s/kvit-ui/text"
	"github.com/kvit-s/kvit-ui/uitest"
	"github.com/richardwilkes/toolbox/v2/geom"
	"github.com/richardwilkes/unison"
	"github.com/richardwilkes/unison/enums/mod"
	"github.com/richardwilkes/unison/enums/role"
)

type fixture struct {
	t  *testing.T
	ui *uitest.Session
	v  *View
	s  *kvitterm.Session
}

// open shows a view of a session running termstub with args, not yet
// started; nil args shows a session with no child.
func open(t *testing.T, args ...string) *fixture {
	t.Helper()
	f := &fixture{t: t, s: kvitterm.NewSession()}
	// As an application built on unison does, so events arrive on the
	// interface's thread in the order they happened.
	f.s.Dispatch = unison.InvokeTask
	if args != nil {
		f.s.Program = stub.Path(t)
		f.s.Args = args
	}
	t.Cleanup(f.s.Close)
	f.ui = uitest.Open(t, uitest.Options{Width: 900, Height: 600}, func(ui *kvitui.UI) unison.Paneler {
		f.v = New(ui.Fonts)
		f.v.Accessibility.Name = "terminal"
		f.v.SetSession(f.s)
		return f.v
	})
	return f
}

func (f *fixture) start() {
	f.t.Helper()
	if err := f.s.Start(); err != nil {
		f.t.Fatal(err)
	}
}

// waitFor runs the interface until cond holds.
func (f *fixture) waitFor(what string, cond func() bool) {
	f.t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for {
		ok := false
		f.ui.Do(func() { ok = cond() })
		if ok {
			return
		}
		if time.Now().After(deadline) {
			f.t.Fatalf("waited for %s; the screen shows %q", what, f.s.ScreenText())
		}
		time.Sleep(10 * time.Millisecond)
		f.ui.Sync()
	}
}

func (f *fixture) screenHas(s string) func() bool {
	return func() bool { return strings.Contains(f.s.ScreenText(), s) }
}

func (f *fixture) focus() {
	f.ui.Do(func() { f.v.RequestFocus() })
	f.ui.Sync()
}

// point is the middle of a cell, in the window.
func (f *fixture) point(column, row int) geom.Point {
	var p geom.Point
	f.ui.Do(func() {
		r := f.v.CellRect(column, row)
		p = f.v.PointToRoot(r.Center())
	})
	return p
}

func TestTheGridFollowsTheViewSize(t *testing.T) {
	// The child is told a size derived from the view's geometry and the
	// font, which is what makes a full-screen program fit.
	f := open(t, "size")
	var cols, rows int
	var r geom.Rect
	var cw, ch int
	f.ui.Do(func() {
		cols, rows = f.v.Grid()
		r = f.v.ContentRect(false)
		cw, ch = f.v.CellSize()
	})
	if cols != int(r.Width)/cw || rows != int(r.Height)/ch || cols < 10 || rows < 3 {
		t.Errorf("the view is %d × %d cells in %v with %d × %d cells", cols, rows, r, cw, ch)
	}
	if c, r := f.s.Size(); c != cols || r != rows {
		t.Errorf("the session is %d × %d", c, r)
	}
	f.start()
	f.waitFor("the size report", f.screenHas(fmt.Sprintf("size %dx%d", cols, rows)))
}

func TestAProgramsOutputReachesTheScreen(t *testing.T) {
	f := open(t, "scenario", "colour")
	f.start()
	f.waitFor("the colours", f.screenHas("truecolour"))
}

func TestWhatIsTypedReachesTheChild(t *testing.T) {
	f := open(t, "echo")
	f.start()
	f.focus()
	f.ui.Screen.Type("hi\r")
	f.waitFor("the echo", f.screenHas("echo:hi"))
}

func TestKeysReachTheProgramAsATerminalSendsThem(t *testing.T) {
	f := open(t, "raw")
	f.start()
	f.waitFor("raw mode", f.screenHas("raw ready"))
	f.focus()
	press := func(code unison.KeyCode, m mod.Modifiers, want string) {
		t.Helper()
		f.ui.Screen.KeyPress(code, m)
		f.waitFor("in:"+want, f.screenHas("in:"+want))
	}
	press(unison.KeyC, mod.Control, "03")              // Ctrl+C is a byte, not a signal, in raw mode
	press(unison.KeyUp, 0, "1b5b41")                   // ESC [ A
	press(unison.KeyF5, 0, "1b5b31357e")               // ESC [ 1 5 ~
	press(unison.KeyTab, mod.Shift, "1b5b5a")          // ESC [ Z
	press(unison.KeyBackspace, 0, "7f")                // DEL
	press(unison.KeyLeft, mod.Control, "1b5b313b3544") // ESC [ 1 ; 5 D
	f.ui.Screen.Type("é")
	f.waitFor("é", f.screenHas("in:c3a9"))
	// Alt+x is ESC x everywhere but macOS, where Option types characters.
	press(unison.KeyX, mod.Option, "1b78")
}

func TestTheMouseIsReportedWhenTheProgramAsks(t *testing.T) {
	f := open(t, "raw")
	f.start()
	f.waitFor("raw mode", f.screenHas("raw ready"))
	f.s.Feed([]byte("\x1b[?1000h\x1b[?1006h")) // what the program would write to ask
	f.ui.Screen.Click(f.point(3, 2))
	// SGR form: button 0 at column 4, row 3, pressed (M) and released (m).
	f.waitFor("the press", f.screenHas("1b5b3c303b343b334d"))
	f.waitFor("the release", f.screenHas("1b5b3c303b343b336d"))
	var sel bool
	f.ui.Do(func() { sel = f.v.HasSelection() })
	if sel {
		t.Error("a click reported to the program also selected text")
	}
}

func TestSelectionCanBeReadBack(t *testing.T) {
	f := open(t, "scenario", "links")
	f.start()
	f.waitFor("the output", f.screenHas("example"))
	f.ui.Do(func() {
		f.v.SelectAll()
		if !f.v.HasSelection() || !strings.Contains(f.v.SelectedText(), "example.invalid") {
			t.Errorf("selected %q", f.v.SelectedText())
		}
		f.v.ClearSelection()
		if f.v.HasSelection() {
			t.Error("the selection was not cleared")
		}
	})
}

func TestDraggingAndDoubleClickingSelect(t *testing.T) {
	f := open(t)
	f.s.Feed([]byte("alpha beta gamma\r\nsrc/core/screen.cpp:42 here\r\n"))
	f.ui.Sync()
	f.ui.Screen.Drag(f.point(6, 0), f.point(9, 0), 4)
	var got string
	f.ui.Do(func() { got = f.v.SelectedText() })
	if got != "beta" {
		t.Errorf("dragged over %q", got)
	}
	f.ui.Screen.DoubleClick(f.point(5, 1))
	f.ui.Do(func() { got = f.v.SelectedText() })
	if got != "src/core/screen.cpp:42" {
		t.Errorf("double-clicked %q", got)
	}
}

func TestCopyAndPasteGoThroughTheClipboard(t *testing.T) {
	f := open(t, "echo")
	f.start()
	f.s.Feed([]byte("copy me\r\n"))
	f.ui.Sync()
	f.focus()
	f.ui.Screen.Drag(f.point(0, 0), f.point(6, 0), 4)
	f.ui.Screen.KeyPress(unison.KeyC, mod.Control|mod.Shift)
	var clip string
	f.ui.Do(func() { clip = unison.ClipboardGetText() })
	if clip != "copy me" {
		t.Fatalf("the clipboard holds %q", clip)
	}
	f.ui.Do(func() { unison.ClipboardSetText("pasted\n") })
	f.ui.Screen.KeyPress(unison.KeyV, mod.Control|mod.Shift)
	f.waitFor("the paste", f.screenHas("echo:pasted"))
}

func TestScrollingBackAndReturning(t *testing.T) {
	f := open(t, "scenario", "scroll")
	f.start()
	f.waitFor("scrollback", func() bool { return f.v.ScrollbackCount() > 5 })
	f.ui.Do(func() { f.v.ScrollBy(5) })
	var offset int
	f.ui.Do(func() { offset = f.v.ScrollOffset() })
	if offset != 5 {
		t.Fatalf("scrolled back %d", offset)
	}
	// Typing returns to where new output appears, which is what makes
	// scrolling back safe.
	f.focus()
	f.ui.Screen.KeyPress(unison.KeyX, 0)
	f.ui.Do(func() { offset = f.v.ScrollOffset() })
	if offset != 0 {
		t.Errorf("after typing, scrolled back %d", offset)
	}
}

func TestTheWheelScrollsTheHistoryOrBecomesArrowKeys(t *testing.T) {
	f := open(t)
	for i := 0; i < 60; i++ {
		f.s.Feed([]byte("line\r\n"))
	}
	f.ui.Sync()
	f.ui.Screen.Wheel(f.point(10, 5), geom.NewPoint(0, 1), 0)
	var offset int
	f.ui.Do(func() { offset = f.v.ScrollOffset() })
	if offset != 3 {
		t.Errorf("one notch scrolled %d lines", offset)
	}
	// New output does not drag a reader back to the bottom.
	f.s.Feed([]byte("more\r\nmore\r\n"))
	f.waitFor("the view to hold still", func() bool { return f.v.ScrollOffset() == 5 })
}

func TestABlinkingCursorIsNotActivity(t *testing.T) {
	// The blink is the view redrawing one cell on a timer of its own. It
	// never reaches the emulator, so a shell waiting at a prompt must not
	// look busy.
	f := open(t, "sleep")
	f.s.SetActivityPeriod(200 * time.Millisecond)
	f.start()
	f.focus()
	time.Sleep(time.Second) // more than one blink
	f.ui.Sync()
	if f.s.Activity() {
		t.Error("a blinking cursor counted as activity")
	}
}

func TestAReservedShortcutIsNotConsumed(t *testing.T) {
	// The application asked for this one, so the view must neither treat
	// it as its own nor send it to the child.
	f := open(t, "raw")
	f.start()
	f.waitFor("raw mode", f.screenHas("raw ready"))
	f.ui.Do(func() { f.v.SetReservedShortcuts("Ctrl+Shift+C", "F6") })
	f.focus()
	for _, k := range []struct {
		code unison.KeyCode
		m    mod.Modifiers
	}{{unison.KeyC, mod.Control | mod.Shift}, {unison.KeyF6, 0}} {
		used := true
		f.ui.Do(func() { used = f.v.keyDown(k.code, k.m, false) })
		if used {
			t.Errorf("the view took the reserved %v", k)
		}
	}
	f.ui.Do(func() {
		if f.v.Claims(unison.KeyF6, 0) || !f.v.Claims(unison.KeyK, mod.Control) {
			t.Error("the keys the view claims ahead of the application are wrong")
		}
	})
	f.ui.Screen.KeyPress(unison.KeyF6, 0)
	f.ui.Screen.KeyPress(unison.KeyA, 0)
	f.waitFor("the a", f.screenHas("in:61"))
	if strings.Contains(f.s.ScreenText(), "in:1b5b31377e") {
		t.Error("the reserved F6 reached the program")
	}
}

func TestShortcutsAreReadFromTheirWrittenForm(t *testing.T) {
	for s, want := range map[string]unison.KeyCode{"Ctrl+Shift+T": unison.KeyT, "F6": unison.KeyF6, "Shift+F6": unison.KeyF6,
		"Alt+Return": unison.KeyReturn, "Ctrl+PgUp": unison.KeyPageUp, "Ctrl+[": unison.KeyOpenBracket} {
		if k, _, ok := ParseShortcut(s); !ok || k != want {
			t.Errorf("%s read as %v", s, k)
		}
	}
	if _, _, ok := ParseShortcut("Hyper+X"); ok {
		t.Error("an unknown modifier was accepted")
	}
}

func TestCtrlClickActivatesALink(t *testing.T) {
	f := open(t)
	f.s.Feed([]byte("see src/core/screen.cpp:42 and https://example.invalid/x\r\n"))
	f.ui.Sync()
	var got []string
	var lines []int
	f.ui.Do(func() {
		f.v.OnLinkActivated = func(link string, line, _ int) {
			got = append(got, link)
			lines = append(lines, line)
		}
	})
	f.ui.Screen.MouseMove(f.point(8, 0), mod.Control)
	var hovered string
	f.ui.Do(func() { hovered = f.v.HoveredLink() })
	if hovered != "src/core/screen.cpp:42" {
		t.Errorf("hovering with Ctrl shows %q", hovered)
	}
	f.ui.Screen.ClickWith(f.point(8, 0), unison.ButtonLeft, mod.Control)
	f.ui.Screen.ClickWith(f.point(40, 0), unison.ButtonLeft, mod.Control)
	if len(got) != 2 || got[0] != "src/core/screen.cpp:42" || lines[0] != 42 || got[1] != "https://example.invalid/x" {
		t.Errorf("activated %q at lines %v", got, lines)
	}
}

func TestALinkBrokenByTheWindowIsFoundWhole(t *testing.T) {
	f := open(t)
	f.s.Resize(20, 5)
	f.s.Feed([]byte("look at https://example.invalid/long/path now\r\n"))
	f.ui.Sync()
	var got string
	f.ui.Do(func() { f.v.OnLinkActivated = func(link string, _, _ int) { got = link } })
	f.ui.Screen.ClickWith(f.point(3, 1), unison.ButtonLeft, mod.Control) // on the second row of it
	if got != "https://example.invalid/long/path" {
		t.Errorf("activated %q", got)
	}
}

func TestTheStickyCommandNamesTheOutputAtTheTop(t *testing.T) {
	f := open(t)
	si := kvitterm.NewShellIntegration(f.s)
	f.ui.Do(func() { f.v.SetShellIntegration(si) })
	f.s.Feed([]byte("\x1b]133;A\x1b\\$ \x1b]133;B\x1b\\make\r\n\x1b]133;C\x1b\\"))
	for i := 0; i < 60; i++ {
		f.s.Feed([]byte("compiling\r\n"))
	}
	f.waitFor("the sticky command", func() bool { return f.v.StickyCommand() == "make" })
}

func TestTheCurrentMatchIsScrolledIntoView(t *testing.T) {
	f := open(t)
	f.s.Feed([]byte("the needle\r\n"))
	for i := 0; i < 80; i++ {
		f.s.Feed([]byte("hay\r\n"))
	}
	se := kvitterm.NewSearch(f.s)
	f.ui.Do(func() {
		f.v.SetSearch(se)
		se.SetQuery("needle")
		se.Next()
	})
	var offset, kept int
	f.ui.Do(func() { offset, kept = f.v.ScrollOffset(), f.v.ScrollbackCount() })
	m, _ := se.CurrentMatch()
	if m.Row >= 0 || -m.Row > offset || offset > kept {
		t.Errorf("the match on row %d is not in view at offset %d", m.Row, offset)
	}
}

func TestAScreenReaderIsToldTheVisibleText(t *testing.T) {
	f := open(t)
	f.s.Feed([]byte("hello\r\nworld"))
	f.ui.Sync()
	n := f.ui.Node(f.v)
	if n == nil {
		t.Fatal("no node")
	}
	if n.Role != role.TextArea || !n.ReadOnly || n.Name != "terminal" {
		t.Errorf("role %v, read-only %v, name %q", n.Role, n.ReadOnly, n.Name)
	}
	if n.Text == nil || !strings.HasPrefix(n.Text.Text, "hello\nworld") {
		t.Fatalf("text %+v", n.Text)
	}
	if n.Text.Caret != len("hello\nworld") {
		t.Errorf("the caret is at %d", n.Text.Caret)
	}
	if len(n.Text.Lines) != PreferredRows {
		t.Errorf("%d lines", len(n.Text.Lines))
	}
	f.ui.CheckNamed()
}

func TestAnUnknownFamilyFallsBackToAFixedWidthFont(t *testing.T) {
	// An application names a font it likes without knowing the machine has
	// it, so what a missing family falls back to is the common case, and it
	// has to be another fixed-width face.
	f := open(t)
	f.ui.Do(func() {
		f.v.SetFont("no such family, and none like it", 13)
		if fam, _ := f.v.Font(); fam != text.Monospace || !f.v.FixedWidth() {
			t.Errorf("fell back to %q, fixed width %v", fam, f.v.FixedWidth())
		}
	})
}

// ink counts the pixels in a rectangle of the window that differ from the
// terminal's ground.
func ink(img image.Image, r geom.Rect, ground [3]uint32) int {
	n := 0
	for y := int(r.Y); y < int(r.Bottom()); y++ {
		for x := int(r.X); x < int(r.Right()); x++ {
			cr, cg, cb, _ := img.At(x, y).RGBA()
			if [3]uint32{cr >> 8, cg >> 8, cb >> 8} != ground {
				n++
			}
		}
	}
	return n
}

func TestEveryCellKeepsItsColumn(t *testing.T) {
	// A grid is a claim about where things are drawn, which the screen
	// cannot make: it holds "M M M" whatever the drawing does with it. So
	// this reads the pixels. Letters in columns 0, 2 and 4, spaces between,
	// all one run of one style, in the fixed-width face and in a
	// proportional one an application asked for, where a space is about a
	// third of a cell.
	for _, family := range []string{text.Monospace, text.SansSerif} {
		t.Run(family, func(t *testing.T) {
			f := open(t)
			f.ui.Do(func() { f.v.SetFont(family, 14) })
			f.s.Feed([]byte("M M M\r\n"))
			f.ui.Sync()
			img := f.ui.Capture()
			bg := f.v.Palette().Background
			ground := [3]uint32{uint32(bg.R), uint32(bg.G), uint32(bg.B)}
			for col := 0; col <= 4; col++ {
				var r geom.Rect
				f.ui.Do(func() {
					c := f.v.CellRect(col, 0)
					c = c.Inset(geom.NewSymmetricInsets(c.Width/4, 0))
					r = f.v.RectToRoot(c)
				})
				got := ink(img, r, ground)
				if col%2 == 0 && got == 0 {
					t.Errorf("nothing drawn in column %d", col)
				}
				if col%2 == 1 && got != 0 {
					t.Errorf("column %d holds a space and has %d pixels drawn in it", col, got)
				}
			}
		})
	}
}

func TestPaletteAndCursorAreDrawn(t *testing.T) {
	f := open(t)
	f.s.Feed([]byte("\x1b[41m  \x1b[0m"))
	f.focus()
	f.ui.Sync()
	img := f.ui.Capture()
	var red, cursor geom.Rect
	f.ui.Do(func() {
		red = f.v.RectToRoot(f.v.CellRect(0, 0))
		cursor = f.v.RectToRoot(f.v.CellRect(2, 0))
	})
	p := screen.DefaultPalette()
	at := func(r geom.Rect) [3]uint32 {
		cr, cg, cb, _ := img.At(int(r.CenterX()), int(r.CenterY())).RGBA()
		return [3]uint32{cr >> 8, cg >> 8, cb >> 8}
	}
	if got := at(red); got != [3]uint32{uint32(p.ANSI[1].R), uint32(p.ANSI[1].G), uint32(p.ANSI[1].B)} {
		t.Errorf("the red ground is %v", got)
	}
	if got := at(cursor); got != [3]uint32{uint32(p.Cursor.R), uint32(p.Cursor.G), uint32(p.Cursor.B)} {
		t.Errorf("the cursor is %v", got)
	}
}
