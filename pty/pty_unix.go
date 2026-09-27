//go:build unix

package pty

import (
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"sync"
	"syscall"

	cpty "github.com/creack/pty"
	"golang.org/x/sys/unix"
)

// Pty is a child process on a pseudo-terminal.
//
// On Linux and macOS openpty hands back both ends of a new device pair. The
// child is started in a session of its own with the slave end as its
// controlling terminal on all three standard descriptors, which is exactly
// how a terminal application starts a shell. This end keeps the master.
type Pty struct {
	master *os.File
	cmd    *exec.Cmd

	mu      sync.Mutex
	closed  bool
	done    chan struct{} // closed when the child has exited
	code    int
	columns int
	rows    int
}

// Start allocates a pseudo-terminal and starts the child on it. A program
// that cannot be started is an error here; one that starts and exits at once
// is a success, whose exit Wait reports.
func Start(p Params) (*Pty, error) {
	if err := p.normalize(); err != nil {
		return nil, err
	}
	path, err := exec.LookPath(p.Program)
	if err != nil {
		return nil, fmt.Errorf("could not start %s: %w", p.Program, err)
	}
	master, slave, err := cpty.Open()
	if err != nil {
		return nil, fmt.Errorf("could not allocate a pseudo-terminal: %w", err)
	}
	size := &cpty.Winsize{Cols: uint16(p.Columns), Rows: uint16(p.Rows)}
	if err := cpty.Setsize(master, size); err != nil {
		master.Close()
		slave.Close()
		return nil, fmt.Errorf("could not size the pseudo-terminal: %w", err)
	}
	if master, err = pollable(master); err != nil {
		slave.Close()
		return nil, fmt.Errorf("could not allocate a pseudo-terminal: %w", err)
	}

	cmd := exec.Command(path, p.Args...)
	cmd.Args[0] = p.Program
	cmd.Dir = p.Dir
	cmd.Env = p.environment()
	cmd.Stdin, cmd.Stdout, cmd.Stderr = slave, slave, slave
	// A session of its own, with the terminal as its controlling terminal:
	// job control works, Ctrl+C reaches the foreground job, and closing the
	// master hangs the session up.
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true, Setctty: true, Ctty: 0}
	defaultSignalsInChildren.Do(resetIgnoredSignals)
	if err := cmd.Start(); err != nil {
		master.Close()
		slave.Close()
		return nil, fmt.Errorf("could not start %s: %w", p.Program, err)
	}
	// The child has its own copies; this end must not keep the slave open,
	// or reading the master would never see the child's end close.
	slave.Close()

	t := &Pty{master: master, cmd: cmd, done: make(chan struct{}), columns: p.Columns, rows: p.Rows}
	go func() {
		err := cmd.Wait()
		code := 0
		var exit *exec.ExitError
		if errors.As(err, &exit) {
			code = exit.ExitCode()
			if ws, ok := exit.Sys().(syscall.WaitStatus); ok && ws.Signaled() {
				code = 128 + int(ws.Signal())
			}
		}
		t.mu.Lock()
		t.code = code
		t.mu.Unlock()
		close(t.done)
	}()
	return t, nil
}

// Read reads what the child wrote. It returns io.EOF once the terminal is
// closed: the child and everything it started have closed their ends, or
// Hangup was called.
func (t *Pty) Read(b []byte) (int, error) {
	n, err := t.master.Read(b)
	if err != nil {
		// EIO is the ordinary end of a session on Linux: the last descriptor
		// on the slave side has closed.
		return n, eof(err)
	}
	return n, nil
}

// Write sends bytes to the child's standard input.
func (t *Pty) Write(b []byte) (int, error) { return t.master.Write(b) }

// Resize tells the child the terminal's new size. The kernel sends
// SIGWINCH to the foreground process group as a consequence, which is how a
// full-screen program learns to redraw.
func (t *Pty) Resize(columns, rows int) error {
	columns, rows = max(1, columns), max(1, rows)
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.closed || (columns == t.columns && rows == t.rows) {
		return nil
	}
	t.columns, t.rows = columns, rows
	conn, err := t.master.SyscallConn()
	if err != nil {
		return err
	}
	var ioctlErr error
	err = conn.Control(func(fd uintptr) {
		ioctlErr = unix.IoctlSetWinsize(int(fd), unix.TIOCSWINSZ, &unix.Winsize{Col: uint16(columns), Row: uint16(rows)})
	})
	if err != nil {
		return err
	}
	return ioctlErr
}

