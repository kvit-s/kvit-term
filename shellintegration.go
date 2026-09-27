package kvitterm

import (
	"embed"
	"net/url"
	"path/filepath"
	"runtime"
	"slices"
	"strconv"
	"strings"

	"github.com/kvit-s/kvit-term/screen"
)

// Shell is a shell a snippet is written for.
type Shell uint8

// The shells with a snippet.
const (
	Bash Shell = iota
	Zsh
	Fish
	PowerShell
)

//go:embed shell
var shellScripts embed.FS

// ShellScriptName is the snippet's file name.
func ShellScriptName(sh Shell) string {
	switch sh {
	case Zsh:
		return "kvitterm.zsh"
	case Fish:
		return "kvitterm.fish"
	case PowerShell:
		return "kvitterm.ps1"
	}
	return "kvitterm.bash"
}

// ShellScript is the snippet a shell must run to emit the marks, as text.
// Writing it anywhere — an init file, a directory the shell is pointed at —
// changes a user's configuration, so it is the application's decision; the
// pattern to copy is the demonstration program's, which writes it to a
// directory of its own and starts bash with an init file that sources the
// user's own configuration first. The user's files are read and never
// written.
func ShellScript(sh Shell) string {
	b, err := shellScripts.ReadFile("shell/" + ShellScriptName(sh))
	if err != nil {
		return ""
	}
	return string(b)
}

// Command is one command, from the moment the shell said it was about to
// run to the moment it said what it exited with.
type Command struct {
	Text      string // the command line, as typed
	Directory string // where the shell was when it ran
	// OutputFirstRow and OutputLastRow are counted from the first line the
	// screen ever showed, so they stay put while output scrolls; see
	// ShellIntegration.ScreenRow.
	OutputFirstRow int
	OutputLastRow  int
	ExitCode       int // -1 until it finishes, and for one that never said
	Finished       bool
}

// ShellEventKind is what a shell said.
type ShellEventKind uint8

// The events of a ShellIntegration.
const (
	// Activated: the first mark arrived.
	Activated ShellEventKind = iota
	// DirectoryChanged: the shell is somewhere else.
	DirectoryChanged
	// CommandStarted and CommandFinished carry the command.
	CommandStarted
	CommandFinished
	// CommandsChanged: the list of commands changed in any way.
	CommandsChanged
)

// ShellEvent is one thing a shell said.
type ShellEvent struct {
	Kind    ShellEventKind
	Command Command // for CommandStarted and CommandFinished
	Index   int     // the command's index, for the same two
}

// ShellIntegration turns what a shell says about itself into a list of
// commands.
//
// A terminal on its own sees characters: it cannot say where one command
// ended and the next began, what any of them exited with, or which
// directory the shell is in. A shell can be made to say so, by writing
// marks around each prompt and command: the standard OSC 133 marks, OSC 7
// for the directory, and the OSC 633 forms Visual Studio Code's snippets
// write. Everything that makes an integrated terminal different from a
// plain one is built on them — a pass or fail mark beside each command,
// jumping between commands, opening the next terminal where this one is,
// and reading a command's output without selecting it.
//
// Its methods are safe from any goroutine; its events arrive through the
// session's Dispatch.
type ShellIntegration struct {
	s      *Session
	remove func()

	// Guarded by the session's lock.
	active       bool
	directory    string
	commands     []Command
	running      bool
	commandStart screen.Point
	haveStart    bool
	listeners    []*shellListener
}

type shellListener struct{ fn func(ShellEvent) }

// NewShellIntegration follows what the session's shell says from now on.
func NewShellIntegration(s *Session) *ShellIntegration {
	si := &ShellIntegration{s: s}
	si.remove = s.hook(&screenHooks{osc: si.handle})
	return si
}

// Stop stops following the session.
func (si *ShellIntegration) Stop() { si.remove() }

// Observe calls fn for every event from now on. The returned function stops
// it.
func (si *ShellIntegration) Observe(fn func(ShellEvent)) (stop func()) {
	l := &shellListener{fn: fn}
	si.s.mu.Lock()
	si.listeners = append(si.listeners, l)
	si.s.mu.Unlock()
	return func() {
		si.s.mu.Lock()
		si.listeners = slices.DeleteFunc(si.listeners, func(x *shellListener) bool { return x == l })
		si.s.mu.Unlock()
	}
}

func (si *ShellIntegration) postLocked(e ShellEvent) {
	si.s.queueLocked(func() {
		si.s.mu.Lock()
		ls := slices.Clone(si.listeners)
		si.s.mu.Unlock()
		for _, l := range ls {
			l.fn(e)
		}
	})
}

