// Package cmd is mdbrand's command line. The help text is the documentation:
// anything a person or an agent needs in order to drive this tool should be
// reachable with --help, without opening a file.
package cmd

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime/debug"
	"strings"

	"github.com/carlosprados/mdbrand/internal/brand"
	"github.com/carlosprados/mdbrand/internal/exit"
	"github.com/carlosprados/mdbrand/internal/paths"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

// Version is set at build time: -ldflags "-X github.com/carlosprados/mdbrand/cmd.Version=v0.2.0"
var Version = "dev"

// version is the running mdbrand: the stamped Version, or for a binary from
// `go install …@v0.18.0`, which stamps nothing, the module version Go
// recorded. A bundle's requires: is checked against it.
func version() string {
	if Version != "dev" {
		return Version
	}
	if bi, ok := debug.ReadBuildInfo(); ok && bi.Main.Version != "" && bi.Main.Version != "(devel)" {
		return bi.Main.Version
	}
	return Version
}

var cfgFile string

// Root is the mdbrand command tree.
func Root() *cobra.Command {
	root := &cobra.Command{
		Use:   "mdbrand",
		Short: "Markdown to a branded A4 PDF, in one command",
		Long: `mdbrand turns a Markdown document into a branded A4 PDF with pandoc and
XeLaTeX: cover with your logo, running header with your logo, D2 diagrams and
Vega-Lite charts rendered and sized so their text is legible on paper. With
style slides, the same document is a 16:9 deck for a talk.

The brand lives in a bundle outside this tool — a directory with a brand.yaml,
a logo and colours — so the same document can be published under a different
identity by changing one word.

  mdbrand example                      worked examples to start from: report, note, letter,
                                       data, essay, talk with speaker notes
  mdbrand example carta                write one out, then build it
  mdbrand build report.md              build using the document's own front matter
  mdbrand new report.md                a bare scaffold, when no example is near
  mdbrand doctor                       check the toolchain and say how to fix it
  mdbrand brand list                   what bundles are installed
  mdbrand brand validate amplia        diagnose a bundle before it bites
  mdbrand skill install                install the agent skill, for AI assistants

Exit status says who has to act: 0 built; 1 the document or its bundle would
make a defective PDF, and the message names the fix; 2 the command line is
wrong; 3 the machine lacks a tool, font or bundle — mdbrand doctor.

Front matter drives everything, so a build needs no flags:

  ---
  title: "Iniciativas de Inteligencia Artificial"
  subtitle: "Qué tendremos y en qué se basa"
  author: "Departamento de Tecnología — Amplía Soluciones S.L."
  date: "7 de septiembre de 2026"
  lang: es-ES
  toc: true
  mdbrand:
    brand: amplia
    style: report      # report | note | letter | slides
  ---`,
		SilenceUsage:               true,
		SilenceErrors:              true,
		Args:                       cobra.ArbitraryArgs,
		SuggestionsMinimumDistance: 2, // what cobra sets for its own message
		RunE:                       groupRun,
	}

	root.PersistentFlags().StringVar(&cfgFile, "config", "", "config file (default $XDG_CONFIG_HOME/mdbrand/config.yaml)")
	root.PersistentFlags().String("brands-dir", "", "directory holding brand bundles (env MDBRAND_BRANDS_DIR)")
	_ = viper.BindPFlag("brands_dir", root.PersistentFlags().Lookup("brands-dir"))

	// A wrong flag is the typist's to fix, not the document's.
	root.SetFlagErrorFunc(func(_ *cobra.Command, err error) error { return exit.AsUsage(err) })
	cobra.OnInitialize(initConfig)

	root.AddCommand(buildCmd(), newCmd(), exampleCmd(), doctorCmd(), brandCmd(), diagramsCmd(), dataCmd(), skillCmd(), configCmd(), versionCmd())
	return root
}

func initConfig() {
	brand.ToolVersion = version()
	viper.SetEnvPrefix("MDBRAND")
	viper.AutomaticEnv()
	viper.SetDefault("brands_dir", defaultBrandsDir())
	viper.SetDefault("brand", "")
	viper.SetDefault("style", "report")
	viper.SetDefault("wordcount", "ib")

	if cfgFile != "" {
		viper.SetConfigFile(cfgFile)
	} else {
		viper.SetConfigName("config")
		viper.SetConfigType("yaml")
		// Without a home there is no user configuration to read, and looking
		// in a relative .config/ would read whatever the current directory has.
		if dir, err := configDir(); err == nil {
			viper.AddConfigPath(dir)
		}
	}
	_ = viper.ReadInConfig() // absent config is the normal case, not an error
}