// pollable returns the master as a non-blocking file that Go's poller
// watches. The library opens it blocking, and a read blocked in the kernel
// cannot be interrupted: closing the file would wait for the read to
// return, so the terminal would never be hung up. (Asking the file for its
// descriptor makes it blocking again, which is why Resize goes through
// SyscallConn.)
func pollable(f *os.File) (*os.File, error) {
	fd, err := unix.FcntlInt(f.Fd(), unix.F_DUPFD_CLOEXEC, 0)
	f.Close()
	if err != nil {
		return nil, err
	}
	if err := unix.SetNonblock(fd, true); err != nil {
		unix.Close(fd)
		return nil, err
	}
	return os.NewFile(uintptr(fd), "/dev/ptmx"), nil
}

// Pid is the child's process id.
func (t *Pty) Pid() int { return t.cmd.Process.Pid }

// Done is closed when the child has exited.
func (t *Pty) Done() <-chan struct{} { return t.done }

// ExitCode is the child's exit status once Done is closed: its own, or 128
// plus the signal that ended it.
func (t *Pty) ExitCode() int {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.code
}

// Hangup closes this end. The child's session is hung up and its
// foreground process group gets SIGHUP, which is what a program sees when a
// person closes the window.
func (t *Pty) Hangup() {
	t.mu.Lock()
	defer t.mu.Unlock()
	if !t.closed {
		t.closed = true
		t.master.Close()
	}
}

// Release closes this end once nothing more will be read from it.
func (t *Pty) Release() { t.Hangup() }

// Terminate asks the child to end, with SIGTERM.
func (t *Pty) Terminate() { t.signal(syscall.SIGTERM) }

// Kill ends the child, for one that ignores Terminate.
func (t *Pty) Kill() { t.signal(syscall.SIGKILL) }

func (t *Pty) signal(sig syscall.Signal) {
	select {
	case <-t.done:
	default:
		t.cmd.Process.Signal(sig)
	}
}

// childSignals are the signals a shell expects to start with at their
// defaults, rather than as whatever started this application left them.
var childSignals = []os.Signal{syscall.SIGHUP, syscall.SIGINT, syscall.SIGQUIT, syscall.SIGTERM, syscall.SIGPIPE}

var defaultSignalsInChildren sync.Once

// resetIgnoredSignals makes every child start with childSignals at their
// defaults.
//
// A signal ignored in a process stays ignored across exec, so a child of an
// application started with SIGHUP ignored — by nohup, or by a tool that runs
// commands that way — ignores the hangup that closing its terminal sends and
// outlives the terminal. Go resets the signals it handles to their defaults
// in a forked child before the exec, but not the ones it found ignored. So
// this process takes over each of those and discards them, which leaves it
// ignoring them as before while its children start with the defaults.
func resetIgnoredSignals() {
	var ignored []os.Signal
	for _, sig := range childSignals {
		if signal.Ignored(sig) {
			ignored = append(ignored, sig)
		}
	}
	if len(ignored) == 0 {
		return
	}
	c := make(chan os.Signal, 8)
	signal.Notify(c, ignored...)
	go func() {
		for range c {
		}
	}()
}

func eof(err error) error {
	if errors.Is(err, syscall.EIO) || errors.Is(err, os.ErrClosed) {
		return io.EOF
	}
	return err
}

// sameKey compares environment names, which are case-sensitive here.
func sameKey(a, b string) bool { return a == b }

// DefaultShell is the user's login shell, or a reasonable one where the
// system does not say.
func DefaultShell() string {
	if sh := os.Getenv("SHELL"); sh != "" {
		if fi, err := os.Stat(sh); err == nil && fi.Mode()&0o111 != 0 && !fi.IsDir() {
			return sh
		}
	}
	for _, sh := range []string{"/bin/bash", "/bin/sh"} {
		if fi, err := os.Stat(sh); err == nil && fi.Mode()&0o111 != 0 {
			return sh
		}
	}
	return "/bin/sh"
}
