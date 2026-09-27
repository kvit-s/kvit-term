// Package kvitterm is a terminal for Go applications: a child process on a
// pseudo-terminal, the screen its output is interpreted onto, and what an
// application builds on them — what a shell says about the commands it
// runs, searching, and finding links. The view package draws a Session on a
// unison window.
//
// Four layers, each usable without the ones above it: pty runs a program on
// a pseudo-terminal; screen interprets its output; Session holds one of each
// together; and view draws a session and turns input into bytes for it.
package kvitterm

import (
	"os"
	"slices"
	"sort"
	"sync"
	"time"

	"github.com/kvit-s/kvit-term/pty"
	"github.com/kvit-s/kvit-term/screen"
)

// EventKind is what happened to a session.
type EventKind uint8

// The events a Session reports.
const (
	// Started: the child is running.
	Started EventKind = iota
	// Exited: the child exited, with Event.ExitCode, and everything it
	// wrote has been read.
	Exited
	// Failed: the child could not be started, with Event.Message.
	Failed
	// ContentChanged: the screen changed. Several changes between two
	// deliveries arrive as one event.
	ContentChanged
	// TitleChanged: the program set a title.
	TitleChanged
	// Bell: the program rang the bell.
	Bell
	// Resized: the session has a new size.
	Resized
	// ActivityStarted and ActivityEnded are the two edges of Activity.
	ActivityStarted
	ActivityEnded
)

// Event is one thing a session reports.
type Event struct {
	Kind     EventKind
	ExitCode int    // for Exited
	Message  string // for Failed
}

// DefaultActivityPeriod is how long a screen has to stay unchanged before
// its activity counts as over.
const DefaultActivityPeriod = time.Second

// Session is one terminal: a child process on a pseudo-terminal, and the
// screen its output is interpreted onto. It exists independently of any
// view, so an application can run a command and read its screen without
// drawing anything.
//
// Its methods are safe to call from any goroutine. Events are delivered by
// Dispatch, one at a time and in order; an application built on unison sets
// Dispatch to unison.InvokeTask so they arrive on the interface's thread.
type Session struct {
	// Program is what to run; "" is the user's shell. Args, Dir and Env
	// complete it: Env is added to this process's environment. They are
	// read when the session starts.
	Program string
	Args    []string
	Dir     string
	Env     map[string]string
	// Dispatch delivers events; nil calls listeners on whichever goroutine
	// noticed the change. Set it before starting the session.
	Dispatch func(func())

	mu        sync.Mutex
	scr       *screen.Screen
	child     *pty.Pty
	running   bool
	listeners []*listener
	hooks     []*screenHooks

	// Events waiting to be delivered, and whether a delivery is scheduled.
	queue          []func()
	scheduled      bool
	contentPending bool

	// Activity: whether the screen is changing, and the timer that ends it.
	activity       bool
	activityPeriod time.Duration
	lastChange     time.Time
	activityTimer  *time.Timer

	// Output held back: suspended by the application, or waiting for a view
	// to draw what it already has.
	suspended   bool
	holdForDraw bool
	unseen      int
	wake        chan struct{}

	// Bytes for the child, written on a goroutine of their own so a child
	// that is not reading cannot stall whoever typed.
	out     [][]byte
	outWake chan struct{}
}

type listener struct{ fn func(Event) }

// screenHooks are called while output is interpreted, with the session's
// lock held, at the exact point in the stream where they happen. The shell
// integration is built on them, since where the cursor is when a mark
// arrives is part of what the mark means.
type screenHooks struct {
	osc     func(command int, payload string)
	cleared func()
}

// NewSession makes a session with an 80 × 24 screen. It starts when Start
// is called.
func NewSession() *Session {
	s := &Session{
		scr:            screen.New(80, 24),
		activityPeriod: DefaultActivityPeriod,
		wake:           make(chan struct{}, 1),
		outWake:        make(chan struct{}, 1),
	}
	s.scr.OnWrite = s.writeLocked
	// Activity means the screen changed, so it is taken from the screen
	// rather than from the child's bytes: some of what a child writes — a
	// query, a mode change — alters nothing on the display, and a resize or
	// a clear alters it with the child writing nothing at all.
	s.scr.OnDamage = func(int, int) { s.changedLocked() }
	s.scr.OnScroll = func(int) { s.changedLocked() }
	s.scr.OnCursorMove = func(screen.Point) { s.changedLocked() }
	s.scr.OnTitle = func(string) {
		s.changedLocked()
		s.postLocked(Event{Kind: TitleChanged})
	}
	s.scr.OnBell = func() { s.postLocked(Event{Kind: Bell}) }
	s.scr.OnScrollbackCleared = func() {
		s.changedLocked()
		for _, h := range s.hooks {
			if h.cleared != nil {
				h.cleared()
			}
		}
	}
	s.scr.OnOSC = func(cmd int, payload string) {
		for _, h := range s.hooks {
			if h.osc != nil {
				h.osc(cmd, payload)
			}
		}
	}
	return s
}

