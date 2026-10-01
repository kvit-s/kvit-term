package screen

import (
	"strings"

	"github.com/kvit-s/kvit-term/third_party/xterm"
)

// CursorShape is how the cursor is drawn.
type CursorShape uint8

// The cursor shapes a program can ask for.
const (
	BlockCursor CursorShape = iota
	UnderlineCursor
	BarCursor
)

// MouseTracking is what a program has asked to be told about the mouse. A
// terminal reports nothing until a program asks, which is why selecting
// text with the mouse works in a shell and stops working inside a
// full-screen editor.
type MouseTracking uint8

// The levels of mouse reporting.
const (
	NoMouse     MouseTracking = iota
	MouseClick                // presses and releases
	MouseDrag                 // and movement while a button is held
	MouseMotion               // and all movement
)

// MouseButton is a button a mouse report can name.
type MouseButton uint8

// The buttons, and the two directions of the wheel.
const (
	NoButton MouseButton = iota
	LeftButton
	MiddleButton
	RightButton
	WheelUp
	WheelDown
)

// DefaultScrollback is how many lines a new screen keeps above the visible
// rows.
const DefaultScrollback = 10000

// Screen is the emulator: it holds the visible grid, the scrollback above
// it, the cursor, the title and the modes a program can select that change
// how keys and the mouse are reported.
//
// Rows are numbered from the top of the visible screen: row 0 is the first
// visible line, row -1 the line most recently scrolled off, and
// -ScrollbackCount() the oldest line still kept. The scrollback belongs to
// the normal screen; a full-screen program's alternate screen has none.
//
// A Screen is not safe for use from several goroutines at once; a Session
// guards the one it owns.
type Screen struct {
	t     *xterm.Terminal
	limit int
	title string
	cd    *xterm.CellData

	// The state last reported, so a change can be reported once.
	altScreen     bool
	tracking      MouseTracking
	cursor        Point
	cursorVisible bool

	// normal is the normal buffer whose Trimmed counter numbers rows from
	// the first line written; base is subtracted from that count after the
	// scrollback is cleared, so the numbering starts again from zero.
	normal *xterm.Buffer
	base   int

	// Mouse state for reports.
	mouse    Point
	held     MouseButton
	focused  bool
	oscID    int
	oscData  strings.Builder
	oscValid bool

	// Called while Feed, SetSize or an input method runs, on the same
	// goroutine; they must not call back into the Screen.

	// OnWrite receives bytes the child must be sent: a key press, a mouse
	// report, or an answer to a question the program asked the terminal.
	OnWrite func([]byte)
	// OnDamage reports that rows of the visible screen changed, first and
	// last inclusive.
	OnDamage func(first, last int)
	// OnScroll reports lines moving off the top of the screen into the
	// scrollback.
	OnScroll func(lines int)
	// OnScrollbackCleared reports that the scrollback was erased, by the
	// program or by ClearScrollback; rows are numbered from zero again.
	OnScrollbackCleared func()
	OnResize            func(columns, rows int)
	OnCursorMove        func(Point)
	OnTitle             func(string)
	OnBell              func()
	OnAlternateScreen   func(active bool)
	OnMouseTracking     func(MouseTracking)
	// OnOSC receives the operating-system commands the emulator does not
	// act on itself. The shell-integration marks (7, 133, 633) arrive here.
	OnOSC func(command int, payload string)
}

