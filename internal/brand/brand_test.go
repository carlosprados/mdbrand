package brand

import (
	"os"
	"path/filepath"
	"testing"

	"gopkg.in/yaml.v3"
)

// A bundle is shared between machines, so fonts.display.path must accept both
// shapes: the single path older bundles were written with, and the candidate
// list that makes a bundle portable.
func TestPathListAcceptsBothShapes(t *testing.T) {
	var one struct {
		Path PathList `yaml:"path"`
	}
	if err := yaml.Unmarshal([]byte("path: /opt/fonts\n"), &one); err != nil {
		t.Fatal(err)
	}
	if len(one.Path) != 1 || one.Path[0] != "/opt/fonts" {
		t.Errorf("scalar path = %v", one.Path)
	}

	var many struct {
		Path PathList `yaml:"path"`
	}
	if err := yaml.Unmarshal([]byte("path:\n  - /a\n  - /b\n"), &many); err != nil {
		t.Fatal(err)
	}
	if len(many.Path) != 2 || many.Path[1] != "/b" {
		t.Errorf("list path = %v", many.Path)
	}

	var bad struct {
		Path PathList `yaml:"path"`
	}
	if err := yaml.Unmarshal([]byte("path: {a: 1}\n"), &bad); err == nil {
		t.Error("a mapping must be rejected, not silently ignored")
	}
}

func TestExpandPath(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil {
		t.Skip("no home directory")
	}
	if got := expandPath("~/fonts"); got != filepath.Join(home, "fonts") {
		t.Errorf("~ not expanded: %q", got)
	}
	t.Setenv("MDBRAND_TEST_FONTS", "/opt/x")
	if got := expandPath("$MDBRAND_TEST_FONTS/gotham"); got != "/opt/x/gotham" {
		t.Errorf("variable not expanded: %q", got)
	}
	// An unset variable must collapse to nothing rather than to a plausible
	// path: "/gotham" could exist and would be the wrong font.
	if got := expandPath("$MDBRAND_UNSET_XYZ"); got != "" {
		t.Errorf("unset variable = %q, want empty", got)
	}
}

func TestDisplayFontDirPicksFirstExistingCandidate(t *testing.T) {
	real := t.TempDir()
	if err := os.WriteFile(filepath.Join(real, "Gotham-Light.otf"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	b := Default()
	b.Fonts.Display = Display{
		Family:  "Gotham",
		Regular: "Gotham-Light.otf",
		Path:    PathList{"$MDBRAND_UNSET_XYZ", "/definitely/not/here", real},
	}
	dir, ok := b.DisplayFontDir()
	if !ok {
		t.Fatal("declared font not found")
	}
	if dir != real+string(os.PathSeparator) {
		t.Errorf("dir = %q, want %q with a trailing separator", dir, real)
	}

	// Absent everywhere: not an error, and the hint has to name the fixes.
	b.Fonts.Display.Regular = "Gotham-Nonexistent.otf"
	if _, ok := b.DisplayFontDir(); ok {
		t.Error("reported a font that is not installed")
	}
	hint := b.DisplayFontHint()
	for _, want := range []string{"MDBRAND_FONT_DIR", "fontconfig", "not set", b.Fonts.Body} {
		if !contains(hint, want) {
			t.Errorf("hint does not mention %q:\n%s", want, hint)
		}
	}
}

func TestCheckWarnsButDoesNotFailOnAbsentDisplayFont(t *testing.T) {
	b := Default()
	b.Fonts.Display = Display{Family: "Gotham", Regular: "Nope.otf", Path: PathList{"/nowhere"}}
	problems, warnings := b.Check()
	for _, p := range problems {
		if contains(p, "display") {
			t.Errorf("an absent display font must warn, not fail: %s", p)
		}
	}
	found := false
	for _, w := range warnings {
		if contains(w, "fonts.display") {
			found = true
		}
	}
	if !found {
		t.Error("no warning about the absent display font")
	}
}

func contains(s, sub string) bool {
	return len(sub) == 0 || (len(s) >= len(sub) && indexOf(s, sub) >= 0)
}

func indexOf(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}
