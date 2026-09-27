//go:build windows

package pty

import (
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"slices"
	"strings"
	"sync"
	"time"
	"unicode/utf16"
	"unsafe"

	"golang.org/x/sys/windows"
)

// Pty is a child process on a pseudoconsole.
//
// Windows has no fork, no controlling terminal and no SIGWINCH. It has the
// pseudoconsole: a kernel object made with a pair of ordinary pipes, which a
// child is attached to through a process-creation attribute. The console
// host on the other side turns the child's console output into the escape
// sequences a Unix terminal would see, so nothing above this file differs.
//
// Two behaviours do differ and show up in tests: the console host repaints
// the whole screen on a resize, and it writes sequences of its own into the
// stream (hiding the cursor, clearing the screen, setting the title from the
// program's path), sometimes between the child's characters.
type Pty struct {
	console windows.Handle // the pseudoconsole
	in      windows.Handle // this end of the child's input
	out     windows.Handle // this end of the child's output
	process windows.Handle
	thread  windows.Handle
	pid     int
	attrs   *windows.ProcThreadAttributeListContainer

	mu            sync.Mutex
	consoleClosed bool
	done          chan struct{}
	code          int
	columns, rows int
}

// startMu is held while a child starts, because this process's standard
// handles are cleared for the duration; see Start.
var startMu sync.Mutex

// Start creates a pseudoconsole and starts the child on it. A program that
// cannot be started is an error here; one that starts and exits at once is
// a success, whose exit Done reports.
func Start(p Params) (t *Pty, err error) {
	if err := p.normalize(); err != nil {
		return nil, err
	}
	path, err := exec.LookPath(p.Program)
	if err != nil {
		return nil, fmt.Errorf("could not start %s: %w", p.Program, err)
	}
	t = &Pty{done: make(chan struct{}), columns: p.Columns, rows: p.Rows}
	defer func() {
		if err != nil {
			t.release()
		}
	}()

	var inRead, outWrite windows.Handle
	if err := windows.CreatePipe(&inRead, &t.in, nil, 0); err != nil {
		return nil, fmt.Errorf("could not create the pipes for a pseudoconsole: %w", err)
	}
	if err := windows.CreatePipe(&t.out, &outWrite, nil, 0); err != nil {
		windows.CloseHandle(inRead)
		return nil, fmt.Errorf("could not create the pipes for a pseudoconsole: %w", err)
	}
	err = windows.CreatePseudoConsole(windows.Coord{X: int16(p.Columns), Y: int16(p.Rows)}, inRead, outWrite, 0, &t.console)
	// The console host keeps its own copies of both, whether it started or
	// not.
	windows.CloseHandle(inRead)
	windows.CloseHandle(outWrite)
	if err != nil {
		t.console = 0
		return nil, fmt.Errorf("could not create a pseudoconsole: %w", err)
	}

	if t.attrs, err = windows.NewProcThreadAttributeList(1); err != nil {
		return nil, fmt.Errorf("could not prepare the process attributes: %w", err)
	}
	// The attribute's value is the pseudoconsole handle itself, not a
	// pointer to it.
	if err = t.attrs.Update(windows.PROC_THREAD_ATTRIBUTE_PSEUDOCONSOLE,
		*(*unsafe.Pointer)(unsafe.Pointer(&t.console)), unsafe.Sizeof(t.console)); err != nil {
		return nil, fmt.Errorf("could not attach the pseudoconsole to the child: %w", err)
	}

	cmdline, err := windows.UTF16PtrFromString(windows.ComposeCommandLine(append([]string{path}, p.Args...)))
	if err != nil {
		return nil, err
	}
	env := environmentBlock(p.environment())
	var dir *uint16
	if p.Dir != "" {
		if dir, err = windows.UTF16PtrFromString(p.Dir); err != nil {
			return nil, err
		}
	}
	si := &windows.StartupInfoEx{
		StartupInfo:             windows.StartupInfo{Cb: uint32(unsafe.Sizeof(windows.StartupInfoEx{}))},
		ProcThreadAttributeList: t.attrs.List(),
	}
	var pi windows.ProcessInformation

	// This process's own standard handles are kept away from the child.
	// Windows copies them into a child's process parameters, where they take
	// precedence over the ones the pseudoconsole installs; where they are
	// pipes — any redirected parent, a test runner included — the child
	// writes to the parent's pipe instead of the terminal, decides it is not
	// on a terminal and turns its colours off. Clearing them for the call,
	// rather than passing empty ones with STARTF_USESTDHANDLES, leaves the
	// console to fill the child's in from the pseudoconsole.
	startMu.Lock()
	saved := [3]windows.Handle{}
	ids := [3]uint32{windows.STD_INPUT_HANDLE, windows.STD_OUTPUT_HANDLE, windows.STD_ERROR_HANDLE}
	for i, id := range ids {
		saved[i], _ = windows.GetStdHandle(id)
		windows.SetStdHandle(id, 0)
	}
	err = windows.CreateProcess(nil, cmdline, nil, nil, false,
		windows.EXTENDED_STARTUPINFO_PRESENT|windows.CREATE_UNICODE_ENVIRONMENT, &env[0], dir, &si.StartupInfo, &pi)
	for i, id := range ids {
		windows.SetStdHandle(id, saved[i])
	}
	startMu.Unlock()
	if err != nil {
		return nil, fmt.Errorf("could not start %s: %w", p.Program, err)
	}
	t.process, t.thread, t.pid = pi.Process, pi.Thread, int(pi.ProcessId)

	go func() {
		windows.WaitForSingleObject(t.process, windows.INFINITE)
		var code uint32
		windows.GetExitCodeProcess(t.process, &code)
		t.mu.Lock()
		t.code = int(code)
		t.mu.Unlock()
		// The child has gone, but its last output may still be in the
		// console host. Closing the console makes the host flush it and
		// then close the pipe, so a reader sees everything and then the end.
		t.closeConsole()
		close(t.done)
	}()
	return t, nil
}