// New makes a screen of the given size.
func New(columns, rows int) *Screen {
	columns, rows = max(xterm.MinimumCols, columns), max(xterm.MinimumRows, rows)
	s := &Screen{
		t:             xterm.New(xterm.WithCols(columns), xterm.WithRows(rows), xterm.WithScrollback(DefaultScrollback)),
		limit:         DefaultScrollback,
		cd:            xterm.NewCellData(),
		cursorVisible: true,
	}
	s.normal = s.t.NormalBuffer()
	s.t.OnData(func(d string) { s.write([]byte(d)) })
	s.t.OnBinary(func(d string) { s.write([]byte(d)) })
	s.t.OnBell(func() {
		if s.OnBell != nil {
			s.OnBell()
		}
	})
	s.t.OnTitleChange(func(title string) {
		s.title = title
		if s.OnTitle != nil {
			s.OnTitle(title)
		}
	})
	s.t.OnRender(func(r xterm.RowRange) { s.damage(r.Start, r.End) })
	last := s.ScrolledAway()
	s.t.OnScroll(func(int) {
		now := s.ScrolledAway()
		if now > last && s.OnScroll != nil {
			s.OnScroll(now - last)
		}
		last = now
	})
	s.t.OnRequestSendFocus(func() { s.reportFocus() })

	p := s.t.Parser()
	// Erasing the scrollback (CSI 3 J, which `clear` sends) is done here
	// rather than by the emulator, so that rows are numbered from zero
	// again at the same point in the stream.
	p.RegisterCsiHandler(xterm.FunctionIdentifier{Final: 'J'}, func(params *xterm.Params) bool {
		if params.Length == 0 || params.Params[0] != 3 || s.t.IsAltBufferActive() {
			return false
		}
		s.clearScrollback()
		return true
	})
	// Every OSC the emulator does not claim is handed on whole.
	p.SetOscHandlerFallback(func(id int, action string, payload ...interface{}) {
		switch action {
		case "START":
			s.oscID, s.oscValid = id, true
			s.oscData.Reset()
		case "PUT":
			if len(payload) > 0 {
				if text, ok := payload[0].(string); ok && s.oscData.Len() < 1<<20 {
					s.oscData.WriteString(text)
				}
			}
		case "END":
			ok := len(payload) > 0 && payload[0] == true
			if ok && s.oscValid && s.OnOSC != nil {
				s.OnOSC(s.oscID, s.oscData.String())
			}
			s.oscValid = false
			s.oscData.Reset()
		}
	})
	return s
}

func (s *Screen) write(b []byte) {
	if s.OnWrite != nil && len(b) > 0 {
		s.OnWrite(b)
	}
}

func (s *Screen) damage(first, last int) {
	if s.OnDamage != nil {
		s.OnDamage(max(0, first), min(last, s.Rows()-1))
	}
}

// Feed interprets bytes a program wrote. A character split across two calls
// is put together again.
func (s *Screen) Feed(b []byte) {
	if len(b) == 0 {
		return
	}
	s.t.Write(b)
	s.noteChanges()
}

// noteChanges reports the state that changed while bytes were interpreted.
func (s *Screen) noteChanges() {
	if alt := s.t.IsAltBufferActive(); alt != s.altScreen {
		s.altScreen = alt
		if s.OnAlternateScreen != nil {
			s.OnAlternateScreen(alt)
		}
		s.damage(0, s.Rows()-1)
	}
	if tr := s.MouseTracking(); tr != s.tracking {
		s.tracking = tr
		if s.OnMouseTracking != nil {
			s.OnMouseTracking(tr)
		}
	}
	c, vis := s.Cursor(), s.CursorVisible()
	if c != s.cursor || vis != s.cursorVisible {
		s.cursor, s.cursorVisible = c, vis
		if s.OnCursorMove != nil {
			s.OnCursorMove(c)
		}
	}
}

// Reset returns the terminal to how it started. A hard reset also clears
// the screen and the scrollback; a soft one keeps what is shown and resets
// the modes, as CSI ! p does.
func (s *Screen) Reset(hard bool) {
	if hard {
		s.t.Reset()
		s.ScrolledAway() // notices the new buffer
	} else {
		s.t.WriteString("\x1b[!p")
	}
	s.noteChanges()
	s.damage(0, s.Rows()-1)
}

// Columns is the width of the screen in cells.
func (s *Screen) Columns() int { return s.t.Cols() }

// Rows is the height of the visible screen.
func (s *Screen) Rows() int { return s.t.Rows() }

// SetSize changes the size. Lines that were wrapped are wrapped again at
// the new width, in the scrollback as well as on the screen, so widening
// the window makes old output readable rather than ragged. The emulator
// needs at least two columns.
func (s *Screen) SetSize(columns, rows int) {
	columns, rows = max(xterm.MinimumCols, columns), max(xterm.MinimumRows, rows)
	if columns == s.Columns() && rows == s.Rows() {
		return
	}
	s.t.Resize(columns, rows)
	s.noteChanges()
	if s.OnResize != nil {
		s.OnResize(columns, rows)
	}
	s.damage(0, rows-1)
}

