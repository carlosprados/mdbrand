package cmd

import (
	"strings"
	"testing"

	"github.com/carlosprados/mdbrand/internal/exit"
)

// A mistake on the command line exits 2, wherever cobra catches it. An
// unknown subcommand inside a group used to print the help and exit 0.
func TestUsageExitsTwo(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("MDBRAND_BRANDS_DIR", t.TempDir())
	for _, args := range []string{
		"frobnicate",
		"brand frob",
		"skill frob",
		"config frob",
		"build",
		"build a.md b.md",
		"build --bogus x.md",
		"build no-such-file.md",
	} {
		_, err := runSkill(t, strings.Fields(args)...)
		if got := exit.Code(err); got != exit.Usage {
			t.Errorf("mdbrand %s: exit %d, want %d (%v)", args, got, exit.Usage, err)
		}
	}
	for _, args := range []string{"", "brand", "skill", "config"} {
		if _, err := runSkill(t, strings.Fields(args)...); err != nil {
			t.Errorf("mdbrand %s: want the help and exit 0, got %v", args, err)
		}
	}
}

func TestMissingBundleExitsThree(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	_, err := runSkill(t, "brand", "show", "nosuchbundle", "--brands-dir", t.TempDir())
	if got := exit.Code(err); got != exit.Environment {
		t.Errorf("exit %d, want %d (%v)", got, exit.Environment, err)
	}
}
