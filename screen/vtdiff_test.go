package screen

// The regression test the migration plan asks for: every recorded stream in
// testdata/vtdiff fed through the Go screen and compared, cell by cell, with
// what libvterm 0.3.3 — the emulator of the kvit-term — made of the same
// bytes. The libvterm screens were recorded with tools/vtdiff/vtdump.c
// while libvterm's source was still at hand; tools/vtdiff/README.md says how
// to record them again.
//
// Streams that differ are listed in knownDifferences with the number of
// cells that differ and why. Any other change, in either direction, fails:
// a stream starting to differ is a regression, and one that stops differing
// should have its entry removed.

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"testing"
)

const vtRows, vtCols = 24, 80

// vtDump is a screen in the form vtdump.c prints: the cursor, each row's
// text with ¤ for the right half of a wide character, and the style of every
// cell that is not in the default one.
type vtDump struct {
	cursorRow, cursorCol int
	rows                 [vtRows]string
	styles               map[[2]int][3]string // fg, bg, attributes
}

func readDump(path string) (*vtDump, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	d := &vtDump{styles: map[[2]int][3]string{}}
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 1<<20), 1<<20)
	for sc.Scan() {
		line := sc.Text()
		switch {
		case strings.HasPrefix(line, "cursor "):
			fmt.Sscanf(line, "cursor %d %d", &d.cursorRow, &d.cursorCol)
		case strings.HasPrefix(line, "R"):
			n, text, _ := strings.Cut(line[1:], " ")
			r, _ := strconv.Atoi(n)
			if r >= 0 && r < vtRows {
				d.rows[r] = text
			}
		case strings.HasPrefix(line, "S"):
			parts := strings.Fields(line[1:])
			if len(parts) != 4 {
				continue
			}
			var r, c int
			fmt.Sscanf(parts[0], "%d,%d", &r, &c)
			attrs := parts[3]
			if attrs == "-" {
				attrs = ""
			}
			d.styles[[2]int{r, c}] = [3]string{parts[1], parts[2], attrs}
		}
	}
	return d, sc.Err()
}

func dumpColour(c Color, def string) string {
	switch c.Kind {
	case Indexed:
		return "i" + strconv.Itoa(int(c.Index))
	case RGB:
		return fmt.Sprintf("#%02x%02x%02x", c.R, c.G, c.B)
	}
	return def
}

// dumpScreen feeds a stream to a new screen and describes the result the
// way vtdump.c describes libvterm's.
func dumpScreen(data []byte) *vtDump {
	s := New(vtCols, vtRows)
	s.Feed(data)
	d := &vtDump{styles: map[[2]int][3]string{}}
	c := s.Cursor()
	d.cursorRow, d.cursorCol = c.Y, c.X
	for y := 0; y < vtRows; y++ {
		l := s.Line(y)
		var b strings.Builder
		keep := 0
		for x, cell := range l.Cells {
			if cell.Width == 0 && cell.Ch == 0 {
				b.WriteString("¤")
				keep = b.Len()
				continue
			}
			text := cell.Text()
			b.WriteString(text)
			if text != " " {
				keep = b.Len()
			}
			var a strings.Builder
			st := cell.Style
			if st.Bold {
				a.WriteByte('b')
			}
			if st.Italic {
				a.WriteByte('i')
			}
			if st.Underline != NoUnderline {
				a.WriteByte('u')
			}
			if st.Reverse {
				a.WriteByte('r')
			}
			if st.Strike {
				a.WriteByte('s')
			}
			fg, bg := dumpColour(st.Foreground, "d"), dumpColour(st.Background, "D")
			if fg != "d" || bg != "D" || a.Len() > 0 {
				d.styles[[2]int{y, x}] = [3]string{fg, bg, a.String()}
			}
		}
		d.rows[y] = b.String()[:keep]
	}
	return d
}

// visible is a style as it would be drawn: reverse video swaps the colours
// and bold brightens the eight basic ones, so two emulators that store the
// same look differently still compare equal.
func visible(st [3]string) [3]string {
	fg, bg, at := st[0], st[1], st[2]
	if strings.Contains(at, "r") {
		fg, bg = bg, fg
		at = strings.ReplaceAll(at, "r", "")
	}
	if strings.Contains(at, "b") && strings.HasPrefix(fg, "i") {
		if n, _ := strconv.Atoi(fg[1:]); n < 8 {
			fg = "i" + strconv.Itoa(n+8)
		}
	}
	chars := strings.Split(at, "")
	sort.Strings(chars)
	return [3]string{fg, bg, strings.Join(chars, "")}
}

func cellsOf(text string) []string {
	var out []string
	for _, r := range text {
		out = append(out, string(r))
	}
	for len(out) < vtCols {
		out = append(out, " ")
	}
	return out[:vtCols]
}

type vtResult struct{ text, style, cursor int }

