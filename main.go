// mdbrand turns Markdown into a branded A4 PDF: pandoc and XeLaTeX underneath,
// a brand bundle for the identity, D2 and Vega-Lite figures sized so their text
// is legible on paper.
package main

import (
	"embed"
	"fmt"
	"os"

	"github.com/carlosprados/mdbrand/cmd"
)

// The agent skill travels inside the binary, so `mdbrand skill install` works
// for someone who downloaded a release and has no checkout of this repository.
// The embed lives here because a //go:embed directive cannot reach a file
// outside its own package directory, and SKILL.md belongs at the repository
// root where people expect to read it.
//
//go:embed SKILL.md
var docs embed.FS

func main() {
	if raw, err := docs.ReadFile("SKILL.md"); err == nil {
		cmd.SkillDoc = string(raw)
	}
	if err := cmd.Root().Execute(); err != nil {
		fmt.Fprintln(os.Stderr, "mdbrand: "+err.Error())
		os.Exit(1)
	}
}
