package kvitterm

import (
	"sync"
	"testing"
	"time"

	"github.com/kvit-s/kvit-term/internal/stub"
)

// eventually waits for cond, failing the test after the timeout.
func eventually(t *testing.T, what string, timeout time.Duration, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for !cond() {
		if time.Now().After(deadline) {
			t.Fatalf("waited %v for %s", timeout, what)
		}
		time.Sleep(5 * time.Millisecond)
	}
}

// stubSession is a session running termstub with the given arguments, not
// yet started, closed when the test ends.
func stubSession(t *testing.T, args ...string) *Session {
	s := NewSession()
	s.Program = stub.Path(t)
	s.Args = args
	t.Cleanup(s.Close)
	return s
}

// events records a session's events.
type events struct {
	mu   sync.Mutex
	list []Event
}

func record(s *Session) *events {
	e := &events{}
	s.Observe(func(ev Event) {
		e.mu.Lock()
		e.list = append(e.list, ev)
		e.mu.Unlock()
	})
	return e
}

func (e *events) count(k EventKind) int {
	e.mu.Lock()
	defer e.mu.Unlock()
	n := 0
	for _, ev := range e.list {
		if ev.Kind == k {
			n++
		}
	}
	return n
}

func (e *events) last(k EventKind) (Event, bool) {
	e.mu.Lock()
	defer e.mu.Unlock()
	for i := len(e.list) - 1; i >= 0; i-- {
		if e.list[i].Kind == k {
			return e.list[i], true
		}
	}
	return Event{}, false
}
