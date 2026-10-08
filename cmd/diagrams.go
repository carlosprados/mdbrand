package cmd

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/carlosprados/mdbrand/internal/brand"
	"github.com/carlosprados/mdbrand/internal/build"
	"github.com/carlosprados/mdbrand/internal/data"
	"github.com/carlosprados/mdbrand/internal/doc"
	"github.com/carlosprados/mdbrand/internal/fig"
	"github.com/carlosprados/mdbrand/internal/tex"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

func diagramsCmd() *cobra.Command {
	var brandName, outDir, styleFlag string

	c := &cobra.Command{
		Use:   "diagrams <document.md | diagram.d2 | chart.vl.json>...",
		Short: "Render diagrams and report how they will sit on the page",
		Long: `Render diagram sources and report, for each, the size it will occupy on the
page and the point size of its smallest label. Use it to iterate on a diagram
without rebuilding the document.

The numbers come from the brand bundle: the text measure follows from its
margin, and the legibility band from its diagrams settings. A figure that
cannot be placed inside that band is reported as an error with the fix.

  mdbrand diagrams informe.md            every diagram in the document
  mdbrand diagrams arq.d2 --out figs/    render one and keep the PDF`,
		Args: usageArgs(cobra.MinimumNArgs(1)),
		RunE: func(cmd *cobra.Command, args []string) error {
			work, err := os.MkdirTemp("", "mdbrand-diagrams-")
			if err != nil {
				return err
			}
			defer os.RemoveAll(work)

			src, err := collectFigs(args, work)
			if err != nil {
				return err
			}
			figs, sets := src.figs, src.sets
			if len(figs) == 0 {
				return fmt.Errorf("no diagram sources found")
			}

			// The same precedence as build: a document declaring brand: none
			// was measured with the configured bundle's page and band.
			b, err := brand.Load(brandsDir(), build.Pick(brandName, src.brand, viper.GetString("brand")))
			if err != nil {
				return err
			}
			// A slide's measure, band and height are not the page's.
			style := build.Pick(styleFlag, src.style, viper.GetString("style"), "report")
			if !tex.ValidStyle(style) {
				return fmt.Errorf("unknown style %q: pick one of %s", style, strings.Join(tex.Styles, ", "))
			}
			fb, textWidth, err := tex.FigureBox(b, style)
			if err != nil {
				return err
			}

			out := cmd.OutOrStdout()
			fmt.Fprintf(out, "brand %s · style %s · measure %.1f mm · text band %.1f–%.1f pt · max height %.0f mm\n\n",
				b.Name, style, textWidth, fb.Diagrams.MinTextPt, fb.Diagrams.MaxTextPt, fb.Diagrams.MaxHeightMM)
			bad := 0
			for _, f := range figs {
				res, err := fig.Render(f, fb, work, textWidth, sets[f])
				if err != nil {
					bad++
					fmt.Fprintf(out, "%-26s FAIL\n%v\n\n", filepath.Base(f.SrcPath), err)
					continue
				}
				fmt.Fprintf(out, "%-26s %6.1f × %-6.1f mm   text %4.1f pt   %s\n",
					filepath.Base(f.SrcPath), res.WidthMM, res.HeightMM, res.TextPt, res.Note)
				if outDir != "" {
					if err := keepPDF(out, outDir, f, res); err != nil {
						return err
					}
				}
			}
			if bad > 0 {
				return fmt.Errorf("%d diagram(s) failed", bad)
			}
			return nil
		},
	}
	c.Flags().StringVar(&brandName, "brand", "", "brand bundle whose page numbers to use; overrides the front matter")
	c.Flags().StringVar(&outDir, "out", "", "also write the rendered PDFs here")
	c.Flags().StringVar(&styleFlag, "style", "", "style whose measure to use (slides differs from the page); overrides the front matter")
	return c
}

// diagramSources is what the arguments hold: the figures, the data each chart
// reads by name, and the brand and style the first document declares.
type diagramSources struct {
	figs         []*doc.Fig
	sets         map[*doc.Fig]fig.Datasets
	brand, style string
}

// collectFigs reads every argument: a document's figures, or a source file
// rendered on its own.
func collectFigs(args []string, work string) (*diagramSources, error) {
	src := &diagramSources{sets: map[*doc.Fig]fig.Datasets{}}
	for _, a := range args {
		if !strings.HasSuffix(a, ".md") {
			p, err := filepath.Abs(a)
			if err != nil {
				return nil, err
			}
			kind := "vega"
			if strings.HasSuffix(p, ".d2") {
				kind = "d2"
			}
			src.figs = append(src.figs, &doc.Fig{Kind: kind, SrcPath: p, BaseDir: filepath.Dir(p), Attrs: map[string]string{}, Index: len(src.figs)})
			continue
		}
		d, err := doc.Read(a)
		if err != nil {
			return nil, err
		}
		src.brand = build.Pick(src.brand, d.Meta.Options.Brand)
		src.style = build.Pick(src.style, d.Meta.Options.Style)
		_, fs, err := d.ExtractFigs(work)
		if err != nil {
			return nil, err
		}
		// A chart may name its data; that needs the document it came from.
		store, err := data.Open(a, d.Meta.Options.Data)
		if err != nil {
			return nil, err
		}
		for _, f := range fs {
			src.sets[f] = build.Datasets(store, d.Meta.Lang)
		}
		src.figs = append(src.figs, fs...)
	}
	return src, nil
}

// keepPDF copies a rendered figure's PDF into dir, for --out.
func keepPDF(out io.Writer, dir string, f *doc.Fig, res *fig.Result) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	dst := filepath.Join(dir, strings.TrimSuffix(filepath.Base(f.SrcPath), filepath.Ext(f.SrcPath))+".pdf")
	raw, err := os.ReadFile(res.PDF)
	if err != nil {
		return err
	}
	if err := os.WriteFile(dst, raw, 0o644); err != nil {
		return err
	}
	fmt.Fprintf(out, "%-26s -> %s\n", "", dst)
	return nil
}
