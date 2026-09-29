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
// expands both. An unset variable collapses to nothing rather than to a
// plausible path, and ~ is left as written when there is no home directory:
// either way the path fails to exist, loudly, instead of naming the wrong
// file — ~/brands must never quietly become /brands.
func Expand(p string) string {
	p = os.ExpandEnv(strings.TrimSpace(p))
	if p == "~" || strings.HasPrefix(p, "~/") {
		if home, err := os.UserHomeDir(); err == nil {
			p = filepath.Join(home, strings.TrimPrefix(p, "~"))
		}
	}
	return p
}
