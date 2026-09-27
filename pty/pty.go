// Package pty runs a program on a pseudo-terminal: a child process that
// believes it is talking to a person.
//
// A pseudo-terminal is a pair of kernel devices. The child has one end as
// its controlling terminal and cannot tell it from a real one, so it keeps
// its colours on, flushes a line at a time, draws progress bars, and has
// somewhere to ask a question. A Pty is the other end: what the child writes
// is read from it, what is written to it arrives at the child's standard
// input, and a resize reaches the child as the window-size change it
// expects.
//
// Linux and macOS use openpty; Windows uses the pseudoconsole, from Windows
// 10 version 1809. There is no emulation here: what is read is a raw
// terminal stream, which the screen package interprets.
package pty

import (
	"errors"
	"os"
	"slices"
	"strconv"
	"strings"
)

// Params say what to run and on how large a terminal.
type Params struct {
	Program string   // an absolute path, or a name looked up on PATH
	Args    []string // the arguments after the program's name
	Dir     string   // the working directory; "" inherits this process's
	// Env is the child's environment as KEY=value lines; nil is this
	// process's own.
	Env     []string
	Columns int
	Rows    int
	// Term and ColorTerm are added to the environment unless it sets them.
	// TERM is what a program reads to decide which escape sequences it may
	// use, so a terminal that claims more than it implements produces
	// garbage; the emulator implements what xterm-256color describes.
	// "" uses xterm-256color and truecolor.
	Term      string
	ColorTerm string
}

// ErrNoProgram is returned for Params without a program.
var ErrNoProgram = errors.New("no program was named")

// environment is the child's environment: the one asked for, or this
// process's, with TERM and COLORTERM added unless it sets them, and COLUMNS
// and LINES set to the size, which some programs read instead of asking
// the terminal. They go stale on the first resize, which is why telling the
// terminal the size is what actually matters.
func (p Params) environment() []string {
	env := p.Env
	if env == nil {
		env = os.Environ()
	}
	env = slices.Clone(env)
	term, colorTerm := p.Term, p.ColorTerm
	if term == "" {
		term = "xterm-256color"
	}
	if colorTerm == "" {
		colorTerm = "truecolor"
	}
	if !hasKey(env, "TERM") {
		env = append(env, "TERM="+term)
	}
	if !hasKey(env, "COLORTERM") {
		env = append(env, "COLORTERM="+colorTerm)
	}
	env = setKey(env, "COLUMNS", strconv.Itoa(p.Columns))
	env = setKey(env, "LINES", strconv.Itoa(p.Rows))
	return env
}

func keyOf(entry string) string {
	k, _, _ := strings.Cut(entry, "=")
	return k
}

func hasKey(env []string, key string) bool {
	for _, e := range env {
		if sameKey(keyOf(e), key) {
			return true
		}
	}
	return false
}

func setKey(env []string, key, value string) []string {
	out := env[:0]
	for _, e := range env {
		if !sameKey(keyOf(e), key) {
			out = append(out, e)
		}
	}
	return append(out, key+"="+value)
}

func (p *Params) normalize() error {
	if p.Program == "" {
		return ErrNoProgram
	}
	if p.Columns <= 0 {
		p.Columns = 80
	}
	if p.Rows <= 0 {
		p.Rows = 24
	}
	return nil
}
