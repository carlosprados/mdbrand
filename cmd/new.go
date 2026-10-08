package cmd

import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/carlosprados/mdbrand/internal/tex"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

func newCmd() *cobra.Command {
	var style, brandName, title, subtitle, author string
	var toc bool

	c := &cobra.Command{
		Use:   "new <document.md>",
		Short: "Scaffold a Markdown document with the front matter already right",
		Long: `Write a new Markdown document carrying front matter mdbrand understands, so
"mdbrand build" on it needs no flags. Includes a commented example of a D2
diagram and a Vega-Lite chart.`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			path := args[0]
			if _, err := os.Stat(path); err == nil {
				return fmt.Errorf("%s already exists", path)
			}
			if style == "" {
				style = viper.GetString("style")
			}
			if !tex.ValidStyle(style) {
				return fmt.Errorf("unknown style %q: pick one of %s", style, strings.Join(tex.Styles, ", "))
			}
			if brandName == "" {
				brandName = viper.GetString("brand")
			}
			if title == "" {
				title = "Título del documento"
			}
			body := scaffold(style, brandName, title, subtitle, author, toc)
			if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "wrote %s\n  build it with: mdbrand build %s\n", path, path)
			return nil
		},
	}
	c.Flags().StringVar(&style, "style", "", "report | note | letter | slides")
	c.Flags().StringVar(&brandName, "brand", "", "brand bundle name")
	c.Flags().StringVar(&title, "title", "", "document title")
	c.Flags().StringVar(&subtitle, "subtitle", "", "document subtitle")
	c.Flags().StringVar(&author, "author", "", "author line for the cover")
	c.Flags().BoolVar(&toc, "toc", false, "include a table of contents")
	return c
}

// slidesBody shows the two levels a deck has: # opens a section, with a
// cover slide of its own, and ## is one slide. A diagram for a slide is wide
// and low, because a frame is 16:9.
var slidesBody = `# Primer bloque

## Una diapositiva

- Un nivel dos (` + "`##`" + `) es una diapositiva; un nivel uno (` + "`#`" + `) abre una
  sección con su propia portada
- Lo que no cabe en la diapositiva para el build: pártela en dos

## Un diagrama

` + "```d2 caption=\"Flujo de datos\"" + `
direction: right
origen: Origen
motor: Motor
salida: Informe
origen -> motor -> salida
` + "```" + `

## Dos columnas

:::: columns
::: column
- A la izquierda, el argumento
:::
::: column
- A la derecha, el ejemplo
:::
::::

::: notes
Notas del ponente: no salen en la diapositiva.
:::
`

var months = []string{"enero", "febrero", "marzo", "abril", "mayo", "junio",
	"julio", "agosto", "septiembre", "octubre", "noviembre", "diciembre"}

func scaffold(style, brandName, title, subtitle, author string, toc bool) string {
	now := time.Now()
	date := fmt.Sprintf("%d de %s de %d", now.Day(), months[int(now.Month())-1], now.Year())

	var fm strings.Builder
	fm.WriteString("---\n")
	fmt.Fprintf(&fm, "title: %q\n", title)
	if subtitle != "" {
		fmt.Fprintf(&fm, "subtitle: %q\n", subtitle)
	}
	if author != "" {
		fmt.Fprintf(&fm, "author: %q\n", author)
	}
	fmt.Fprintf(&fm, "date: %q\nlang: es-ES\n", date)
	if toc {
		fm.WriteString("toc: true\ntoc-depth: 2\n")
	}
	fm.WriteString("mdbrand:\n")
	if brandName != "" {
		fmt.Fprintf(&fm, "  brand: %s\n", brandName)
	} else {
		fm.WriteString("  brand: none      # name of a bundle in your brands dir\n")
	}
	fmt.Fprintf(&fm, "  style: %s\n", style)
	switch style {
	case "report":
		fm.WriteString("  # reference: \"Oferta AS-0000-26\"\n  # confidential: \"Confidencial\"\n")
	case "letter":
		fm.WriteString(`  to:
    - "Nombre del destinatario"
    - "Empresa"
  place: "Madrid"
  greeting: "Estimado equipo:"
  signature: |
    Carlos Javier Prados Hijón
    Amplía Soluciones S.L.
`)
	}
	fm.WriteString("---\n\n")

	body := `# Primer apartado

Texto. Las tablas, listas y énfasis de Markdown funcionan como esperas.

Un diagrama. Sin tamaños de fuente: mdbrand los pone él para que el texto se
lea en papel, y uno declarado en el bloque le quita esa decisión:

` + "```d2 caption=\"Flujo de datos\"" + `
origen: Origen de datos
motor: Motor
salida: Informe
origen -> motor -> salida
` + "```" + `

Una gráfica de datos:

` + "```vegalite caption=\"Muestras por trimestre\"" + `
{
  "$schema": "https://vega.github.io/schema/vega-lite/v5.json",
  "width": 340, "height": 150,
  "data": {"values": [{"t": "Q1", "n": 28}, {"t": "Q2", "n": 55}, {"t": "Q3", "n": 43}]},
  "mark": "bar",
  "encoding": {"x": {"field": "t", "type": "nominal", "title": null},
               "y": {"field": "n", "type": "quantitative", "title": "muestras"}},
  "config": {"axis": {"labelFontSize": 13, "titleFontSize": 13, "labelAngle": 0}}
}
` + "```" + `
`
	switch style {
	case "letter":
		body = "Cuerpo de la carta. El membrete, el destinatario y la firma salen del front\nmatter; aquí va solo el texto.\n"
	case "slides":
		body = slidesBody
	}
	return fm.String() + body
}
