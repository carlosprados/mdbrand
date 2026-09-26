package brand

import (
	"slices"
	"testing"
)

func TestParseCharset(t *testing.T) {
	c, err := ParseCharset("20-7e a0 26d4 2b50-2b51")
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range []rune{' ', 'A', '~', 0xA0, '⛔', '⭐', 0x2B51} {
		if !c.Has(r) {
			t.Errorf("%U should be covered", r)
		}
	}
	for _, r := range []rune{0x7F, 0xA1, 0x26D3, 0x2B52} {
		if c.Has(r) {
			t.Errorf("%U should not be covered", r)
		}
	}
	if _, err := ParseCharset("20-zz"); err == nil {
		t.Error("a malformed range must be an error, not an empty charset")
	}
}

// TestFallbackRunesRedirectsOnlyWhatTheBodyLacks pins the rule that keeps the
// fallback from hiding a missing glyph: a character NEITHER face has must not
// be selected, so the missing-glyph check still stops the build over it.
func TestFallbackRunesRedirectsOnlyWhatTheBodyLacks(t *testing.T) {
	body, _ := ParseCharset("20-7e a0-17f 2192 26a0")   // Latin, →, ⚠
	fallback, _ := ParseCharset("20-7e 26a0 26d4 2b50") // ⚠, ⛔, ⭐
	text := "ok → ⛔ ⭐ ⛔ ⚠️ ☐ ñ"

	got := FallbackRunes(text, body, fallback)
	want := []rune{'⛔', '⭐'} // first-seen order, no duplicates
	if !slices.Equal(got, want) {
		t.Fatalf("got %q, want %q", string(got), string(want))
	}
	// ⚠ the body has; U+FE0F is a variation selector; ☐ neither face has.
}

func TestFamilyMatchesComparesTheAnswerWithTheQuestion(t *testing.T) {
	if !familyMatches("MdbrandTestFace,MdbrandTestFace Medium", "mdbrandtestface") {
		t.Error("a listed family name must match, case-insensitively")
	}
	if familyMatches("DejaVu Sans", "MdbrandTestFace") {
		t.Error("fc-match's substitute must not count as the family asked for")
	}
}
