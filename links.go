package kvitterm

import (
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/kvit-s/kvit-term/screen"
)

// LinkKind is what a link points at.
type LinkKind uint8

// The kinds of link found in output.
const (
	URL LinkKind = iota
	Path
)

// Link is something in output worth clicking: a web address, or a file path
// with an optional line and column after it, which is the shape every
// compiler and test runner prints when something goes wrong. Finding them
// is the terminal's job; what to do with one is the application's, since a
// terminal library has no business choosing an editor.
type Link struct {
	Kind LinkKind
	Text string // exactly the characters that matched
	// Column and Length place it: in characters for FindLinks, in cells
	// for FindLinksInLine.
	Column, Length int
	Line           int // the line number after a path, or -1
	Character      int // the column number after that, or -1
}

// urlPattern is a web address, ended by whitespace or by a character that
// commonly surrounds one rather than belonging to it.
var urlPattern = regexp.MustCompile("(?:https?|ftp|file)://[^\\s<>\"'`\\[\\]{}|\\\\^]+")

// pathPattern is a path with at least one separator in it, optionally
// followed by a line and a column: src/core/screen.cpp:42:7,
// ./build/log.txt, ~/notes/todo.md. A bare word is not a path, since most
// words in output are not. A path does not start just after a word
// character, a slash, a tilde or a dot, which pathStart checks.
var pathPattern = regexp.MustCompile(`((?:~|\.{1,2})?(?:/[\w.+-]+)+/?|(?:[\w.+-]+/)+[\w.+-]+)(?::(\d+))?(?::(\d+))?`)

func pathStart(text string, at int) bool {
	if at == 0 {
		return true
	}
	c := text[at-1]
	return !(c == '_' || c == '/' || c == '~' || c == '.' ||
		(c >= '0' && c <= '9') || (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z'))
}

// trimTrailing drops punctuation that is nearly always the sentence's
// rather than the link's, keeping a closing bracket that has an opening one
// inside the link: en.wikipedia.org/wiki/Terminal_(disambiguation).
func trimTrailing(s string) string {
	for s != "" && strings.ContainsRune(".,;:!?'\")]}", rune(s[len(s)-1])) {
		if s[len(s)-1] == ')' && strings.Contains(s, "(") {
			break
		}
		s = s[:len(s)-1]
	}
	return s
}

// FindLinks finds the links in one line of text, in order. A line that was
// wrapped from the one above should be joined to it first, or an address
// split across the wrap is missed.
func FindLinks(text string) []Link {
	var links []Link
	var claimed [][2]int // byte ranges a web address covers
	for _, loc := range urlPattern.FindAllStringIndex(text, -1) {
		t := trimTrailing(text[loc[0]:loc[1]])
		links = append(links, Link{Kind: URL, Text: t, Column: loc[0], Length: len(t), Line: -1, Character: -1})
		claimed = append(claimed, [2]int{loc[0], loc[0] + len(t)})
	}
	for from := 0; from < len(text); {
		m := pathPattern.FindStringSubmatchIndex(text[from:])
		if m == nil {
			break
		}
		for i := range m {
			if m[i] >= 0 {
				m[i] += from
			}
		}
		start, end := m[0], m[1]
		if !pathStart(text, start) {
			// Try again one character on, as a pattern with a look-behind
			// would.
			from = start + 1
			continue
		}
		from = end
		path := text[m[2]:m[3]]
		// A run of numbers separated by slashes is a count rather than a
		// path: "Compiling 12/34", "3/10 tests passed".
		numeric := true
		segments := 0
		for _, seg := range strings.Split(path, "/") {
			if seg == "" {
				continue
			}
			segments++
			if _, err := strconv.ParseInt(seg, 10, 64); err != nil {
				numeric = false
			}
		}
		if numeric || segments == 0 {
			continue
		}
		inside := false
		for _, r := range claimed {
			if start >= r[0] && start < r[1] {
				inside = true
			}
		}
		if inside {
			continue
		}
		l := Link{Kind: Path, Text: text[start:end], Column: start, Length: end - start, Line: -1, Character: -1}
		if m[4] >= 0 {
			l.Line, _ = strconv.Atoi(text[m[4]:m[5]])
		}
		if m[6] >= 0 {
			l.Character, _ = strconv.Atoi(text[m[6]:m[7]])
		}
		links = append(links, l)
	}
	sort.SliceStable(links, func(i, j int) bool { return links[i].Column < links[j].Column })
	// Byte offsets to characters.
	for i := range links {
		links[i].Column = len([]rune(text[:links[i].Column]))
		links[i].Length = len([]rune(links[i].Text))
	}
	return links
}

// FindLinksInLine finds the links in a line of cells, placed in cells.
func FindLinksInLine(l screen.Line) []Link {
	text, starts, ends := lineRunes(&l)
	links := FindLinks(string(text))
	for i := range links {
		k := &links[i]
		from, to := k.Column, k.Column+k.Length
		k.Column, k.Length = starts[from], ends[to-1]-starts[from]
	}
	return links
}
