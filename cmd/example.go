package cmd

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"strings"
	"text/tabwriter"

	"github.com/carlosprados/mdbrand/internal/exit"
	"github.com/spf13/cobra"
)

// Examples is examples/ from the repository, embedded by main: one directory
// per example, holding <name>.md and whatever it reads beside it.
var Examples fs.FS

// example is one entry of the catalogue: what someone asks for that it
// answers, and what it shows. The order is the order they are listed in.
type example struct {
	name, ask, shows string
}

var catalogue = []example{
	{"informe", "a report or proposal for a client",
		"style report: cover, reference, confidential label, table of contents, d2 diagram, chart, table, code"},
	{"nota", "an internal note, also as Word or Google Docs",
		"style note, formats [pdf, docx]: title block, header from page 1, a diagram and a table in both"},
	{"carta", "a formal letter",
		"style letter: letterhead, recipient lines, place and date, subject, greeting, signature"},
	{"datos", "a document quoting numbers kept in YAML, JSON or CSV",
		"{{data…}} values, ```table blocks filtered, sorted, totalled and transposed, a CSV linked whole, a chart by name"},
	{"ensayo", "an essay with a word limit, with citations",
		"{{words}} on the cover, {.nocount} sections, bibliography: refs.bib and [@key, p. 12] citations"},
	{"charla", "a deck for a talk, with speaker notes",
		"style slides, formats [pdf, notes]: sections, a diagram and a chart for the frame, columns, pauses, incremental lists, notes"},
}

func exampleCmd() *cobra.Command {
	var show bool
	c := &cobra.Command{
		Use:   "example [name] [dir]",
		Short: "List the worked examples, or write one out to start from",
		Long: `Worked examples that build as they are, one for each kind of document, carried
inside the binary. Start a document from the nearest one rather than from an
empty file: its front matter is right and commented, and it shows the syntax
of everything it uses.

  mdbrand example                  list them, with what each is for
  mdbrand example carta            write carta.md here, then: mdbrand build carta.md
  mdbrand example datos ./oferta   write datos.md and its data/ into ./oferta
  mdbrand example charla --show    print charla.md without writing anything

Every example says brand: none, so it builds on a machine with no bundle; set
the bundle's name there to brand it. A file that already exists is never
overwritten.`,
		Args: usageArgs(cobra.MaximumNArgs(2)),
		RunE: func(cmd *cobra.Command, args []string) error {
			out := cmd.OutOrStdout()
			if len(args) == 0 {
				w := tabwriter.NewWriter(out, 0, 0, 2, ' ', 0)
				for _, e := range catalogue {
					fmt.Fprintf(w, "%s\t%s\n\t  %s\n", e.name, e.ask, e.shows)
				}
				if err := w.Flush(); err != nil {
					return err
				}
				fmt.Fprintln(out, "\nmdbrand example <name> [dir] writes one out; --show prints it.")
				return nil
			}
			name := args[0]
			if !hasExample(name) {
				return exit.AsUsage(fmt.Errorf("no example %q: the examples are %s", name, strings.Join(exampleNames(), ", ")))
			}
			if show {
				raw, err := fs.ReadFile(Examples, path.Join(name, name+".md"))
				if err != nil {
					return err
				}
				_, err = out.Write(raw)
				return err
			}
			dir := "."
			if len(args) == 2 {
				dir = args[1]
			}
			written, err := writeExample(name, dir)
			if err != nil {
				return err
			}
			for _, f := range written {
				fmt.Fprintf(out, "wrote %s\n", f)
			}
			fmt.Fprintf(out, "  build it with: mdbrand build %s\n", filepath.Join(dir, name+".md"))
			return nil
		},
	}
	c.Flags().BoolVar(&show, "show", false, "print the example's Markdown instead of writing it")
	return c
}

func hasExample(name string) bool {
	for _, e := range catalogue {
		if e.name == name {
			return true
		}
	}
	return false
}

func exampleNames() []string {
	names := make([]string, len(catalogue))
	for i, e := range catalogue {
		names[i] = e.name
	}
	return names
}

// writeExample copies the example's directory into dir. Every target is
// checked before anything is written, so a clash leaves dir as it was.
func writeExample(name, dir string) ([]string, error) {
	var files []string
	err := fs.WalkDir(Examples, name, func(p string, d fs.DirEntry, err error) error {
		if err == nil && !d.IsDir() {
			files = append(files, p)
		}
		return err
	})
	if err != nil {
		return nil, err
	}
	target := func(p string) string { return filepath.Join(dir, filepath.FromSlash(strings.TrimPrefix(p, name+"/"))) }
	for _, p := range files {
		if _, err := os.Stat(target(p)); err == nil {
			return nil, exit.AsUsage(fmt.Errorf("%s already exists: write the example somewhere else (mdbrand example %s <dir>)", target(p), name))
		} else if !errors.Is(err, fs.ErrNotExist) {
			return nil, err
		}
	}
	var written []string
	for _, p := range files {
		raw, err := fs.ReadFile(Examples, p)
		if err != nil {
			return written, err
		}
		t := target(p)
		if err := os.MkdirAll(filepath.Dir(t), 0o755); err != nil {
			return written, err
		}
		if err := os.WriteFile(t, raw, 0o644); err != nil {
			return written, err
		}
		written = append(written, t)
	}
	return written, nil
}
