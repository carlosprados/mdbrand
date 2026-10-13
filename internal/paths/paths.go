// Package paths expands the paths a person writes into something shared — a
// bundle's font candidates, the configured brands directory, a document's
// mdbrand.data — so that none of them has to carry one machine's home.
package paths

import (
	"os"
	"path/filepath"
	"strings"
)

// Expand resolves $VARS and a leading ~. Variables go first, so ~/$CLIENT
// expands both. An unset variable is left as written: os.ExpandEnv collapsed
// it to nothing, and $UNSET/brands became /brands, a plausible path on the
// wrong machine. Kept, it names a path that does not exist and says which
// variable it wanted. ~ is left as written when there is no home directory,
// for the same reason.
func Expand(p string) string {
	p = os.Expand(strings.TrimSpace(p), func(name string) string {
		if v, ok := os.LookupEnv(name); ok {
			return v
		}
		return "${" + name + "}"
	})
	if p == "~" || strings.HasPrefix(p, "~/") {
		if home, err := os.UserHomeDir(); err == nil {
			p = filepath.Join(home, strings.TrimPrefix(p, "~"))
		}
	}
	return p
}
