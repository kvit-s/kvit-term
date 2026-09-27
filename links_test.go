package kvitterm

// What in a line of output is worth clicking, and what is not. The second
// half matters more than the first: a detector that turns half of ordinary
// prose into links is worse than none. These are the Qt library's tests
// (tests/unit/test_links.cpp), case for case.

import (
	"testing"

	"github.com/kvit-s/kvit-term/screen"
)

func TestWebAddressesAreFound(t *testing.T) {
	l := FindLinks("see https://example.invalid/a/b?c=1 now")
	if len(l) != 1 || l[0].Kind != URL || l[0].Text != "https://example.invalid/a/b?c=1" || l[0].Column != 4 {
		t.Errorf("links %+v", l)
	}
}

func TestPunctuationAfterAnAddressIsNotPartOfIt(t *testing.T) {
	l := FindLinks("read https://example.invalid/page.")
	if len(l) != 1 || l[0].Text != "https://example.invalid/page" {
		t.Errorf("links %+v", l)
	}
}

func TestAClosingBracketThatBelongsToTheAddressIsKept(t *testing.T) {
	l := FindLinks("https://example.invalid/wiki/Terminal_(disambiguation)")
	if len(l) != 1 || l[0].Text[len(l[0].Text)-1] != ')' {
		t.Errorf("links %+v", l)
	}
}

func TestAPathWithALineNumberIsFound(t *testing.T) {
	// The shape every compiler and test runner prints, which is the reason
	// for detecting paths at all.
	l := FindLinks("src/core/screen.cpp:42:7: error: no such thing")
	if len(l) == 0 {
		t.Fatal("no link")
	}
	if l[0].Kind != Path || l[0].Text != "src/core/screen.cpp:42:7" || l[0].Line != 42 || l[0].Character != 7 {
		t.Errorf("link %+v", l[0])
	}
}

func TestPathsInSeveralShapes(t *testing.T) {
	for _, text := range []string{"./build/log.txt", "~/notes/todo.md", "/usr/share/doc/README"} {
		if l := FindLinks(text); len(l) != 1 {
			t.Errorf("%q: %+v", text, l)
		}
	}
	if l := FindLinks("tests/unit/test_links.cpp:9"); len(l) != 1 || l[0].Line != 9 {
		t.Errorf("line number: %+v", l)
	}
}

func TestAPathInsideAnAddressIsNotReportedTwice(t *testing.T) {
	if l := FindLinks("https://example.invalid/a/b/c.txt"); len(l) != 1 || l[0].Kind != URL {
		t.Errorf("links %+v", l)
	}
}

func TestOrdinaryProseIsLeftAlone(t *testing.T) {
	for _, text := range []string{
		"this line has no links in it at all",
		"total 0",
		"100% tests passed, 0 tests failed",
		"Compiling 12/34",
		"a~/x",
	} {
		if l := FindLinks(text); len(l) != 0 {
			t.Errorf("%q: %+v", text, l)
		}
	}
}

func TestLinksAreReportedInTheOrderTheyAppear(t *testing.T) {
	l := FindLinks("see src/a.cpp:1 and https://example.invalid/x")
	if len(l) != 2 || l[0].Column >= l[1].Column || l[0].Kind != Path || l[1].Kind != URL {
		t.Errorf("links %+v", l)
	}
}

func TestALineOfCellsCanBeSearchedDirectly(t *testing.T) {
	var line screen.Line
	for _, r := range "日 open /tmp/x.log" {
		line.Cells = append(line.Cells, screen.Cell{Ch: r, Width: 1})
		if r == '日' {
			line.Cells[len(line.Cells)-1].Width = 2
			line.Cells = append(line.Cells, screen.Cell{})
		}
	}
	l := FindLinksInLine(line)
	if len(l) != 1 || l[0].Text != "/tmp/x.log" || l[0].Column != 8 {
		t.Errorf("links %+v", l)
	}
}
