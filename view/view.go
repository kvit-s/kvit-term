// Package view draws a terminal session on a unison window and turns
// keyboard, mouse and clipboard input into what the child process expects.
//
// Only the rows on screen are drawn, from the session's screen, and the
// view redraws when the session reports a change. Which keystrokes belong to
// the terminal and which to the application around it has no neutral
// answer, so it is the application's to make: SetReservedShortcuts names
// the ones the view must leave alone.
package view

import (
	"image/color"
	"math"
	"strings"

	"github.com/kvit-s/kvit-term"
	"github.com/kvit-s/kvit-term/screen"
	"github.com/kvit-s/kvit-ui/text"
	"github.com/richardwilkes/toolbox/v2/geom"
	"github.com/richardwilkes/unison"
)

// DefaultSize is the font size a view starts with, in pixels.
const DefaultSize = 13

// View is a terminal on the screen.
type View struct {
	unison.Panel
	fonts   *text.Fonts
	session *kvitterm.Session
	stop    func()
	palette screen.Palette
	family  string
	size    float32

	cellW, cellH int
	baseline     float32
	uniform      bool
	cols, rows   int

	offset    int // lines scrolled back from the bottom
	lastAway  int // lines the screen had scrolled away at the last change
	wheelOwed float32

	reserved []shortcut

	anchor, head  screen.Point
	hasSel        bool
	selecting     bool
	unit          int // what a drag selects by: 1 characters, 2 words, 3 lines
	unitFrom      screen.Point
	unitTo        screen.Point
	dragScrolling bool

	search    *kvitterm.Search
	shell     *kvitterm.ShellIntegration
	stopShell func()
	sticky    string

	link     kvitterm.Link // the link under the pointer while Ctrl is held
	linkRow  int
	hasLink  bool
	pointer  geom.Point
	blinkOn  bool
	blinkGen int
	focused  bool

	// Rows copied out of the screen for one frame.
	frame frameState

	// OnLinkActivated is called for a link Ctrl+clicked, with the line and
	// column that followed a path, or -1. What to do with it is the
	// application's decision.
	OnLinkActivated func(link string, line, character int)
	// OnChange is called after the scroll position, the selection, the
	// sticky command, the hovered link or the grid changed.
	OnChange func()
}

// New makes a view drawing with fonts, the application's text layer. It
// shows nothing until SetSession.
func New(fonts *text.Fonts) *View {
	v := &View{fonts: fonts, palette: screen.DefaultPalette(), family: text.Monospace, size: DefaultSize, cols: 80, rows: 24, unit: 1, blinkOn: true}
	v.Self = v
	v.SetFocusable(true)
	v.DrawCallback = v.draw
	v.FrameChangeCallback = v.updateGrid
	v.KeyDownCallback = v.keyDown
	v.RuneTypedCallback = v.runeTyped
	v.MouseDownCallback = v.mouseDown
	v.MouseDragCallback = v.mouseDrag
	v.MouseUpCallback = v.mouseUp
	v.MouseMoveCallback = v.mouseMove
	v.MouseEnterCallback = v.mouseMove
	v.MouseExitCallback = v.mouseExit
	v.MouseWheelCallback = v.mouseWheel
	v.GainedFocusCallback = v.gainedFocus
	v.LostFocusCallback = v.lostFocus
	v.UpdateCursorCallback = v.cursorShape
	v.SetSizer(v.sizes)
	v.measure()
	return v
}

// PreferredColumns and PreferredRows are the size a view asks its layout
// for, 80 × 24 cells unless set; it takes whatever it is given.
var (
	PreferredColumns = 80
	PreferredRows    = 24
)

func (v *View) sizes(geom.Size) (minSize, prefSize, maxSize geom.Size) {
	cw, ch := float32(v.cellW), float32(v.cellH)
	return geom.NewSize(2*cw, ch), geom.NewSize(float32(PreferredColumns)*cw, float32(PreferredRows)*ch),
		geom.NewSize(unison.DefaultMaxSize, unison.DefaultMaxSize)
}

// Session is the session shown, or nil.
func (v *View) Session() *kvitterm.Session { return v.session }

// SetSession shows a session, telling it the view's size. Several views can
// show one session; a view without one draws an empty screen.
func (v *View) SetSession(s *kvitterm.Session) {
	if s == v.session {
		return
	}
	if v.session != nil {
		v.stop()
		v.session.HoldForDraw(false)
	}
	v.session = s
	v.offset, v.hasSel, v.hasLink = 0, false, false
	if s != nil {
		v.stop = s.Observe(func(e kvitterm.Event) {
			unison.InvokeTask(func() {
				if v.session == s {
					v.sessionEvent(e)
				}
			})
		})
		s.HoldForDraw(true)
		s.View(func(scr *screen.Screen) { v.lastAway = scr.ScrolledAway() })
		s.Resize(v.cols, v.rows)
		if v.focused {
			s.SetFocused(true)
		}
	}
	v.changed()
	v.MarkForRedraw()
}

