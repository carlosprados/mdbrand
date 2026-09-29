package paths

import (
	"os"
	"path/filepath"
	"testing"
)

func TestExpand(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil {
		t.Skip("no home directory")
	}
	t.Setenv("MDBRAND_TEST_FONTS", "/opt/x")
	t.Setenv("MDBRAND_TEST_CLIENT", "acme")
	for in, want := range map[string]string{
		"~/fonts":                    filepath.Join(home, "fonts"),
		"~":                          home,
		"$MDBRAND_TEST_FONTS/gotham": "/opt/x/gotham",
		// The configured brands directory used to leave the variable as
		// written after a ~.
		"~/$MDBRAND_TEST_CLIENT/brands": filepath.Join(home, "acme", "brands"),
		"  ~/fonts ":                    filepath.Join(home, "fonts"),
		// An unset variable must collapse to nothing rather than to a
		// plausible path: "/gotham" could exist and would be the wrong font.
		"$MDBRAND_UNSET_XYZ": "",
		// Another user's home is not ours to guess.
		"~other/fonts": "~other/fonts",
	} {
		if got := Expand(in); got != want {
			t.Errorf("Expand(%q) = %q, want %q", in, got, want)
		}
	}
}