// Active is false until a mark arrives: a shell without the snippet never
// says anything, and an application should be able to tell that from a
// shell that has run nothing yet.
func (si *ShellIntegration) Active() bool {
	si.s.mu.Lock()
	defer si.s.mu.Unlock()
	return si.active
}

// Directory is where the shell last said it was.
func (si *ShellIntegration) Directory() string {
	si.s.mu.Lock()
	defer si.s.mu.Unlock()
	return si.directory
}

// CommandRunning reports a command started and not yet finished.
func (si *ShellIntegration) CommandRunning() bool {
	si.s.mu.Lock()
	defer si.s.mu.Unlock()
	return si.running
}

// Commands are the commands so far, oldest first.
func (si *ShellIntegration) Commands() []Command {
	si.s.mu.Lock()
	defer si.s.mu.Unlock()
	return slices.Clone(si.commands)
}

// CommandCount is how many commands there have been.
func (si *ShellIntegration) CommandCount() int {
	si.s.mu.Lock()
	defer si.s.mu.Unlock()
	return len(si.commands)
}

// Command is one command by index, and whether there is one.
func (si *ShellIntegration) Command(i int) (Command, bool) {
	si.s.mu.Lock()
	defer si.s.mu.Unlock()
	if i < 0 || i >= len(si.commands) {
		return Command{}, false
	}
	return si.commands[i], true
}

// Clear forgets the commands.
func (si *ShellIntegration) Clear() {
	si.s.mu.Lock()
	defer si.s.mu.Unlock()
	si.commands = nil
	si.running = false
	si.postLocked(ShellEvent{Kind: CommandsChanged})
}

// absoluteRowLocked numbers a screen row from the first line ever shown.
// The top of the screen moves, but the count of lines scrolled away plus a
// screen row is a number that stays put.
func (si *ShellIntegration) absoluteRowLocked(row int) int { return si.s.scr.ScrolledAway() + row }

// ScreenRow is where a row counted from the first line ever shown is now,
// as screen rows are numbered: negative in the scrollback, or below
// -ScrollbackCount once it has scrolled out of what is kept.
func (si *ShellIntegration) ScreenRow(absolute int) int {
	si.s.mu.Lock()
	defer si.s.mu.Unlock()
	return absolute - si.s.scr.ScrolledAway()
}

// lastOutputRowLocked is the last row a running command's output has
// reached. When the output ends with a line break, as nearly all output
// does, the cursor is already on the row below, which belongs to what
// comes next rather than to this command.
func (si *ShellIntegration) lastOutputRowLocked() int {
	c := si.s.scr.Cursor()
	row := c.Y
	if c.X == 0 && c.Y > 0 {
		row--
	}
	return si.absoluteRowLocked(row)
}

func (si *ShellIntegration) setDirectoryLocked(dir string) {
	if dir == "" || dir == si.directory {
		return
	}
	si.directory = dir
	si.postLocked(ShellEvent{Kind: DirectoryChanged})
}

func (si *ShellIntegration) finishLocked(code int) {
	n := len(si.commands)
	if n == 0 || si.commands[n-1].Finished {
		return
	}
	c := &si.commands[n-1]
	c.ExitCode = code
	c.Finished = true
	// This can be one row above the first: a command that printed nothing
	// has no output rows, and the row where its output would have started
	// holds the next prompt.
	c.OutputLastRow = si.lastOutputRowLocked()
	si.running = false
	si.postLocked(ShellEvent{Kind: CommandsChanged})
	si.postLocked(ShellEvent{Kind: CommandFinished, Command: *c, Index: n - 1})
}

// handle is called with the session's lock held, at the point in the
// stream where the mark arrives.
func (si *ShellIntegration) handle(cmd int, payload string) {
	switch cmd {
	case 7:
		// The directory as a file URL, host included.
		if u, err := url.Parse(payload); err == nil && u.Path != "" {
			si.setDirectoryLocked(fileURLPath(u.Path))
		}
	case 133, 633:
		if payload == "" {
			return
		}
		rest := strings.TrimPrefix(payload[1:], ";")
		si.markLocked(payload[0], rest)
	}
}