func (v *View) sessionEvent(e kvitterm.Event) {
	switch e.Kind {
	case kvitterm.ContentChanged, kvitterm.Resized:
		var away, kept int
		v.session.View(func(scr *screen.Screen) { away, kept = scr.ScrolledAway(), scr.ScrollbackCount() })
		if v.offset > 0 {
			// Hold the view still while the user reads back through the
			// history, rather than dragging them to the bottom.
			v.offset = min(max(0, v.offset+away-v.lastAway), kept)
		}
		v.lastAway = away
		v.updateSticky()
		v.MarkForRedraw()
		v.changed()
	}
}

// SetPalette changes the colours. The emulator keeps colours as the program
// named them, so what is already on the screen is recoloured without the
// program redrawing.
func (v *View) SetPalette(p screen.Palette) {
	v.palette = p
	v.MarkForRedraw()
}

// Palette is the colours in use.
func (v *View) Palette() screen.Palette { return v.palette }

// SetFont changes the font. The default is the platform's own fixed-width
// face (text.Monospace). A family this machine does not have falls back to
// that fixed-width face rather than to the interface's proportional one,
// since an application naming a font cannot know it is installed. A
// proportional family asked for by name is drawn as asked, each character
// at the start of its cell, with cells as wide as the widest character, so
// the grid holds and a space still takes a column.
func (v *View) SetFont(family string, size float32) {
	if family == "" || (family != text.Monospace && !strings.EqualFold(v.fonts.ResolveFamily(family), family)) {
		family = text.Monospace
	}
	if size <= 0 {
		size = DefaultSize
	}
	if family == v.family && size == v.size {
		return
	}
	v.family, v.size = family, size
	v.measure()
	v.updateGrid()
	v.MarkForRedraw()
}

// Font is the family and size in use.
func (v *View) Font() (family string, size float32) { return v.family, v.size }

// style is the text style for a cell's text.
func (v *View) style(bold, italic bool, c color.NRGBA) text.Style {
	st := text.Style{Family: v.family, Size: v.size, Italic: italic, Color: text.Color{R: c.R, G: c.G, B: c.B, A: c.A}}
	if bold {
		st.Weight = text.Bold
	}
	return st
}

// measure finds the cell size. It measures rather than assumes: a family
// the machine lacks falls back to another face, and an application may ask
// for a proportional one, so whether every character advances alike is
// checked across printable ASCII. A fixed-width font gets cells of its own
// advance rounded to whole pixels, so every column lands on the same pixel
// and the hundredth is not half a pixel off; a proportional one gets cells
// as wide as its widest character.
func (v *View) measure() {
	st := v.style(false, false, color.NRGBA{A: 255})
	one := text.Options{KeepTrailingSpace: true}
	m := v.fonts.Layout([]text.Span{{Text: "M", Style: st}}, one)
	natural, height := m.Size()
	widest := natural
	v.uniform = true
	for c := ' '; c <= '~'; c++ {
		w, _ := v.fonts.Layout([]text.Span{{Text: string(c), Style: st}}, one).Size()
		widest = max(widest, w)
		if math.Abs(float64(w-natural)) > 0.01 {
			v.uniform = false
		}
	}
	cell := natural
	if !v.uniform {
		cell = widest
	}
	v.cellW = max(1, int(math.Round(float64(cell))))
	v.cellH = max(1, int(math.Ceil(float64(height))))
	v.baseline = m.Baseline()
}

// CellSize is the size of one cell in pixels.
func (v *View) CellSize() (width, height int) { return v.cellW, v.cellH }

// FixedWidth reports whether the font in use advances every printable
// ASCII character alike.
func (v *View) FixedWidth() bool { return v.uniform }

// updateGrid derives the columns and rows from the view's size and the
// cell, and tells the session, which tells the child.
func (v *View) updateGrid() {
	r := v.ContentRect(false)
	cols := max(1, int(r.Width)/v.cellW)
	rows := max(1, int(r.Height)/v.cellH)
	if cols == v.cols && rows == v.rows {
		return
	}
	v.cols, v.rows = cols, rows
	if v.session != nil {
		v.session.Resize(cols, rows)
	}
	v.changed()
	v.MarkForRedraw()
}

// Grid is the view's size in cells.
func (v *View) Grid() (columns, rows int) { return v.cols, v.rows }

// CellRect is where a cell is drawn, for an application drawing something
// of its own over the terminal. Rows are numbered as the screen numbers
// them.
func (v *View) CellRect(column, row int) geom.Rect {
	y := row + v.offset
	return geom.NewRect(float32(column*v.cellW), float32(y*v.cellH), float32(v.cellW), float32(v.cellH))
}

