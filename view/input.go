package view

import (
	"runtime"
	"strings"
	"unicode"

	"github.com/kvit-s/kvit-term"
	"github.com/kvit-s/kvit-term/screen"
	"github.com/richardwilkes/toolbox/v2/geom"
	"github.com/richardwilkes/unison"
	"github.com/richardwilkes/unison/enums/mod"
)

// shortcut is a key with modifiers, as the application names one.
type shortcut struct {
	key  unison.KeyCode
	mods mod.Modifiers
}

// ParseShortcut reads a key sequence as Qt writes one: "Ctrl+Shift+T",
// "F6", "Shift+F6", "Alt+Return". Ctrl means the platform's command key,
// Command on macOS, as it does in Qt; Meta means the Control key there.
func ParseShortcut(s string) (key unison.KeyCode, mods mod.Modifiers, ok bool) {
	parts := strings.Split(s, "+")
	if len(parts) > 1 && parts[len(parts)-1] == "" {
		// "Ctrl++" names the plus key.
		parts = append(parts[:len(parts)-2], "+")
	}
	for _, p := range parts[:len(parts)-1] {
		switch strings.ToLower(strings.TrimSpace(p)) {
		case "ctrl", "control":
			if runtime.GOOS == "darwin" {
				mods |= mod.Command
			} else {
				mods |= mod.Control
			}
		case "meta":
			if runtime.GOOS == "darwin" {
				mods |= mod.Control
			} else {
				mods |= mod.Command
			}
		case "cmd", "command":
			mods |= mod.Command
		case "shift":
			mods |= mod.Shift
		case "alt", "option":
			mods |= mod.Option
		default:
			return 0, 0, false
		}
	}
	key = keyNamed(strings.TrimSpace(parts[len(parts)-1]))
	return key, mods, key != 0
}

var namedKeys = map[string]unison.KeyCode{
	"return": unison.KeyReturn, "enter": unison.KeyReturn, "tab": unison.KeyTab,
	"backspace": unison.KeyBackspace, "esc": unison.KeyEscape, "escape": unison.KeyEscape,
	"space": unison.KeySpace, "ins": unison.KeyInsert, "insert": unison.KeyInsert,
	"del": unison.KeyDelete, "delete": unison.KeyDelete, "home": unison.KeyHome, "end": unison.KeyEnd,
	"pgup": unison.KeyPageUp, "pageup": unison.KeyPageUp, "pgdown": unison.KeyPageDown,
	"pagedown": unison.KeyPageDown, "up": unison.KeyUp, "down": unison.KeyDown,
	"left": unison.KeyLeft, "right": unison.KeyRight, "menu": unison.KeyMenu,
}

// punctuation keys by the character they type unshifted, on a US layout.
var punctuationKeys = map[rune]unison.KeyCode{
	'\'': unison.KeyApostrophe, ',': unison.KeyComma, '-': unison.KeyMinus, '.': unison.KeyPeriod,
	'/': unison.KeySlash, ';': unison.KeySemiColon, '=': unison.KeyEqual, '[': unison.KeyOpenBracket,
	'\\': unison.KeyBackslash, ']': unison.KeyCloseBracket, '`': unison.KeyBackQuote,
}

func keyNamed(name string) unison.KeyCode {
	if k, ok := namedKeys[strings.ToLower(name)]; ok {
		return k
	}
	r := []rune(name)
	if len(r) == 1 {
		c := unicode.ToUpper(r[0])
		switch {
		case c >= 'A' && c <= 'Z':
			return unison.KeyA + unison.KeyCode(c-'A')
		case c >= '0' && c <= '9':
			return unison.Key0 + unison.KeyCode(c-'0')
		}
		if k, ok := punctuationKeys[r[0]]; ok {
			return k
		}
	}
	if len(name) > 1 && (name[0] == 'F' || name[0] == 'f') {
		n := 0
		for _, c := range name[1:] {
			if c < '0' || c > '9' {
				return 0
			}
			n = n*10 + int(c-'0')
		}
		if n >= 1 && n <= 25 {
			return unison.KeyF1 + unison.KeyCode(n-1)
		}
	}
	return 0
}

