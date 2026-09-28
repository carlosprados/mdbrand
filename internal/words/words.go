// Package words counts a document's words by a stated criterion and puts the
// number where the document asks for it, with {{words}}.
//
// The count walks pandoc's own AST rather than guessing at Markdown, so a table,
// a footnote or a citation is whatever pandoc says it is. The default criterion
// is the International Baccalaureate's for the Extended Essay and the TOK essay,
// the strictest in common use and the same as a journal's "main text": prose,
// lists, headings, block quotes and content footnotes count; code, figures and
// captions, tables, mathematics, citations, notes that only cite, and the
// bibliography do not. A heading or a div marked {.nocount} leaves out what it
// holds — appendices, acknowledgements, an abstract counted separately.
//
// {{words}} is a placeholder and not a template. Running the document through
// text/template would turn every {{ in a code block, a d2 source or a formula
// into code to execute, and a template error — or worse, a quiet mis-render —
// is the defect class mdbrand exists to stop.
package words

import (
	"fmt"
	"slices"
	"strings"
)

// Parts a criterion may add to a profile.
var Parts = []string{"captions", "tables", "footnotes", "citations", "references", "code", "math"}

// Profiles, each a set of Parts. ib adds nothing; all adds everything, which is
// the "count every word printed" convention some journals use.
var Profiles = []string{"ib", "all"}

// Rules is a resolved criterion.
type Rules struct {
	Profile string
	include map[string]bool
}

// NewRules validates a profile and the parts added to it. An unknown name stops
// the build: a misspelt part would otherwise give a different number in silence.
func NewRules(profile string, include []string) (Rules, error) {
	if profile == "" {
		profile = "ib"
	}
	if !slices.Contains(Profiles, profile) {
		return Rules{}, fmt.Errorf("word count: unknown profile %q: pick one of %s",
			profile, strings.Join(Profiles, ", "))
	}
	r := Rules{Profile: profile, include: map[string]bool{}}
	if profile == "all" {
		for _, p := range Parts {
			r.include[p] = true
		}
	}
	for _, p := range include {
		if !slices.Contains(Parts, p) {
			return Rules{}, fmt.Errorf("word count: unknown part %q in include: pick from %s",
				p, strings.Join(Parts, ", "))
		}
		r.include[p] = true
	}
	return r, nil
}

// Has reports whether the criterion counts part.
func (r Rules) Has(part string) bool { return r.include[part] }

// NeedsCiteproc reports whether the count must see citations as printed, which
// only exist after citeproc has run. Counting footnotes counts the citations in
// them.
func (r Rules) NeedsCiteproc() bool {
	return r.Has("citations") || r.Has("references") || r.Has("footnotes")
}

// String names the criterion as the build report prints it.
func (r Rules) String() string {
	if r.Profile == "all" {
		return "all"
	}
	var extra []string
	for _, p := range Parts {
		if r.include[p] {
			extra = append(extra, p)
		}
	}
	if len(extra) == 0 {
		return r.Profile
	}
	return r.Profile + "+" + strings.Join(extra, "+")
}

// Format writes n the way the document's language groups thousands. Spanish,
// like most of continental Europe, groups with a point; English with a comma.
// An unknown or absent language gets no separator rather than a wrong one.
func Format(n int, lang string) string {
	sep, _ := Separators(lang)
	return Group(fmt.Sprint(n), sep)
}

// Separators are the thousands and decimal marks of the document's language.
// An unknown language gets no thousands mark and a decimal point.
func Separators(lang string) (thousands, decimal string) {
	switch strings.ToLower(strings.SplitN(lang, "-", 2)[0]) {
	case "en":
		return ",", "."
	case "es", "ca", "gl", "eu", "pt", "it", "de", "nl", "da":
		return ".", ","
	}
	return "", "."
}

// Group puts sep between every three digits of an unsigned integer's digits.
func Group(digits, sep string) string {
	if sep == "" || len(digits) <= 3 {
		return digits
	}
	var b strings.Builder
	for i, c := range digits {
		if i > 0 && (len(digits)-i)%3 == 0 {
			b.WriteString(sep)
		}
		b.WriteRune(c)
	}
	return b.String()
}
