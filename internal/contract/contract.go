// Package contract holds the tests to the lists other people's files and
// scripts depend on: bundle keys, front matter options, commands and flags,
// exit statuses. Each list is derived from the code and compared with its
// golden in testdata/contract, so renaming a key is a failing test instead of
// a bundle that stops building on someone else's machine.
//
// After a deliberate change: MDBRAND_UPDATE_CONTRACT=1 go test ./... (or just
// contract), and say in the release notes what was removed.
package contract

import (
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"
)

// Check compares lines, sorted, with testdata/contract/<name>.txt.
func Check(t *testing.T, name string, lines []string) {
	t.Helper()
	lines = slices.Clone(lines)
	slices.Sort(lines)
	lines = slices.Compact(lines)
	_, here, _, _ := runtime.Caller(0)
	path := filepath.Join(filepath.Dir(here), "..", "..", "testdata", "contract", name+".txt")
	got := strings.Join(lines, "\n") + "\n"

	if os.Getenv("MDBRAND_UPDATE_CONTRACT") != "" {
		if err := os.WriteFile(path, []byte(got), 0o644); err != nil {
			t.Fatal(err)
		}
		return
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("%v\n  write it with: MDBRAND_UPDATE_CONTRACT=1 go test ./...", err)
	}
	want := strings.Split(strings.TrimRight(string(raw), "\n"), "\n")
	var removed, added []string
	for _, w := range want {
		if !slices.Contains(lines, w) {
			removed = append(removed, w)
		}
	}
	for _, g := range lines {
		if !slices.Contains(want, g) {
			added = append(added, g)
		}
	}
	if len(removed) > 0 {
		t.Errorf("%s: gone from the contract — documents, bundles or scripts using these stop working:\n  %s\n"+
			"  if that is meant, update the golden (MDBRAND_UPDATE_CONTRACT=1 go test ./...) and say so in the release notes",
			name, strings.Join(removed, "\n  "))
	}
	if len(added) > 0 {
		t.Errorf("%s: new in the contract, not yet in its golden:\n  %s\n"+
			"  record them: MDBRAND_UPDATE_CONTRACT=1 go test ./...",
			name, strings.Join(added, "\n  "))
	}
}
