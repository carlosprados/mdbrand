package cmd

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const fixture = `---
name: mdbrand
description: test fixture
---

# Body
`

func runSkill(t *testing.T, args ...string) (string, error) {
	t.Helper()
	root := Root()
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&out)
	root.SetArgs(args)
	err := root.Execute()
	return out.String(), err
}

func TestSkillInstall(t *testing.T) {
	old := SkillDoc
	SkillDoc = fixture
	t.Cleanup(func() { SkillDoc = old })

	dir := filepath.Join(t.TempDir(), "mdbrand")
	out, err := runSkill(t, "skill", "install", "--dir", dir)
	if err != nil {
		t.Fatalf("install: %v\n%s", err, out)
	}
	body, err := os.ReadFile(filepath.Join(dir, "SKILL.md"))
	if err != nil {
		t.Fatal(err)
	}
	// The front matter is what a skills directory reads; the stamp is what makes
	// a stale copy detectable after an upgrade.
	if !strings.HasPrefix(string(body), "---\nname: mdbrand\n") {
		t.Errorf("front matter mangled:\n%s", body)
	}
	if !strings.Contains(string(body), "installed by mdbrand") {
		t.Error("no version stamp")
	}

	// Installing again must recognise its own work rather than rewriting it.
	if out, err = runSkill(t, "skill", "install", "--dir", dir); err != nil {
		t.Fatalf("second install: %v", err)
	}
	if !strings.Contains(out, "already up to date") {
		t.Errorf("not idempotent: %q", out)
	}
}

// A developer's skill directory holds a symlink into the checkout. Writing
// through it would silently edit the repository's own SKILL.md, so it must be
// refused, and the file it points at must be untouched.
func TestSkillInstallRefusesToWriteThroughASymlink(t *testing.T) {
	old := SkillDoc
	SkillDoc = fixture
	t.Cleanup(func() { SkillDoc = old })

	repo := t.TempDir()
	source := filepath.Join(repo, "SKILL.md")
	const original = "# the checkout's own copy\n"
	if err := os.WriteFile(source, []byte(original), 0o644); err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(t.TempDir(), "mdbrand")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(dir, "SKILL.md")
	if err := os.Symlink(source, link); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}

	out, err := runSkill(t, "skill", "install", "--dir", dir)
	if err == nil {
		t.Fatalf("install through a symlink was allowed:\n%s", out)
	}
	if got, _ := os.ReadFile(source); string(got) != original {
		t.Fatalf("the checkout's file was overwritten: %q", got)
	}

	// --force is the documented escape, and it must replace the link, not follow it.
	if out, err = runSkill(t, "skill", "install", "--dir", dir, "--force"); err != nil {
		t.Fatalf("--force: %v\n%s", err, out)
	}
	if got, _ := os.ReadFile(source); string(got) != original {
		t.Fatalf("--force wrote through the link: %q", got)
	}
	st, err := os.Lstat(link)
	if err != nil {
		t.Fatal(err)
	}
	if st.Mode()&os.ModeSymlink != 0 {
		t.Error("--force left the symlink in place")
	}
}
