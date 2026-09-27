package watch

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// TestRun drives the watcher the way an editor and a build do: a save by
// rename-over must rebuild (a watch on the file itself loses it), a write to a
// file that is not an input must not (that is how the output PDF stays out of
// the loop), and a burst of writes must cost one build, not one per event.
func TestRun(t *testing.T) {
	dir := t.TempDir()
	input := filepath.Join(dir, "doc.md")
	write(t, input, "v1")

	builds := make(chan struct{}, 16)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		done <- Run(ctx, func() []string {
			builds <- struct{}{}
			return []string{input}
		}, t.Logf)
	}()
	expect(t, builds, 1, "the first build")

	tmp := filepath.Join(dir, ".doc.md.swp")
	write(t, tmp, "v2")
	if err := os.Rename(tmp, input); err != nil {
		t.Fatal(err)
	}
	expect(t, builds, 1, "a save by rename-over")

	write(t, filepath.Join(dir, "doc.pdf"), "output")
	expect(t, builds, 0, "a write to a file that is not an input")

	for _, s := range []string{"a", "b", "c"} {
		write(t, input, s)
	}
	expect(t, builds, 1, "a burst of three writes")

	cancel()
	if err := <-done; err != nil {
		t.Fatalf("Run returned %v on cancel", err)
	}
}

func write(t *testing.T, p, s string) {
	t.Helper()
	if err := os.WriteFile(p, []byte(s), 0o644); err != nil {
		t.Fatal(err)
	}
}

// expect counts the builds that arrive within a window comfortably longer than
// the debounce.
func expect(t *testing.T, builds <-chan struct{}, want int, what string) {
	t.Helper()
	got := 0
	window := time.After(4 * Debounce)
	for {
		select {
		case <-builds:
			got++
		case <-window:
			if got != want {
				t.Fatalf("%s: %d build(s), want %d", what, got, want)
			}
			return
		}
	}
}
