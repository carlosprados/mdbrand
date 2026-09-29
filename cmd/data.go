package cmd

import (
	"fmt"

	"github.com/carlosprados/mdbrand/internal/data"
	"github.com/carlosprados/mdbrand/internal/doc"
	"github.com/spf13/cobra"
)

func dataCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "data <document.md> [data.path]",
		Short: "List the data a document can print, or show one path",
		Long: `List every data file the document can reach, with its namespace and shape,
or show what one path names. Every file is read, so a CSV that will not parse
is reported here before any document uses it.

Data lives in data/ beside the document, or wherever mdbrand.data in the front
matter points (directories or files, relative to the document). The file name
is the namespace: data/maquinas.yaml is data.maquinas, data/aws/ec2.csv is
data.aws.ec2. YAML, JSON, CSV (header row; , ; or tab, detected) and TSV.

In the document, {{data.path}} prints one value, as written in the file:

  {{data.cliente.nombre}}
  {{data.maquinas[m5.large].ram}}     brackets for keys with dots or spaces
  {{data.equipo.0.nombre}}            a list, by index from 0
  {{data.aws.ec2[t3.micro].vcpu}}     a list of records, by its id field

The value prints as its characters: * $ @ < and the rest do not become
Markdown. Tag a YAML value !md when it is Markdown. Placeholders work in the
prose, figure captions and front matter (quote the scalar), and are left as
written in code. A path that leads nowhere stops the build listing the keys
that exist. Keys starting with _ (an anchor to merge, _base: &base {…}) are
left out of every listing but still answer when named.

A table comes from the same data, declared in a fenced block:

  ` + "```table" + `
  source: maquinas            the key of a mapping of records is its id column
  where: {familia: m5}        equality only; or rows: [m5.large, m5.xlarge]
  columns:                    omitted: every field
    id: Tipo
    ram: {label: RAM, unit: GiB, align: right}
    precio: {label: "€/hora", decimals: 3}
  sort: -ram                  one field; - descends; numeric or collated
  transpose: true             records become columns
  total: [ram, precio]        a Total row, summed exactly in each column's format
  caption: Instancias m5
  ` + "```" + `

or ![Caption](data/sedes.csv) alone on its line, for a whole file. decimals
rounds exactly and writes the document's lang (0,192 in es). A decimal comma
in the data is a number only where lang writes one. Any id, field or key that
does not exist stops the build naming the ones that do; a record missing a
column's field too (field: ~ is a deliberately empty cell), and a blank or
text cell in a totalled column.

A Vega-Lite chart reads the same records with "data": {"name": "maquinas"},
numbers parsed as the tables parse them. A field it plots that mixes numbers
and text stops the build, since Vega-Lite would drop the text without a word.

  mdbrand data propuesta.md
  mdbrand data propuesta.md 'data.maquinas[m5.large]'`,
		Args: cobra.RangeArgs(1, 2),
		RunE: func(cmd *cobra.Command, args []string) error {
			d, err := doc.Read(args[0])
			if err != nil {
				return err
			}
			s, err := data.Open(args[0], d.Meta.Options.Data)
			if err != nil {
				return err
			}
			out := cmd.OutOrStdout()
			if len(args) == 2 {
				v, err := s.Dump(args[1])
				if err != nil {
					return err
				}
				fmt.Fprintln(out, v)
				return nil
			}
			entries := s.Namespaces()
			if len(entries) == 0 {
				fmt.Fprintf(out, "no data files in %v\n", s.Roots())
				return nil
			}
			width := 0
			for _, e := range entries {
				width = max(width, len(e.Name))
			}
			bad := 0
			for _, e := range entries {
				mark := " "
				if e.Err {
					mark, bad = "!", bad+1
				}
				fmt.Fprintf(out, "%s %-*s  %s\n", mark, width, e.Name, e.Shape)
			}
			if bad > 0 {
				return fmt.Errorf("%d data file(s) cannot be read", bad)
			}
			return nil
		},
	}
}
