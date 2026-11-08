package build

import (
	"encoding/json"
	"strings"
	"testing"
)

// Word counts characters including spaces and not paragraph marks; the UPM's
// 4000 is Word's. Two paragraphs of "ab cd" are 10, not 11 or 12.
func TestAbstractScanCountsAsWordDoes(t *testing.T) {
	var s abstractScan
	for _, b := range []string{
		`{"t":"Para","c":[{"t":"Str","c":"ab"},{"t":"Space"},{"t":"Emph","c":[{"t":"Str","c":"cd"}]}]}`,
		`{"t":"Para","c":[{"t":"Str","c":"ab"},{"t":"SoftBreak"},{"t":"Str","c":"cd"}]}`,
	} {
		s.block(json.RawMessage(b))
	}
	if got := len([]rune(s.text.String())); got != 10 || s.offender != "" {
		t.Errorf("counted %d (%q), offender %q; want 10 and none", got, s.text.String(), s.offender)
	}
}

// A citation, a formula or a footnote prints as something other than words,
// however deep in emphasis it sits; a level-2 heading is the summary's own
// structure and passes.
func TestAbstractScanRefusesWhatIsNotPlainText(t *testing.T) {
	for _, c := range []struct{ block, want string }{
		{`{"t":"Para","c":[{"t":"Emph","c":[{"t":"Cite","c":[[{"citationId":"k"}],[]]}]}]}`, "a citation (@k)"},
		{`{"t":"Para","c":[{"t":"Math","c":[{"t":"InlineMath"},"x"]}]}`, "a formula"},
		{`{"t":"Para","c":[{"t":"Note","c":[]}]}`, "a footnote"},
		{`{"t":"BulletList","c":[]}`, "a list"},
		{`{"t":"Header","c":[1,["",[],[]],[{"t":"Str","c":"X"}]]}`, "a heading other than ## (a section of the summary)"},
		{`{"t":"Header","c":[2,["",[],[]],[{"t":"Str","c":"Objetivos"}]]}`, ""},
	} {
		var s abstractScan
		s.block(json.RawMessage(c.block))
		if s.offender != c.want {
			t.Errorf("%s: offender %q, want %q", c.block, s.offender, c.want)
		}
	}
}

// The likeliest slip is the two summaries swapped: the shares tell a Spanish
// text from an English one by a wide margin.
func TestStopwordShareTellsTheLanguages(t *testing.T) {
	es, en := stopwordShare("Esta tesis estudia los colectivos de agentes que aprenden a coordinarse en la planta y con el entorno.")
	if es < 0.2 || en > 0.1 {
		t.Errorf("Spanish text: es %.2f, en %.2f", es, en)
	}
	es, en = stopwordShare("This thesis studies the collectives of agents that learn to coordinate in the plant and with the world.")
	if en < 0.2 || es > 0.1 {
		t.Errorf("English text: es %.2f, en %.2f", es, en)
	}
}

// The appendices start at the first {.appendix} heading outside code: one in
// a Markdown sample is an example, not the thesis's appendix.
func TestSplitAppendicesIgnoresCode(t *testing.T) {
	body := "# Intro\n\n```markdown\n# Sample {.appendix}\n```\n\n# Cuestionario {.appendix}\n\nX\n"
	main, app := splitAppendices(body)
	if !strings.HasPrefix(app, "# Cuestionario") || !strings.Contains(main, "# Sample {.appendix}") {
		t.Errorf("main %q, appendices %q", main, app)
	}
}

// A summary's sections under an unnumbered chapter printed as "0.1"; marking
// them must not touch a heading in code or one with its own attributes.
func TestFrontSectionsUnnumbersOnlyBareSubheadings(t *testing.T) {
	got := frontSections("## Objetivos\n\nX\n\n```\n## code\n```\n\n## Done {#done}\n")
	want := "## Objetivos {.unnumbered .unlisted}\n\nX\n\n```\n## code\n```\n\n## Done {#done}\n"
	if got != want {
		t.Errorf("got %q", got)
	}
}
