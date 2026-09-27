//go:build unix

package main

import (
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"golang.org/x/sys/unix"
)

func isTerminal(f *os.File) bool {
	_, err := unix.IoctlGetTermios(int(f.Fd()), ioctlGetTermios)
	return err == nil
}

func terminalSize() string {
	ws, err := unix.IoctlGetWinsize(int(os.Stdout.Fd()), unix.TIOCGWINSZ)
	if err != nil {
		return "size unknown"
	}
	return fmt.Sprintf("size %dx%d", ws.Col, ws.Row)
}

// waitForResize returns the first time the terminal is resized, which
// arrives as SIGWINCH.
func waitForResize() {
	c := make(chan os.Signal, 1)
	signal.Notify(c, syscall.SIGWINCH)
	<-c
}

// makeRaw turns off line editing, echo and the signal keys, and returns
// what puts them back.
func makeRaw() func() {
	fd := int(os.Stdin.Fd())
	old, err := unix.IoctlGetTermios(fd, ioctlGetTermios)
	if err != nil {
		return func() {}
	}
	raw := *old
	raw.Iflag &^= unix.IGNBRK | unix.BRKINT | unix.PARMRK | unix.ISTRIP | unix.INLCR | unix.IGNCR | unix.ICRNL | unix.IXON
	raw.Oflag &^= unix.OPOST
	raw.Lflag &^= unix.ECHO | unix.ECHONL | unix.ICANON | unix.ISIG | unix.IEXTEN
	raw.Cflag &^= unix.CSIZE | unix.PARENB
	raw.Cflag |= unix.CS8
	raw.Cc[unix.VMIN] = 1
	raw.Cc[unix.VTIME] = 0
	unix.IoctlSetTermios(fd, ioctlSetTermios, &raw)
	return func() { unix.IoctlSetTermios(fd, ioctlSetTermios, old) }
}
