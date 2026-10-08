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
		"de-DE": "-f", "pt": "-f", "ca": "-f",
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
