package build

import (
	"slices"
	"testing"
)

// A section that never printed still has its entry in the contents: the
// heading must be found as often as the document prints it.
func TestLostHeadings(t *testing.T) {
	out := "MDBRAND-HEADING 1 listed\tInteligencia artificial en el proyecto\n" +
		"MDBRAND-HEADING 2 listed\tPropiedad de las herramientas\n" +
		"MDBRAND-HEADING 1 listed\tEvolución: agente autónomo\n" +
		"MDBRAND-HEADING 3 unlisted\tLa configuración final\n"
	hs := reportedHeadings(out)
	if len(hs) != 4 || !hs[3].unlisted || hs[1].level != 2 {
		t.Fatalf("reported headings read as %+v", hs)
	}
	toc := "Índice\n1 Inteligencia artificial en el proyecto . . . 9\n" +
		"1.1 Propiedad de las herramientas . . 9\n2 Evolución: agente autónomo . . 11\n"
	// The body lost the first section; it hyphenates one heading at a line
	// end, sets a ligature, and numbers the headings.
	body := "2 Evolución: agente autó-\nnomo\nLa conﬁguración final\n"

	got := lostHeadings(toc+body, hs, 3)
	want := []string{"Inteligencia artificial en el proyecto", "Propiedad de las herramientas"}
	if !slices.Equal(got, want) {
		t.Errorf("lost = %q, want %q", got, want)
	}
	// Without contents each heading is sought once, and the entries count.
	if got := lostHeadings(toc+body, hs, 0); len(got) != 0 {
		t.Errorf("without contents: lost = %q, want nothing", got)
	}
}
