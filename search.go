package kvitterm

import (
	"regexp"
	"slices"
	"sync/atomic"
	"unicode"

	"github.com/kvit-s/kvit-term/screen"
)

// Match is one place a search found, in cells: Row is numbered as the
// screen numbers rows, so negative rows are the scrollback.
type Match struct {
	Row, Column, Length int
}

// Search finds text in what a terminal has shown, the screen and the
// scrollback together.
//
// Matches are found when asked for rather than kept up to date as output
// arrives: a search runs when somebody asks, and output arrives thousands
// of times more often than that. New output makes the held matches stale,
// and Refresh, or the next question about them, finds them again.
//
// A Search is used from one goroutine, the application's.
type Search struct {
	s             *Session
	stop          func()
	query         string
	caseSensitive bool
	regex         bool
	stale         atomic.Bool
	matches       []Match
	current       int
	// OnChange is called after the matches or the current one changed.
	OnChange func()
}

// NewSearch searches a session.
func NewSearch(s *Session) *Search {
	se := &Search{s: s, current: -1}
	se.stale.Store(true)
	se.stop = s.Observe(func(e Event) {
		if e.Kind == ContentChanged {
			se.stale.Store(true)
		}
	})
	return se
}

// Close stops following the session.
func (se *Search) Close() { se.stop() }

// Query is what is searched for.
func (se *Search) Query() string { return se.query }

// SetQuery searches for something else, from no current match.
func (se *Search) SetQuery(q string) {
	if q == se.query {
		return
	}
	se.query = q
	se.current = -1
	se.Refresh()
}

// CaseSensitive reports whether case must match; by default it need not.
func (se *Search) CaseSensitive() bool { return se.caseSensitive }

// SetCaseSensitive changes whether case must match.
func (se *Search) SetCaseSensitive(on bool) {
	if on != se.caseSensitive {
		se.caseSensitive = on
		se.Refresh()
	}
}

// RegularExpression reports whether the query is a regular expression (Go's
// syntax) rather than plain text.
func (se *Search) RegularExpression() bool { return se.regex }

// SetRegularExpression changes it. A query that is not a valid expression
// finds nothing rather than failing.
func (se *Search) SetRegularExpression(on bool) {
	if on != se.regex {
		se.regex = on
		se.Refresh()
	}
}

// Matches are every match, oldest first, so "next" moves the way reading
// does.
func (se *Search) Matches() []Match {
	if se.stale.Load() {
		se.scan()
	}
	return se.matches
}

// MatchCount is how many matches there are.
func (se *Search) MatchCount() int { return len(se.Matches()) }

// MatchesOnRow are the matches on one row.
func (se *Search) MatchesOnRow(row int) []Match {
	var out []Match
	for _, m := range se.Matches() {
		if m.Row == row {
			out = append(out, m)
		}
	}
	return out
}

// Current is the index of the current match, or -1.
func (se *Search) Current() int { return se.current }

// CurrentMatch is the current match, and whether there is one.
func (se *Search) CurrentMatch() (Match, bool) {
	m := se.Matches()
	if se.current < 0 || se.current >= len(m) {
		return Match{}, false
	}
	return m[se.current], true
}

// SetCurrent makes a match current.
func (se *Search) SetCurrent(i int) {
	n := se.MatchCount()
	if n == 0 {
		i = -1
	} else {
		i = min(max(i, 0), n-1)
	}
	if i != se.current {
		se.current = i
		se.changed()
	}
}

// Next moves to the next match, wrapping at the end. The first Next goes to
// the most recent match, the one nearest what the user is looking at.
func (se *Search) Next() {
	n := se.MatchCount()
	if n == 0 {
		return
	}
	if se.current < 0 {
		se.SetCurrent(n - 1)
	} else {
		se.SetCurrent((se.current + 1) % n)
	}
}

// Previous moves to the previous match, wrapping at the start.
func (se *Search) Previous() {
	n := se.MatchCount()
	if n == 0 {
		return
	}
	if se.current <= 0 {
		se.SetCurrent(n - 1)
	} else {
		se.SetCurrent(se.current - 1)
	}
}

// Clear searches for nothing.
func (se *Search) Clear() { se.SetQuery("") }

// Refresh finds the matches again, after new output.
func (se *Search) Refresh() {
	se.scan()
	if se.current >= len(se.matches) {
		se.current = len(se.matches) - 1
	}
	se.changed()
}

func (se *Search) changed() {
	if se.OnChange != nil {
		se.OnChange()
	}
}

func (se *Search) scan() {
	se.stale.Store(false)
	se.matches = nil
	if se.query == "" {
		return
	}
	var re *regexp.Regexp
	if se.regex {
		pattern := se.query
		if !se.caseSensitive {
			pattern = "(?i)" + pattern
		}
		var err error
		if re, err = regexp.Compile(pattern); err != nil {
			return
		}
	}
	query := []rune(se.query)
	if !se.caseSensitive {
		query = lowerRunes(query)
	}
	se.s.View(func(scr *screen.Screen) {
		var l screen.Line
		for row := -scr.ScrollbackCount(); row < scr.Rows(); row++ {
			scr.LineInto(row, &l)
			text, starts, ends := lineRunes(&l)
			if len(text) == 0 {
				continue
			}
			if re != nil {
				s := string(text)
				for _, loc := range re.FindAllStringIndex(s, -1) {
					if loc[1] == loc[0] {
						continue
					}
					from := len([]rune(s[:loc[0]]))
					to := from + len([]rune(s[loc[0]:loc[1]]))
					se.matches = append(se.matches, cellMatch(row, starts, ends, from, to))
				}
				continue
			}
			hay := text
			if !se.caseSensitive {
				hay = lowerRunes(slices.Clone(text))
			}
			for from := 0; from+len(query) <= len(hay); {
				at := indexRunes(hay[from:], query)
				if at < 0 {
					break
				}
				at += from
				se.matches = append(se.matches, cellMatch(row, starts, ends, at, at+len(query)))
				from = at + len(query)
			}
		}
	})
}

// lineRunes is a line's text without its trailing blanks, one rune per
// character, with the cell each rune starts in and the cell after the one
// it ends in: a double-width character takes two cells and one rune, and a
// character with combining marks one cell and several runes, so rune
// offsets and columns part after either.
func lineRunes(l *screen.Line) (text []rune, starts, ends []int) {
	for c, cell := range l.Cells {
		for _, r := range cell.Text() {
			text = append(text, r)
			starts = append(starts, c)
			ends = append(ends, c+max(1, int(cell.Width)))
		}
	}
	n := len(text)
	for n > 0 && text[n-1] == ' ' {
		n--
	}
	return text[:n], starts[:n], ends[:n]
}

// cellMatch turns a rune range of a line into cells.
func cellMatch(row int, starts, ends []int, from, to int) Match {
	return Match{Row: row, Column: starts[from], Length: ends[to-1] - starts[from]}
}

func lowerRunes(r []rune) []rune {
	for i, c := range r {
		r[i] = unicode.ToLower(c)
	}
	return r
}

func indexRunes(hay, needle []rune) int {
	if len(needle) == 0 {
		return -1
	}
	for i := 0; i+len(needle) <= len(hay); i++ {
		if hay[i] == needle[0] && slices.Equal(hay[i:i+len(needle)], needle) {
			return i
		}
	}
	return -1
}
