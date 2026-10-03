package docx

import (
	"strings"
	"unicode"
)

// advance is a proportional glyph's width in em, by class, taken from the wider of the
// usual sans faces (Inter, Arial, Montserrat) so that none of them overflows:
// the face is on the reader's machine, so this is a deliberate over-estimate
// rather than a measurement. A column a little too wide costs nothing, one a
// little too narrow breaks a word in two.
func advance(r rune) float64 {
	switch {
	case strings.ContainsRune("iljI.,:;'|!¡()[]{}`", r):
		return 0.32
	case strings.ContainsRune("frt/\\-\"", r):
		return 0.42
	case r == ' ':
		return 0.28
	case strings.ContainsRune("mwMW@%", r):
		return 0.92
	case unicode.IsDigit(r):
		return 0.62
	case unicode.IsUpper(r):
		return 0.72
	case unicode.IsLetter(r):
		return 0.58
	default:
		return 0.62
	}
}

// MonoAdvance is the width of one glyph of a monospaced face, in em. Every
// glyph of the face has it, so a block's longest line is its column count
// times this. Unknown faces are taken at 0.6, the common value.
func MonoAdvance(face string) float64 {
	switch strings.ToLower(face) {
	case "consolas":
		return 0.55
	case "inconsolata":
		return 0.5
	case "dejavu sans mono", "menlo", "bitstream vera sans mono":
		return 0.602
	default:
		return 0.6
	}
}
