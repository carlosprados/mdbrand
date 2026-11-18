package build

import (
	"fmt"
	"strings"
	"testing"
)

// layout writes pdftotext -bbox-layout for pages of rows, each "y:text";
// a text that starts with ">" is indented.
func layout(pages ...[]string) string {
	var b strings.Builder
	for _, rows := range pages {
		b.WriteString(`<page width="595.28" height="841.89">` + "\n")
		for _, r := range rows {
			y, text, _ := strings.Cut(r, ":")
			x := "70"
			if t, ok := strings.CutPrefix(text, ">"); ok {
				x, text = "85", t
			}
			fmt.Fprintf(&b, `<line xMin="%s" yMin="%s" xMax="500" yMax="%s">`, x, y, y)
			for _, w := range strings.Fields(text) {
				fmt.Fprintf(&b, `<word xMin="70" yMin="%s" xMax="90" yMax="%s">%s</word>`, y, y, w)
			}
			b.WriteString("</line>\n")
		}
		b.WriteString("</page>\n")
	}
	return b.String()
}

// A page that ends on a heading or on a table's caption, above the running
// footer, is named; one that ends on text, on a bullet that repeats a
// heading, on a number before a heading's word, the cover and the last page
// are not — nor is the footer lost among more pages of figures alone.
func TestStrandedAtFoot(t *testing.T) {
	bbox := layout(
		[]string{"300:A title page with no footer", "400:3. Cartera y estado"},
		[]string{"60:A line of prose, long enough to set the text's left margin.", "700:3. Cartera y estado", "800:Confidencial 2"},
		[]string{"60:Rows.", "690:Tabla 4: Cartera de", "700:evolutivos.", "800:Confidencial 3"},
		[]string{"60:Rows.", "700:the end of a sentence: estado", "800:Confidencial 4"},
		[]string{"60:Rows.", "700:Text of the page.", "800:Confidencial 5"},
		[]string{"60:Rows.", "700:Albacete 2023, 4 escaños.", "800:Confidencial 7"},
		[]string{"60:Rows.", "700:>• Arquitectura", "800:Confidencial 8"},
		[]string{"300:Figure 1: a page of figures alone, with no footer."},
		[]string{"300:Figure 2: another."},
		[]string{"300:Figure 3: and another, more than the pages of text."},
		[]string{"300:Figure 4."},
		[]string{"300:Figure 5."},
		[]string{"300:Figure 6."},
		[]string{"300:Figure 7."},
		[]string{"300:Figure 8."},
		[]string{"300:Figure 9."},
		[]string{"300:Figure 10."},
		[]string{"60:Rows.", "700:4. Arquitectura", "800:Confidencial 16"},
		[]string{"60:Rows.", "700:4. Arquitectura", "800:Confidencial 6"},
	)
	got := strandedAtFoot(bbox, []string{"Cartera y estado", "Arquitectura", "Estado", "Escaños"}, []string{"Cartera de evolutivos."})
	want := []string{
		`page 2 ends on the heading "Cartera y estado"; its text is on page 3`,
		`page 3 ends on the caption "Cartera de evolutivos."; its table is on page 4`,
		`page 18 ends on the heading "Arquitectura"; its text is on page 19`,
	}
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Errorf("got\n  %s\nwant\n  %s", strings.Join(got, "\n  "), strings.Join(want, "\n  "))
	}
}
