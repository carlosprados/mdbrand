package fig

import "testing"

// A wrong answer here is silent either way: black boxes in every labelled d2
// diagram, or a working rsvg-convert refused.
func TestRsvgDrawsMasks(t *testing.T) {
	for _, tc := range []struct {
		out       string
		known, ok bool
	}{
		{"rsvg-convert version 2.40.20\n", true, false}, // the usual Windows build
		{"rsvg-convert version 2.40.5", true, false},
		{"rsvg-convert version 1.9.0", true, false},
		{"rsvg-convert version 2.41.0", true, true},
		{"rsvg-convert version 2.58.0", true, true},
		{"rsvg-convert version 2.60.1 (MSYS2)", true, true},
		{"rsvg-convert version 3.0.0", true, true},
		{"rsvg-convert, something unexpected", false, true},
	} {
		major, minor, known := ParseRsvgVersion(tc.out)
		if known != tc.known {
			t.Errorf("%q: parsed=%v, want %v", tc.out, known, tc.known)
			continue
		}
		if known && RsvgDrawsMasks(major, minor) != tc.ok {
			t.Errorf("%q (%d.%d): draws masks=%v, want %v", tc.out, major, minor, !tc.ok, tc.ok)
		}
	}
}
