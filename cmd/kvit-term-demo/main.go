// Command kvit-term-demo is a window with one terminal: the demonstration
// program of the kvit-term, rebuilt on unison and the Kvit components.
// It shows a find bar on Ctrl+Shift+F, a heading naming the command whose
// output is at the top of the window, Ctrl+click on paths and addresses,
// and one shortcut the application keeps for itself (Ctrl+Shift+N).
//
// Where the shell is bash or PowerShell it installs the shell-integration
// snippet into a directory of its own and starts the shell with it, after
// the user's own configuration; the user's files are read and never
// written, which is the pattern an application should copy.
//
//	kvit-term-demo [program [arguments]]   run a program, or the user's shell
//	kvit-term-demo --shots DIR             draw sample screens headlessly into DIR
//	kvit-term-demo --check 20s             drive a real window with the shell, then close
package main

import (
	"flag"
	"fmt"
	"image/color"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/kvit-s/kvit-term"
	"github.com/kvit-s/kvit-term/pty"
	"github.com/kvit-s/kvit-term/screen"
	"github.com/kvit-s/kvit-term/view"
	kvitui "github.com/kvit-s/kvit-ui"
	"github.com/kvit-s/kvit-ui/palette"
	"github.com/kvit-s/kvit-ui/tokens"
	"github.com/richardwilkes/toolbox/v2/geom"
	"github.com/richardwilkes/unison"
	"github.com/richardwilkes/unison/enums/align"
	"github.com/richardwilkes/unison/enums/mod"
)

const title = "kvit-term demonstration"

func main() {
	theme := flag.String("theme", "", "light, dark, sepia or highContrast; the desktop's choice unless set")
	shots := flag.String("shots", "", "draw sample screens headlessly into this `directory`, and exit")
	check := flag.Duration("check", 0, "open a window on the user's shell, drive it through a scripted check, print the result, and close after this long")
	flag.Parse()

	if *shots != "" {
		if err := writeShots(*shots); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		return
	}

	opt := kvitui.Options{SettingsPath: kvitui.DefaultSettingsPath("kvit-term-demo")}
	if *check > 0 {
		opt = kvitui.Options{IgnoreDesktop: true}
	}
	ui, err := kvitui.New(opt)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	if *theme != "" {
		ui.Theme.SetThemeID(*theme)
	}
	program, args := pty.DefaultShell(), []string(nil)
	if flag.NArg() > 0 {
		program, args = flag.Arg(0), flag.Args()[1:]
	}
	started := time.Now()
	unison.Start(
		unison.ThemeChangedCallback(ui.Appearance.Refresh),
		unison.StartupFinishedCallback(func() {
			d, err := newDemo(ui, program, args)
			if err != nil {
				fmt.Fprintln(os.Stderr, err)
				os.Exit(1)
			}
			if *check > 0 {
				d.win.SetTitle(checkTitle)
				d.win.SetContentRect(geom.NewRect(80, 80, 900, 560))
				draw := d.term.DrawCallback
				first := true
				d.term.DrawCallback = func(gc *unison.Canvas, r geom.Rect) {
					draw(gc, r)
					if first {
						first = false
						fmt.Printf("first frame after %d ms\n", time.Since(started).Milliseconds())
						runCheck(d, started, *check)
					}
				}
			}
			d.win.ToFront()
			d.term.RequestFocus()
			if err := d.session.Start(); err != nil {
				d.note(err.Error())
			}
		}))
}

// demo is the window and what is in it.
type demo struct {
	ui      *kvitui.UI
	win     *kvitui.Window
	session *kvitterm.Session
	shell   *kvitterm.ShellIntegration
	search  *kvitterm.Search
	term    *view.View

	column  *unison.Panel
	heading *unison.Panel
	sticky  *kvitui.Label
	dir     *kvitui.Label
	back    *kvitui.Label
	findBar *unison.Panel
	find    *kvitui.SearchField
	count   *kvitui.Label
	status  *kvitui.Label
	shown   map[unison.Paneler]bool
}

