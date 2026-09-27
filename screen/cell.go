// Package screen is the terminal emulator: a byte stream from a program in,
// a grid of cells out, and the bytes a key press or a mouse click becomes on
// the way back.
//
// The escape sequences are interpreted by xterm-go, a Go port of xterm.js
// kept as a copy in third_party/xterm. This package holds what an
// application needs on top of it: the cells in a form that says what is
// drawn and how, the scrollback numbered from the visible screen, the key
// encoder, the modes a view has to know about, and the screen's contents
// as plain or styled text. It has no child process, no window and no
// drawing code, so all of its behaviour can be proved by feeding it
// recorded bytes and reading the grid back.
package screen

import "strings"

// ColorKind is one of the three ways a program names a colour.
type ColorKind uint8

// The kinds of colour. Default means the program asked for no colour, so
// the palette's own foreground or background applies; that is what lets a
// colour scheme change without the program redrawing.
const (
	Default ColorKind = iota
	Indexed
	RGB
)

// Color is a colour as the program named it: the default, an index into the
// 256-colour table, or exact red, green and blue values. It is resolved to
// pixels only when drawn; see Palette.
type Color struct {
	Kind    ColorKind
	Index   uint8 // 0–255, when Kind is Indexed
	R, G, B uint8 // when Kind is RGB
}

// Underline is how a cell is underlined.
type Underline uint8

// The underline styles a program can ask for.
const (
	NoUnderline Underline = iota
	SingleUnderline
	DoubleUnderline
	CurlyUnderline
	DottedUnderline
	DashedUnderline
)

// Style is how a cell is drawn, apart from which character it holds. Two
// cells with equal styles can be drawn in one operation.
type Style struct {
	Foreground Color
	Background Color
	Underline  Underline
	Bold       bool
	Dim        bool
	Italic     bool
	Blink      bool
	Reverse    bool
	Conceal    bool
	Strike     bool
	Overline   bool
}

// Cell is one character position on the screen.
//
// A character can take two cells: East Asian ideographs and most emoji are
// drawn double width, so they sit in a cell whose Width is 2, and the cell
// to their right is a placeholder with Ch 0 and Width 0.
//
// A cell can hold more than one code point. A base letter followed by
// combining marks is one character position made of several code points,
// so the rare cell that has them keeps the whole sequence in Extra and the
// base in Ch.
type Cell struct {
	Ch    rune  // 0 marks the right half of a double-width character
	Width uint8 // 1 or 2; the right half reports 0
	Style Style
	Extra string // the whole sequence, when the cell holds combining marks
}

// Blank is an empty cell in the default style: what lies past the end of
// anything a program wrote.
var Blank = Cell{Ch: ' ', Width: 1}

// Text is what the cell shows, empty for the right half of a double-width
// character.
func (c Cell) Text() string {
	if c.Extra != "" {
		return c.Extra
	}
	if c.Ch == 0 {
		return ""
	}
	return string(c.Ch)
}

// IsBlank reports a cell that shows no character.
func (c Cell) IsBlank() bool { return c.Extra == "" && (c.Ch == ' ' || c.Ch == 0) }

// TrailingBlanks says what Line.TextRange does with the blanks at the end of
// what it was asked for.
//
// Normally they are the width of the window rather than anything the
// program wrote, so reading a line back drops them. On a row that a longer
// line wrapped through they are spaces inside that line, and a caller
// joining the rows of such a line back together asks to keep them:
// dropping them closes up the words on either side of the break.
type TrailingBlanks uint8

// The two choices for trailing blanks.
const (
	DropBlanks TrailingBlanks = iota
	KeepBlanks
)

// Line is one line of the screen or of the scrollback.
//
// Cells can be shorter than the terminal is wide; anything past the end is
// a blank cell in the default style.
//
// Continuation says the line began as the overflow of the line above rather
// than at a line break of its own. It is what a re-wrap on resize joins
// back together, and what reading text back uses to join a wrapped line.
// It says nothing about the row below, so a caller joining rows decides
// what to do with this one's trailing blanks by looking at the next row.
type Line struct {
	Cells        []Cell
	Continuation bool
}

// CellAt is the cell in a column, or a blank one past the end.
func (l Line) CellAt(column int) Cell {
	if column >= 0 && column < len(l.Cells) {
		return l.Cells[column]
	}
	return Blank
}

// Text is the whole line with its trailing blanks dropped.
func (l Line) Text() string { return l.TextRange(0, -1, DropBlanks) }

// TextRange is the text of columns from to to, inclusive; a negative to, or
// one past the end, runs to the end of the line.
//
// Trailing blanks are the width of the window rather than anything the
// program wrote, so they are dropped, but only when the range reaches the
// end of the line, since spaces in the middle of a selection are the
// user's, and only when trailing is DropBlanks.
func (l Line) TextRange(from, to int, trailing TrailingBlanks) string {
	last := len(l.Cells) - 1
	toEnd := to < 0 || to >= last
	if toEnd {
		to = last
	}
	var b strings.Builder
	for c := max(0, from); c <= to; c++ {
		b.WriteString(l.Cells[c].Text())
	}
	s := b.String()
	if toEnd && trailing == DropBlanks {
		s = strings.TrimRight(s, " ")
	}
	return s
}

// IsBlank reports a line that shows no character.
func (l Line) IsBlank() bool {
	for _, c := range l.Cells {
		if !c.IsBlank() {
			return false
		}
	}
	return true
}

// Point is a position on the screen: X is the column and Y the row, with
// row 0 the top of the visible screen and negative rows the scrollback.
type Point struct{ X, Y int }

// Before reports whether p comes before q in reading order.
func (p Point) Before(q Point) bool { return p.Y < q.Y || (p.Y == q.Y && p.X < q.X) }
