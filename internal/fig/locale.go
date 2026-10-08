package fig

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/carlosprados/mdbrand/internal/run"
	"github.com/carlosprados/mdbrand/internal/words"
)

// A chart in a Spanish document printed 30,000 and Jan: Vega formats in
// English unless its view is given a locale. Neither of the obvious ways
// reaches it. A spec's config.locale is overwritten by the {number: null,
// time: null} every vega CLI hands the view; vl2svg's -f and -t pass the
// file's text where an object is wanted, so -f drops the grouping and -t
// crashes; vega-cli's own -f and -t were a stub that read nothing until
// 6.4.0. What works is compiling with vl2vg and rendering with vg2svg -f -t
// from vega-cli 6.4.0 on, and probe checks that before a chart relies on it.
// Reported upstream as vega/vega#4361 and vega/vega-lite#9955: once both are
// fixed, a spec's config.locale through vl2svg would do, and this can go.

// numberLocale is the d3-format locale for lang, or nil where Vega's own
// English is right. The separators are the ones the document's tables use.
func numberLocale(lang string) map[string]any {
	thousands, decimal := words.Separators(lang)
	if decimal != "," {
		return nil
	}
	currency := []string{"", " €"}
	if strings.HasPrefix(strings.ToLower(lang), "da") {
		currency = []string{"", " kr."}
	}
	return map[string]any{"decimal": decimal, "thousands": thousands, "grouping": []int{3}, "currency": currency}
}

// timeLocale is the d3-time-format locale for lang, or nil. Only Spanish is
// here: month and day names nobody has checked are worse than English ones.
func timeLocale(lang string) map[string]any {
	if strings.ToLower(strings.SplitN(lang, "-", 2)[0]) != "es" {
		return nil
	}
	return map[string]any{
		"dateTime": "%A, %e de %B de %Y, %X", "date": "%d/%m/%Y", "time": "%H:%M:%S",
		"periods":     []string{"AM", "PM"},
		"days":        []string{"domingo", "lunes", "martes", "miércoles", "jueves", "viernes", "sábado"},
		"shortDays":   []string{"dom", "lun", "mar", "mié", "jue", "vie", "sáb"},
		"months":      []string{"enero", "febrero", "marzo", "abril", "mayo", "junio", "julio", "agosto", "septiembre", "octubre", "noviembre", "diciembre"},
		"shortMonths": []string{"ene", "feb", "mar", "abr", "may", "jun", "jul", "ago", "sep", "oct", "nov", "dic"},
	}
}

// localeArgs writes lang's locale files into dir and returns the vg2svg flags
// that load them, or nil when Vega's English is right for lang.
func localeArgs(dir, lang string) ([]string, error) {
	num, tim := numberLocale(lang), timeLocale(lang)
	if num == nil && tim == nil {
		return nil, nil
	}
	var args []string
	for _, l := range []struct {
		flag, name string
		v          map[string]any
	}{{"-f", ".mdbrand-number-locale.json", num}, {"-t", ".mdbrand-time-locale.json", tim}} {
		if l.v == nil {
			continue
		}
		raw, err := json.Marshal(l.v)
		if err != nil {
			return nil, err
		}
		if err := os.WriteFile(filepath.Join(dir, l.name), raw, 0o644); err != nil {
			return nil, err
		}
		args = append(args, l.flag, l.name)
	}
	return args, nil
}

var (
	probeOnce sync.Once
	probeErr  error
)

// probe renders a thousand under a locale with a dot for thousands and checks
// that the dot arrived. It is measured rather than read from a version, since
// vg2svg --version prints "unknown"; once per run is enough.
func probe(dir string) error {
	probeOnce.Do(func() {
		if missing := run.Missing("vl2vg", "vg2svg"); len(missing) > 0 {
			probeErr = fmt.Errorf("charts in this language need %s: npm i -g vega-cli vega-lite", strings.Join(missing, ", "))
			return
		}
		spec := `{"width":60,"height":20,"marks":[{"type":"text","encode":{"enter":{"text":{"signal":"format(1000, ',')"}}}}]}`
		num := `{"decimal":",","thousands":".","grouping":[3],"currency":["",""]}`
		if probeErr = os.WriteFile(filepath.Join(dir, ".mdbrand-probe.vg.json"), []byte(spec), 0o644); probeErr != nil {
			return
		}
		if probeErr = os.WriteFile(filepath.Join(dir, ".mdbrand-probe-locale.json"), []byte(num), 0o644); probeErr != nil {
			return
		}
		out, err := run.Cmd(dir, "vg2svg", "-f", ".mdbrand-probe-locale.json", ".mdbrand-probe.vg.json")
		if err == nil && strings.Contains(out, ">1.000<") {
			return
		}
		probeErr = fmt.Errorf(`this document's charts need numbers in its own language (1.000, not 1,000),
and the installed vega-cli cannot apply a locale: its -f and -t read nothing
before 6.4.0, so the charts would print in English and the build exit 0.
Upgrade it: npm i -g vega-cli@latest`)
	})
	return probeErr
}