func (si *ShellIntegration) markLocked(mark byte, rest string) {
	if !si.active {
		si.active = true
		si.postLocked(ShellEvent{Kind: Activated})
	}
	scr := si.s.scr
	switch mark {
	case 'A':
		// A prompt is about to be drawn, so whatever ran before it has
		// finished even if no status arrived: a signal or an interrupted
		// line can end a command silently.
		si.finishLocked(-1)
		si.haveStart = false
	case 'B':
		// The prompt has ended: what the user types starts here.
		si.commandStart = scr.Cursor()
		si.haveStart = true
	case 'C':
		// The command has been read and is about to run.
		c := Command{Directory: si.directory, ExitCode: -1}
		if si.haveStart {
			end := scr.Cursor()
			c.Text = strings.TrimSpace(scr.TextInRange(si.commandStart, screen.Point{X: max(0, end.X-1), Y: end.Y}, false))
		}
		c.OutputFirstRow = si.absoluteRowLocked(scr.Cursor().Y)
		c.OutputLastRow = c.OutputFirstRow
		si.commands = append(si.commands, c)
		si.running = true
		si.postLocked(ShellEvent{Kind: CommandsChanged})
		si.postLocked(ShellEvent{Kind: CommandStarted, Command: c, Index: len(si.commands) - 1})
	case 'D':
		// The command finished; its status follows if the shell sent one.
		code := -1
		for _, part := range strings.Split(rest, ";") {
			if v, err := strconv.Atoi(strings.TrimSpace(part)); err == nil {
				code = v
				break
			}
		}
		si.finishLocked(code)
	case 'E':
		// Visual Studio Code's snippets give the command line explicitly
		// rather than leaving it to be read off the screen.
		if n := len(si.commands); rest != "" && n > 0 && !si.commands[n-1].Finished {
			si.commands[n-1].Text = unescape633(rest)
		}
	case 'P':
		// 633;P;Cwd=/some/path
		if i := strings.Index(rest, "Cwd="); i >= 0 {
			si.setDirectoryLocked(unescape633(rest[i+4:]))
		}
	}
}

// fileURLPath is the directory a file URL's path names. A Windows path
// arrives after a slash of the URL's own — /C:/Users/x, or ///server/share
// for a network path — which is dropped, and the slashes become the
// system's separators.
func fileURLPath(p string) string {
	if len(p) >= 3 && p[0] == '/' && p[2] == ':' && (p[1]|0x20 >= 'a' && p[1]|0x20 <= 'z') || strings.HasPrefix(p, "///") {
		p = p[1:]
		if runtime.GOOS == "windows" {
			p = filepath.FromSlash(p)
		}
	}
	return p
}

// unescape633 undoes the escaping Visual Studio Code's marks use for
// semicolons, backslashes and control characters: \\ and \xAB.
func unescape633(s string) string {
	if !strings.Contains(s, `\`) {
		return s
	}
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		if s[i] == '\\' && i+1 < len(s) {
			if s[i+1] == '\\' {
				b.WriteByte('\\')
				i++
				continue
			}
			if s[i+1] == 'x' && i+3 < len(s) {
				if v, err := strconv.ParseUint(s[i+2:i+4], 16, 8); err == nil {
					b.WriteByte(byte(v))
					i += 3
					continue
				}
			}
		}
		b.WriteByte(s[i])
	}
	return b.String()
}

// Output is the output of a command as text: the thing a scrollback alone
// cannot give, which bytes belonged to which command. It is empty for a
// command that printed nothing, and for one whose output has scrolled out of
// what the scrollback keeps.
func (si *ShellIntegration) Output(i int) string {
	si.s.mu.Lock()
	defer si.s.mu.Unlock()
	if i < 0 || i >= len(si.commands) {
		return ""
	}
	c := si.commands[i]
	scr := si.s.scr
	away := scr.ScrolledAway()
	first := c.OutputFirstRow - away
	last := c.OutputLastRow
	if !c.Finished {
		last = si.lastOutputRowLocked()
	}
	last -= away
	if last < first || last < -scr.ScrollbackCount() {
		return ""
	}
	return scr.Text(max(first, -scr.ScrollbackCount()), min(last, scr.Rows()-1))
}

// CommandAtRow is the index of the command whose output a screen row
// belongs to, or -1. Rows are numbered as the screen numbers them, so
// negative rows are the scrollback.
func (si *ShellIntegration) CommandAtRow(row int) int {
	si.s.mu.Lock()
	defer si.s.mu.Unlock()
	abs := si.absoluteRowLocked(row)
	for i := len(si.commands) - 1; i >= 0; i-- {
		c := si.commands[i]
		last := c.OutputLastRow
		if !c.Finished {
			last = si.lastOutputRowLocked()
		}
		if abs >= c.OutputFirstRow && abs <= last {
			return i
		}
	}
	return -1
}