// Observe calls fn for every event from now on, through Dispatch. The
// returned function stops it.
func (s *Session) Observe(fn func(Event)) (stop func()) {
	l := &listener{fn: fn}
	s.mu.Lock()
	s.listeners = append(s.listeners, l)
	s.mu.Unlock()
	return func() {
		s.mu.Lock()
		s.listeners = slices.DeleteFunc(s.listeners, func(x *listener) bool { return x == l })
		s.mu.Unlock()
	}
}

func (s *Session) hook(h *screenHooks) (remove func()) {
	s.mu.Lock()
	s.hooks = append(s.hooks, h)
	s.mu.Unlock()
	return func() {
		s.mu.Lock()
		s.hooks = slices.DeleteFunc(s.hooks, func(x *screenHooks) bool { return x == h })
		s.mu.Unlock()
	}
}

// postLocked queues an event for the listeners.
func (s *Session) postLocked(e Event) {
	if e.Kind == ContentChanged {
		if s.contentPending {
			return
		}
		s.contentPending = true
	}
	s.queueLocked(func() {
		s.mu.Lock()
		if e.Kind == ContentChanged {
			s.contentPending = false
		}
		ls := slices.Clone(s.listeners)
		s.mu.Unlock()
		for _, l := range ls {
			l.fn(e)
		}
	})
}

// queueLocked schedules f to run through Dispatch after everything queued
// before it.
func (s *Session) queueLocked(f func()) {
	s.queue = append(s.queue, f)
	if s.scheduled {
		return
	}
	s.scheduled = true
	deliver := func() {
		for {
			s.mu.Lock()
			if len(s.queue) == 0 {
				s.scheduled = false
				s.mu.Unlock()
				return
			}
			next := s.queue[0]
			s.queue = s.queue[1:]
			s.mu.Unlock()
			next()
		}
	}
	if s.Dispatch != nil {
		s.Dispatch(deliver)
	} else {
		go deliver()
	}
}

// changedLocked notes that the screen changed: content for the listeners,
// and the start or continuation of activity.
func (s *Session) changedLocked() {
	s.postLocked(Event{Kind: ContentChanged})
	s.lastChange = time.Now()
	if s.activityTimer == nil {
		s.activityTimer = time.AfterFunc(s.activityPeriod, s.activityCheck)
	} else {
		s.activityTimer.Reset(s.activityPeriod)
	}
	if !s.activity {
		s.activity = true
		s.postLocked(Event{Kind: ActivityStarted})
	}
}

func (s *Session) activityCheck() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.activity {
		return
	}
	if left := s.activityPeriod - time.Since(s.lastChange); left > 0 {
		s.activityTimer.Reset(left)
		return
	}
	s.activity = false
	s.postLocked(Event{Kind: ActivityEnded})
}

// Activity reports whether the screen is changing: output from the child,
// a line scrolling off, the cursor moving, a new title. It turns off once
// nothing has changed for the activity period. A terminal sitting at a
// prompt reports nothing, and a blinking cursor never reaches the screen.
func (s *Session) Activity() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.activity
}

// ActivityPeriod is how long the screen has to stay unchanged before the
// activity counts as over.
func (s *Session) ActivityPeriod() time.Duration {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.activityPeriod
}

// SetActivityPeriod changes it; a period already running takes the new
// length from where it is now.
func (s *Session) SetActivityPeriod(d time.Duration) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.activityPeriod = max(0, d)
	if s.activityTimer != nil && s.activity {
		s.activityTimer.Reset(max(0, s.activityPeriod-time.Since(s.lastChange)))
	}
}

// View runs fn with the screen, holding the session's lock, for reading
// what the terminal shows. fn must not call the session's other methods.
func (s *Session) View(fn func(*screen.Screen)) {
	s.mu.Lock()
	defer s.mu.Unlock()
	fn(s.scr)
}

// Running reports whether the child is running.
func (s *Session) Running() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.running
}

// Pid is the running child's process id, or 0.
func (s *Session) Pid() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.child == nil || !s.running {
		return 0
	}
	return s.child.Pid()
}