// configDir is where mdbrand keeps its settings. With neither XDG_CONFIG_HOME
// nor a home directory there is no such place, and it says so: the home it
// ignored the error for was "", so the path came out relative, config init
// wrote into the current directory and a stray .config/ there was read as ours.
func configDir() (string, error) {
	if x := os.Getenv("XDG_CONFIG_HOME"); x != "" {
		return filepath.Join(x, "mdbrand"), nil
	}
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return "", fmt.Errorf("no home directory and XDG_CONFIG_HOME is unset, so mdbrand has nowhere to keep its configuration: set HOME or XDG_CONFIG_HOME")
	}
	return filepath.Join(home, ".config", "mdbrand"), nil
}

// defaultBrandsDir is empty when there is no configuration directory, which
// brand.Load refuses by name rather than resolving against the working
// directory.
func defaultBrandsDir() string {
	dir, err := configDir()
	if err != nil {
		return ""
	}
	return filepath.Join(dir, "brands")
}

func brandsDir() string { return paths.Expand(viper.GetString("brands_dir")) }

// needBrandsDir is brandsDir for the commands that write or list bundles.
func needBrandsDir() (string, error) {
	if dir := brandsDir(); dir != "" {
		return dir, nil
	}
	return "", brand.ErrNoBrandsDir
}

func versionCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print the version",
		Run: func(*cobra.Command, []string) {
			fmt.Println("mdbrand " + version())
		},
	}
}

func configCmd() *cobra.Command {
	c := &cobra.Command{
		Use:   "config",
		Short: "Show where mdbrand reads its settings from",
		Args:  usageArgs(cobra.NoArgs),
		Long: `Settings resolve in this order, later wins:
  built-in default  ->  config file  ->  MDBRAND_* environment  ->  flag`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			out := cmd.OutOrStdout()
			fmt.Fprintf(out, "config file : %s\n", orNone(viper.ConfigFileUsed()))
			if dir := brandsDir(); dir != "" {
				fmt.Fprintf(out, "brands_dir  : %s\n", dir)
			} else {
				fmt.Fprintf(out, "brands_dir  : (not set) %v\n", brand.ErrNoBrandsDir)
			}
			fmt.Fprintf(out, "brand       : %s\n", orNone(viper.GetString("brand")))
			fmt.Fprintf(out, "style       : %s\n", viper.GetString("style"))
			fmt.Fprintf(out, "wordcount   : %s\n", viper.GetString("wordcount"))
			formats := strings.Join(viper.GetStringSlice("formats"), ",")
			if formats == "" {
				formats = "pdf (default)"
			}
			fmt.Fprintf(out, "formats     : %s\n", formats)
			return nil
		},
	}
	c.AddCommand(&cobra.Command{
		Use:   "init",
		Short: "Write a starter config file",
		RunE: func(cmd *cobra.Command, _ []string) error {
			dir, err := configDir()
			if err != nil {
				return err
			}
			if err := os.MkdirAll(dir, 0o755); err != nil {
				return err
			}
			p := filepath.Join(dir, "config.yaml")
			if _, err := os.Stat(p); err == nil {
				return fmt.Errorf("%s already exists", p)
			}
			body := fmt.Sprintf(`# mdbrand settings.
# brands_dir is where brand bundles live. Keep it outside any code repository:
# logos and licensed fonts should not be committed.
brands_dir: %s

# Used when a document's front matter does not name one.
brand: ""
style: report

# The criterion {{words}} counts by when a document does not set one: ib or all.
wordcount: ib
`, defaultBrandsDir())
			if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "wrote %s\n", p)
			return nil
		},
	})
	return c
}

func orNone(s string) string {
	if s == "" {
		return "(none)"
	}
	return s
}

// groupRun is the Run of a command that only groups others. cobra reports an
// unknown command from inside Find as an untyped error, and below the root
// not at all: `mdbrand brand frob` printed the help and exited 0. A group
// that runs takes the stray argument itself and calls it a usage error, in
// cobra's own words.
func groupRun(cmd *cobra.Command, args []string) error {
	if len(args) == 0 {
		return cmd.Help()
	}
	msg := fmt.Sprintf("unknown command %q for %q", args[0], cmd.CommandPath())
	if s := cmd.SuggestionsFor(args[0]); len(s) > 0 {
		msg += "\n\nDid you mean this?\n\t" + strings.Join(s, "\n\t") + "\n"
	}
	return exit.AsUsage(errors.New(msg))
}

// usageArgs marks what an argument validator refuses as a usage error.
func usageArgs(v cobra.PositionalArgs) cobra.PositionalArgs {
	return func(cmd *cobra.Command, args []string) error { return exit.AsUsage(v(cmd, args)) }
}
