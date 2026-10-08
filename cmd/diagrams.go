package cmd

import (
	"fmt"
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
		Args: cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if brandName == "" {
				brandName = viper.GetString("brand")
			}
			b, err := brand.Load(brandsDir(), brandName)
			if err != nil {
				return err
			}
			work, err := os.MkdirTemp("", "mdbrand-diagrams-")
			if err != nil {
				return err
			}
			defer os.RemoveAll(work)

			var figs []*doc.Fig
			docStyle := ""
			// A chart may name its data; that needs the document it came from.
			sets := map[*doc.Fig]fig.Datasets{}
			for _, a := range args {
				switch {
				case strings.HasSuffix(a, ".md"):
					d, err := doc.Read(a)
					if err != nil {
						return err
					}
					if docStyle == "" {
						docStyle = d.Meta.Options.Style
					}
					_, fs, err := d.ExtractFigs(work)
					if err != nil {
						return err
					}
					store, err := data.Open(a, d.Meta.Options.Data)
					if err != nil {
						return err
					}
					for _, f := range fs {
						sets[f] = build.Datasets(store, d.Meta.Lang)
					}
					figs = append(figs, fs...)
				default:
					p, err := filepath.Abs(a)
					if err != nil {
						return err
					}
					kind := "vega"
					if strings.HasSuffix(p, ".d2") {
						kind = "d2"
					}
					figs = append(figs, &doc.Fig{Kind: kind, SrcPath: p, BaseDir: filepath.Dir(p), Attrs: map[string]string{}, Index: len(figs)})
				}
			}
			if len(figs) == 0 {
				return fmt.Errorf("no diagram sources found")
			}

			// A slide's measure, band and height are not the page's.
			style := build.Pick(styleFlag, docStyle, viper.GetString("style"), "report")
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
					if err := os.MkdirAll(outDir, 0o755); err != nil {
						return err
					}
					dst := filepath.Join(outDir, strings.TrimSuffix(filepath.Base(f.SrcPath), filepath.Ext(f.SrcPath))+".pdf")
					raw, err := os.ReadFile(res.PDF)
					if err != nil {
						return err
					}
					if err := os.WriteFile(dst, raw, 0o644); err != nil {
						return err
					}
					fmt.Fprintf(out, "%-26s -> %s\n", "", dst)
				}
			}
			if bad > 0 {
				return fmt.Errorf("%d diagram(s) failed", bad)
			}
			return nil
		},
	}
	c.Flags().StringVar(&brandName, "brand", "", "brand bundle whose page numbers to use")
	c.Flags().StringVar(&outDir, "out", "", "also write the rendered PDFs here")
	c.Flags().StringVar(&styleFlag, "style", "", "style whose measure to use (slides differs from the page); overrides the front matter")
	return c
}