func newDemo(ui *kvitui.UI, program string, args []string) (*demo, error) {
	win, err := kvitui.NewWindow(ui, title)
	if err != nil {
		return nil, err
	}
	d := &demo{ui: ui, win: win, shown: map[unison.Paneler]bool{}}
	d.session = kvitterm.NewSession()
	d.session.Dispatch = unison.InvokeTask
	d.session.Program = program
	d.session.Args = args
	if len(args) == 0 {
		d.session.Args = shellIntegrationArguments(program)
	}
	d.session.SetScrollbackLimit(10000)
	d.shell = kvitterm.NewShellIntegration(d.session)
	d.search = kvitterm.NewSearch(d.session)

	d.term = view.New(ui.Fonts)
	d.term.Accessibility.Name = "terminal"
	d.term.SetSession(d.session)
	d.term.SetSearch(d.search)
	d.term.SetShellIntegration(d.shell)
	// The application keeps these; everything else goes to the shell.
	d.term.SetReservedShortcuts("Ctrl+Shift+F", "Ctrl+Shift+N")
	d.term.SetLayoutData(&unison.FlexLayoutData{HAlign: align.Fill, VAlign: align.Fill, HGrab: true, VGrab: true})
	d.term.OnLinkActivated = d.openLink

	d.sticky = kvitui.NewLabel(ui, "")
	d.sticky.Mono = true
	d.dir = kvitui.NewLabel(ui, "")
	d.dir.Ink = kvitui.InkTextMuted
	d.back = kvitui.NewLabel(ui, "")
	d.back.Ink = kvitui.InkTextMuted
	d.heading = kvitui.Row(ui, kvitui.SizeSpace, d.sticky, d.dir, d.back)

	d.find = kvitui.NewSearchField(ui)
	d.find.Placeholder = "Find in the scrollback"
	d.find.OnChange = func(text string) { d.search.SetQuery(text); d.refresh() }
	prev, next, done := kvitui.NewButton(ui, "Previous"), kvitui.NewButton(ui, "Next"), kvitui.NewButton(ui, "Close")
	prev.OnClick = func() { d.search.Previous(); d.refresh() }
	next.OnClick = func() { d.search.Next(); d.refresh() }
	done.OnClick = d.closeFind
	d.count = kvitui.NewLabel(ui, "")
	d.findBar = kvitui.Row(ui, kvitui.SizeSpace, kvitui.Width(ui, kvitui.Px(260), d.find), d.count, prev, next, done)
	d.status = kvitui.NewLabel(ui, "")

	d.column = kvitui.Column(ui, kvitui.SizeSpace, d.term)
	d.column.SetBorder(kvitui.Padding(ui, kvitui.SizeSpace))
	win.SetBody(d.column)
	d.term.OnChange = d.refresh
	d.applyTheme()
	ui.OnChanged(d.applyTheme)

	d.session.Observe(func(e kvitterm.Event) {
		switch e.Kind {
		case kvitterm.Exited:
			d.note(fmt.Sprintf("the shell exited with status %d", e.ExitCode))
		case kvitterm.Failed:
			d.note(e.Message)
		case kvitterm.TitleChanged:
			if t := d.session.Title(); t != "" && d.win.Title() != checkTitle {
				d.win.SetTitle(t)
			}
		}
	})
	d.shell.Observe(func(e kvitterm.ShellEvent) {
		if e.Kind == kvitterm.CommandFinished && e.Command.ExitCode > 0 {
			d.note(fmt.Sprintf("%q exited with status %d", e.Command.Text, e.Command.ExitCode))
		}
		d.refresh()
	})
	win.OnKeyDown = d.keyDown
	d.refresh()
	return d, nil
}

