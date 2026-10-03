package build

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/carlosprados/mdbrand/internal/brand"
	"github.com/carlosprados/mdbrand/internal/data"
	"github.com/carlosprados/mdbrand/internal/doc"
	"github.com/carlosprados/mdbrand/internal/fig"
	"github.com/carlosprados/mdbrand/internal/run"
	"github.com/carlosprados/mdbrand/internal/tex"
)

// prepared is a document made ready for any output format: its brand and style
// settled, its data printed, its words counted and its figures rendered and
// sized. What is left is what one format alone decides.
type prepared struct {
	o     Options
	d     *doc.File
	b     *brand.Brand
	style string
	work  string
	keep  bool
	store *data.Store
	rep   *Report
	// body still holds each figure's placeholder: the figure is sized here, but
	// how it is referenced belongs to the output format.
	body    string
	figs    []*doc.Fig
	results []*fig.Result
}

// close removes the work directory unless it was asked to be kept.
func (p *prepared) close() {
	if !p.keep && p.work != "" {
		os.RemoveAll(p.work)
	}
}

// refs is what the document's data files contributed to the build's inputs,
// charts reading by name included.
func (p *prepared) refs() []string {
	if p.store == nil {
		return nil
	}
	return p.store.Refs()
}

// prepare does every step no output format can skip. Like Run's Report, the
// result outlives an error: it is non-nil once it holds a work directory, so
// the caller can always close it and collect its refs.
func prepare(o Options, inputs *[]string) (*prepared, error) {
	d, err := doc.Read(o.Input)
	if err != nil {
		return nil, err
	}
	if keys := d.Meta.ReservedKeys(); len(keys) > 0 {
		return nil, fmt.Errorf(`the front matter sets %s, and the build would discard it in silence.
mdbrand injects its preamble, its cover and its closing matter through pandoc's
--include-in-header, --include-before-body and --include-after-body, and a
variable set on pandoc's command line replaces the metadata field of the same
name. Nothing written under those keys ever reaches the LaTeX; the document
would build, and simply not be what it asked to be.
Put the setting in the brand bundle, where every document of that identity gets
it, or open an issue for the knob you need:
  https://github.com/carlosprados/mdbrand/issues`, strings.Join(keys, ", "))
	}

	name := Pick(o.BrandName, d.Meta.Options.Brand, o.DefaultBrand)
	b, err := brand.Load(o.BrandsDir, name)
	if err != nil {
		return nil, err
	}
	if b.Dir != "" {
		*inputs = append(*inputs, filepath.Join(b.Dir, "brand.yaml"))
		for _, logo := range []string{b.LogoPath(), b.LogoSecondaryPath()} {
			if logo != "" {
				*inputs = append(*inputs, logo)
			}
		}
	}
	if probs, _ := b.Check(); len(probs) > 0 {
		return nil, fmt.Errorf("brand %q has problems that would break the build:\n  - %s\n  see: mdbrand brand validate %s",
			b.Name, strings.Join(probs, "\n  - "), b.Name)
	}

	style := Pick(o.Style, d.Meta.Options.Style, o.DefaultStyle, "report")
	if !tex.ValidStyle(style) {
		return nil, fmt.Errorf("unknown style %q: pick one of %s", style, strings.Join(tex.Styles, ", "))
	}

	work, keep, err := workDir(o.WorkDir)
	if err != nil {
		return nil, err
	}
	p := &prepared{
		o: o, d: d, b: b, style: style, work: work, keep: keep,
		rep: &Report{Brand: b.Name, Style: style, WorkDir: work, Kept: keep},
	}
	return p, p.fill(inputs)
}

// workDir is the directory every external tool runs in: a temporary one, or
// the one --work names, which is kept.
func workDir(asked string) (string, bool, error) {
	if asked == "" {
		work, err := os.MkdirTemp("", "mdbrand-")
		return work, false, err
	}
	if err := os.MkdirAll(asked, 0o755); err != nil {
		return "", true, err
	}
	// Absolute from here on. Every external tool is run with its cwd set
	// somewhere inside this directory, so a relative --work would be resolved
	// a second time against itself: `--work ./out` handed d2 out/out/fig00.d2
	// and the build died on the flag that exists to diagnose builds.
	return mustAbs(asked), true, nil
}

// fill takes the body from Markdown as written to Markdown as it will be
// typeset: figures extracted, data printed, words counted, figures rendered.
func (p *prepared) fill(inputs *[]string) error {
	o, d := p.o, p.d
	textWidth, err := tex.TextWidthMM(p.b)
	if err != nil {
		return err
	}

	body, figs, err := d.ExtractFigs(p.work)
	*inputs = append(*inputs, d.Refs...)
	if err != nil {
		return err
	}
	if p.store, err = data.Open(o.Input, d.Meta.Options.Data); err != nil {
		return err
	}
	if body, err = fillData(o, d, p.store, body, figs); err != nil {
		return err
	}
	if body, err = wordCount(o, d, body, figs, p.work, p.rep); err != nil {
		return err
	}
	if len(figs) > 0 {
		if missing := run.Missing(figTools(figs)...); len(missing) > 0 {
			return fmt.Errorf("this document has diagrams but these are missing: %s\n  run: mdbrand doctor",
				strings.Join(missing, ", "))
		}
	}
	for _, f := range figs {
		if f.Kind == "vega" {
			*inputs = append(*inputs, fig.DataRefs(f)...)
		}
		o.logf("  fig %s", filepath.Base(f.SrcPath))
		res, ferr := fig.Render(f, p.b, p.work, textWidth, Datasets(p.store, d.Meta.Lang))
		if ferr != nil {
			return ferr
		}
		p.rep.Figures = append(p.rep.Figures, res)
		if res.Note != "" {
			p.rep.Warnings = append(p.rep.Warnings, fmt.Sprintf("%s: %s", filepath.Base(f.SrcPath), res.Note))
		}
		p.results = append(p.results, res)
	}
	p.body, p.figs = body, figs
	return nil
}
