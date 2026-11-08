package cmd

import (
	"fmt"

	"github.com/carlosprados/mdbrand/internal/build"
	"github.com/spf13/cobra"
)

func auditCmd() *cobra.Command {
	var o build.AuditOptions

	c := &cobra.Command{
		Use:   "audit <document.pdf>",
		Short: "Check a PDF mdbrand did not build: log, text layer, fonts, pictures, pages",
		Long: `Check a PDF built by something else — a template mandated by a university or
a journal, Overleaf, a colleague's LaTeX — for the defects a build checks on
its own PDFs. It reads the finished PDF and the LaTeX log beside it.

Defects (exit 1):
  - characters the face has no glyph for, printed as nothing (log)
  - a style the face lacks, set in its regular: emphasis printed plain (log)
  - references and citations to nothing, printed as ?? or [?] (log)
  - a page TeX overfilled while shipping it out (log)
  - private-use characters in the text layer: copy, search and plagiarism
    checkers read garbage
  - lines cut at the edge of the sheet
  - faces not embedded
  - pictures under 72 ppi
  - more pages than --max-pages, or another paper than --paper

Warnings (exit 0): lines over the measure by more than 5pt, pictures under
150 ppi, Type 3 faces (journals' checkers refuse them; matplotlib writes
them), a header taller than \headheight, a log asking for another run, and a
log that is not this PDF's.

The log is the .log beside the PDF, or --log; Overleaf gives it under "Logs
and output files". Without one, only the PDF is read, and that is said.

  mdbrand audit sample.pdf
  mdbrand audit sample.pdf --max-pages 10 --body-until References --paper a4
  mdbrand audit paper.pdf --log output.log`,
		Args: usageArgs(cobra.ExactArgs(1)),
		RunE: func(cmd *cobra.Command, args []string) error {
			o.PDF = args[0]
			a, err := build.RunAudit(o)
			if err != nil {
				return err
			}
			out, errOut := cmd.OutOrStdout(), cmd.ErrOrStderr()
			pages := plural(a.Pages, "page")
			if a.BodyPages > 0 {
				pages += fmt.Sprintf(", %d up to %q", a.BodyPages, o.BodyUntil)
			}
			log := "no log"
			if a.Log != "" {
				log = "log " + a.Log
			}
			fmt.Fprintf(out, "%s  (%s, %s, %s)\n", o.PDF, pages, a.Paper, log)
			for _, w := range a.Warnings {
				fmt.Fprintf(errOut, "  ! %s\n", w)
			}
			if len(a.Defects) == 0 {
				return nil
			}
			for _, d := range a.Defects {
				fmt.Fprintf(errOut, "  ✗ %s\n", d)
			}
			return fmt.Errorf("%s: %d defect(s)", o.PDF, len(a.Defects))
		},
	}
	c.Flags().StringVar(&o.Log, "log", "", "the LaTeX log (default: the .log beside the PDF)")
	c.Flags().IntVar(&o.MaxPages, "max-pages", 0, "the most pages the document may have")
	c.Flags().StringVar(&o.BodyUntil, "body-until", "", `count pages only up to this heading, e.g. "References"`)
	c.Flags().StringVar(&o.Paper, "paper", "", "the paper the PDF must be: a4 or letter")
	return c
}
