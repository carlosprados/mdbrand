package brand

import (
	"slices"
	"testing"
)

// The UPM's template asks for the centre's official name, unabbreviated.
// Words are compared whole: Ingenieros must not trip on ETSI.
func TestAbbreviationsAreWholeWords(t *testing.T) {
	for _, c := range []struct {
		s    string
		want []string
	}{
		{"Escuela Técnica Superior de Ingenieros de Telecomunicación", nil},
		{"Universidad Politécnica de Madrid", nil},
		{"ETSI de Telecomunicación", []string{"ETSI"}},
		{"E.T.S.I. Telecomunicación (UPM)", []string{"E.T.S.I.", "UPM"}},
		{"Fac. de Informática", []string{"Fac."}},
	} {
		if got := Abbreviations(c.s); !slices.Equal(got, c.want) {
			t.Errorf("Abbreviations(%q) = %q, want %q", c.s, got, c.want)
		}
	}
}