// cellAt is the cell under a point in the view, as a screen position.
func (v *View) cellAt(p geom.Point) screen.Point {
	col := min(max(0, int(p.X)/v.cellW), max(0, v.cols-1))
	row := min(max(0, int(math.Floor(float64(p.Y)/float64(v.cellH)))), max(0, v.rows-1))
	return screen.Point{X: col, Y: row - v.offset}
}

// ScrollOffset is how far back the view is scrolled, in lines; 0 is the
// bottom, where new output appears.
func (v *View) ScrollOffset() int { return v.offset }

// SetScrollOffset scrolls the view to a number of lines back.
func (v *View) SetScrollOffset(lines int) {
	kept := 0
	if v.session != nil {
		kept = v.session.ScrollbackCount()
	}
	lines = min(max(0, lines), kept)
	if lines == v.offset {
		return
	}
	// The offset is from the screen as it is now; only lines that scroll
	// away from here on move it.
	if v.session != nil {
		v.session.View(func(scr *screen.Screen) { v.lastAway = scr.ScrolledAway() })
	}
	v.offset = lines
	v.updateSticky()
	v.changed()
	v.MarkForRedraw()
}

// ScrollBy scrolls the view back (positive) or forward (negative).
func (v *View) ScrollBy(lines int) { v.SetScrollOffset(v.offset + lines) }

// ScrollToBottom returns the view to where new output appears.
func (v *View) ScrollToBottom() { v.SetScrollOffset(0) }

// ScrollbackCount is how many lines the session keeps above its screen.
func (v *View) ScrollbackCount() int {
	if v.session == nil {
		return 0
	}
	return v.session.ScrollbackCount()
}

func (v *View) changed() {
	if v.OnChange != nil {
		v.OnChange()
	}
}

// SetSearch draws a search's matches over the screen and scrolls to its
// current match as it moves.
func (v *View) SetSearch(se *kvitterm.Search) {
	v.search = se
	if se != nil {
		se.OnChange = func() {
			if m, ok := se.CurrentMatch(); ok && (m.Row < -v.offset || m.Row >= v.rows-v.offset) {
				// Bring it into view with a few lines of context above it.
				v.SetScrollOffset(-m.Row + min(3, v.rows/4))
			}
			v.MarkForRedraw()
		}
	}
	v.MarkForRedraw()
}

// SetShellIntegration lets the view say which command the output at the
// top of the window belongs to, which is what a sticky heading needs.
func (v *View) SetShellIntegration(si *kvitterm.ShellIntegration) {
	if v.stopShell != nil {
		v.stopShell()
		v.stopShell = nil
	}
	v.shell = si
	if si != nil {
		v.stopShell = si.Observe(func(e kvitterm.ShellEvent) {
			if e.Kind == kvitterm.CommandsChanged {
				unison.InvokeTask(v.updateSticky)
			}
		})
	}
	v.updateSticky()
}

// StickyCommand is the command whose output is at the top of the window,
// or "".
func (v *View) StickyCommand() string { return v.sticky }

func (v *View) updateSticky() {
	s := ""
	if v.shell != nil {
		if i := v.shell.CommandAtRow(-v.offset); i >= 0 {
			if c, ok := v.shell.Command(i); ok {
				s = c.Text
			}
		}
	}
	if s != v.sticky {
		v.sticky = s
		v.changed()
	}
}

// HoveredLink is the link under the pointer while Ctrl is held, or "".
func (v *View) HoveredLink() string {
	if !v.hasLink {
		return ""
	}
	return v.link.Text
}

// AccessibleText is the visible screen as text.
func (v *View) AccessibleText() string {
	if v.session == nil {
		return ""
	}
	var s string
	v.session.View(func(scr *screen.Screen) { s = scr.Text(-v.offset, v.rows-1-v.offset) })
	return s
}

// blink toggles the cursor while the view has the keyboard and the program
// wants a blinking cursor. The blink never reaches the session, so a shell
// waiting at a prompt does not look busy.
func (v *View) blink(gen int) {
	if gen != v.blinkGen || !v.focused {
		return
	}
	v.blinkOn = !v.blinkOn
	v.MarkForRedraw()
	unison.InvokeTaskAfter(func() { v.blink(gen) }, blinkInterval)
}

func (v *View) restartBlink() {
	v.blinkGen++
	v.blinkOn = true
	if v.focused {
		gen := v.blinkGen
		unison.InvokeTaskAfter(func() { v.blink(gen) }, blinkInterval)
	}
}

func (v *View) gainedFocus() {
	v.focused = true
	if v.session != nil {
		v.session.SetFocused(true)
	}
	v.restartBlink()
	v.MarkForRedraw()
}

func (v *View) lostFocus() {
	v.focused = false
	if v.session != nil {
		v.session.SetFocused(false)
	}
	v.restartBlink()
	v.MarkForRedraw()
}

func firstRune(c screen.Cell) rune {
	if c.Ch == 0 {
		return 0
	}
	return c.Ch
}
