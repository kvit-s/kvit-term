package screen

import (
	"fmt"
	"html"
	"strings"
)

// PlainText is rows first to last as text, with wrapped lines joined. Rows
// are numbered as in Screen, so -s.ScrollbackCount() to s.Rows()-1 is
// everything the screen holds.
func PlainText(s *Screen, first, last int) string { return s.Text(first, last) }

// HTML is rows first to last as a <pre> block with one <span> per run of
// identical styling. The colours are written inline, so it can be handed to
// any rich-text view without a stylesheet: this is how an application shows
// a build log with its colours and its carriage-return redraws applied
// without having a terminal in its interface.
func HTML(s *Screen, first, last int, p Palette) string {
	var out strings.Builder
	fmt.Fprintf(&out, `<pre style="color:%s;background-color:%s;white-space:pre-wrap">`, Hex(p.Foreground), Hex(p.Background))
	var cur, below Line
	for row := first; row <= last; row++ {
		if row == first {
			s.LineInto(row, &cur)
		}
		// Whether this row's last blanks belong to the window or to a line
		// running past it is the row below's to say.
		if row < last {
			s.LineInto(row+1, &below)
		} else {
			below = Line{}
		}
		if row != first && !cur.Continuation {
			out.WriteByte('\n')
		}
		for col := 0; col < len(cur.Cells); {
			st := cur.Cells[col].Style
			end := col
			var text strings.Builder
			for end < len(cur.Cells) && cur.Cells[end].Style == st {
				text.WriteString(cur.Cells[end].Text())
				end++
			}
			t := text.String()
			if end == len(cur.Cells) && !below.Continuation {
				t = strings.TrimRight(t, " ")
			}
			if t != "" {
				fmt.Fprintf(&out, `<span style="%s">%s</span>`, styleAttribute(st, &p), escape(t))
			}
			col = end
		}
		cur, below = below, cur
	}
	out.WriteString("</pre>")
	return out.String()
}

// escape is HTML escaping of the three characters that matter inside <pre>.
func escape(s string) string {
	if !strings.ContainsAny(s, "&<>") {
		return s
	}
	return html.EscapeString(s)
}

func styleAttribute(st Style, p *Palette) string {
	// Reverse video swaps the two colours rather than setting a third,
	// which is how a terminal draws a selected or highlighted region.
	fg, bg := p.Resolve(st.Foreground, false), p.Resolve(st.Background, true)
	if st.Reverse {
		fg, bg = bg, fg
	}
	if st.Conceal {
		fg = bg
	}
	parts := []string{"color:" + Hex(fg)}
	if bg != p.Background {
		parts = append(parts, "background-color:"+Hex(bg))
	}
	if st.Bold {
		parts = append(parts, "font-weight:bold")
	}
	if st.Italic {
		parts = append(parts, "font-style:italic")
	}
	switch {
	case st.Underline != NoUnderline && st.Strike:
		parts = append(parts, "text-decoration:underline line-through")
	case st.Underline != NoUnderline:
		parts = append(parts, "text-decoration:underline")
	case st.Strike:
		parts = append(parts, "text-decoration:line-through")
	}
	return strings.Join(parts, ";")
}
