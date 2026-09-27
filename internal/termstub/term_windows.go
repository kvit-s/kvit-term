package main

import (
	"fmt"
	"os"
	"time"

	"golang.org/x/sys/windows"
)

// isTerminal reports a console handle, which is what a child on a
// pseudoconsole has; a pipe here means the pseudoconsole was not attached
// and the parent's handles were inherited instead.
func isTerminal(f *os.File) bool {
	var mode uint32
	return windows.GetConsoleMode(windows.Handle(f.Fd()), &mode) == nil
}

// terminalSize reports the window, which is what a program should size
// itself to, and the buffer, which can be taller; which of them a
// pseudoconsole sets is the kind of thing that differs.
func terminalSize() string {
	var info windows.ConsoleScreenBufferInfo
	if err := windows.GetConsoleScreenBufferInfo(windows.Handle(os.Stdout.Fd()), &info); err != nil {
		return "size unknown"
	}
	w := info.Window
	return fmt.Sprintf("size %dx%d buffer %dx%d", w.Right-w.Left+1, w.Bottom-w.Top+1, info.Size.X, info.Size.Y)
}

// waitForResize polls, since Windows has no resize signal.
func waitForResize() {
	first := terminalSize()
	for terminalSize() == first {
		time.Sleep(20 * time.Millisecond)
	}
}

// makeRaw turns off line input, echo and Ctrl+C processing, and asks the
// console for the escape sequences a terminal sends, and returns what puts
// the modes back.
func makeRaw() func() {
	in := windows.Handle(os.Stdin.Fd())
	var old uint32
	if windows.GetConsoleMode(in, &old) != nil {
		return func() {}
	}
	raw := old &^ (windows.ENABLE_ECHO_INPUT | windows.ENABLE_LINE_INPUT | windows.ENABLE_PROCESSED_INPUT)
	raw |= windows.ENABLE_VIRTUAL_TERMINAL_INPUT
	windows.SetConsoleMode(in, raw)
	return func() { windows.SetConsoleMode(in, old) }
}
