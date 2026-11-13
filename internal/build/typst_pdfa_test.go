package build

import (
	"slices"
	"testing"

	"github.com/carlosprados/mdbrand/internal/brand"
	"github.com/carlosprados/mdbrand/internal/doc"
)

// A standard only Typst can keep: the document's list wins over the bundle's,
// an unknown one and one under XeLaTeX or on a deck stop the build.
func TestPickPDFStandards(t *testing.T) {
	withStd := func(s ...string) *doc.File {
		d := &doc.File{}
		d.Meta.Options.PDFStandard = s
		return d
	}
	bundle := &brand.Brand{PDFStandard: []string{"a-2b"}}
	cases := []struct {
		d             *doc.File
		b             *brand.Brand
		engine, style string
		want          []string
		fails         bool
	}{
		{withStd(), &brand.Brand{}, EngineLaTeX, "report", nil, false},
		{withStd(), bundle, EngineTypst, "report", []string{"a-2b"}, false},
		{withStd("ua-1"), bundle, EngineTypst, "note", []string{"ua-1"}, false},
		{withStd("a-2b"), &brand.Brand{}, EngineLaTeX, "report", nil, true},
		{withStd(), bundle, EngineLaTeX, "report", nil, true},
		{withStd("pdfa"), &brand.Brand{}, EngineTypst, "report", nil, true},
		{withStd("a-2b"), &brand.Brand{}, EngineTypst, "slides", nil, true},
	}
	for i, c := range cases {
		got, err := pickPDFStandards(c.d, c.b, c.engine, c.style)
		if (err != nil) != c.fails || !slices.Equal(got, c.want) {
			t.Errorf("case %d: %v, err %v; want %v, fails %v", i, got, err, c.want, c.fails)
		}
	}
	for s, want := range map[string]bool{"ua-1": true, "a-2a": true, "a-2b": false, "a-4": false, "a-4e": false} {
		if needsAlt([]string{s}) != want {
			t.Errorf("needsAlt(%s) != %v", s, want)
		}
	}
}
