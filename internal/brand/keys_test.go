package brand

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestParseVersion(t *testing.T) {
	for in, want := range map[string][3]int{
		"0.18": {0, 18, 0}, "0.18.2": {0, 18, 2}, "v0.18.0": {0, 18, 0},
		"v0.17.0-3-gabc123-dirty": {0, 17, 0}, "v0.17.0+dirty": {0, 17, 0}, " 1.2 ": {1, 2, 0},
	} {
		if got, ok := parseVersion(in); !ok || got != want {
			t.Errorf("parseVersion(%q) = %v, %v; want %v", in, got, ok, want)
		}
	}
	for _, in := range []string{"", "dev", "(devel)", "1", "1.x", "1.2.3.4"} {
		if _, ok := parseVersion(in); ok {
			t.Errorf("parseVersion(%q) must not parse", in)
		}
	}
}

func loadYAML(t *testing.T, y string) error {
	t.Helper()
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "b"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "b", "brand.yaml"), []byte(y), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := Load(dir, "b")
	return err
}

// A bundle is read by whatever mdbrand each reader has. One written for a
// newer tool must say so, before any key it carries can be taken for a typo.
func TestRequires(t *testing.T) {
	old := ToolVersion
	t.Cleanup(func() { ToolVersion = old })

	ToolVersion = "v0.17.0"
	err := loadYAML(t, "name: b\nrequires: \"0.18\"\nslides:\n  hologram: true\n")
	if err == nil || !strings.Contains(err.Error(), "requires mdbrand 0.18 or later") {
		t.Errorf("an older tool must be told to upgrade, before the unknown key: %v", err)
	}
	for _, tool := range []string{"v0.18.0", "v0.19.1-2-gabc", "dev", ""} {
		ToolVersion = tool
		if err := loadYAML(t, "name: b\nrequires: \"0.18\"\n"); err != nil {
			t.Errorf("tool %q: %v", tool, err)
		}
	}
	if err := loadYAML(t, "name: b\nrequires: soon\n"); err == nil || !strings.Contains(err.Error(), "not a version") {
		t.Errorf("a requires that is not a version must be refused: %v", err)
	}
}

// An unknown key used to vanish: colors.acent built every document with the
// primary as its accent. The path is named in full, with the nearest key.
func TestUnknownKeysAreRefusedByPath(t *testing.T) {
	old := ToolVersion
	t.Cleanup(func() { ToolVersion = old })
	ToolVersion = ""
	for y, want := range map[string]string{
		"name: b\ncolors:\n  acent: \"B35A00\"\n":                      "colors.acent is not a setting mdbrand reads — did you mean accent?",
		"name: b\nslides:\n  diagrams:\n    min_txt_pt: 9\n":           "slides.diagrams.min_txt_pt is not a setting mdbrand reads — did you mean min_text_pt?",
		"name: b\nfonts:\n  display:\n    path: [a, b]\n    bolt: x\n": "fonts.display.bolt",
		"name: b\nfooterr: x\n":                                        "did you mean footer?",
	} {
		if err := loadYAML(t, y); err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("%q: got %v, want %q", y, err, want)
		}
	}
}

// What brand new writes must load: a scaffold the tool itself refuses is the
// first thing a new bundle meets.
func TestScaffoldLoads(t *testing.T) {
	dir := t.TempDir()
	if _, err := Scaffold(dir, "fresh"); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(dir, "fresh"); err != nil {
		t.Errorf("the scaffold does not load: %v", err)
	}
}
