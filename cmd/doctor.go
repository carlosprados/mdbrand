package cmd

import (
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/carlosprados/mdbrand/internal/brand"
	"github.com/carlosprados/mdbrand/internal/run"
	"github.com/spf13/cobra"
)

type check struct {
	name, hint string
	required   bool
	ok         bool
	detail     string
}

func doctorCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "doctor",
		Short: "Check the toolchain and print exactly how to fix what is missing",
		Long: `Verify every external piece a build needs and print the install command for
what is absent. Required for any build: pandoc, xelatex, rsvg-convert and a few
LaTeX packages. Required only if the document has diagrams: d2, vl2svg.`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			out := cmd.OutOrStdout()
			var checks []check

			bin := func(name, hint string, required bool) {
				c := check{name: name, hint: hint, required: required, ok: run.Have(name)}
				if c.ok {
					c.detail, _ = exec.LookPath(name)
				}
				checks = append(checks, c)
			}
			bin("pandoc", "apt install pandoc", true)
			bin("xelatex", "apt install texlive-xetex", true)
			bin("rsvg-convert", "apt install librsvg2-bin", true)
			bin("d2", "https://d2lang.com/tour/install (or: brew install d2)", false)
			bin("vl2svg", "npm i -g vega-cli vega-lite", false)

			// LaTeX packages: a missing .sty is a build failure whose message
			// names the file and not the package that carries it.
			for _, p := range []struct{ sty, pkg string }{
				{"fancyhdr", "texlive-latex-extra"},
				{"geometry", "texlive-latex-base"},
				{"fontspec", "texlive-xetex"},
				{"etoolbox", "texlive-latex-recommended"},
				{"microtype", "texlive-latex-recommended"},
				{"caption", "texlive-latex-recommended"},
				{"xcolor", "texlive-latex-recommended"},
			} {
				c := check{name: "latex: " + p.sty, hint: "apt install " + p.pkg, required: true}
				if o, err := run.Cmd("", "kpsewhich", p.sty+".sty"); err == nil && strings.TrimSpace(o) != "" {
					c.ok = true
				}
				checks = append(checks, c)
			}

			// d2's own PDF export is deliberately unused; say so once here so
			// nobody "fixes" the pipeline by reaching for it.
			fails := 0
			for _, c := range checks {
				mark, label := "ok  ", ""
				if !c.ok {
					if c.required {
						mark, fails = "MISSING", fails+1
					} else {
						mark = "absent"
					}
					label = "   -> " + c.hint
				}
				fmt.Fprintf(out, "  %-7s %-22s %s%s\n", mark, c.name, c.detail, label)
			}

			// Brands.
			dir := brandsDir()
			names := brand.List(dir)
			fmt.Fprintf(out, "\n  brands dir: %s\n", dir)
			if _, err := os.Stat(dir); err != nil {
				fmt.Fprintf(out, "    does not exist yet -> mdbrand brand new <name>\n")
			} else if len(names) == 0 {
				fmt.Fprintf(out, "    no bundles yet -> mdbrand brand new <name>\n")
			}
			for _, n := range names {
				b, err := brand.Load(dir, n)
				if err != nil {
					fmt.Fprintf(out, "    %-12s ERROR %v\n", n, err)
					continue
				}
				probs, warns := b.Check()
				status := "ok"
				if len(probs) > 0 {
					status = fmt.Sprintf("%d problem(s)", len(probs))
				} else if len(warns) > 0 {
					status = fmt.Sprintf("%d warning(s)", len(warns))
				}
				fmt.Fprintf(out, "    %-12s %s\n", n, status)
			}

			// Fonts declared by the installed bundles.
			if run.Have("fc-list") {
				fmt.Fprintln(out)
				for _, n := range names {
					b, err := brand.Load(dir, n)
					if err != nil {
						continue
					}
					o, _ := run.Cmd("", "fc-list", b.Fonts.Body)
					if strings.TrimSpace(o) == "" {
						fmt.Fprintf(out, "  MISSING body font %q for brand %s -> install it, or change fonts.body\n", b.Fonts.Body, n)
						fails++
					} else {
						fmt.Fprintf(out, "  ok      body font %-14s (brand %s)\n", b.Fonts.Body, n)
					}
				}
			}

			if fails > 0 {
				return fmt.Errorf("%d required item(s) missing", fails)
			}
			fmt.Fprintln(out, "\n  all good")
			return nil
		},
	}
}
