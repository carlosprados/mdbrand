package cmd

import (
	"fmt"
	"testing"

	"github.com/carlosprados/mdbrand/internal/build"
	"github.com/carlosprados/mdbrand/internal/contract"
	"github.com/carlosprados/mdbrand/internal/exit"
	"github.com/carlosprados/mdbrand/internal/tex"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

// What scripts and agents type and test for: every command and flag with its
// type, the exit statuses, and the values style and formats take. Help text
// is not here — it is meant to get better.
func TestContractCommandLine(t *testing.T) {
	var lines []string
	var walk func(c *cobra.Command)
	walk = func(c *cobra.Command) {
		if c.Hidden || c.Name() == "help" || c.Name() == "completion" {
			return
		}
		lines = append(lines, "command "+c.CommandPath())
		c.LocalFlags().VisitAll(func(f *pflag.Flag) {
			if f.Name == "help" {
				return
			}
			short := ""
			if f.Shorthand != "" {
				short = " -" + f.Shorthand
			}
			lines = append(lines, fmt.Sprintf("flag %s --%s%s (%s)", c.CommandPath(), f.Name, short, f.Value.Type()))
		})
		for _, sub := range c.Commands() {
			walk(sub)
		}
	}
	walk(Root())
	for code, meaning := range map[int]string{exit.OK: "ok", exit.Defect: "defect", exit.Usage: "usage", exit.Environment: "environment"} {
		lines = append(lines, fmt.Sprintf("exit %d %s", code, meaning))
	}
	for _, s := range tex.Styles {
		lines = append(lines, "style "+s)
	}
	for _, f := range build.Formats {
		lines = append(lines, "format "+f)
	}
	contract.Check(t, "command-line", lines)
}