// SetReservedShortcuts names the key sequences the view must leave alone,
// written as ParseShortcut reads them. They reach the application instead,
// and they take precedence over the view's own copy, paste and scrolling
// keys. Everything else goes to the program in the terminal, Ctrl+C
// included, because inside a terminal a key usually belongs to what is
// running there. Sequences that cannot be read are ignored.
func (v *View) SetReservedShortcuts(seqs ...string) {
	v.reserved = v.reserved[:0]
	for _, s := range seqs {
		if k, m, ok := ParseShortcut(s); ok {
			v.reserved = append(v.reserved, shortcut{k, m})
		}
	}
}

const shortcutMods = mod.Shift | mod.Control | mod.Option | mod.Command

// Claims reports whether the view takes a key for the program in it: while
// it has the keyboard and a program is running, every key but the reserved
// ones. A window that sees keys before the focused panel, as a kvit-ui
// window's OnKeyDown does, asks this before trying its own shortcuts, so
// that Ctrl+K reaches the shell rather than the application; kvit-works'
// terminal does the same with Qt's shortcut override.
func (v *View) Claims(key unison.KeyCode, mods mod.Modifiers) bool {
	return v.Focused() && v.session != nil && v.session.Running() && !v.isReserved(key, mods)
}

func (v *View) isReserved(key unison.KeyCode, mods mod.Modifiers) bool {
	if key == unison.KeyNumPadEnter {
		key = unison.KeyReturn
	}
	for _, r := range v.reserved {
		if r.key == key && r.mods == mods&shortcutMods {
			return true
		}
	}
	return false
}

// terminalMods are the modifiers a key carries to the program: the Control
// key, Alt, and Shift. On macOS Option types characters of its own, so it is
// the Alt of named keys only, and Command belongs to the application.
func terminalMods(mods mod.Modifiers, named bool) screen.Modifiers {
	var m screen.Modifiers
	if mods.ShiftDown() {
		m |= screen.Shift
	}
	if mods.ControlDown() {
		m |= screen.Ctrl
	}
	if mods.OptionDown() && (named || runtime.GOOS != "darwin") {
		m |= screen.Alt
	}
	return m
}

var terminalKeys = map[unison.KeyCode]screen.Key{
	unison.KeyReturn: screen.KeyEnter, unison.KeyNumPadEnter: screen.KeyEnter, unison.KeyTab: screen.KeyTab,
	unison.KeyBackspace: screen.KeyBackspace, unison.KeyEscape: screen.KeyEscape,
	unison.KeyUp: screen.KeyUp, unison.KeyDown: screen.KeyDown, unison.KeyLeft: screen.KeyLeft,
	unison.KeyRight: screen.KeyRight, unison.KeyInsert: screen.KeyInsert, unison.KeyDelete: screen.KeyDelete,
	unison.KeyHome: screen.KeyHome, unison.KeyEnd: screen.KeyEnd, unison.KeyPageUp: screen.KeyPageUp,
	unison.KeyPageDown: screen.KeyPageDown,
}

// keyChar is the character a key types unshifted, for a key pressed with
// Ctrl or Alt, which the platforms deliver as a key rather than as typing.
func keyChar(key unison.KeyCode) rune {
	switch {
	case key >= unison.KeyA && key <= unison.KeyZ:
		return 'a' + rune(key-unison.KeyA)
	case key >= unison.Key0 && key <= unison.Key9:
		return '0' + rune(key-unison.Key0)
	case key == unison.KeySpace:
		return ' '
	}
	for r, k := range punctuationKeys {
		if k == key {
			return r
		}
	}
	return 0
}

