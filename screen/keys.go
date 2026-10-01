package screen

import (
	"fmt"
	"strings"
	"unicode/utf8"
)

// Modifiers are the modifier keys held with a key, a character or a mouse
// report.
type Modifiers uint8

// The modifiers a terminal can report.
const (
	Shift Modifiers = 1 << iota
	Alt
	Ctrl
)

// Key is a key that sends a sequence of its own rather than a character.
type Key uint8

// The keys with sequences of their own.
const (
	KeyNone Key = iota
	KeyEnter
	KeyTab
	KeyBackspace
	KeyEscape
	KeyUp
	KeyDown
	KeyLeft
	KeyRight
	KeyInsert
	KeyDelete
	KeyHome
	KeyEnd
	KeyPageUp
	KeyPageDown
	KeyF1
	KeyF2
	KeyF3
	KeyF4
	KeyF5
	KeyF6
	KeyF7
	KeyF8
	KeyF9
	KeyF10
	KeyF11
	KeyF12
)

// How a key's sequence is formed. The table and the rules below are
// libvterm's (keyboard.c), so a program gets the bytes a terminal built on
// libvterm sends it.
type keyKind uint8

const (
	keyLiteral   keyKind = iota // one byte, or CSI byte;mod u with Shift or Ctrl
	keyTab                      // a tab, or CSI Z with Shift
	keyEnter                    // a carriage return
	keySS3                      // SS3 letter, or the CSI form with modifiers
	keyCSI                      // CSI letter, or CSI 1;mod letter
	keyCSICursor                // SS3 in application cursor mode, CSI otherwise
	keyCSINum                   // CSI n ~, or CSI n;mod ~
)

type keyCode struct {
	kind    keyKind
	literal byte
	number  int
}

var keyCodes = map[Key]keyCode{
	KeyEnter:     {keyEnter, '\r', 0},
	KeyTab:       {keyTab, '\t', 0},
	KeyBackspace: {keyLiteral, 0x7f, 0},
	KeyEscape:    {keyLiteral, 0x1b, 0},
	KeyUp:        {keyCSICursor, 'A', 0},
	KeyDown:      {keyCSICursor, 'B', 0},
	KeyRight:     {keyCSICursor, 'C', 0},
	KeyLeft:      {keyCSICursor, 'D', 0},
	KeyInsert:    {keyCSINum, '~', 2},
	KeyDelete:    {keyCSINum, '~', 3},
	KeyHome:      {keyCSICursor, 'H', 0},
	KeyEnd:       {keyCSICursor, 'F', 0},
	KeyPageUp:    {keyCSINum, '~', 5},
	KeyPageDown:  {keyCSINum, '~', 6},
	KeyF1:        {keySS3, 'P', 0},
	KeyF2:        {keySS3, 'Q', 0},
	KeyF3:        {keySS3, 'R', 0},
	KeyF4:        {keySS3, 'S', 0},
	KeyF5:        {keyCSINum, '~', 15},
	KeyF6:        {keyCSINum, '~', 17},
	KeyF7:        {keyCSINum, '~', 18},
	KeyF8:        {keyCSINum, '~', 19},
	KeyF9:        {keyCSINum, '~', 20},
	KeyF10:       {keyCSINum, '~', 21},
	KeyF11:       {keyCSINum, '~', 23},
	KeyF12:       {keyCSINum, '~', 24},
}

// PressKey sends the sequence for a key. Which sequence depends on the
// modifiers and on modes the program selected: the same Up arrow sends
// ESC [ A to a shell and ESC O A to an editor that asked for application
// cursor keys.
func (s *Screen) PressKey(k Key, m Modifiers) {
	if seq := s.keySequence(k, m); seq != "" {
		s.write([]byte(seq))
	}
}

func (s *Screen) keySequence(k Key, m Modifiers) string {
	code, ok := keyCodes[k]
	if !ok {
		return ""
	}
	mod := int(m)
	kind := code.kind
	if kind == keyCSICursor {
		kind = keyCSI
		if s.ApplicationCursorKeys() {
			kind = keySS3
		}
	}
	switch kind {
	case keyTab:
		switch {
		case m == Shift:
			return "\x1b[Z"
		case m&Shift != 0:
			return fmt.Sprintf("\x1b[1;%dZ", mod+1)
		}
		return literal(code.literal, m)
	case keyEnter, keyLiteral:
		return literal(code.literal, m)
	case keySS3:
		if m == 0 {
			return "\x1bO" + string(code.literal)
		}
		return fmt.Sprintf("\x1b[1;%d%c", mod+1, code.literal)
	case keyCSI:
		if m == 0 {
			return "\x1b[" + string(code.literal)
		}
		return fmt.Sprintf("\x1b[1;%d%c", mod+1, code.literal)
	case keyCSINum:
		if m == 0 {
			return fmt.Sprintf("\x1b[%d%c", code.number, code.literal)
		}
		return fmt.Sprintf("\x1b[%d;%d%c", code.number, mod+1, code.literal)
	}
	return ""
}

func literal(b byte, m Modifiers) string {
	if m&(Shift|Ctrl) != 0 {
		return fmt.Sprintf("\x1b[%d;%du", b, int(m)+1)
	}
	if m&Alt != 0 {
		return "\x1b" + string(b)
	}
	return string(b)
}

// TypeChar sends a character typed with modifiers held: Ctrl+C is the
// interrupt byte, Alt+x is ESC x, and the combinations with no byte of
// their own use the CSI u form.
func (s *Screen) TypeChar(r rune, m Modifiers) {
	s.write([]byte(charSequence(r, m)))
}

func charSequence(r rune, m Modifiers) string {
	// Shift matters for no character but space: it has already chosen the
	// character.
	if r != ' ' {
		m &^= Shift
	}
	if m == 0 {
		return string(r)
	}
	var needsCSIu bool
	switch r {
	case 'i', 'j', 'm', '[':
		// Ctrl with these would be Tab, newline, Return and Escape.
		needsCSIu = true
	case '\\', ']', '^', '_':
		needsCSIu = false
	case ' ':
		needsCSIu = m&Shift != 0
	default:
		needsCSIu = r < 'a' || r > 'z'
	}
	if needsCSIu && m&^Alt != 0 {
		return fmt.Sprintf("\x1b[%d;%du", r, int(m)+1)
	}
	if m&Ctrl != 0 {
		r &= 0x1f
	}
	if m&Alt != 0 {
		return "\x1b" + string(r)
	}
	return string(r)
}

// SendText sends text as if it were typed, one character at a time with no
// modifiers.
func (s *Screen) SendText(text string) {
	if text != "" {
		s.write([]byte(text))
	}
}

// Paste sends text as a paste. Line breaks become carriage returns, as
// typing Return would send, and the text is marked as pasted where the
// program asked for that, which is how an editor tells pasted text from
// typing and stops indenting it.
func (s *Screen) Paste(text string) {
	text = strings.ReplaceAll(text, "\r\n", "\r")
	text = strings.ReplaceAll(text, "\n", "\r")
	if !utf8.ValidString(text) {
		text = strings.ToValidUTF8(text, "�")
	}
	if s.BracketedPaste() {
		// The markers must not be forged from inside the text.
		text = strings.ReplaceAll(text, "\x1b[201~", "")
		s.write([]byte("\x1b[200~" + text + "\x1b[201~"))
		return
	}
	s.SendText(text)
}
