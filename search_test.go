package kvitterm

// Finding text in what a terminal has shown, screen and scrollback
// together. These are the Qt library's tests
// (tests/unit/test_terminalsearch.cpp), case for case.

import (
	"fmt"
	"testing"
)

func sessionWithLines(n int) *Session {
	s := NewSession()
	s.Resize(40, 5)
	for i := 1; i <= n; i++ {
		s.Feed([]byte(fmt.Sprintf("line %d of output\r\n", i)))
	}
	return s
}

func TestMatchesAreFoundInTheScrollbackAsWellAsOnScreen(t *testing.T) {
	s := sessionWithLines(20)
	if s.ScrollbackCount() <= 10 {
		t.Fatalf("scrollback %d", s.ScrollbackCount())
	}
	se := NewSearch(s)
	se.SetQuery("line 3 ")
	if se.MatchCount() != 1 || se.Matches()[0].Row >= 0 {
		t.Errorf("matches %+v", se.Matches())
	}
}

func TestEveryOccurrenceOnALineIsFound(t *testing.T) {
	s := NewSession()
	s.Feed([]byte("aa bb aa bb aa\r\n"))
	se := NewSearch(s)
	se.SetQuery("aa")
	if se.MatchCount() != 3 || se.Matches()[1].Column != 6 {
		t.Errorf("matches %+v", se.Matches())
	}
}

func TestCaseIsIgnoredUnlessAskedFor(t *testing.T) {
	s := NewSession()
	s.Feed([]byte("Error: ERROR error\r\n"))
	se := NewSearch(s)
	se.SetQuery("error")
	if se.MatchCount() != 3 {
		t.Errorf("%d matches ignoring case", se.MatchCount())
	}
	se.SetCaseSensitive(true)
	if se.MatchCount() != 1 {
		t.Errorf("%d matches with case", se.MatchCount())
	}
}

func TestARegularExpressionCanBeUsed(t *testing.T) {
	s := NewSession()
	s.Feed([]byte("test 1 passed\r\ntest 22 failed\r\n"))
	se := NewSearch(s)
	se.SetRegularExpression(true)
	se.SetQuery(`test \d+ failed`)
	if se.MatchCount() != 1 || se.Matches()[0].Length != len("test 22 failed") {
		t.Errorf("matches %+v", se.Matches())
	}
}

func TestAnInvalidRegularExpressionFindsNothingRatherThanFailing(t *testing.T) {
	s := NewSession()
	s.Feed([]byte("anything\r\n"))
	se := NewSearch(s)
	se.SetRegularExpression(true)
	se.SetQuery("(")
	if se.MatchCount() != 0 {
		t.Errorf("%d matches", se.MatchCount())
	}
}

func TestNextAndPreviousMoveThroughTheMatchesAndWrap(t *testing.T) {
	s := sessionWithLines(6)
	se := NewSearch(s)
	se.SetQuery("output")
	if se.MatchCount() != 6 || se.Current() != -1 {
		t.Fatalf("%d matches, current %d", se.MatchCount(), se.Current())
	}
	// The first Next goes to the most recent match, the nearest to what the
	// user is looking at.
	se.Next()
	if se.Current() != 5 {
		t.Errorf("after next: %d", se.Current())
	}
	se.Next()
	if se.Current() != 0 {
		t.Errorf("after next again: %d", se.Current())
	}
	se.Previous()
	if se.Current() != 5 {
		t.Errorf("after previous: %d", se.Current())
	}
}

func TestNewOutputIsSearchedToo(t *testing.T) {
	s := NewSession()
	s.Feed([]byte("first needle\r\n"))
	se := NewSearch(s)
	se.SetQuery("needle")
	if se.MatchCount() != 1 {
		t.Fatalf("%d matches", se.MatchCount())
	}
	s.Feed([]byte("second needle\r\n"))
	se.Refresh()
	if se.MatchCount() != 2 {
		t.Errorf("%d matches after refreshing", se.MatchCount())
	}
}

func TestAnEmptyQueryMatchesNothing(t *testing.T) {
	s := sessionWithLines(3)
	se := NewSearch(s)
	se.SetQuery("line")
	if se.MatchCount() == 0 {
		t.Fatal("no matches")
	}
	se.Clear()
	if se.MatchCount() != 0 || se.Current() != -1 {
		t.Errorf("%d matches, current %d", se.MatchCount(), se.Current())
	}
}

func TestMatchesAreInCellsAfterWideCharacters(t *testing.T) {
	// A double-width character takes two cells and one character, so a
	// match after it is two cells further right than its character count.
	s := NewSession()
	s.Feed([]byte("日本 needle\r\n"))
	se := NewSearch(s)
	se.SetQuery("needle")
	if m := se.Matches(); len(m) != 1 || m[0].Column != 5 || m[0].Length != 6 {
		t.Errorf("matches %+v", m)
	}
}
