package fig

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// English is Vega's own and takes the old route; a decimal-comma language
// takes the locale route, and only Spanish has its month names here.
func TestLocaleForLang(t *testing.T) {
	for lang, want := range map[string]string{
		"": "", "en-GB": "", "en": "",
		"es-ES": "-f -t", "es": "-f -t",
		"de-DE": "-f -t", "pt": "-f -t", "ca": "-f -t", "fr-FR": "-f -t",
		"gl": "-f", // a decimal comma, and no month names to give
	} {
		args, err := localeArgs(t.TempDir(), lang)
		if err != nil {
			t.Fatal(err)
		}
		var flags []string
		for _, a := range args {
			if strings.HasPrefix(a, "-") {
				flags = append(flags, a)
			}
		}
		if got := strings.Join(flags, " "); got != want {
			t.Errorf("lang %q: flags %q, want %q", lang, got, want)
		}
	}
}

// A chart of dates in a language with no month names stops: Vega would print
// Jan on a Galician page. Numbers alone, or English, or a table, pass.
func TestTimelessStopsDatesWithoutNames(t *testing.T) {
	dated := map[string]any{"encoding": map[string]any{"x": map[string]any{"field": "d", "type": "temporal"}}}
	unit := map[string]any{"encoding": map[string]any{"x": map[string]any{"field": "d", "timeUnit": "month"}}}
	plain := map[string]any{"encoding": map[string]any{"x": map[string]any{"field": "n", "type": "quantitative"}}}
	if timeless(dated, "gl") == nil || timeless(unit, "eu-ES") == nil {
		t.Error("dates in Galician or Basque passed")
	}
	for _, c := range []struct {
		v    any
		lang string
	}{{plain, "gl"}, {dated, "en-GB"}, {dated, ""}, {dated, "de-DE"}, {dated, "fr"}} {
		if err := timeless(c.v, c.lang); err != nil {
			t.Errorf("%s: %v", c.lang, err)
		}
	}
}

// The number locale is what makes 30,000 print as 30.000; written wrong, the
// chart prints in English and the build exits 0.
func TestSpanishLocaleFiles(t *testing.T) {
	dir := t.TempDir()
	if _, err := localeArgs(dir, "es-ES"); err != nil {
		t.Fatal(err)
	}
	num, _ := os.ReadFile(filepath.Join(dir, ".mdbrand-number-locale.json"))
	tim, _ := os.ReadFile(filepath.Join(dir, ".mdbrand-time-locale.json"))
	for _, want := range []string{`"decimal":","`, `"thousands":"."`, `"grouping":[3]`} {
		if !strings.Contains(string(num), want) {
			t.Errorf("number locale lacks %s: %s", want, num)
		}
	}
	for _, want := range []string{`"shortMonths":["ene"`, `"months":["enero"`, `"periods":["AM","PM"]`} {
		if !strings.Contains(string(tim), want) {
			t.Errorf("time locale lacks %s", want)
		}
	}
}