// Title is what the program set as the window title.
func (s *Session) Title() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.scr.Title()
}

// Size is the screen's width and height in cells.
func (s *Session) Size() (columns, rows int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.scr.Columns(), s.scr.Rows()
}

// ScrollbackCount is how many lines are kept above the visible screen.
func (s *Session) ScrollbackCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.scr.ScrollbackCount()
}

// ScrollbackLimit is the most lines the scrollback keeps.
func (s *Session) ScrollbackLimit() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.scr.ScrollbackLimit()
}

// SetScrollbackLimit changes how many lines the scrollback keeps.
func (s *Session) SetScrollbackLimit(lines int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.scr.SetScrollbackLimit(lines)
}

// ScreenText is the visible screen as text.
func (s *Session) ScreenText() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.scr.Text(0, s.scr.Rows()-1)
}

// LineText is one row as text; negative rows are the scrollback.
func (s *Session) LineText(row int) string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.scr.Line(row).Text()
}

// HTML is everything the session holds, scrollback included, as styled
// HTML in the given colours.
func (s *Session) HTML(p screen.Palette) string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return screen.HTML(s.scr, -s.scr.ScrollbackCount(), s.scr.Rows()-1, p)
}

// Feed interprets bytes as if the child had written them. It is how tests
// and applications without a child put output on the screen.
func (s *Session) Feed(b []byte) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.scr.Feed(b)
}

// Start starts the child with the session's program, arguments, directory
// and environment, on a terminal of the screen's size. A session already
// running is left alone. A child that exited can be started again, on the
// same screen, which keeps what the one before it wrote.
func (s *Session) Start() error {
	s.mu.Lock()
	if s.running {
		s.mu.Unlock()
		return nil
	}
	if s.child != nil {
		s.child.Release()
		s.child = nil
	}
	p := pty.Params{
		Program: s.Program,
		Args:    s.Args,
		Dir:     s.Dir,
		Columns: s.scr.Columns(),
		Rows:    s.scr.Rows(),
	}
	if p.Program == "" {
		p.Program = pty.DefaultShell()
	}
	if len(s.Env) > 0 {
		env := os.Environ()
		keys := make([]string, 0, len(s.Env))
		for k := range s.Env {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			env = append(env, k+"="+s.Env[k])
		}
		p.Env = env
	}
	child, err := pty.Start(p)
	if err != nil {
		s.postLocked(Event{Kind: Failed, Message: err.Error()})
		s.mu.Unlock()
		return err
	}
	s.child = child
	s.running = true
	s.out = nil
	s.postLocked(Event{Kind: Started})
	s.mu.Unlock()

	readerDone := make(chan struct{})
	var lastRead time.Time
	var lastMu sync.Mutex
	go s.writeLoop(child)
	go func() {
		defer close(readerDone)
		buf := make([]byte, 64<<10)
		for {
			s.waitUntilReading(child)
			n, err := child.Read(buf)
			if n > 0 {
				lastMu.Lock()
				lastRead = time.Now()
				lastMu.Unlock()
				s.mu.Lock()
				if s.child == child {
					s.scr.Feed(buf[:n])
					s.unseen++
				}
				s.mu.Unlock()
			}
			if err != nil {
				return
			}
		}
	}()
	go func() {
		<-child.Done()
		// What the child wrote just before it exited may not be read yet.
		// Wait for the end of the output, or for it to go quiet: a process
		// the child left running in the background can keep the terminal
		// open indefinitely.
		deadline := time.Now().Add(time.Second)
	wait:
		for time.Now().Before(deadline) {
			select {
			case <-readerDone:
				break wait
			case <-time.After(10 * time.Millisecond):
				lastMu.Lock()
				quiet := time.Since(lastRead) > 50*time.Millisecond
				lastMu.Unlock()
				if quiet {
					break wait
				}
			}
		}
		s.mu.Lock()
		if s.child == child {
			s.running = false
			s.postLocked(Event{Kind: Exited, ExitCode: child.ExitCode()})
		}
		s.mu.Unlock()
		s.nudge()
	}()
	return nil
}