// environmentBlock is the environment as CreateProcess takes it: KEY=value
// strings, each ended by a null, sorted by name without regard to case, and
// a final null.
func environmentBlock(env []string) []uint16 {
	env = slices.Clone(env)
	slices.SortStableFunc(env, func(a, b string) int {
		return strings.Compare(strings.ToUpper(keyOf(a)), strings.ToUpper(keyOf(b)))
	})
	var block []uint16
	for _, e := range env {
		if strings.IndexByte(e, 0) >= 0 {
			continue
		}
		block = append(block, utf16.Encode([]rune(e))...)
		block = append(block, 0)
	}
	return append(block, 0)
}

// Read reads what the child wrote, through the console host. It returns
// io.EOF once the pseudoconsole is closed.
func (t *Pty) Read(b []byte) (int, error) {
	var n uint32
	err := windows.ReadFile(t.out, b, &n, nil)
	if err != nil {
		if errors.Is(err, windows.ERROR_BROKEN_PIPE) || errors.Is(err, windows.ERROR_INVALID_HANDLE) {
			return int(n), io.EOF
		}
		return int(n), err
	}
	if n == 0 {
		return 0, io.EOF
	}
	return int(n), nil
}

// Write sends bytes to the child's input.
func (t *Pty) Write(b []byte) (int, error) {
	var n uint32
	err := windows.WriteFile(t.in, b, &n, nil)
	return int(n), err
}

// Resize tells the console host the new size. It repaints the whole screen
// in response.
func (t *Pty) Resize(columns, rows int) error {
	columns, rows = max(1, columns), max(1, rows)
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.consoleClosed || (columns == t.columns && rows == t.rows) {
		return nil
	}
	t.columns, t.rows = columns, rows
	return windows.ResizePseudoConsole(t.console, windows.Coord{X: int16(columns), Y: int16(rows)})
}

// Pid is the child's process id.
func (t *Pty) Pid() int { return t.pid }

// Done is closed when the child has exited.
func (t *Pty) Done() <-chan struct{} { return t.done }

// ExitCode is the child's exit status once Done is closed.
func (t *Pty) ExitCode() int {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.code
}

// closeConsole closes the pseudoconsole, once. It can wait until the host
// has written what it holds, so it is not called with the lock held.
func (t *Pty) closeConsole() {
	t.mu.Lock()
	console := t.console
	already := t.consoleClosed || console == 0
	t.consoleClosed = true
	t.mu.Unlock()
	if !already {
		windows.ClosePseudoConsole(console)
	}
}

// Hangup closes the pseudoconsole, which is what a closed window looks
// like to the child: the host tells it the console has gone. On Unix a
// program that does not handle the hangup dies at once; here the child is
// asked rather than told, so one still running three seconds later is
// ended.
func (t *Pty) Hangup() {
	go t.closeConsole()
	go func() {
		select {
		case <-t.done:
		case <-time.After(3 * time.Second):
			t.Terminate()
		}
	}()
}

// Terminate ends the child.
func (t *Pty) Terminate() {
	select {
	case <-t.done:
	default:
		windows.TerminateProcess(t.process, 1)
	}
}

// Kill ends the child; on Windows it is the same as Terminate.
func (t *Pty) Kill() { t.Terminate() }

// Release frees the handles once the child has exited and its output has
// been read.
func (t *Pty) Release() {
	go func() {
		<-t.done
		t.release()
	}()
}

func (t *Pty) release() {
	t.closeConsole()
	for _, h := range []*windows.Handle{&t.in, &t.out, &t.thread, &t.process} {
		if *h != 0 {
			windows.CloseHandle(*h)
			*h = 0
		}
	}
	if t.attrs != nil {
		t.attrs.Delete()
		t.attrs = nil
	}
}

// sameKey compares environment names, which ignore case on Windows.
func sameKey(a, b string) bool { return strings.EqualFold(a, b) }

// DefaultShell is PowerShell where it is installed, and the command prompt
// where it is not; cmd.exe is always present, which is the only reason it
// is the fallback.
func DefaultShell() string {
	for _, name := range []string{"pwsh.exe", "powershell.exe"} {
		if path, err := exec.LookPath(name); err == nil {
			return path
		}
	}
	if comspec := os.Getenv("COMSPEC"); comspec != "" {
		return comspec
	}
	return "cmd.exe"
}
