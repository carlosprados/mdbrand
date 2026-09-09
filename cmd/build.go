package cmd

import (
	"fmt"
	"path/filepath"

	"github.com/carlosprados/mdbrand/internal/build"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

func buildCmd() *cobra.Command {
	var o build.Options
	var quiet bool

	c := &cobra.Command{
		Use:   "build <document.md>",
		Short: "Build a branded A4 PDF from a Markdown document",
		Long: `Build a branded A4 PDF from Markdown.

Everything can come from the document's front matter, so the usual invocation
carries no flags at all:

  mdbrand build informe.md

Diagrams are rendered and placed automatically. Write them either as a fenced
block or as a link to a side file:

    ` + "```d2 caption=\"Arquitectura del motor\"" + `
    mesa: Mesa de Teleservicios
    mesa -> og.trainer: listas
    ` + "```" + `

    ![Latencia por percentil](diagrams/latencia.vl.json)

Each figure is placed at the widest size that keeps its labels inside the
legibility band (min_text_pt..max_text_pt in the brand bundle) without growing
taller than max_height_mm. If a diagram cannot satisfy that, the build stops and
says what to change — it does not ship an illegible figure.

A document whose text uses a glyph the font lacks also stops the build: those
characters print as nothing and only the XeLaTeX log would ever know.

Citations need no flag either — declaring a bibliography in the front matter is
what turns them on:

    bibliography: refs.bib      # a path, or a list of them
    csl: apa.csl                # optional

Those paths resolve against the document, not the working directory, and one
that is not there stops the build by name instead of shipping a PDF full of
[@key?].`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			o.Input = args[0]
			o.BrandsDir = brandsDir()
			o.DefaultBrand = viper.GetString("brand")
			o.DefaultStyle = viper.GetString("style")
			if !quiet {
				o.Log = func(f string, a ...any) { fmt.Fprintf(cmd.ErrOrStderr(), f+"\n", a...) }
			}
			rep, err := build.Run(o)
			if err != nil {
				return err
			}
			out := cmd.OutOrStdout()
			fmt.Fprintf(out, "%s  (%d pages, brand %s, style %s)\n",
				rep.Output, rep.Pages, rep.Brand, rep.Style)
			for _, f := range rep.Figures {
				fmt.Fprintf(out, "  fig %-22s %.0f×%.0f mm   text %.1fpt\n",
					filepath.Base(f.Fig.SrcPath), f.WidthMM, f.HeightMM, f.TextPt)
			}
			for _, w := range rep.Warnings {
				fmt.Fprintf(cmd.ErrOrStderr(), "  ! %s\n", w)
			}
			if rep.Kept {
				fmt.Fprintf(cmd.ErrOrStderr(), "  work dir kept: %s\n", rep.WorkDir)
			}
			return nil
		},
	}
	c.Flags().StringVarP(&o.Output, "out", "o", "", "output PDF (default: alongside the input)")
	c.Flags().StringVar(&o.BrandName, "brand", "", "brand bundle to use; overrides the front matter")
	c.Flags().StringVar(&o.Style, "style", "", "report | note | letter; overrides the front matter")
	c.Flags().StringVar(&o.WorkDir, "work", "", "keep intermediates here (LaTeX, figures, log) for debugging")
	c.Flags().BoolVar(&o.AllowHoles, "allow-missing-glyphs", false, "build even if the font lacks glyphs the text uses")
	c.Flags().BoolVarP(&quiet, "quiet", "q", false, "only print the result line")
	return c
}