func (d *demo) keyDown(key unison.KeyCode, mods mod.Modifiers, _ bool) bool {
	if d.term.Claims(key, mods) {
		return false
	}
	cmd, shift := mods.OSMenuCommandDown(), mods.ShiftDown()
	switch {
	case cmd && shift && key == unison.KeyF:
		d.openFind()
		return true
	case cmd && shift && key == unison.KeyN:
		d.note("Ctrl+Shift+N is kept by this application, so the shell never saw it")
		return true
	case d.shown[d.findBar] && key == unison.KeyEscape:
		d.closeFind()
		return true
	case d.shown[d.findBar] && (key == unison.KeyReturn || key == unison.KeyNumPadEnter) && !d.term.Focused():
		if shift {
			d.search.Previous()
		} else {
			d.search.Next()
		}
		d.refresh()
		return true
	}
	return false
}

// applyTheme gives the terminal the Kvit theme: its ground, text, accent
// and selection. The sixteen colours are a terminal scheme chosen for the
// ground's darkness rather than taken from the theme, since to a program
// red means an error, which is what kvit-works does too.
func (d *demo) applyTheme() {
	t := d.ui.Theme.Tokens()
	nrgba := func(c palette.Color) color.NRGBA {
		u := kvitui.Color(c)
		return color.NRGBA{R: uint8(u.Red()), G: uint8(u.Green()), B: uint8(u.Blue()), A: 255}
	}
	p := screen.DefaultPalette()
	p.Background = nrgba(t.CodePanelBackground)
	p.Foreground = nrgba(t.TextPrimary)
	p.Cursor = nrgba(t.Accent)
	p.CursorText = p.Background
	p.SelectionBackground = nrgba(t.SelectionActiveTint)
	p.SelectionForeground = p.Foreground
	ansi := darkANSI
	if d.ui.Theme.ResolvedTheme() == tokens.Light || d.ui.Theme.ResolvedTheme() == tokens.Sepia {
		ansi = lightANSI
	}
	for i, h := range ansi {
		p.ANSI[i] = screen.ParseHex(h)
	}
	d.term.SetPalette(p)
	d.term.SetFont("", float32(d.ui.Size(kvitui.RoleBody)))
}

// The sixteen colours kvit-works gives its terminals, for dark and for
// light grounds.
var (
	darkANSI = [16]string{"#1c1f24", "#e06c75", "#98c379", "#e5c07b", "#61afef", "#c678dd", "#56b6c2", "#c8ccd4",
		"#5c6370", "#ff7b86", "#b6e3a1", "#ffd88a", "#7cc4ff", "#dc9cf0", "#78d3dd", "#ffffff"}
	lightANSI = [16]string{"#2f3337", "#c0392b", "#2e7d32", "#a06800", "#1565c0", "#7b1fa2", "#00838f", "#5c6370",
		"#7a828c", "#e05545", "#3f9e45", "#c08a00", "#2f7fe0", "#9b45c0", "#0f9aa8", "#1c1f24"}
)

// show puts the optional parts of the column in place: the heading when
// the shell has said something or a search is open, the find bar while it
// is open, and the status line once there is something to say. They are
// taken out rather than hidden, since a hidden child keeps its room.
func (d *demo) layout(heading, find, status bool) {
	want := []unison.Paneler{}
	if heading {
		want = append(want, d.heading)
	}
	want = append(want, d.term)
	if find {
		want = append(want, d.findBar)
	}
	if status {
		want = append(want, d.status)
	}
	same := len(want) == len(d.column.Children())
	for i, p := range want {
		if same && d.column.Children()[i] != p.AsPanel() {
			same = false
		}
	}
	if same {
		return
	}
	d.column.RemoveAllChildren()
	d.shown = map[unison.Paneler]bool{}
	for _, p := range want {
		d.column.AddChild(p)
		d.shown[p] = true
	}
	d.column.MarkForLayoutRecursively()
	d.column.MarkForRedraw()
}