// ScrollbackCount is how many lines are kept above the visible screen.
func (s *Screen) ScrollbackCount() int { return s.t.NormalBuffer().YBase }

// ScrollbackLimit is the most lines the scrollback keeps.
func (s *Screen) ScrollbackLimit() int { return s.limit }

// SetScrollbackLimit changes how many lines the scrollback keeps, dropping
// the oldest at once when it shrinks.
func (s *Screen) SetScrollbackLimit(lines int) {
	lines = max(0, lines)
	if lines == s.limit {
		return
	}
	s.limit = lines
	s.t.SetScrollback(lines)
}

// ClearScrollback erases the lines kept above the visible screen.
func (s *Screen) ClearScrollback() {
	s.clearScrollback()
	s.damage(0, s.Rows()-1)
}

func (s *Screen) clearScrollback() {
	s.ScrolledAway()
	b := s.t.NormalBuffer()
	if n := b.YBase; n > 0 {
		b.Lines.TrimStart(n)
		b.YBase, b.YDisp = 0, 0
		b.SavedState.Y = max(b.SavedState.Y-n, 0)
	}
	s.base = b.Trimmed
	if s.OnScrollbackCleared != nil {
		s.OnScrollbackCleared()
	}
}

// ScrolledAway is how many lines have moved off the top of the normal
// screen since it started or since the scrollback was last cleared,
// whether they are still kept or not. A screen row plus this number stays
// the same while output scrolls, which is how a command's output is found
// again later.
func (s *Screen) ScrolledAway() int {
	b := s.t.NormalBuffer()
	if b != s.normal {
		// A hard reset made new buffers, and with them a new count.
		s.normal, s.base = b, 0
		if s.OnScrollbackCleared != nil {
			s.OnScrollbackCleared()
		}
	}
	return b.Trimmed - s.base + b.YBase
}

// bufferLine is the emulator's line for a row, or nil.
func (s *Screen) bufferLine(row int) *xterm.BufferLine {
	var b *xterm.Buffer
	if row >= 0 {
		if row >= s.Rows() {
			return nil
		}
		b = s.t.Buffer()
	} else {
		b = s.t.NormalBuffer()
		if -row > b.YBase {
			return nil
		}
	}
	i := b.YBase + row
	if i < 0 || i >= b.Lines.Length() {
		return nil
	}
	return b.Lines.Get(i)
}

// Line is one row of the screen or the scrollback. A row outside both is
// empty.
func (s *Screen) Line(row int) Line {
	var l Line
	s.LineInto(row, &l)
	return l
}

// LineInto reads a row into l, reusing its cells, for a caller reading every
// row on every frame.
func (s *Screen) LineInto(row int, l *Line) {
	l.Cells = l.Cells[:0]
	l.Continuation = false
	bl := s.bufferLine(row)
	if bl == nil {
		return
	}
	l.Continuation = bl.IsWrapped
	n := min(bl.Len, s.Columns())
	for x := 0; x < n; x++ {
		bl.LoadCell(x, s.cd)
		l.Cells = append(l.Cells, s.cellFrom(s.cd))
	}
}

// Cell is one cell of the screen or the scrollback.
func (s *Screen) Cell(row, column int) Cell {
	bl := s.bufferLine(row)
	if bl == nil || column < 0 || column >= min(bl.Len, s.Columns()) {
		return Blank
	}
	bl.LoadCell(column, s.cd)
	return s.cellFrom(s.cd)
}

func colorFrom(mode uint32, v int) Color {
	switch mode {
	case xterm.AttrCMP16, xterm.AttrCMP256:
		return Color{Kind: Indexed, Index: uint8(v)}
	case xterm.AttrCMRGB:
		return Color{Kind: RGB, R: uint8(v >> 16), G: uint8(v >> 8), B: uint8(v)}
	}
	return Color{}
}

