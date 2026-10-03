package cmd

import (
	"fmt"
	"io"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/carlosprados/mdbrand/internal/build"
	"github.com/carlosprados/mdbrand/internal/watch"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

func buildCmd() *cobra.Command {
	var o build.Options
	var quiet, watching bool

	c := &cobra.Command{
		Use:   "build <document.md>",
		Short: "Build a branded A4 PDF, and a .docx if asked, from a Markdown document",
		Long: `Build a branded A4 PDF from Markdown — and, with --to docx, a Word file.

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

A chart's "data": {"url": "data/x.csv"} resolves against the document, or
against the spec's own file when it is linked. A missing file or a remote URL
stops the build: vl2svg alone would draw an empty chart and exit 0.

Label sizes are mdbrand's to set: it appends the font-size globs to a copy of
the .d2 before compiling it, so the source stays as you wrote it and a vars
block does not collide with a glob you had to paste.

Code blocks are fitted the same way figures are. A bare fence is set at the
largest size whose longest line stays inside the measure, stepping down as far
as \footnotesize; one that still does not fit is reported with the columns it
has and the columns that fit. A fence that declares a language is wrapped
instead — a bare fence never is, because it may well be an ASCII diagram and
wrapping one destroys its alignment in silence.

A document whose text uses a glyph the font lacks also stops the build: those
characters print as nothing and only the XeLaTeX log would ever know. A bundle
may name fonts.fallback (Noto Sans Symbols2, say): the characters the body face
lacks and that face has are set in it, and anything neither covers still stops.

The finished PDF is read back with pdftotext (poppler), and a private-use
character in its text layer that the source does not hold stops the build: the
page prints right, but copy-paste, search and plagiarism checkers read garbage.
Contextual alternates are off for that reason — Inter's turned "(SD1)" into
U+EE4E SD1 U+EE4F. Without pdftotext the build warns that it was not checked.

[words]{.accent} sets words in the brand's primary colour, in the PDF and the
.docx alike. A span or div with color=, colour= or style= stops the build:
pandoc drops those attributes in silence, so the words would print black.

A picture linked as .svg is treated as a figure — checked for <foreignObject>,
sized for legible labels and converted by rsvg-convert. PNG, JPEG and PDF
pictures keep their markup; every relative path resolves against the document,
and one that is not there stops the build by name.

Citations need no flag either — declaring a bibliography in the front matter is
what turns them on:

    bibliography: refs.bib      # a path, or a list of them
    csl: apa.csl                # optional

Those paths resolve against the document, not the working directory, and one
that is not there stops the build by name instead of shipping a PDF full of
[@key?].

mdbrand owns the preamble, so a document that sets header-includes,
include-before or include-after is refused rather than built without them:
pandoc's --include-in-header and its two siblings, which is how the design gets
in, replace the metadata fields of those names. Settings that should outlive one
document belong in the brand bundle.

{{data.maquinas[m5.large].ram}} prints a value from the data files beside the
document (data/maquinas.yaml), literally and as written; a key that is not
there stops the build. See mdbrand data --help.

Every build counts the document's words and prints the number; {{words}},
anywhere in the body, the front matter or a caption, puts it in the PDF,
grouped the way the document's lang writes thousands (4.512 in es, 4,512 in
en). It is a placeholder, not a template: inside code or mathematics it is left
as written, and a {{name}} that does not exist stops the build.

The default criterion is the International Baccalaureate's (Extended Essay, TOK
essay), which is also a journal's "main text": prose, lists, headings, block
quotes and footnotes with content count; the front matter and cover, code,
figures and their captions, tables, mathematics, citations, notes that only
cite and the bibliography do not. A heading marked {.nocount} leaves out its
whole section — appendices, acknowledgements, an abstract with its own limit —
and so does a ::: {.nocount} div.

    mdbrand:
      wordcount: all              # every word printed, bibliography included
      wordcount:                  # or a profile with parts added to it
        base: ib
        include: [captions, tables]

Parts: captions, tables, footnotes (every note, citations in it included), citations,
references, code, math (one word per formula). --wordcount picks a profile and
replaces the front matter's criterion, parts included. Word processors disagree
with each other by a percent or two over dashes, numbers and URLs, and so does
this: near a hard limit, leave a margin.

--to docx writes a branded .docx for Word, LibreOffice or Google Docs; --to
pdf,docx writes both from one preparation, each beside the other under --out's
name. The front matter can say it instead (mdbrand: {formats: [pdf, docx]}),
and pdf is the default. The .docx has the cover, header, figures at the size
the PDF gives them, numbered captions, tables that never break a word, and a
filled table of contents Word updates on opening. A .docx carries no fonts, so
a named bundle must declare fonts.office (body, display, mono), faces the
readers have; the build stops without them, and refuses a .docx asking for any
other face, or holding a picture linked as PDF. For Google Docs, upload it
converted: gog drive upload informe.docx --convert-to doc

--watch keeps mdbrand running and rebuilds whenever a file the build read
changes: the document, its linked figures and pictures, the bibliography and
CSL, the bundle's brand.yaml and logos. The set is taken from each build, so a
figure linked a minute ago is watched from the next save. A failed build prints
its error and leaves the last good PDF where it was; the output is replaced by
rename, so a viewer that reloads on change (zathura, evince) never reads half a
file. Ctrl-C stops it.

    mdbrand build informe.md --watch`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			o.Input = args[0]
			o.BrandsDir = brandsDir()
			o.DefaultBrand = viper.GetString("brand")
			o.DefaultStyle = viper.GetString("style")
			o.DefaultWordCount = viper.GetString("wordcount")
			o.DefaultFormats = viper.GetStringSlice("formats")
			if !quiet {
				o.Log = func(f string, a ...any) { fmt.Fprintf(cmd.ErrOrStderr(), f+"\n", a...) }
			}
			if !watching {
				rep, err := build.Run(o)
				if err != nil {
					return err
				}
				printReport(cmd.OutOrStdout(), cmd.ErrOrStderr(), rep)
				return nil
			}

			ctx, stop := signal.NotifyContext(cmd.Context(), os.Interrupt, syscall.SIGTERM)
			defer stop()
			errOut := cmd.ErrOrStderr()
			logf := func(f string, a ...any) { fmt.Fprintf(errOut, f+"\n", a...) }
			return watch.Run(ctx, func() []string {
				start := time.Now()
				rep, err := build.Run(o)
				stamp := start.Format("15:04:05")
				if err != nil {
					fmt.Fprintf(errOut, "%s  build failed, last good PDF kept:\n%v\n", stamp, err)
				} else {
					fmt.Fprintf(cmd.OutOrStdout(), "%s  %.1fs  ", stamp, time.Since(start).Seconds())
					printReport(cmd.OutOrStdout(), errOut, rep)
				}
				fmt.Fprintf(errOut, "  watching %d file(s) — Ctrl-C to stop\n", len(rep.Inputs))
				return rep.Inputs
			}, logf)
		},
	}
	c.Flags().StringVarP(&o.Output, "out", "o", "", "output file (default: alongside the input); with several formats, each takes its own extension")
	c.Flags().StringSliceVar(&o.Formats, "to", nil, "pdf | docx | pdf,docx; overrides the front matter's formats (default pdf)")
	c.Flags().StringVar(&o.BrandName, "brand", "", "brand bundle to use; overrides the front matter")
	c.Flags().StringVar(&o.Style, "style", "", "report | note | letter; overrides the front matter")
	c.Flags().StringVar(&o.WordCount, "wordcount", "", "ib | all: the {{words}} criterion; replaces the front matter's entirely")
	c.Flags().StringVar(&o.WorkDir, "work", "", "keep intermediates here (LaTeX, figures, log) for debugging")
	c.Flags().BoolVar(&o.AllowHoles, "allow-missing-glyphs", false, "build even if the font lacks glyphs the text uses")
	c.Flags().BoolVarP(&quiet, "quiet", "q", false, "only print the result line")
	c.Flags().BoolVarP(&watching, "watch", "w", false, "stay running and rebuild when a file the build read changes")
	return c
}

func printReport(out, errOut io.Writer, rep *build.Report) {
	for _, o := range rep.Outputs {
		if o.Format == "pdf" {
			fmt.Fprintf(out, "%s  (%d pages, %d words by %s, brand %s, style %s)\n",
				o.Path, o.Pages, rep.Words, rep.WordRule, rep.Brand, rep.Style)
			continue
		}
		// A .docx has no page count of its own: whatever opens it paginates it.
		fmt.Fprintf(out, "%s  (%d words by %s, brand %s, style %s)\n",
			o.Path, rep.Words, rep.WordRule, rep.Brand, rep.Style)
	}
	for _, f := range rep.Figures {
		fmt.Fprintf(out, "  fig %-22s %.0f×%.0f mm   text %.1fpt\n",
			filepath.Base(f.Fig.SrcPath), f.WidthMM, f.HeightMM, f.TextPt)
	}
	for _, w := range rep.Warnings {
		fmt.Fprintf(errOut, "  ! %s\n", w)
	}
	if rep.Kept {
		fmt.Fprintf(errOut, "  work dir kept: %s\n", rep.WorkDir)
	}
}
