package xterm

// Ported from xterm.js src/common/services/UnicodeService.ts and src/common/input/UnicodeV6.ts.

import (
	"unicode"
	"unicode/utf8"

	"golang.org/x/text/width"
)

// Character widths follow the current Unicode data rather than the
// Unicode 6 tables upstream ported from xterm.js, which gave every emoji one
// column. A character is two columns wide where Unicode's East Asian Width
// property says Wide or Fullwidth (which since Unicode 9 includes the emoji
// that are drawn as emoji by default), zero where it is a combining or
// formatting mark that attaches to the character before it, and one
// otherwise. This is what wcwidth(3) in current C libraries answers, so a
// program measuring its own output agrees with the terminal about where the
// cursor is. (Kvit's change; see KVIT-PATCH.md.)

// bmpWidthTable holds the width of every character in the Basic
// Multilingual Plane, computed once.
var bmpWidthTable [65536]byte

func init() {
	for i := range bmpWidthTable {
		bmpWidthTable[i] = byte(computeWidth(rune(i)))
	}
}

// computeWidth is the width of one code point, from the Unicode data.
func computeWidth(r rune) int {
	switch {
	case r < 32 || (r >= 0x7f && r < 0xa0):
		return 0
	case r == 0xad: // soft hyphen, skipped by the printer
		return 1
	case r >= 0x1160 && r <= 0x11ff, r >= 0xd7b0 && r <= 0xd7ff:
		// Hangul vowel and final consonant jamo, which join the syllable
		// before them.
		return 0
	case r == 0x200b: // zero width space
		return 0
	case unicode.In(r, unicode.Mn, unicode.Me, unicode.Cf):
		return 0
	}
	switch width.LookupRune(r).Kind() {
	case width.EastAsianWide, width.EastAsianFullwidth:
		return 2
	}
	return 1
}

// UnicodeService provides character width calculation for terminal rendering.
type UnicodeService struct{}

// NewUnicodeService creates a UnicodeService.
func NewUnicodeService() *UnicodeService {
	return &UnicodeService{}
}

// Wcwidth returns the display width of a codepoint.
// Control chars and combining marks return 0, East Asian wide chars return 2, others return 1.
func (u *UnicodeService) Wcwidth(cp rune) int {
	num := int(cp)
	if num < 32 {
		return 0
	}
	if num < 127 {
		return 1
	}
	if num < 65536 {
		return int(bmpWidthTable[num])
	}
	return computeWidth(cp)
}

// UnicodeCharProperties bit layout (mirrors xterm.js UnicodeService):
//   bit 0:     shouldJoin
//   bits 1-2:  width (0-2)
//   bits 3+:   charKind (unused in V6, always 0)

// ExtractShouldJoin returns the shouldJoin flag from a UnicodeCharProperties value.
func ExtractShouldJoin(value int) bool {
	return (value & 1) != 0
}

// ExtractCharPropsWidth returns the character width from a UnicodeCharProperties value.
func ExtractCharPropsWidth(value int) int {
	return (value >> 1) & 0x3
}

// CreatePropertyValue packs width and shouldJoin into a UnicodeCharProperties value.
func CreatePropertyValue(charKind, width int, shouldJoin bool) int {
	sj := 0
	if shouldJoin {
		sj = 1
	}
	return ((charKind & 0xffffff) << 3) | ((width & 3) << 1) | sj
}

// CharProperties determines character properties for terminal rendering,
// including whether a character should join with the preceding cell.
// Mirrors xterm.js UnicodeV6.charProperties().
func (u *UnicodeService) CharProperties(codepoint rune, preceding int) int {
	w := u.Wcwidth(codepoint)
	if w == 0 && preceding != 0 {
		oldWidth := ExtractCharPropsWidth(preceding)
		if oldWidth > 0 {
			return CreatePropertyValue(0, oldWidth, true)
		}
	}
	return CreatePropertyValue(0, w, false)
}

// GetStringCellWidth returns the total display width of a string.
func (u *UnicodeService) GetStringCellWidth(s string) int {
	result := 0
	for i := 0; i < len(s); {
		r, size := utf8.DecodeRuneInString(s[i:])
		if r == utf8.RuneError && size <= 1 {
			// Invalid UTF-8 byte, treat as width 1
			result++
			i++
			continue
		}
		result += u.Wcwidth(r)
		i += size
	}
	return result
}
