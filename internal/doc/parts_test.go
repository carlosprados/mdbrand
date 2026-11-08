package doc

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// A chapter in capitulos/ links ../diagramas/x.d2; read from the master's
// directory that path led nowhere. Rebase rewrites it relative to the master,
// and leaves alone what is not a relative path to a file: anchors, URLs,
// absolute paths, and anything in code, which is an example.
func TestRebaseRewritesRelativeTargetsOnly(t *testing.T) {
	in := "![Fig](../diagramas/x.d2){width=80mm} [web](https://x.org) [up](#intro)\n" +
		"![abs](/srv/p.png) `![](code.png)`\n\n```\n![](fenced.png)\n```\n\n[ref]: datos/t.csv\n" +
		"![sp](<mis fotos/a.png>)\n"
	got := Rebase(in, "/doc/capitulos", "/doc")
	for _, want := range []string{"![Fig](diagramas/x.d2){width=80mm}", "(https://x.org)", "(#intro)", "(/srv/p.png)",
		"`![](code.png)`", "![](fenced.png)", "[ref]: capitulos/datos/t.csv", "![sp](<capitulos/mis fotos/a.png>)"} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in\n%s", want, got)
		}
	}
	if Rebase(in, "/doc", "/doc") != in {
		t.Error("a part beside the master must come back as it was")
	}
}

// Chapters are read in the order their names give: 10-x after 02-y.
func TestExpandPartsSortsByName(t *testing.T) {
	dir := t.TempDir()
	for _, n := range []string{"10-fin.md", "02-estado.md", "01-intro.md", "notas.txt"} {
		if err := os.WriteFile(filepath.Join(dir, n), nil, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	got, err := ExpandParts(dir, []string{"."})
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, p := range got {
		names = append(names, filepath.Base(p))
	}
	if !slices.Equal(names, []string{"01-intro.md", "02-estado.md", "10-fin.md"}) {
		t.Errorf("got %v", names)
	}
	if _, err := ExpandParts(dir, []string{"*.tex"}); err == nil {
		t.Error("a glob matching nothing must stop: the chapters would be silently absent")
	}
}

// A chapter's file opens with its title. Text before it would land at the
// end of the previous chapter, and a # inside code is not a heading.
func TestCheckChapterWantsItsHeadingFirst(t *testing.T) {
	if err := CheckChapter("c.md", "<!-- notes -->\n# Uno\n\n```\n# not a heading\n```\n"); err != nil {
		t.Errorf("a comment, then the heading: %v", err)
	}
	if err := CheckChapter("c.md", "Texto suelto.\n\n# Uno\n"); err == nil || !strings.Contains(err.Error(), "comes before") {
		t.Errorf("text before the heading: %v", err)
	}
}
