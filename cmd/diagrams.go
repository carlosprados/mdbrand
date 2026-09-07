package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/carlosprados/mdbrand/internal/brand"
	"github.com/carlosprados/mdbrand/internal/doc"
	"github.com/carlosprados/mdbrand/internal/fig"
	"github.com/carlosprados/mdbrand/internal/tex"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

func diagramsCmd() *cobra.Command {
	var brandName, outDir string

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
			textWidth, err := tex.TextWidthMM(b)
			if err != nil {
				return err
			}
			work, err := os.MkdirTemp("", "mdbrand-diagrams-")
			if err != nil {
				return err
			}
			defer os.RemoveAll(work)

			var figs []*doc.Fig
			for _, a := range args {
				switch {
				case strings.HasSuffix(a, ".md"):
					d, err := doc.Read(a)
					if err != nil {
						return err
					}
					_, fs, err := d.ExtractFigs(work)
					if err != nil {
						return err
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
					figs = append(figs, &doc.Fig{Kind: kind, SrcPath: p, Attrs: map[string]string{}, Index: len(figs)})
				}
			}
			if len(figs) == 0 {
				return fmt.Errorf("no diagram sources found")
			}

			out := cmd.OutOrStdout()
			fmt.Fprintf(out, "brand %s · measure %.1f mm · text band %.1f–%.1f pt · max height %.0f mm\n\n",
				b.Name, textWidth, b.Diagrams.MinTextPt, b.Diagrams.MaxTextPt, b.Diagrams.MaxHeightMM)
			bad := 0
			for _, f := range figs {
				res, err := fig.Render(f, b, work, textWidth)
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
	return c
}
