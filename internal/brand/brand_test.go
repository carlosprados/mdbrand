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

// A bundle may carry its own font, and then the candidate is relative: it must
// resolve inside the bundle directory, not against the process's working
// directory, or a clone would only work when built from one place.
//
// The face name is deliberately one no system can have installed: otherwise the
// fontconfig fallback answers and the test passes or fails depending on whose
// machine it runs on.
func TestDisplayFontDirResolvesRelativeToTheBundle(t *testing.T) {
	const face = "MdbrandTestFace-Regular.otf"
	bundle := t.TempDir()
	fonts := filepath.Join(bundle, "fonts", "otf")
	if err := os.MkdirAll(fonts, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(fonts, face), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	b := Default()
	b.Dir = bundle
	b.Fonts.Display = Display{Family: "Test", Regular: face, Path: PathList{"fonts/otf"}}

	dir, ok := b.DisplayFontDir()
	if !ok {
		t.Fatal("font shipped inside the bundle was not found")
	}
	if dir != fonts+string(os.PathSeparator) {
		t.Errorf("dir = %q, want %q", dir, fonts)
	}

	// An absolute candidate must be used as given, never joined to the bundle.
	b.Fonts.Display.Path = PathList{filepath.Join(bundle, "fonts", "otf")}
	if dir, ok = b.DisplayFontDir(); !ok || dir != fonts+string(os.PathSeparator) {
		t.Errorf("absolute candidate: dir = %q ok = %v", dir, ok)
	}
	b.Fonts.Display.Path = PathList{"/definitely/not/here"}
	if _, ok = b.DisplayFontDir(); ok {
		t.Error("a non-existent absolute candidate must not resolve")
	}
}

// Regression: the two faces of a display font need not share a directory, and
// only the regular used to be verified. The bold was then handed to fontspec
// with the regular's Path, so the build died inside XeLaTeX with "the font
// cannot be found" — after validate had already reported ok.
func TestResolveDisplayChecksEveryFace(t *testing.T) {
	const reg, bold = "MdbrandTestFace-Light.otf", "MdbrandTestFace-Medium.otf"
	bundle := t.TempDir()
	other := t.TempDir()
	regDir := filepath.Join(bundle, "fonts")
	if err := os.MkdirAll(regDir, 0o755); err != nil {
		t.Fatal(err)
	}
	write := func(dir, name string) {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write(regDir, reg)
	write(other, bold) // the bold lives somewhere else entirely

	b := Default()
	b.Dir = bundle
	b.Fonts.Display = Display{Family: "Test", Regular: reg, Bold: bold, Path: PathList{"fonts", other}}

	res, ok := b.ResolveDisplay()
	if !ok {
		t.Fatal("regular not found")
	}
	if res.RegularDir != regDir+string(os.PathSeparator) {
		t.Errorf("regular dir = %q", res.RegularDir)
	}
	if res.BoldDir != other+string(os.PathSeparator) || res.BoldFile != bold {
		t.Errorf("bold = %q in %q, want %q in %q", res.BoldFile, res.BoldDir, bold, other)
	}
	if res.BoldIsRegular {
		t.Error("bold was found; it must not be reported as substituted")
	}

	// Bold nowhere: the regular stands in, the build stays possible, and it is
	// reported instead of exploding in fontspec.
	b.Fonts.Display.Bold = "MdbrandTestFace-Absent.otf"
	res, ok = b.ResolveDisplay()
	if !ok {
		t.Fatal("a missing bold must not disable the display font")
	}
	if !res.BoldIsRegular || res.BoldFile != reg {
		t.Errorf("substitution not applied: %+v", res)
	}
	if len(res.Missing) != 1 {
		t.Errorf("Missing = %v, want the absent bold only", res.Missing)
	}
	_, warns := b.Check()
	found := false
	for _, w := range warns {
		if contains(w, "Absent.otf") {
			found = true
		}
	}
	if !found {
		t.Errorf("validate must warn about the substituted face, got %v", warns)
	}
}