func (v *View) keyDown(key unison.KeyCode, mods mod.Modifiers, _ bool) bool {
	if v.isReserved(key, mods) {
		return false
	}
	// The view's own keys, which a reserved sequence above overrides:
	// Ctrl+Shift+C, V and A to copy, paste and select everything (Command
	// on macOS, where Command+C and V work without Shift too, since Command
	// never goes to the program), and Shift+Page Up and Down to scroll.
	cmd := mods.OSMenuCommandDown()
	shift := mods.ShiftDown()
	if cmd && (shift || runtime.GOOS == "darwin") {
		switch key {
		case unison.KeyC:
			v.Copy()
			return true
		case unison.KeyV:
			v.Paste()
			return true
		case unison.KeyA:
			v.SelectAll()
			return true
		}
	}
	if shift && !cmd && !mods.OptionDown() {
		switch key {
		case unison.KeyPageUp:
			v.ScrollBy(v.rows - 1)
			return true
		case unison.KeyPageDown:
			v.ScrollBy(-(v.rows - 1))
			return true
		}
	}
	if v.session == nil {
		return false
	}
	if key >= unison.KeyF1 && key <= unison.KeyF12 {
		v.typed()
		v.session.PressKey(screen.KeyF1+screen.Key(key-unison.KeyF1), terminalMods(mods, true))
		return true
	}
	if k, ok := terminalKeys[key]; ok {
		v.typed()
		v.session.PressKey(k, terminalMods(mods, true))
		return true
	}
	if runtime.GOOS == "darwin" && mods.CommandDown() {
		return false
	}
	// A character with Ctrl or Alt held. Ctrl and Alt together on Windows
	// is AltGr, which types a character of its own (@ on a German layout),
	// and that arrives as typing.
	ctrl, alt := mods.ControlDown(), mods.OptionDown() && runtime.GOOS != "darwin"
	if runtime.GOOS == "windows" && ctrl && mods.OptionDown() {
		return false
	}
	if !ctrl && !alt {
		return false
	}
	r := keyChar(key)
	if r == 0 {
		return false
	}
	v.typed()
	v.session.TypeChar(r, terminalMods(mods, false))
	return true
}

func (v *View) runeTyped(r rune) bool {
	if v.session == nil {
		return false
	}
	// Control characters arrive as keys; Windows also delivers them as
	// typing (Ctrl+A as U+0001), which is dropped here.
	if r < 0x20 || r == 0x7f {
		return true
	}
	v.typed()
	v.session.TypeChar(r, 0)
	return true
}

// typed returns the view to where new output appears, which is what every
// terminal does and what makes scrolling back safe, and shows the cursor.
func (v *View) typed() {
	v.ScrollToBottom()
	v.restartBlink()
}

// HasSelection reports whether something is selected.
func (v *View) HasSelection() bool { return v.hasSel }

// SelectedText is the selection as text, with wrapped lines joined.
func (v *View) SelectedText() string {
	if v.session == nil || !v.hasSel {
		return ""
	}
	var s string
	v.session.View(func(scr *screen.Screen) { s = scr.TextInRange(v.anchor, v.head, false) })
	return s
}

// Copy puts the selection on the clipboard.
func (v *View) Copy() {
	if t := v.SelectedText(); t != "" {
		unison.ClipboardSetText(t)
	}
}

// Paste sends the clipboard's text to the program as a paste.
func (v *View) Paste() {
	if v.session == nil {
		return
	}
	if t := unison.ClipboardGetText(); t != "" {
		v.typed()
		v.session.Paste(t)
	}
}

// SelectAll selects the scrollback and the screen.
func (v *View) SelectAll() {
	if v.session == nil {
		return
	}
	cols, rows := v.session.Size()
	v.setSelection(screen.Point{X: 0, Y: -v.session.ScrollbackCount()}, screen.Point{X: cols - 1, Y: rows - 1})
}

// ClearSelection selects nothing.
func (v *View) ClearSelection() {
	if v.hasSel {
		v.hasSel = false
		v.changed()
		v.MarkForRedraw()
	}
}

func (v *View) setSelection(anchor, head screen.Point) {
	v.anchor, v.head = anchor, head
	v.hasSel = anchor != head
	v.changed()
	v.MarkForRedraw()
}

