// mdbrand turns Markdown into a branded A4 PDF: pandoc and XeLaTeX underneath,
// a brand bundle for the identity, D2 and Vega-Lite figures sized so their text
// is legible on paper.
package main

import (
	"fmt"
	"os"

	"github.com/carlosprados/mdbrand/cmd"
)

func main() {
	if err := cmd.Root().Execute(); err != nil {
		fmt.Fprintln(os.Stderr, "mdbrand: "+err.Error())
		os.Exit(1)
	}
}