func (s *Screen) cellFrom(cd *xterm.CellData) Cell {
	var c Cell
	switch w := cd.GetWidth(); {
	case w == 0:
		c.Ch, c.Width = 0, 0
	default:
		c.Width = uint8(w)
		chars := cd.GetChars()
		if chars == "" {
			c.Ch = ' '
		} else {
			for i, r := range chars {
				if i == 0 {
					c.Ch = r
				} else {
					c.Extra = chars
					break
				}
			}
		}
	}
	st := &c.Style
	st.Foreground = colorFrom(cd.GetFgColorMode(), cd.GetFgColor())
	st.Background = colorFrom(cd.GetBgColorMode(), cd.GetBgColor())
	st.Bold = cd.IsBold() != 0
	st.Dim = cd.IsDim() != 0
	st.Italic = cd.IsItalic() != 0
	st.Blink = cd.IsBlink() != 0
	st.Reverse = cd.IsInverse() != 0
	st.Conceal = cd.IsInvisible() != 0
	st.Strike = cd.IsStrikethrough() != 0
	st.Overline = cd.IsOverline() != 0
	if cd.IsUnderline() != 0 {
		switch cd.GetUnderlineStyle() {
		case xterm.UnderlineStyleDouble:
			st.Underline = DoubleUnderline
		case xterm.UnderlineStyleCurly:
			st.Underline = CurlyUnderline
		case xterm.UnderlineStyleDotted:
			st.Underline = DottedUnderline
		case xterm.UnderlineStyleDashed:
			st.Underline = DashedUnderline
		default:
			st.Underline = SingleUnderline
		}
	}
	return c
}

// Text is rows from to to, inclusive, as text. A line that was wrapped is
// joined back onto the one above, so copying a long command line gives the
// command rather than the shape the window happened to have.
func (s *Screen) Text(from, to int) string {
	var b strings.Builder
	var cur, below Line
	for row := from; row <= to; row++ {
		if row == from {
			s.LineInto(row, &cur)
		}
		// Each row is read once and kept for the next turn, because what to
		// do with the blanks at the end of this one is the row below's to
		// say: where the line carries on, they are spaces inside it.
		if row < to {
			s.LineInto(row+1, &below)
		} else {
			below = Line{}
		}
		if row != from && !cur.Continuation {
			b.WriteByte('\n')
		}
		trailing := DropBlanks
		if below.Continuation {
			trailing = KeepBlanks
		}
		b.WriteString(cur.TextRange(0, -1, trailing))
		cur, below = below, cur
	}
	return b.String()
}

// TextInRange is the text of a selection. A linear selection runs from one
// point to the other through the ends of lines; a block selection takes the
// same columns out of each row and joins nothing.
func (s *Screen) TextInRange(start, end Point, block bool) string {
	if end.Before(start) {
		start, end = end, start
	}
	var b strings.Builder
	var cur, below Line
	for row := start.Y; row <= end.Y; row++ {
		if row == start.Y {
			s.LineInto(row, &cur)
		}
		if row < end.Y {
			s.LineInto(row+1, &below)
		} else {
			below = Line{}
		}
		first, last := 0, -1
		if block {
			first, last = min(start.X, end.X), max(start.X, end.X)
		} else {
			if row == start.Y {
				first = start.X
			}
			if row == end.Y {
				last = end.X
			}
		}
		if row != start.Y && (block || !cur.Continuation) {
			b.WriteByte('\n')
		}
		// Only a selection that runs on into the row below keeps the blanks
		// at the end of this one.
		trailing := DropBlanks
		if !block && below.Continuation {
			trailing = KeepBlanks
		}
		b.WriteString(cur.TextRange(first, last, trailing))
		cur, below = below, cur
	}
	return b.String()
}

// Cursor is where the cursor is on the visible screen. A cursor waiting to
// wrap after the last column reports the last column.
func (s *Screen) Cursor() Point {
	return Point{X: min(s.t.CursorX(), s.Columns()-1), Y: s.t.CursorY()}
}

// CursorVisible reports whether the program shows the cursor.
func (s *Screen) CursorVisible() bool { return !s.t.IsCursorHidden() }

// CursorBlinks reports whether the cursor should blink.
func (s *Screen) CursorBlinks() bool {
	m := s.t.DecPrivateModes()
	if m.CursorBlinkOverride != nil {
		return *m.CursorBlinkOverride
	}
	if m.CursorBlink != nil {
		return *m.CursorBlink
	}
	return true
}

