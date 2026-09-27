package view

import (
	"image/color"
	"time"

	"github.com/kvit-s/kvit-term"
	"github.com/kvit-s/kvit-term/screen"
	"github.com/kvit-s/kvit-ui/text"
	"github.com/richardwilkes/toolbox/v2/geom"
	"github.com/richardwilkes/unison"
	"github.com/richardwilkes/unison/enums/paintstyle"
)

const blinkInterval = 600 * time.Millisecond

// frameState is what one frame draws, copied out of the screen while the
// session's lock is held so that drawing does not hold it.
type frameState struct {
	lines         []screen.Line
	cursor        screen.Point
	cursorVisible bool
	shape         screen.CursorShape
	blinks        bool
	matches       []kvitterm.Match
	current       kvitterm.Match
	hasCurrent    bool
}

func unisonColor(c color.NRGBA) unison.Color {
	return unison.ARGB(float32(c.A)/255, int(c.R), int(c.G), int(c.B))
}

func fillRect(gc *unison.Canvas, r geom.Rect, c color.NRGBA) {
	gc.DrawRect(r, unisonColor(c).Paint(gc, r, paintstyle.Fill))
}

func withAlpha(c color.NRGBA, a uint8) color.NRGBA { c.A = a; return c }

// mix is a between b and a: share 0 is a, 1 is b.
func mix(a, b color.NRGBA, share float32) color.NRGBA {
	f := func(x, y uint8) uint8 { return uint8(float32(x) + (float32(y)-float32(x))*share + 0.5) }
	return color.NRGBA{R: f(a.R, b.R), G: f(a.G, b.G), B: f(a.B, b.B), A: 255}
}

func (v *View) draw(gc *unison.Canvas, _ geom.Rect) {
	p := &v.palette
	fillRect(gc, v.ContentRect(false), p.Background)
	if v.session == nil {
		return
	}
	f := &v.frame
	f.matches, f.hasCurrent = nil, false
	if v.search != nil {
		f.matches = v.search.Matches()
		f.current, f.hasCurrent = v.search.CurrentMatch()
	}
	for len(f.lines) < v.rows {
		f.lines = append(f.lines, screen.Line{})
	}
	v.session.View(func(scr *screen.Screen) {
		for i := 0; i < v.rows; i++ {
			scr.LineInto(i-v.offset, &f.lines[i])
		}
		f.cursor = scr.Cursor()
		f.cursorVisible = scr.CursorVisible()
		f.shape = scr.CursorShape()
		f.blinks = scr.CursorBlinks()
	})
	for i := 0; i < v.rows; i++ {
		v.drawRow(gc, i, &f.lines[i])
	}
	v.drawCursor(gc)
	v.session.Drawn()
}

// selected reports whether a cell is in the selection, which runs from one
// point to the other through the ends of lines.
func (v *View) selected(row, col int) bool {
	if !v.hasSel {
		return false
	}
	from, to := v.anchor, v.head
	if to.Before(from) {
		from, to = to, from
	}
	if row < from.Y || row > to.Y {
		return false
	}
	if row == from.Y && col < from.X {
		return false
	}
	return row != to.Y || col <= to.X
}

// colours are a cell's text and ground as drawn: reverse video swaps them,
// dim text is halfway to the ground, and the selection has its own.
func (v *View) colours(c screen.Cell, selected bool) (fg, bg color.NRGBA) {
	p := &v.palette
	fg, bg = p.Resolve(c.Style.Foreground, false), p.Resolve(c.Style.Background, true)
	if c.Style.Reverse {
		fg, bg = bg, fg
	}
	if c.Style.Dim {
		fg = mix(fg, bg, 0.45)
	}
	if selected {
		bg = p.SelectionBackground
		if p.SelectionForeground.A > 0 {
			fg = p.SelectionForeground
		}
	}
	return fg, bg
}