func (r vtResult) String() string {
	cursor := "cursor ok"
	if r.cursor != 0 {
		cursor = "cursor differs"
	}
	return fmt.Sprintf("%d text / %d style / %s", r.text, r.style, cursor)
}

// compareDumps counts the cells whose character differs, the cells whose
// visible style differs, and whether the cursor differs. A blank cell is
// compared only on what shows: its background, and any line under or
// through it.
func compareDumps(got, want *vtDump) vtResult {
	var r vtResult
	for y := 0; y < vtRows; y++ {
		g, w := cellsOf(got.rows[y]), cellsOf(want.rows[y])
		for x := 0; x < vtCols; x++ {
			if g[x] != w[x] {
				r.text++
			}
			a := visible(orDefault(want.styles, y, x))
			b := visible(orDefault(got.styles, y, x))
			if g[x] == " " && w[x] == " " {
				a = [3]string{a[1], strconv.FormatBool(strings.Contains(a[2], "u")), strconv.FormatBool(strings.Contains(a[2], "s"))}
				b = [3]string{b[1], strconv.FormatBool(strings.Contains(b[2], "u")), strconv.FormatBool(strings.Contains(b[2], "s"))}
			}
			if a != b {
				r.style++
			}
		}
	}
	if got.cursorRow != want.cursorRow || got.cursorCol != want.cursorCol {
		r.cursor = 1
	}
	return r
}

func orDefault(m map[[2]int][3]string, y, x int) [3]string {
	if st, ok := m[[2]int{y, x}]; ok {
		return st
	}
	return [3]string{"d", "D", ""}
}

// knownDifferences are the streams where the Go screen and libvterm
// disagree, with the size of the disagreement and whose behaviour it is.
var knownDifferences = map[string]struct {
	result vtResult
	why    string
}{
	// Where the Go screen does what xterm does and libvterm does not.
	"micro-sgr-colon": {vtResult{0, 2, 0},
		"a colour written with colons, 38:2::10:20:30, is red 10, green 20, blue 30; libvterm reads #ff0a14"},
	"micro-wide-overwrite": {vtResult{1, 0, 0},
		"typing over half of a wide character clears all of it; libvterm leaves the other half"},
	"micro-bs-over-wide": {vtResult{1, 0, 0},
		"the same, after moving back into the character with a backspace"},
	"micro-alt-screen-47": {vtResult{4, 0, 0},
		"mode 47 switches to the alternate screen; libvterm acts only on 1047 and 1049"},
	"micro-save-restore-csi": {vtResult{16, 0, 1},
		"CSI s and CSI u save and restore the cursor; libvterm ignores them"},
	// One cell of a real program.
	"streams-top": {vtResult{0, 1, 0},
		"the last cell of top's reversed column-header row stays reversed; libvterm clears it"},
	// 3,000 random operations each: once the two emulators diverge, every
	// later operation lands differently, so the counts say nothing about a
	// cause (tools/vtdiff/README.md). They are kept to notice a change.
	"streams-synth-rand1": {vtResult{5, 240, 0}, "random operations"},
	"streams-synth-rand2": {vtResult{0, 1920, 0}, "random operations"},
	"streams-synth-rand3": {vtResult{247, 1218, 0}, "random operations"},
	"streams-synth-rand4": {vtResult{5, 5, 0}, "random operations"},
	"streams-synth-rand5": {vtResult{64, 1898, 0}, "random operations"},
}

func TestRecordedStreamsMatchLibvterm(t *testing.T) {
	var names []string
	for _, dir := range []string{"streams", "micro"} {
		files, _ := filepath.Glob(filepath.Join("testdata", "vtdiff", dir, "*.bin"))
		for _, f := range files {
			names = append(names, dir+"-"+strings.TrimSuffix(filepath.Base(f), ".bin"))
		}
	}
	if len(names) < 70 {
		t.Fatalf("found only %d streams", len(names))
	}
	matched := 0
	for _, name := range names {
		dir, base, _ := strings.Cut(name, "-")
		data, err := os.ReadFile(filepath.Join("testdata", "vtdiff", dir, base+".bin"))
		if err != nil {
			t.Fatal(err)
		}
		want, err := readDump(filepath.Join("testdata", "vtdiff", "libvterm", name+".txt"))
		if err != nil {
			t.Fatal(err)
		}
		got := compareDumps(dumpScreen(data), want)
		known, isKnown := knownDifferences[name]
		switch {
		case got == (vtResult{}) && !isKnown:
			matched++
		case got == (vtResult{}):
			t.Errorf("%s now matches libvterm; remove it from knownDifferences", name)
		case !isKnown:
			t.Errorf("%s differs from libvterm: %v", name, got)
		case got != known.result:
			t.Errorf("%s differs from libvterm by %v, where %v was recorded (%s)", name, got, known.result, known.why)
		}
	}
	t.Logf("%d of %d streams match libvterm; %d differ as recorded", matched, len(names), len(knownDifferences))
}
