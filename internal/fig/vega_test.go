package fig

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/carlosprados/mdbrand/internal/doc"
)

// A fenced block's source lives in the work directory; its urls must resolve
// against the document, or vl2svg draws an empty chart and exits 0.
func TestVegaSpecResolvesAgainstBaseDir(t *testing.T) {
	docDir, work := t.TempDir(), t.TempDir()
	must(t, os.MkdirAll(filepath.Join(docDir, "data"), 0o755))
	for _, n := range []string{"data/a.csv", "data/b.json"} {
		must(t, os.WriteFile(filepath.Join(docDir, n), []byte("x\n1\n"), 0o644))
	}
	src := filepath.Join(work, "fig00.vl.json")
	must(t, os.WriteFile(src, []byte(`{
	  "width": 1.50,
	  "layer": [
	    {"data": {"url": "data/a.csv"}, "mark": "bar"},
	    {"data": {"values": [{"x": 1}]}, "mark": "rule",
	     "transform": [{"lookup": "x", "from": {"data": {"url": "data/b.json"}, "key": "x"}}]}
	  ]}`), 0o644))

	spec, refs, err := VegaSpec(&doc.Fig{Kind: "vega", SrcPath: src, BaseDir: docDir})
	if err != nil {
		t.Fatal(err)
	}
	s := string(spec)
	for _, n := range []string{"data/a.csv", "data/b.json"} {
		want := "file://" + filepath.ToSlash(filepath.Join(docDir, n))
		if !strings.Contains(s, `"`+want+`"`) {
			t.Errorf("spec lacks %s:\n%s", want, s)
		}
	}
	if len(refs) != 2 {
		t.Errorf("refs = %v, want both data files", refs)
	}
	if !strings.Contains(s, `"width":1.50`) {
		t.Errorf("a number was reformatted on the way through:\n%s", s)
	}
}

func TestVegaSpecReadsYAML(t *testing.T) {
	dir := t.TempDir()
	must(t, os.WriteFile(filepath.Join(dir, "v.csv"), []byte("x\n1\n"), 0o644))
	src := filepath.Join(dir, "chart.vl.yaml")
	must(t, os.WriteFile(src, []byte("mark: bar\ndata: {url: v.csv}\n"), 0o644))

	spec, _, err := VegaSpec(&doc.Fig{Kind: "vega", SrcPath: src})
	if err != nil {
		t.Fatal(err)
	}
	var v map[string]any
	if err := json.Unmarshal(spec, &v); err != nil {
		t.Fatalf("not JSON: %v\n%s", err, spec)
	}
	if got := v["data"].(map[string]any)["url"]; got != "file://"+filepath.ToSlash(filepath.Join(dir, "v.csv")) {
		t.Errorf("url = %v, want the side file's own directory", got)
	}
}

// The missing file must still be reported as a ref: watch mode waits on it.
func TestVegaSpecMissingData(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "c.vl.json")
	must(t, os.WriteFile(src, []byte(`{"data": {"url": "data/nope.csv"}}`), 0o644))

	_, refs, err := VegaSpec(&doc.Fig{Kind: "vega", SrcPath: src, Caption: "Ventas"})
	if err == nil || !strings.Contains(err.Error(), "data that is not there") || !strings.Contains(err.Error(), "data/nope.csv") {
		t.Fatalf("err = %v, want it to name the missing url", err)
	}
	if len(refs) != 1 || refs[0] != filepath.Join(dir, "data/nope.csv") {
		t.Errorf("refs = %v, want the missing file", refs)
	}
}

func TestVegaSpecRefusesRemote(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "c.vl.json")
	for _, u := range []string{"https://example.com/d.csv", "//cdn.example.com/d.csv"} {
		must(t, os.WriteFile(src, []byte(`{"data": {"url": "`+u+`"}}`), 0o644))
		if _, _, err := VegaSpec(&doc.Fig{Kind: "vega", SrcPath: src}); err == nil || !strings.Contains(err.Error(), u) {
			t.Errorf("%s: err = %v, want a refusal naming it", u, err)
		}
	}
}

func must(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}