// unitAround is the word or line around a cell, for a selection made by a
// double or triple click.
func (v *View) unitAround(p screen.Point, unit int) (from, to screen.Point) {
	var l screen.Line
	v.session.View(func(scr *screen.Screen) { scr.LineInto(p.Y, &l) })
	if unit == 3 {
		return screen.Point{X: 0, Y: p.Y}, screen.Point{X: max(0, len(l.Cells)-1), Y: p.Y}
	}
	word := func(col int) bool {
		c := l.CellAt(col)
		if c.Ch == 0 && col > 0 {
			c = l.CellAt(col - 1) // the right half of a wide character
		}
		r := firstRune(c)
		return unicode.IsLetter(r) || unicode.IsNumber(r) || strings.ContainsRune("_-./:@~+=%#?&", r)
	}
	if !word(p.X) {
		return p, p
	}
	first, last := p.X, p.X
	for first > 0 && word(first-1) {
		first--
	}
	for last+1 < len(l.Cells) && word(last+1) {
		last++
	}
	return screen.Point{X: first, Y: p.Y}, screen.Point{X: last, Y: p.Y}
}

// mouseTracking is what the program asked to be told about the mouse.
func (v *View) mouseTracking() screen.MouseTracking {
	t := screen.NoMouse
	if v.session != nil {
		v.session.View(func(scr *screen.Screen) { t = scr.MouseTracking() })
	}
	return t
}

func buttonOf(b int) screen.MouseButton {
	switch b {
	case unison.ButtonRight:
		return screen.RightButton
	case unison.ButtonMiddle:
		return screen.MiddleButton
	}
	return screen.LeftButton
}

func (v *View) mouseDown(where geom.Point, button, clicks int, mods mod.Modifiers) bool {
	v.RequestFocus()
	if v.session == nil {
		return false
	}
	cell := v.cellAt(where)
	if button == unison.ButtonMiddle {
		// The X11 convention of pasting the selection with the middle
		// button; unison offers no X11 selection, so it pastes the
		// clipboard.
		v.Paste()
		return true
	}
	// A program that asked to be told about the mouse is told, unless Shift
	// is held, which is the way every terminal offers to select text inside
	// a full-screen program.
	if v.mouseTracking() != screen.NoMouse && !mods.ShiftDown() {
		v.session.Mouse(cell.Y+v.offset, cell.X, buttonOf(button), true, terminalMods(mods, true))
		return true
	}
	if button == unison.ButtonLeft && mods.OSMenuCommandDown() {
		if l, _, ok := v.linkAt(cell); ok {
			if v.OnLinkActivated != nil {
				v.OnLinkActivated(l.Text, l.Line, l.Character)
			}
			return true
		}
	}
	if button != unison.ButtonLeft {
		return true
	}
	v.selecting = true
	v.unit = min(max(1, clicks), 3)
	if v.unit > 1 {
		from, to := v.unitAround(cell, v.unit)
		v.unitFrom, v.unitTo = from, to
		v.setSelection(from, to)
		if from == to {
			v.hasSel = false
		}
		return true
	}
	v.hasSel = false
	v.setSelection(cell, cell)
	return true
}

func (v *View) mouseDrag(where geom.Point, button int, mods mod.Modifiers) bool {
	if v.session == nil {
		return false
	}
	cell := v.cellAt(where)
	if v.selecting {
		if v.unit > 1 {
			from, to := v.unitAround(cell, v.unit)
			if cell.Before(v.unitFrom) {
				v.setSelection(v.unitTo, from)
			} else {
				v.setSelection(v.unitFrom, to)
			}
		} else {
			v.setSelection(v.anchor, cell)
		}
		// Dragging past the top or bottom scrolls, so a selection can reach
		// further than the window.
		r := v.ContentRect(false)
		if where.Y < 0 {
			v.ScrollBy(1)
		} else if where.Y > r.Height {
			v.ScrollBy(-1)
		}
		return true
	}
	if t := v.mouseTracking(); t == screen.MouseDrag || t == screen.MouseMotion {
		v.session.Mouse(cell.Y+v.offset, cell.X, screen.NoButton, false, terminalMods(mods, true))
		return true
	}
	return false
}

func (v *View) mouseUp(where geom.Point, button int, mods mod.Modifiers) bool {
	if v.session == nil {
		return false
	}
	if !v.selecting && v.mouseTracking() != screen.NoMouse && !mods.ShiftDown() && button != unison.ButtonMiddle {
		cell := v.cellAt(where)
		v.session.Mouse(cell.Y+v.offset, cell.X, buttonOf(button), false, terminalMods(mods, true))
		return true
	}
	v.selecting = false
	return true
}

