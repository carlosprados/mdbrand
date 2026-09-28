package tex

import (
	"fmt"
	"strings"
	"testing"
)

func longtable(rows int) string {
	var b strings.Builder
	b.WriteString("\\begin{longtable}[]{@{}ll@{}}\n\\toprule\\noalign{}\na & b \\\\\n\\midrule\\noalign{}\n\\endhead\n\\bottomrule\\noalign{}\n\\endlastfoot\n")
	for i := range rows {
		fmt.Fprintf(&b, "r%d & x \\\\\n", i)
	}
	b.WriteString("\\end{longtable}\n")
	return b.String()
}

// breaks lists the body rows after which longtable may still break.
func breaks(src string) []string {
	var out []string
	for ln := range strings.SplitSeq(src, "\n") {
		if strings.HasPrefix(ln, "r") && strings.HasSuffix(ln, ` \\`) {
			out = append(out, strings.Fields(ln)[0])
		}
	}
	return out
}

func TestKeepTableRows(t *testing.T) {
	for rows, want := range map[int]string{
		3: "",      // the case that was found: never split
		5: "",      // under twice TableKeep: never split
		6: "r2",    // 3+3
		7: "r2 r3", // 3+4 or 4+3
		9: "r2 r3 r4 r5",
	} {
		got := strings.Join(breaks(KeepTableRows(longtable(rows))), " ")
		if got != want {
			t.Errorf("%d rows: breakable after %q, want %q", rows, got, want)
		}
	}
	// The header row ends with \\ too, and must not be touched.
	if out := KeepTableRows(longtable(3)); !strings.Contains(out, "a & b \\\\\n") {
		t.Errorf("the header row was changed:\n%s", out)
	}
}
