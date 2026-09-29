package cmd

import (
	"os"
	"strings"
	"testing"
)

// Without a home directory every path mdbrand derives from one used to come
// out relative, so these commands wrote into — or read from — whatever
// directory they ran in. Each must refuse by name and leave it untouched.
func TestNoHomeTouchesNothingHere(t *testing.T) {
	t.Setenv("HOME", "")
	t.Setenv("XDG_CONFIG_HOME", "")
	t.Setenv("MDBRAND_BRANDS_DIR", "")
	work := t.TempDir()
	t.Chdir(work)
	old := SkillDoc
	SkillDoc = fixture
	t.Cleanup(func() { SkillDoc = old })

	for args, want := range map[string]string{
		"skill install":  "no home directory",
		"skill path":     "no home directory",
		"config init":    "XDG_CONFIG_HOME is unset",
		"brand new acme": "no brands directory is set",
		"brand list":     "no brands directory is set",
	} {
		out, err := runSkill(t, strings.Fields(args)...)
		if err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("mdbrand %s: err = %v, want %q\n%s", args, err, want, out)
		}
	}
	if ents, _ := os.ReadDir(work); len(ents) > 0 {
		var names []string
		for _, e := range ents {
			names = append(names, e.Name())
		}
		t.Errorf("wrote into the working directory: %v", names)
	}
}