func (v *View) mouseMove(where geom.Point, mods mod.Modifiers) bool {
	v.pointer = where
	if v.session == nil {
		return false
	}
	cell := v.cellAt(where)
	if v.mouseTracking() == screen.MouseMotion {
		v.session.Mouse(cell.Y+v.offset, cell.X, screen.NoButton, false, terminalMods(mods, true))
	}
	v.hoverLink(cell, mods.OSMenuCommandDown())
	return false
}

func (v *View) mouseExit() bool {
	v.hoverLink(screen.Point{}, false)
	return false
}

// hoverLink underlines the link under the pointer while Ctrl is held, so
// ordinary output does not become a field of underlines.
func (v *View) hoverLink(cell screen.Point, ctrl bool) {
	var l kvitterm.Link
	var row int
	ok := false
	if ctrl {
		l, row, ok = v.linkAt(cell)
	}
	if ok == v.hasLink && (!ok || (l == v.link && row == v.linkRow)) {
		return
	}
	v.link, v.linkRow, v.hasLink = l, row, ok
	v.changed()
	v.MarkForRedraw()
}

func (v *View) cursorShape(geom.Point) *unison.Cursor {
	if v.hasLink {
		return unison.PointingCursor()
	}
	return unison.TextCursor()
}

// linkAt is the link under a cell, placed in the cells of that cell's row.
// A wrapped line is joined with the rows it continues from and into
// first, so an address broken across the wrap is found whole.
func (v *View) linkAt(cell screen.Point) (kvitterm.Link, int, bool) {
	var found kvitterm.Link
	ok := false
	v.session.View(func(scr *screen.Screen) {
		first, last := cell.Y, cell.Y
		for first > -scr.ScrollbackCount() && scr.Line(first).Continuation {
			first--
		}
		for last+1 < scr.Rows() && scr.Line(last+1).Continuation {
			last++
		}
		var joined screen.Line
		offset := 0
		for r := first; r <= last; r++ {
			l := scr.Line(r)
			if r == cell.Y {
				offset = len(joined.Cells)
			}
			joined.Cells = append(joined.Cells, l.Cells...)
		}
		col := offset + cell.X
		for _, l := range kvitterm.FindLinksInLine(joined) {
			if col >= l.Column && col < l.Column+l.Length {
				// Its cells on the row under the pointer.
				from := max(l.Column, offset) - offset
				to := min(l.Column+l.Length, offset+len(scr.Line(cell.Y).Cells)) - offset
				found = l
				found.Column, found.Length = from, to-from
				ok = true
				return
			}
		}
	})
	return found, cell.Y, ok
}

func (v *View) mouseWheel(where geom.Point, delta geom.Point, mods mod.Modifiers) bool {
	if v.session == nil {
		return false
	}
	// Three lines a notch, as the Qt terminal scrolled; a Mac's wheel and
	// trackpad report a distance, which unison scales to pixels.
	if runtime.GOOS == "darwin" {
		v.wheelOwed += delta.Y * unison.MouseWheelMultiplier / float32(v.cellH)
	} else {
		v.wheelOwed += delta.Y * linesPerNotch
	}
	lines := int(v.wheelOwed)
	if lines == 0 {
		return true
	}
	v.wheelOwed -= float32(lines)
	n := max(lines, -lines)
	if v.mouseTracking() != screen.NoMouse && !mods.ShiftDown() {
		cell := v.cellAt(where)
		b := screen.WheelUp
		if lines < 0 {
			b = screen.WheelDown
		}
		for i := 0; i < max(1, n/linesPerNotch); i++ {
			v.session.Mouse(cell.Y+v.offset, cell.X, b, true, terminalMods(mods, true))
		}
		return true
	}
	alt := false
	v.session.View(func(scr *screen.Screen) { alt = scr.AlternateScreen() })
	if alt {
		// A full-screen program has no scrollback of its own, so the wheel
		// becomes the arrow keys, which is what makes a pager scroll.
		k := screen.KeyUp
		if lines < 0 {
			k = screen.KeyDown
		}
		for i := 0; i < n; i++ {
			v.session.PressKey(k, 0)
		}
		return true
	}
	v.ScrollBy(lines)
	return true
}

const linesPerNotch = 3
