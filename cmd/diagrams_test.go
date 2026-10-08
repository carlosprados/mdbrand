package cmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// diagrams measured every document with the configured bundle, whatever its
// front matter said: a page or a deck reported in another identity's band.
// A brand that does not exist fails by name before anything is rendered, so
// this needs no toolchain.
func TestDiagramsTakesTheDocumentsBrand(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("MDBRAND_BRAND", "none")
	doc := filepath.Join(dir, "d.md")
	src := "---\ntitle: T\nmdbrand:\n  brand: nowhere\n---\n\n```d2\na -> b\n```\n"
	if err := os.WriteFile(doc, []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := runSkill(t, "diagrams", doc, "--brands-dir", dir)
	if err == nil || !strings.Contains(err.Error(), `brand "nowhere" not found`) {
		t.Errorf("want the document's brand looked up, got %v", err)
	}
	if _, err := runSkill(t, "diagrams", doc, "--brands-dir", dir, "--brand", "elsewhere"); err == nil ||
		!strings.Contains(err.Error(), `brand "elsewhere" not found`) {
		t.Errorf("want --brand to win over the front matter, got %v", err)
	}
}
