package cmd

import (
	"strings"
	"testing"
)

// A font size declared in a d2 source is the author taking the decision back,
// so mdbrand leaves it alone. The scaffold's example declared one, and every
// document started from it lost the sizing it was meant to show off.
func TestScaffoldLeavesD2SizingToMdbrand(t *testing.T) {
	for _, style := range []string{"report", "note", "slides"} {
		if strings.Contains(scaffold(style, "none", "T", "", "", false), "font-size") {
			t.Errorf("style %s: the scaffold's diagram declares a font size", style)
		}
	}
}
