package exit

import (
	"errors"
	"fmt"
	"testing"
)

// The status is part of the contract and nothing on the screen shows it, so
// a mark lost to a wrapper further out would change it in silence.
func TestCode(t *testing.T) {
	base := errors.New("cause")
	for _, c := range []struct {
		name string
		err  error
		want int
	}{
		{"nil", nil, OK},
		{"unmarked", base, Defect},
		{"marked nil stays nil", AsUsage(nil), OK},
		{"usage", AsUsage(base), Usage},
		{"environment through a %w wrapper", fmt.Errorf("building: %w", AsEnvironment(base)), Environment},
		{"the innermost mark wins", AsUsage(fmt.Errorf("x: %w", AsEnvironment(base))), Environment},
	} {
		if got := Code(c.err); got != c.want {
			t.Errorf("%s: Code = %d, want %d", c.name, got, c.want)
		}
	}
}