// refresh brings the heading, the find bar's count and the scroll badge up
// to date.
func (d *demo) refresh() {
	sticky := d.term.StickyCommand()
	d.sticky.Text = ""
	if sticky != "" {
		d.sticky.Text = "$ " + sticky
	}
	d.dir.Text = d.shell.Directory()
	d.back.Text = ""
	if n := d.term.ScrollOffset(); n > 0 {
		d.back.Text = fmt.Sprintf("%d lines back", n)
	}
	switch n := d.search.MatchCount(); {
	case d.search.Query() == "":
		d.count.Text = ""
	case n == 0:
		d.count.Text = "no matches"
	default:
		d.count.Text = fmt.Sprintf("%d of %d", d.search.Current()+1, n)
	}
	findOpen := d.shown[d.findBar]
	heading := d.shell.Active() || d.back.Text != "" || findOpen
	d.layout(heading, findOpen, d.status.Text != "")
	for _, l := range []*kvitui.Label{d.sticky, d.dir, d.back, d.count} {
		l.MarkForLayoutAndRedraw()
	}
	d.heading.MarkForLayoutAndRedraw()
}

func (d *demo) note(s string) {
	d.status.Text = s
	d.status.MarkForLayoutAndRedraw()
	d.refresh()
}

func (d *demo) openFind() {
	d.shown[d.findBar] = true
	d.refresh()
	d.find.RequestFocus()
}

func (d *demo) closeFind() {
	d.find.SetText("")
	d.search.Clear()
	delete(d.shown, d.findBar)
	d.layout(d.shell.Active() || d.term.ScrollOffset() > 0, false, d.status.Text != "")
	d.term.RequestFocus()
	d.refresh()
}

// openLink is what the demonstration does with a link: an address opens in
// the browser, and a path is only named, since which editor to open it in
// is an application's decision.
func (d *demo) openLink(link string, line, _ int) {
	switch {
	case strings.HasPrefix(link, "http://"), strings.HasPrefix(link, "https://"), strings.HasPrefix(link, "file:"):
		if err := unison.OpenBrowser(link); err != nil {
			d.note(err.Error())
		}
	case line > 0:
		d.note(fmt.Sprintf("open %s at line %d", link, line))
	default:
		d.note("open " + link)
	}
}

// shellIntegrationArguments starts bash or PowerShell with the snippet. It
// is written to a directory of this program's own; bash is given an init
// file that sources the user's ~/.bashrc first and then the snippet, and
// PowerShell sources the snippet after its profile. Nothing of the user's
// is written.
func shellIntegrationArguments(program string) []string {
	base := strings.ToLower(filepath.Base(program))
	dir := filepath.Join(os.TempDir(), fmt.Sprintf("kvit-term-demo-%d", os.Getpid()))
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil
	}
	switch {
	case strings.HasPrefix(base, "bash"):
		snippet := filepath.Join(dir, kvitterm.ShellScriptName(kvitterm.Bash))
		init := filepath.Join(dir, "init.bash")
		if os.WriteFile(snippet, []byte(kvitterm.ShellScript(kvitterm.Bash)), 0o600) != nil ||
			os.WriteFile(init, []byte("# Written by kvit-term-demo. Sources the user's own configuration first.\n"+
				"[ -f ~/.bashrc ] && . ~/.bashrc\n. '"+snippet+"'\n"), 0o600) != nil {
			return nil
		}
		return []string{"--rcfile", init, "-i"}
	case strings.HasPrefix(base, "pwsh"), strings.HasPrefix(base, "powershell"):
		snippet := filepath.Join(dir, kvitterm.ShellScriptName(kvitterm.PowerShell))
		if os.WriteFile(snippet, []byte(kvitterm.ShellScript(kvitterm.PowerShell)), 0o600) != nil {
			return nil
		}
		return []string{"-NoLogo", "-NoExit", "-Command", ". '" + snippet + "'"}
	}
	return nil
}