// CursorShape is the shape the program asked for.
func (s *Screen) CursorShape() CursorShape {
	if st := s.t.DecPrivateModes().CursorStyle; st != nil {
		switch *st {
		case xterm.CursorStyleUnderline:
			return UnderlineCursor
		case xterm.CursorStyleBar:
			return BarCursor
		}
	}
	return BlockCursor
}

// Title is the title the program set, if any.
func (s *Screen) Title() string { return s.title }

// AlternateScreen reports a full-screen program's second screen.
func (s *Screen) AlternateScreen() bool { return s.t.IsAltBufferActive() }

// ReverseVideo reports whole-screen reverse video (DECSCNM, mode 5). It is
// tracked and never drawn, so it does not change what is shown.
func (s *Screen) ReverseVideo() bool { return s.t.DecPrivateModes().ReverseVideo }

// MouseTracking is what the program asked to be told about the mouse.
func (s *Screen) MouseTracking() MouseTracking {
	switch s.t.DecPrivateModes().MouseTrackingMode {
	case "X10", "VT200":
		return MouseClick
	case "DRAG":
		return MouseDrag
	case "ANY":
		return MouseMotion
	}
	return NoMouse
}

// FocusReporting reports whether the program asked to be told when the
// terminal gains and loses the keyboard.
func (s *Screen) FocusReporting() bool { return s.t.DecPrivateModes().SendFocus }

// BracketedPaste reports whether the program asked for pasted text to be
// marked, so it can tell it from typing.
func (s *Screen) BracketedPaste() bool { return s.t.DecPrivateModes().BracketedPasteMode }

// ApplicationCursorKeys reports whether the arrow keys send their
// application form, which full-screen programs ask for.
func (s *Screen) ApplicationCursorKeys() bool { return s.t.DecPrivateModes().ApplicationCursorKeys }

// SetFocused tells the program the terminal gained or lost the keyboard,
// if it asked to be told.
func (s *Screen) SetFocused(focused bool) {
	s.focused = focused
	s.reportFocus()
}

func (s *Screen) reportFocus() {
	if !s.FocusReporting() {
		return
	}
	if s.focused {
		s.write([]byte("\x1b[I"))
	} else {
		s.write([]byte("\x1b[O"))
	}
}

// MouseMove moves the pointer to a cell of the visible screen, reporting
// it where the program asked for movement.
func (s *Screen) MouseMove(row, column int, m Modifiers) {
	p := Point{X: column, Y: row}
	if p == s.mouse {
		return
	}
	s.mouse = p
	tr := s.MouseTracking()
	if tr == MouseMotion || (tr == MouseDrag && s.held != NoButton) {
		button := xterm.MouseButtonNone
		if s.held != NoButton {
			button = coreButton(s.held)
		}
		s.mouseEvent(button, xterm.MouseActionMove, m)
	}
}

// MouseButton presses or releases a button at the pointer's cell, or turns
// the wheel, which reports a press only.
func (s *Screen) MouseButton(b MouseButton, pressed bool, m Modifiers) {
	switch b {
	case WheelUp, WheelDown:
		if !pressed {
			return
		}
		action := xterm.MouseActionUp
		if b == WheelDown {
			action = xterm.MouseActionDown
		}
		s.mouseEvent(xterm.MouseButtonWheel, action, m)
		return
	case NoButton:
		return
	}
	action := xterm.MouseActionUp
	if pressed {
		action = xterm.MouseActionDown
		s.held = b
	} else if s.held == b {
		s.held = NoButton
	}
	s.mouseEvent(coreButton(b), action, m)
}

func coreButton(b MouseButton) xterm.CoreMouseButton {
	switch b {
	case MiddleButton:
		return xterm.MouseButtonMiddle
	case RightButton:
		return xterm.MouseButtonRight
	}
	return xterm.MouseButtonLeft
}

func (s *Screen) mouseEvent(b xterm.CoreMouseButton, a xterm.CoreMouseAction, m Modifiers) {
	s.t.TriggerMouseEvent(xterm.CoreMouseEvent{
		Col: s.mouse.X + 1, Row: s.mouse.Y + 1, Button: b, Action: a,
		Ctrl: m&Ctrl != 0, Alt: m&Alt != 0, Shift: m&Shift != 0,
	})
}
