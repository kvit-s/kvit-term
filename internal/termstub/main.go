// termstub is the program the tests put on the other end of a
// pseudo-terminal. Every scenario a test needs is a mode of this one
// program — whether it sees a terminal, what size it is told the terminal
// is, and each family of escape sequence the screen has to interpret — so
// no test depends on a shell, its version or its startup files.
//
//	termstub isatty            report whether the standard streams are terminals
//	termstub size              report the terminal's size
//	termstub size-watch        report it, then again after the first resize (Unix)
//	termstub echo              repeat each line typed as echo:<line>; "quit" ends
//	termstub exit N            exit with status N
//	termstub sleep             wait to be ended
//	termstub scenario NAME     write one named scenario of escape sequences
//	termstub raw               take the terminal out of line mode and print
//	                           each read of input in hex, as in:1b5b41; "q" ends
package main

import (
	"bufio"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

func main() {
	mode := "isatty"
	if len(os.Args) > 1 {
		mode = os.Args[1]
	}
	arg := ""
	if len(os.Args) > 2 {
		arg = os.Args[2]
	}
	switch mode {
	case "isatty":
		fmt.Printf("stdin %d stdout %d stderr %d\n", b(isTerminal(os.Stdin)), b(isTerminal(os.Stdout)), b(isTerminal(os.Stderr)))
	case "size":
		fmt.Println(terminalSize())
	case "size-watch":
		fmt.Println(terminalSize())
		waitForResize()
		fmt.Println(terminalSize())
	case "echo":
		// Proves the write path and the child's line discipline.
		in := bufio.NewScanner(os.Stdin)
		for in.Scan() {
			text := strings.TrimRight(in.Text(), "\r\n")
			if text == "quit" {
				break
			}
			fmt.Printf("echo:%s\n", text)
		}
	case "exit":
		code, _ := strconv.Atoi(arg)
		os.Exit(code)
	case "sleep":
		for {
			time.Sleep(time.Hour)
		}
	case "raw":
		// Every byte a key or the mouse sends, exactly as the program gets
		// it: no line editing, no echo, and Ctrl+C a byte rather than a
		// signal.
		restore := makeRaw()
		defer restore()
		fmt.Print("raw ready\r\n")
		buf := make([]byte, 256)
		for {
			n, err := os.Stdin.Read(buf)
			if n > 0 {
				fmt.Printf("in:%x\r\n", buf[:n])
				if string(buf[:n]) == "q" {
					return
				}
			}
			if err != nil {
				return
			}
		}
	case "scenario":
		if arg == "" {
			arg = "colour"
		}
		scenario(arg)
	default:
		fmt.Fprintf(os.Stderr, "termstub: unknown mode %s\n", mode)
		os.Exit(2)
	}
}

func b(v bool) int {
	if v {
		return 1
	}
	return 0
}

// Every escape sequence is written out in full rather than through a
// helper, so that a failure points at the exact bytes that produced it.
func scenario(name string) {
	w := os.Stdout
	switch name {
	case "colour":
		fmt.Fprint(w, "\x1b[31mred\x1b[0m ")
		fmt.Fprint(w, "\x1b[1;32mbold green\x1b[0m ")
		fmt.Fprint(w, "\x1b[38;5;208m256-orange\x1b[0m ")
		fmt.Fprint(w, "\x1b[38;2;120;180;240mtruecolour\x1b[0m ")
		fmt.Fprint(w, "\x1b[4munderline\x1b[24m \x1b[7mreverse\x1b[27m\n")
	case "altscreen":
		fmt.Fprint(w, "before\n\x1b[?1049h\x1b[2J\x1b[Hinside the alternate screen\x1b[?1049lafter\n")
	case "progress":
		for p := 0; p <= 100; p += 25 {
			fmt.Fprintf(w, "\rworking: %d%%", p)
		}
		// Erase to the end of the line rather than padding with spaces,
		// which is what a tool that redraws a progress line does.
		fmt.Fprint(w, "\rdone\x1b[K\n")
	case "wide":
		fmt.Fprint(w, "[日本]\n")
		fmt.Fprint(w, "é = é\n")
	case "scroll":
		for i := 1; i <= 50; i++ {
			fmt.Fprintf(w, "line %d\n", i)
		}
	case "marks":
		// The shell-integration marks: a prompt, the command typed at it,
		// its output, and the status it finished with.
		fmt.Fprint(w, "\x1b]7;file://host/tmp\x1b\\")
		fmt.Fprint(w, "\x1b]133;A\x1b\\$ \x1b]133;B\x1b\\ls -l\n")
		fmt.Fprint(w, "\x1b]133;C\x1b\\total 0\nfile.txt\n")
		fmt.Fprint(w, "\x1b]133;D;0\x1b\\")
	case "title":
		fmt.Fprint(w, "\x1b]0;a title\x1b\\")
	case "bell":
		fmt.Fprint(w, "\a")
	case "links":
		fmt.Fprint(w, "see src/core/screen.cpp:42 and https://example.invalid/x\n")
	case "flood":
		// Much more output than a pipe holds, for flow control.
		line := strings.Repeat("x", 79) + "\n"
		for i := 0; i < 20000; i++ {
			fmt.Fprint(w, line)
		}
		fmt.Fprint(w, "flood done\n")
	default:
		fmt.Fprintf(w, "unknown scenario: %s\n", name)
	}
}