// Resize changes the screen and tells the child; a view calls it with the
// size its geometry gives.
func (s *Session) Resize(columns, rows int) {
	if columns < 1 || rows < 1 {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if columns == s.scr.Columns() && rows == s.scr.Rows() {
		return
	}
	s.scr.SetSize(columns, rows)
	if s.child != nil && s.running {
		s.child.Resize(s.scr.Columns(), s.scr.Rows())
	}
	s.postLocked(Event{Kind: Resized})
	s.changedLocked()
}

// Close ends the session the way closing a window would: the child's
// terminal is hung up, and the child is asked to terminate as well.
func (s *Session) Close() {
	s.mu.Lock()
	child, running := s.child, s.running
	s.mu.Unlock()
	if child == nil {
		return
	}
	child.Hangup()
	if running {
		child.Terminate()
	}
}

// Clear wipes the screen and the scrollback, as the clear command does.
func (s *Session) Clear() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.scr.Reset(true)
	s.scr.ClearScrollback()
	s.changedLocked()
}

// PressKey sends a key with a sequence of its own.
func (s *Session) PressKey(k screen.Key, m screen.Modifiers) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.scr.PressKey(k, m)
}

// TypeChar sends a character typed with modifiers held.
func (s *Session) TypeChar(r rune, m screen.Modifiers) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.scr.TypeChar(r, m)
}

// SendText sends text as if typed.
func (s *Session) SendText(text string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.scr.SendText(text)
}

// Paste sends text as a paste, marked as one where the program asked.
func (s *Session) Paste(text string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.scr.Paste(text)
}

// Mouse moves the pointer to a cell and presses or releases a button there,
// or only moves it when button is screen.NoButton; the program is told if
// it asked to be.
func (s *Session) Mouse(row, column int, button screen.MouseButton, pressed bool, m screen.Modifiers) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.scr.MouseMove(row, column, m)
	if button != screen.NoButton {
		s.scr.MouseButton(button, pressed, m)
	}
}

// SetFocused tells the program the terminal gained or lost the keyboard.
func (s *Session) SetFocused(focused bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.scr.SetFocused(focused)
}

// writeLocked queues bytes for the child.
func (s *Session) writeLocked(b []byte) {
	if s.child == nil || !s.running {
		return
	}
	s.out = append(s.out, slices.Clone(b))
	select {
	case s.outWake <- struct{}{}:
	default:
	}
}

func (s *Session) writeLoop(child *pty.Pty) {
	for {
		s.mu.Lock()
		if s.child != child {
			s.mu.Unlock()
			return
		}
		batch := s.out
		s.out = nil
		s.mu.Unlock()
		for _, b := range batch {
			if _, err := child.Write(b); err != nil {
				break
			}
		}
		select {
		case <-s.outWake:
		case <-child.Done():
			return
		}
	}
}

// SetReadingSuspended stops reading the child's output, or resumes it. The
// child fills the terminal's buffer and then blocks in its own write, which
// is how a runaway process is kept from outrunning whatever draws it.
func (s *Session) SetReadingSuspended(suspended bool) {
	s.mu.Lock()
	s.suspended = suspended
	s.mu.Unlock()
	s.nudge()
}

// ReadingSuspended reports whether reading is suspended.
func (s *Session) ReadingSuspended() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.suspended
}

// holdChunks is how many reads a view may fall behind before reading waits
// for it to draw, and holdLimit how long reading waits for a view that is
// not drawing, one that is hidden, say.
const (
	holdChunks = 8
	holdLimit  = 250 * time.Millisecond
)

// HoldForDraw makes reading wait while a view has more than a few reads it
// has not drawn yet, so output arrives no faster than it can be shown; the
// view calls Drawn after each frame. A view that stops drawing holds
// reading back for at most a quarter of a second at a time.
func (s *Session) HoldForDraw(hold bool) {
	s.mu.Lock()
	s.holdForDraw = hold
	s.unseen = 0
	s.mu.Unlock()
	s.nudge()
}

// Drawn tells the session a view has drawn what it has read.
func (s *Session) Drawn() {
	s.mu.Lock()
	s.unseen = 0
	s.mu.Unlock()
	s.nudge()
}

func (s *Session) nudge() {
	select {
	case s.wake <- struct{}{}:
	default:
	}
}

func (s *Session) waitUntilReading(child *pty.Pty) {
	for {
		s.mu.Lock()
		suspended := s.suspended && s.child == child
		held := s.holdForDraw && s.unseen >= holdChunks
		s.mu.Unlock()
		switch {
		case suspended:
			select {
			case <-s.wake:
			case <-child.Done():
				// Nothing waits for a child that has gone, but its last output
				// is still read once reading resumes.
				<-s.wake
			}
		case held:
			select {
			case <-s.wake:
			case <-time.After(holdLimit):
				s.mu.Lock()
				s.unseen = 0
				s.mu.Unlock()
			}
		default:
			return
		}
	}
}
