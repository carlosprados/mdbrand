package fig

import (
	"encoding/json"
	"strings"
	"testing"
)

func spec(t *testing.T, s string) any {
	t.Helper()
	var v any
	if err := json.Unmarshal([]byte(s), &v); err != nil {
		t.Fatal(err)
	}
	return v
}

// A plotted field the data lacks is a typo only where every column is known
// and the spec makes none of its own.
func TestUnknownFieldsOnlyWhereTheDataIsKnown(t *testing.T) {
	const values = `"data": {"values": [{"sede": "A", "coste": 3, "geo": {"lat": 1}, "a.b": 2}]}`
	cases := []struct {
		name, spec, want string
	}{
		{"nominal typo", `{` + values + `, "encoding": {"x": {"field": "sedes"}, "y": {"field": "coste"}}}`, `"sedes" — did you mean "sede"?`},
		{"nested and escaped", `{` + values + `, "encoding": {"x": {"field": "geo.lat"}, "y": {"field": "a\\.b"}}}`, ""},
		{"nested typo", `{` + values + `, "encoding": {"x": {"field": "goe.lat"}}}`, `"goe.lat"`},
		{"all present", `{` + values + `, "encoding": {"x": {"field": "sede"}, "y": {"field": "coste"}}}`, ""},
		{"spec's own dataset", `{"datasets": {"d": [{"a": 1}]}, "data": {"name": "d"}, "encoding": {"x": {"field": "b"}}}`, `"b"`},
		{"a transform derives fields", `{` + values + `, "transform": [{"calculate": "1", "as": "uno"}], "encoding": {"x": {"field": "uno"}}}`, ""},
		{"a json url is unknown", `{"data": {"url": "x.json"}, "encoding": {"x": {"field": "z"}}}`, ""},
		{"a layer's url makes it unknown", `{"layer": [{` + values + `}, {"data": {"url": "x.json"}}], "encoding": {"x": {"field": "z"}}}`, ""},
	}
	for _, c := range cases {
		got := strings.Join(unknownFields(spec(t, c.spec)), "\n")
		if c.want == "" && got != "" || c.want != "" && !strings.Contains(got, c.want) {
			t.Errorf("%s: got %q, want %q", c.name, got, c.want)
		}
	}
}

// Vega's own warning names a stacked field as X_start and X_end; the author
// wrote X, once.
func TestEmptyExtentsNameTheFieldAsWritten(t *testing.T) {
	log := "WARN Infinite extent for field \"cost_start\": [Infinity, -Infinity]\n" +
		"WARN Infinite extent for field \"cost_end\": [Infinity, -Infinity]\n"
	got := strings.Join(emptyExtents(log, map[string]bool{"coste": true}), "\n")
	if strings.Count(got, `"cost"`) != 1 || !strings.Contains(got, `did you mean "coste"?`) {
		t.Errorf("got %q", got)
	}
	if got := emptyExtents("WARN something else\n", nil); got != nil {
		t.Errorf("unrelated warning reported: %q", got)
	}
}
