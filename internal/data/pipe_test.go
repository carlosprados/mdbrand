package data

import (
	"strings"
	"testing"
)

const longCell = "a description long enough to push the row past pandoc's seventy-two columns"

func TestSizePipeTables(t *testing.T) {
	cases := []struct {
		name, in, sep string // sep: the separator expected out, "" for unchanged
	}{
		{"equal dashes, wide row", "| Name | n | Note |\n|---|---|---|\n| Demócrata sin CIS | 12 | " + longCell + " |\n",
			"|-----------------|----|" + strings.Repeat("-", len([]rune(longCell))) + "|"},
		{"alignment kept", "| Name | n |\n|:---|---:|\n| x | " + longCell + " |\n",
			":------|" + strings.Repeat("-", len([]rune(longCell))) + ":|"},
		{"empty cell keeps the columns apart", "| a || c |\n|---|---|---|\n| x || " + longCell + " |\n",
			"|---|---|" + strings.Repeat("-", len([]rune(longCell))) + "|"},
		{"the author's dashes", "| a | b |\n|------|--|\n| x | " + longCell + " |\n", ""},
		{"narrow table, pandoc ignores dashes", "| a | b |\n|---|---|\n| x | y |\n", ""},
		{"inside a code block", "```\n| a | b |\n|---|---|\n| x | " + longCell + " |\n```\n", ""},
	}
	for _, c := range cases {
		got := SizePipeTables(c.in)
		if c.sep == "" {
			if got != c.in {
				t.Errorf("%s: changed\n%s", c.name, got)
			}
			continue
		}
		if !strings.Contains(got, c.sep) {
			t.Errorf("%s: want separator containing %q, got\n%s", c.name, c.sep, got)
		}
	}
}
