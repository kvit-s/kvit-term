package screen

import (
	"fmt"
	"image/color"
)

// Palette turns the colours a program names into colours that can be
// drawn.
//
// A program names a colour in three ways: one of sixteen names that every
// terminal renders differently, an index into a 256-entry table, or exact
// red, green and blue values. Only the last is unambiguous; this is where the
// other two are resolved, which is why changing a terminal's colour scheme
// recolours what is already on the screen without the program redrawing.
type Palette struct {
	Background          color.NRGBA
	Foreground          color.NRGBA
	Cursor              color.NRGBA
	CursorText          color.NRGBA
	SelectionBackground color.NRGBA
	// SelectionForeground is the text colour of selected cells; a zero
	// colour keeps each cell's own.
	SelectionForeground color.NRGBA
	// ANSI holds the sixteen named colours in the order every terminal
	// numbers them: black, red, green, yellow, blue, magenta, cyan, white,
	// then the same eight again as the bright variants.
	ANSI [16]color.NRGBA
}

// Hex is a colour written the way a stylesheet or a settings file writes
// it.
func Hex(c color.NRGBA) string { return fmt.Sprintf("#%02x%02x%02x", c.R, c.G, c.B) }

// ParseHex reads "#rrggbb"; anything else is black.
func ParseHex(s string) color.NRGBA {
	var r, g, b uint8
	if len(s) == 7 && s[0] == '#' {
		fmt.Sscanf(s[1:], "%02x%02x%02x", &r, &g, &b)
	}
	return color.NRGBA{R: r, G: g, B: b, A: 255}
}

// DefaultPalette is the scheme a terminal has before an application sets
// one: a dark ground with the sixteen colours tuned to it.
func DefaultPalette() Palette {
	p := Palette{
		Background:          ParseHex("#12141a"),
		Foreground:          ParseHex("#d5d8de"),
		Cursor:              ParseHex("#d5d8de"),
		CursorText:          ParseHex("#12141a"),
		SelectionBackground: ParseHex("#2f4f7f"),
	}
	for i, h := range []string{
		"#22242c", "#e05561", "#8cc265", "#d18f52", "#4aa5f0", "#c162de", "#42b3c2", "#c7ccd6",
		"#4d4f57", "#ff616e", "#a5e075", "#f0a45d", "#4dc4ff", "#de73ff", "#4cd1e0", "#e6e6e6",
	} {
		p.ANSI[i] = ParseHex(h)
	}
	return p
}

// Resolve is the colour to draw. asBackground decides what the default
// colour means, and nothing else.
func (p *Palette) Resolve(c Color, asBackground bool) color.NRGBA {
	switch c.Kind {
	case Indexed:
		return p.Indexed(int(c.Index))
	case RGB:
		return color.NRGBA{R: c.R, G: c.G, B: c.B, A: 255}
	}
	if asBackground {
		return p.Background
	}
	return p.Foreground
}

// cubeSteps are the levels of the 6×6×6 colour cube. They are not evenly
// spaced: the first is black and the rest run from 95 to 255, which is what
// every terminal has done since xterm chose it.
var cubeSteps = [6]uint8{0, 95, 135, 175, 215, 255}

// Indexed is one entry of the 256-colour table: 0–15 are the named colours,
// 16–231 a 6×6×6 colour cube, and 232–255 a 24-step grey ramp.
func (p *Palette) Indexed(i int) color.NRGBA {
	switch {
	case i < 0 || i > 255:
		return p.Foreground
	case i < 16:
		return p.ANSI[i]
	case i < 232:
		o := i - 16
		return color.NRGBA{R: cubeSteps[(o/36)%6], G: cubeSteps[(o/6)%6], B: cubeSteps[o%6], A: 255}
	}
	level := uint8(8 + (i-232)*10)
	return color.NRGBA{R: level, G: level, B: level, A: 255}
}
