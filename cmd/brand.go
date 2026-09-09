package cmd

import (
	"fmt"
	"math"
	"os"
	"path/filepath"

	"github.com/carlosprados/mdbrand/internal/brand"
	"github.com/carlosprados/mdbrand/internal/imgsize"
	"github.com/carlosprados/mdbrand/internal/tex"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
)

func brandCmd() *cobra.Command {
	c := &cobra.Command{
		Use:   "brand",
		Short: "Create, inspect and validate brand bundles",
		Long: `A brand bundle is a directory with a brand.yaml, a logo and colours. It lives
outside this tool, so the same document can be published under another identity
by changing one word.

  <brands dir>/amplia/
    brand.yaml
    logo.svg        vector; an SVG that merely wraps a PNG will look soft
    fonts/otf/      optional: the display font, if you may redistribute it

A relative fonts.display.path is resolved inside the bundle, so a bundle that
carries its font works on a fresh clone with nothing installed. Whether you may
put a font there is a licensing question, not a technical one: a commercial face
like Gotham must never go into a repository anyone can read anonymously. Keep
such a bundle private or internal, or leave the font out and let mdbrand find an
installed copy through fontconfig.`,
	}
	c.AddCommand(brandListCmd(), brandShowCmd(), brandValidateCmd(), brandNewCmd(), brandPathCmd())
	return c
}

func brandListCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List installed bundles",
		RunE: func(cmd *cobra.Command, _ []string) error {
			dir := brandsDir()
			names := brand.List(dir)
			if len(names) == 0 {
				fmt.Fprintf(cmd.OutOrStdout(), "no bundles in %s\n  create one: mdbrand brand new <name>\n", dir)
				return nil
			}
			for _, n := range names {
				b, err := brand.Load(dir, n)
				if err != nil {
					fmt.Fprintf(cmd.OutOrStdout(), "%-14s ERROR %v\n", n, err)
					continue
				}
				logo := "no logo"
				if p := b.LogoPath(); p != "" {
					logo = filepath.Base(p)
				}
				fmt.Fprintf(cmd.OutOrStdout(), "%-14s %-34s %s\n", n, b.DisplayName, logo)
			}
			return nil
		},
	}
}

func brandShowCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "show <name>",
		Short: "Print a bundle's resolved settings, defaults included",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			b, err := brand.Load(brandsDir(), args[0])
			if err != nil {
				return err
			}
			out, err := yaml.Marshal(b)
			if err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "# %s\n%s", filepath.Join(b.Dir, "brand.yaml"), out)
			return nil
		},
	}
}

func brandValidateCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "validate [name...]",
		Short: "Diagnose bundles before they bite",
		Long: `Check a bundle for the failures that are otherwise invisible until a PDF looks
wrong: a logo that is a bitmap wearing an SVG coat, artwork sitting outside the
viewBox, the invalid data:img/ MIME type that makes rsvg-convert render nothing,
text kept as <foreignObject> (which rsvg-convert drops), a display font whose
path no longer exists, colours that are not plain 6-digit hex, and a header
whose declared height cannot hold the logo that goes in it.

With no arguments, validates every installed bundle.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			dir := brandsDir()
			names := args
			if len(names) == 0 {
				names = brand.List(dir)
			}
			if len(names) == 0 {
				return fmt.Errorf("no bundles in %s", dir)
			}
			out := cmd.OutOrStdout()
			bad := 0
			for _, n := range names {
				b, err := brand.Load(dir, n)
				if err != nil {
					bad++
					fmt.Fprintf(out, "%s: %v\n", n, err)
					continue
				}
				probs, warns := b.Check()
				// The header's geometry depends on the logo's proportions, and
				// getting it wrong prints the mark across the first line of every
				// page. Checked here rather than in Check() because the arithmetic
				// lives in tex, which imports brand: this is the one place that can
				// see both without inverting the dependency — and validate saying
				// "ok" before a build dies is the failure mode this whole command
				// exists to prevent.
				if h, err := headerGeometry(b); err != nil {
					probs = append(probs, err.Error())
				} else if h != "" {
					warns = append(warns, h)
				}
				fmt.Fprintf(out, "%s (%s)\n", n, b.Dir)
				for _, p := range probs {
					fmt.Fprintf(out, "  PROBLEM  %s\n", p)
				}
				for _, w := range warns {
					fmt.Fprintf(out, "  warning  %s\n", w)
				}
				if dir, found := b.DisplayFontDir(); found {
					fmt.Fprintf(out, "  display font %s found in %s\n", b.Fonts.Display.Regular, dir)
				}
				if len(probs) == 0 && len(warns) == 0 {
					fmt.Fprintln(out, "  ok")
				}
				bad += len(probs)
			}
			if bad > 0 {
				return fmt.Errorf("%d problem(s) found", bad)
			}
			return nil
		},
	}
}

func brandNewCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "new <name>",
		Short: "Scaffold a bundle, then tell you what to drop into it",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			dir, err := brand.Scaffold(brandsDir(), args[0])
			if err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), `created %s

Next:
  1. put the logo in %s as logo.svg — vector, not an SVG wrapping a PNG
  2. set colors.primary / text / rule to the brand's hex values
  3. optional: point fonts.display at the licensed brand font (by path, never copied)
  4. mdbrand brand validate %s
`, filepath.Join(dir, "brand.yaml"), dir, args[0])
			return nil
		},
	}
}

func brandPathCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "path [name]",
		Short: "Print the brands directory, or one bundle's directory",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 {
				fmt.Fprintln(cmd.OutOrStdout(), brandsDir())
				return nil
			}
			p := filepath.Join(brandsDir(), args[0])
			if _, err := os.Stat(p); err != nil {
				return err
			}
			fmt.Fprintln(cmd.OutOrStdout(), p)
			return nil
		},
	}
}

// headerGeometry resolves the running header's box for a bundle. It returns a
// note when mdbrand has to derive headheight itself (the bundle is fine, but
// the number in brand.yaml is not the one used), and an error when the declared
// value cannot hold the logo.
func headerGeometry(b *brand.Brand) (string, error) {
	logo := b.LogoPath()
	if logo == "" {
		return "", nil
	}
	aspect, err := imgsize.Aspect(logo)
	if err != nil {
		return fmt.Sprintf("page.headheight: cannot measure %s (%v), so the header height stays at %s — wrong unless the mark is wide",
			filepath.Base(logo), err, b.Page.HeadHeight), nil
	}
	h, err := tex.HeaderHeightMM(b, aspect)
	if err != nil {
		return "", err
	}
	declared, derr := tex.ParseLenMM(b.Page.HeadHeight)
	if derr != nil || math.Abs(h-declared) < 0.05 {
		return "", nil
	}
	return fmt.Sprintf("page.headheight: using %.0fpt, derived from a logo %.2f as tall as it is wide; brand.yaml says %s",
		h/(25.4/72.272), aspect, b.Page.HeadHeight), nil
}
