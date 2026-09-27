package view

import (
	"strings"

	"github.com/kvit-s/kvit-term/screen"
	"github.com/richardwilkes/toolbox/v2/geom"
	"github.com/richardwilkes/unison"
	"github.com/richardwilkes/unison/accessibility"
	"github.com/richardwilkes/unison/enums/role"
)

// ProvideAccessibility describes the terminal to a screen reader as a
// read-only text area holding the visible rows, one line per row, with the
// cursor as its caret and the selection. An application names it through
// the panel's Accessibility.Name.
func (v *View) ProvideAccessibility(b *unison.AccessibilityBuilder) {
	n := b.Node()
	n.Role = role.TextArea
	n.ReadOnly = true
	if v.session == nil {
		return
	}
	info := &accessibility.TextInfo{Multiline: v.rows > 1}
	var sb strings.Builder
	offset := 0
	caret := -1
	selFrom, selTo := -1, -1
	from, to := v.anchor, v.head
	if to.Before(from) {
		from, to = to, from
	}
	cw, ch := float32(v.cellW), float32(v.cellH)
	v.session.View(func(scr *screen.Screen) {
		cursor, visible := scr.Cursor(), scr.CursorVisible()
		var l screen.Line
		for i := 0; i < v.rows; i++ {
			row := i - v.offset
			scr.LineInto(row, &l)
			start := offset
			advances := []float32{0}
			n := len(l.Cells)
			for n > 0 && l.Cells[n-1].IsBlank() && !(visible && row == cursor.Y && n-1 <= cursor.X) {
				n--
			}
			for col := 0; col < n; col++ {
				c := l.Cells[col]
				if visible && row == cursor.Y && col == cursor.X && v.offset == 0 {
					caret = offset
				}
				if v.hasSel && row == from.Y && col == from.X {
					selFrom = offset
				}
				if v.hasSel && row == to.Y && col == to.X {
					selTo = offset + len([]rune(c.Text()))
				}
				t := c.Text()
				if t == "" {
					continue
				}
				for range []rune(t) {
					offset++
					advances = append(advances, float32(col+max(1, int(c.Width)))*cw)
				}
				sb.WriteString(t)
			}
			if i < v.rows-1 {
				sb.WriteByte('\n')
				offset++
				advances = append(advances, advances[len(advances)-1])
			}
			info.Lines = append(info.Lines, accessibility.Line{Advances: advances, Start: start, End: offset,
				Bounds: geom.NewRect(0, float32(i)*ch, advances[len(advances)-1], ch)})
		}
	})
	info.Text = sb.String()
	if caret < 0 {
		caret = 0
	}
	info.Caret, info.SelStart, info.SelEnd = caret, caret, caret
	if selFrom >= 0 && selTo > selFrom {
		info.SelStart, info.SelEnd, info.Caret = selFrom, selTo, selTo
	}
	n.Text = info
	n.Value = info.Text
}
