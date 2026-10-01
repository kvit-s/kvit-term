package main

// --shots: sample screens drawn headlessly, for looking at what a change
// did. No child process is run: the bytes a program would write are fed to
// the session directly.

import (
	"fmt"
	"image"
	"image/png"
	"os"
	"path/filepath"

	"github.com/kvit-s/kvit-term"
	"github.com/kvit-s/kvit-term/view"
	kvitui "github.com/kvit-s/kvit-ui"
	"github.com/kvit-s/kvit-ui/tokens"
	"github.com/richardwilkes/toolbox/v2/geom"
	"github.com/richardwilkes/unison"
	"github.com/richardwilkes/unison/enums/align"
)

// qtSample is a sample screen: colours in all three encodings, the text
// attributes, accents, a progress line redrawn in place, and a path a
// compiler would print.
const qtSample = "$ ./demo.sh\r\n" +
	"colours:    \x1b[1;32mbold green\x1b[0m  \x1b[31mred\x1b[0m  \x1b[33myellow\x1b[0m  \x1b[34mblue\x1b[0m  " +
	"\x1b[38;5;208m256-orange\x1b[0m  \x1b[38;2;120;180;240mtruecolour\x1b[0m\r\n" +
	"attributes: \x1b[1mbold\x1b[0m  \x1b[4munderline\x1b[0m  \x1b[3mitalic\x1b[0m  \x1b[7mreverse\x1b[0m  " +
	"\x1b[9mstruck\x1b[0m  accents: é ä ø\r\n\r\n" +
	"building 10%\rbuilding 60%\rbuilt in 1.4s\x1b[K\r\n" +
	"tests:      \x1b[32m81 passed\x1b[0m, \x1b[31m0 failed\x1b[0m   see src/core/screen.cpp:212\r\n$ "

// demoSession is the session the demo window's shots show: a shell with
// the integration marks, a command whose output fills the window, and one
// that failed.
func demoSession(s *kvitterm.Session) {
	mark := func(m string) string { return "\x1b]133;" + m + "\x1b\\" }
	s.Feed([]byte("\x1b]7;file://host/home/someone/kvit-term\x1b\\"))
	s.Feed([]byte(mark("A") + "$ " + mark("B") + "go test ./...\r\n" + mark("C")))
	for _, p := range []string{"", "/pty", "/screen", "/view", "/third_party/xterm"} {
		s.Feed([]byte("\x1b[32mok\x1b[0m  \tgithub.com/kvit-s/kvit-term" + p + "\t0.8s\r\n"))
	}
	s.Feed([]byte(mark("D;0") + mark("A") + "$ " + mark("B") + "ls --color\r\n" + mark("C")))
	s.Feed([]byte("\x1b[1;34mcmd\x1b[0m  go.mod  \x1b[1;34minternal\x1b[0m  \x1b[1;34mpty\x1b[0m  README.md  " +
		"\x1b[1;34mscreen\x1b[0m  session.go  \x1b[1;34mview\x1b[0m\r\n"))
	s.Feed([]byte(mark("D;0") + mark("A") + "$ " + mark("B") + "make lint\r\n" + mark("C")))
	s.Feed([]byte("view/draw.go:212:9: \x1b[31merror:\x1b[0m undefined: cellWidth\r\n" +
		"\x1b[1mmake: *** [lint] Error 1\x1b[0m\r\n"))
	s.Feed([]byte(mark("D;2") + mark("A") + "$ " + mark("B")))
}

func writeShots(dir string) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	ui, err := kvitui.New(kvitui.Options{IgnoreDesktop: true})
	if err != nil {
		return err
	}
	var scr *unison.HeadlessScreen
	scr, err = unison.StartHeadless(unison.HeadlessConfig{Width: 1200, Height: 800})
	if err != nil {
		return err
	}
	defer scr.Stop()

	// The sample, in the terminal's default colours and a 760 × 260 view.
	var w *unison.Window
	var term *view.View
	s := kvitterm.NewSession()
	s.Dispatch = unison.InvokeTask
	scr.Do(func() {
		w, err = unison.NewWindow("kvit-term sample")
		if err != nil {
			return
		}
		term = view.New(ui.Fonts)
		term.SetFont("", 15)
		w.Content().SetBorder(unison.NewEmptyBorder(geom.NewUniformInsets(8)))
		term.SetSession(s)
		term.SetLayoutData(&unison.FlexLayoutData{HAlign: align.Fill, VAlign: align.Fill, HGrab: true, VGrab: true})
		w.Content().SetLayout(&unison.FlexLayout{Columns: 1})
		w.Content().AddChild(term)
		w.SetContentRect(geom.NewRect(0, 0, 760, 260))
		w.ToFront()
	})
	if err != nil {
		return err
	}
	s.Feed([]byte(qtSample))
	scr.Sync()
	sample := scr.CaptureWindow(w)
	if err := savePNG(filepath.Join(dir, "terminal.png"), sample); err != nil {
		return err
	}
	// Windows stay open until the end: closing the last one would end the
	// headless application.

	// The demonstration window in two Kvit themes, with the heading the
	// shell's marks give, a search open, and a selection.
	for _, theme := range []string{tokens.Dark, tokens.Light} {
		var d *demo
		ran := scr.Do(func() {
			ui.Theme.SetThemeID(theme)
			d, err = newDemo(ui, "/bin/sh", []string{"-c", "true"})
			if err != nil {
				return
			}
			d.win.SetContentRect(geom.NewRect(0, 0, 900, 560))
			d.win.ToFront()
		})
		if err != nil {
			return err
		}
		if !ran || d == nil {
			return fmt.Errorf("the %s demo window was not made: %v", theme, scr.Errors())
		}
		demoSession(d.session)
		scr.Sync()
		scr.Do(func() {
			d.term.SetScrollOffset(3)
			d.openFind()
			d.find.SetText("error")
			d.search.Next()
			d.refresh()
		})
		scr.Sync()
		img := scr.CaptureWindow(d.win.Window)
		if err := savePNG(filepath.Join(dir, "demo-"+theme+".png"), img); err != nil {
			return err
		}
		scr.Do(func() { d.win.SetContentRect(geom.NewRect(2000, 2000, 10, 10)) })
	}
	fmt.Printf("shots written to %s\n", dir)
	return nil
}

func savePNG(path string, img image.Image) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	return png.Encode(f, img)
}