func (v *View) drawRow(gc *unison.Canvas, viewRow int, line *screen.Line) {
	p := &v.palette
	cw, ch := float32(v.cellW), float32(v.cellH)
	top := float32(viewRow) * ch
	row := viewRow - v.offset

	// Grounds first, merged into runs, so a line of one colour is one fill
	// rather than eighty.
	for col := 0; col < v.cols; {
		_, bg := v.colours(line.CellAt(col), v.selected(row, col))
		end := col + 1
		for end < v.cols {
			if _, next := v.colours(line.CellAt(end), v.selected(row, end)); next != bg {
				break
			}
			end++
		}
		if bg != p.Background {
			fillRect(gc, geom.NewRect(float32(col)*cw, top, float32(end-col)*cw, ch), bg)
		}
		col = end
	}

	// Search matches above the grounds and below the text, the current one
	// stronger than the rest.
	for _, m := range v.frame.matches {
		if m.Row != row {
			continue
		}
		c := withAlpha(p.ANSI[3], 120)
		if v.frame.hasCurrent && m == v.frame.current {
			c = withAlpha(p.ANSI[11], 220)
		}
		fillRect(gc, geom.NewRect(float32(m.Column)*cw, top, float32(m.Length)*cw, ch), c)
	}

	// Then the text, in runs of one style and colour. Each character of a
	// run is placed at the start of its own cell, so a font whose advance is
	// not a whole number of pixels cannot drift off the grid. A character
	// taking two cells, or holding combining marks, is placed on its own.
	for col := 0; col < v.cols; {
		c := line.CellAt(col)
		if c.Ch == 0 && c.Extra == "" {
			col++
			continue
		}
		fg, _ := v.colours(c, v.selected(row, col))
		alone := c.Width == 2 || c.Extra != ""
		end := col + 1
		var run []rune
		if !alone {
			run = append(run, c.Ch)
			for end < v.cols {
				n := line.CellAt(end)
				if n.Ch == 0 || n.Width == 2 || n.Extra != "" || n.Style != c.Style {
					break
				}
				if nfg, _ := v.colours(n, v.selected(row, end)); nfg != fg {
					break
				}
				run = append(run, n.Ch)
				end++
			}
		} else if c.Width == 2 {
			end = col + 2
		}
		x := float32(col) * cw
		if !c.Style.Conceal {
			st := v.style(c.Style.Bold, c.Style.Italic, fg)
			if alone {
				l := v.fonts.Layout([]text.Span{{Text: c.Text(), Style: st}}, text.Options{})
				w, _ := l.Size()
				// A wide character is centred in its two cells.
				dx := max(0, (float32(end-col)*cw-w)/2)
				l.Draw(gc, x+dx, top+v.baseline-l.Baseline())
			} else if !blankRun(run) {
				l := v.fonts.Layout([]text.Span{{Text: string(run), Style: st}}, text.Options{Grid: cw})
				l.Draw(gc, x, top+v.baseline-l.Baseline())
			}
		}
		v.decorate(gc, c.Style, fg, x, float32(end)*cw, top)
		col = end
	}

	if v.hasLink && v.linkRow == row {
		fg := p.Foreground
		y := top + v.baseline + 2
		line := unisonColor(fg).Paint(gc, geom.Rect{}, paintstyle.Stroke)
		gc.DrawLine(geom.NewPoint(float32(v.link.Column)*cw, y+0.5), geom.NewPoint(float32(v.link.Column+v.link.Length)*cw, y+0.5), line)
	}
}

func blankRun(r []rune) bool {
	for _, c := range r {
		if c != ' ' {
			return false
		}
	}
	return true
}

// decorate draws the lines under, through and over a run of cells.
func (v *View) decorate(gc *unison.Canvas, st screen.Style, fg color.NRGBA, x0, x1, top float32) {
	if st.Underline == screen.NoUnderline && !st.Strike && !st.Overline {
		return
	}
	paint := unisonColor(fg).Paint(gc, geom.Rect{}, paintstyle.Stroke)
	paint.SetStrokeWidth(1)
	line := func(y float32) { gc.DrawLine(geom.NewPoint(x0, y+0.5), geom.NewPoint(x1, y+0.5), paint) }
	under := top + float32(int(v.baseline)) + 2
	switch st.Underline {
	case screen.SingleUnderline:
		line(under)
	case screen.DoubleUnderline:
		line(under)
		line(under + 2)
	case screen.CurlyUnderline:
		path := unison.NewPath()
		path.MoveTo(geom.NewPoint(x0, under+1))
		up := true
		for x := x0; x < x1; x += 2 {
			y := under + 2
			if up {
				y = under
			}
			path.LineTo(geom.NewPoint(min(x+2, x1), y+0.5))
			up = !up
		}
		gc.DrawPath(path, paint)
	case screen.DottedUnderline, screen.DashedUnderline:
		step, on := float32(2), float32(1)
		if st.Underline == screen.DashedUnderline {
			step, on = 6, 4
		}
		for x := x0; x < x1; x += step {
			gc.DrawLine(geom.NewPoint(x, under+0.5), geom.NewPoint(min(x+on, x1), under+0.5), paint)
		}
	}
	if st.Strike {
		line(top + float32(int(v.baseline*0.65)))
	}
	if st.Overline {
		line(top)
	}
}

// drawCursor draws the cursor, last, and only where it is: scrolled back
// through the history there is nothing to point at. A view without the
// keyboard shows an outline, so it is plain which terminal keystrokes go
// to.
func (v *View) drawCursor(gc *unison.Canvas) {
	f := &v.frame
	if !f.cursorVisible || v.offset != 0 {
		return
	}
	if v.focused && f.blinks && !v.blinkOn {
		return
	}
	p := &v.palette
	cw, ch := float32(v.cellW), float32(v.cellH)
	c := v.frame.lines[f.cursor.Y].CellAt(f.cursor.X)
	w := cw
	if c.Width == 2 {
		w *= 2
	}
	box := geom.NewRect(float32(f.cursor.X)*cw, float32(f.cursor.Y)*ch, w, ch)
	if !v.focused {
		paint := unisonColor(p.Cursor).Paint(gc, box, paintstyle.Stroke)
		paint.SetStrokeWidth(1)
		gc.DrawRect(box.Inset(geom.NewUniformInsets(0.5)), paint)
		return
	}
	switch f.shape {
	case screen.UnderlineCursor:
		fillRect(gc, geom.NewRect(box.X, box.Bottom()-2, box.Width, 2), p.Cursor)
	case screen.BarCursor:
		fillRect(gc, geom.NewRect(box.X, box.Y, 2, box.Height), p.Cursor)
	default:
		fillRect(gc, box, p.Cursor)
		if t := c.Text(); t != "" && t != " " {
			l := v.fonts.Layout([]text.Span{{Text: t, Style: v.style(c.Style.Bold, c.Style.Italic, p.CursorText)}}, text.Options{Grid: cw})
			l.Draw(gc, box.X, box.Y+v.baseline-l.Baseline())
		}
	}
}
