// mdbrand turns Markdown into a branded A4 PDF: pandoc and XeLaTeX underneath,
// a brand bundle for the identity, D2 and Vega-Lite figures sized so their text
// is legible on paper.
package main

import (
	"embed"
	"fmt"
	"io/fs"
	"os"

	"github.com/carlosprados/mdbrand/cmd"
	"github.com/carlosprados/mdbrand/internal/exit"
)

// The agent skill travels inside the binary, so `mdbrand skill install` works
// for someone who downloaded a release and has no checkout of this repository.
// The embed lives here because a //go:embed directive cannot reach a file
// outside its own package directory, and SKILL.md belongs at the repository
// root where people expect to read it.
//
//go:embed SKILL.md
var docs embed.FS

// The examples travel the same way, so an agent on any machine can start
// from one that builds. Sources only: a PDF built beside them must not be
// carried into the binary.
//
//go:embed examples/*/*.md examples/*/*.bib examples/*/data/*
var examples embed.FS

func main() {
	if raw, err := docs.ReadFile("SKILL.md"); err == nil {
		cmd.SkillDoc = string(raw)
	}
	if sub, err := fs.Sub(examples, "examples"); err == nil {
		cmd.Examples = sub
	}
	if err := cmd.Root().Execute(); err != nil {
		fmt.Fprintln(os.Stderr, "mdbrand: "+err.Error())
		os.Exit(exit.Code(err))
	}
}
