package main

import (
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// The embed patterns name the kinds of file an example may carry. A new kind
// beside an example — a picture, a .csl — would be left out of the binary,
// and the example would stop building for everyone who writes it out from
// there, while it still built in this checkout.
func TestEveryExampleSourceIsEmbedded(t *testing.T) {
	var onDisk, embedded []string
	_ = filepath.WalkDir("examples", func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		if ext := filepath.Ext(p); ext == ".pdf" || ext == ".docx" {
			return nil // built output, never a source
		}
		onDisk = append(onDisk, filepath.ToSlash(p))
		return nil
	})
	_ = fs.WalkDir(examples, ".", func(p string, d fs.DirEntry, err error) error {
		if err == nil && !d.IsDir() {
			embedded = append(embedded, p)
		}
		return err
	})
	slices.Sort(onDisk)
	slices.Sort(embedded)
	if !slices.Equal(onDisk, embedded) {
		t.Errorf("examples on disk and in the binary differ — widen the //go:embed patterns in main.go\n  disk:     %s\n  embedded: %s",
			strings.Join(onDisk, " "), strings.Join(embedded, " "))
	}
	if _, err := os.Stat("examples"); err != nil {
		t.Fatal(err)
	}
}
