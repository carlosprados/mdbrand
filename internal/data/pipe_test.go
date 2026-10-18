package data

import (
	"strings"
	"testing"
)

const longCell = "a description long enough to push the row past pandoc's seventy-two columns"

func TestSizePipeTables(t *testing.T) {
	cases := []struct {
		name, in string
		changed  bool
		prefix   string // the rewritten separator starts so
	}{
		{"equal dashes, wide row", "| Name | n | Note |\n|---|---|---|\n| Demócrata sin CIS | 12 | " + longCell + " |\n", true, "|---------------|"},
		{"alignment kept", "| Name | n |\n|:---|---:|\n| x | " + longCell + " |\n", true, "|:------|"},
		{"empty cell keeps the columns apart", "| a || c |\n|---|---|---|\n| x || " + longCell + " |\n", true, "|---|---|"},
		{"the author's dashes", "| a | b |\n|------|--|\n| x | " + longCell + " |\n", false, ""},
		{"narrow table, pandoc ignores dashes", "| a | b |\n|---|---|\n| x | y |\n", false, ""},
		{"inside a code block", "```\n| a | b |\n|---|---|\n| x | " + longCell + " |\n```\n", false, ""},
	}
	for _, c := range cases {
		got := SizePipeTables(c.in)
		if !c.changed {
			if got != c.in {
				t.Errorf("%s: changed\n%s", c.name, got)
			}
			continue
		}
		sep := strings.Split(got, "\n")[1]
		if !strings.HasPrefix(sep, c.prefix) {
			t.Errorf("%s: want separator starting %q, got %q", c.name, c.prefix, sep)
		}
		if strings.Count(sep, "|") != strings.Count(strings.Split(c.in, "\n")[1], "|") {
			t.Errorf("%s: columns changed: %q", c.name, sep)
		}
	}
}

// A column of dates beside two of sentences: shared by text alone it got 3%
// of the measure and "Pendiente" ran 19pt into the next column.
func TestColWidthsKeepShortCellsWhole(t *testing.T) {
	sentence := strings.Repeat("Hecho: el Supremo confirma que unos inscritos no votan ", 4)
	cells := [][]string{
		{"Fecha", "Hito", "Por qué importa"},
		{"12-oct", sentence, sentence},
		{"Pendiente", sentence, sentence},
		{"21–26 oct", sentence, sentence},
	}
	w := colWidths(cells, 3)
	total := 0
	for _, v := range w {
		total += v
	}
	// "Pendiente" and air, as a share of the assumed measure.
	if got := float64(w[0]) / float64(total) * measureChars; got < 11 {
		t.Errorf("date column gets %.1f of %d characters, want at least 11 (%v)", got, measureChars, w)
	}
	// A table that fits keeps its dashes: the most, unchanged.
	small := [][]string{{"Tipo", "RAM"}, {"m5.large", "8 GiB"}}
	if got := colWidths(small, 2); got[0] != 10 || got[1] != 5 {
		t.Errorf("fitting table resized: %v", got)
	}
}
