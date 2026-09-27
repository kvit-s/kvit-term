package xterm

// Kvit's test of compacted lines (see KVIT-PATCH.md): the same recorded
// output, with resizes between chunks of it, fed to a terminal whose
// scrollback lines are compacted and to one whose lines are not, must leave
// the two holding the same cells, row for row, after every step.

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

type cellState struct {
	content, fg, bg uint32
	chars           string
	underline       UnderlineStyle
	url             int
}

func bufferState(b *Buffer) (rows []string, cells [][]cellState) {
	var c CellData
	for i := 0; i < b.Lines.Length(); i++ {
		l := b.Lines.Get(i)
		rows = append(rows, fmt.Sprintf("len %d wrapped %v", l.Len, l.IsWrapped))
		var row []cellState
		for x := 0; x < l.Len; x++ {
			l.LoadCell(x, &c)
			row = append(row, cellState{c.Content, c.Fg, c.Bg, c.GetChars(), c.Extended.UnderlineStyle(), c.Extended.URLID()})
		}
		cells = append(cells, row)
	}
	return rows, cells
}

func compareTerminals(t *testing.T, step string, a, b *Terminal) bool {
	t.Helper()
	if a.CursorX() != b.CursorX() || a.CursorY() != b.CursorY() {
		t.Errorf("%s: cursor %d,%d and %d,%d", step, a.CursorX(), a.CursorY(), b.CursorX(), b.CursorY())
		return false
	}
	for _, pair := range [][2]*Buffer{{a.NormalBuffer(), b.NormalBuffer()}, {a.Buffer(), b.Buffer()}} {
		ra, ca := bufferState(pair[0])
		rb, cb := bufferState(pair[1])
		if len(ra) != len(rb) || pair[0].YBase != pair[1].YBase {
			t.Errorf("%s: %d lines at base %d and %d lines at base %d", step, len(ra), pair[0].YBase, len(rb), pair[1].YBase)
			return false
		}
		for i := range ra {
			if ra[i] != rb[i] {
				t.Errorf("%s: line %d is %s and %s", step, i, ra[i], rb[i])
				return false
			}
			for x := range ca[i] {
				if ca[i][x] != cb[i][x] {
					t.Errorf("%s: line %d cell %d is %+v and %+v", step, i, x, ca[i][x], cb[i][x])
					return false
				}
			}
		}
	}
	return true
}

func TestCompactedLinesReadBackAsTheFullLines(t *testing.T) {
	files, _ := filepath.Glob(filepath.Join("..", "..", "screen", "testdata", "vtdiff", "*", "*.bin"))
	if len(files) < 60 {
		t.Fatalf("found %d recordings", len(files))
	}
	sizes := [][2]int{{80, 24}, {50, 20}, {120, 30}, {2, 5}, {200, 12}, {80, 24}}
	compacted, stored, full := 0, 0, 0
	for _, f := range files {
		data, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		compactLines = false
		plain := New(WithCols(80), WithRows(24), WithScrollback(500))
		compactLines = true
		packed := New(WithCols(80), WithRows(24), WithScrollback(500))
		chunk := max(1, len(data)/len(sizes))
		for i, size := range sizes {
			part := data[min(i*chunk, len(data)):min((i+1)*chunk, len(data))]
			compactLines = false
			plain.Write(part)
			plain.Resize(size[0], size[1])
			compactLines = true
			packed.Write(part)
			packed.Resize(size[0], size[1])
			if !compareTerminals(t, fmt.Sprintf("%s, part %d at %dx%d", filepath.Base(f), i, size[0], size[1]), plain, packed) {
				break
			}
		}
		b := packed.NormalBuffer()
		for i := 0; i < b.YBase; i++ {
			l := b.Lines.Get(i)
			stored += l.StoredCells()
			full += l.Len
			if l.StoredCells() < l.Len {
				compacted++
			}
		}
	}
	compactLines = true
	if compacted == 0 || stored*2 > full {
		t.Errorf("the scrollback stores %d of %d cells, %d lines compacted", stored, full, compacted)
	}
	t.Logf("the scrollbacks store %d of %d cells", stored, full)
}
