// Package cmd is mdbrand's command line. The help text is the documentation:
// anything a person or an agent needs in order to drive this tool should be
// reachable with --help, without opening a file.
package cmd

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

// Version is set at build time: -ldflags "-X github.com/carlosprados/mdbrand/cmd.Version=v0.2.0"
var Version = "dev"

var cfgFile string

// Root is the mdbrand command tree.
func Root() *cobra.Command {
	root := &cobra.Command{
		Use:   "mdbrand",
		Short: "Markdown to a branded A4 PDF, in one command",
		Long: `mdbrand turns a Markdown document into a branded A4 PDF with pandoc and
XeLaTeX: cover with your logo, running header with your logo, D2 diagrams and
Vega-Lite charts rendered and sized so their text is legible on paper.

The brand lives in a bundle outside this tool — a directory with a brand.yaml,
a logo and colours — so the same document can be published under a different
identity by changing one word.

  mdbrand build report.md              build using the document's own front matter
  mdbrand new report.md                scaffold a document with the front matter filled in
  mdbrand doctor                       check the toolchain and say how to fix it
  mdbrand brand list                   what bundles are installed
  mdbrand brand validate amplia        diagnose a bundle before it bites

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
    style: report      # report | note | letter
  ---`,
		SilenceUsage:  true,
		SilenceErrors: true,
	}

	root.PersistentFlags().StringVar(&cfgFile, "config", "", "config file (default $XDG_CONFIG_HOME/mdbrand/config.yaml)")
	root.PersistentFlags().String("brands-dir", "", "directory holding brand bundles (env MDBRAND_BRANDS_DIR)")
	_ = viper.BindPFlag("brands_dir", root.PersistentFlags().Lookup("brands-dir"))

	cobra.OnInitialize(initConfig)

	root.AddCommand(buildCmd(), newCmd(), doctorCmd(), brandCmd(), diagramsCmd(), configCmd(), versionCmd())
	return root
}

func initConfig() {
	viper.SetEnvPrefix("MDBRAND")
	viper.AutomaticEnv()
	viper.SetDefault("brands_dir", defaultBrandsDir())
	viper.SetDefault("brand", "")
	viper.SetDefault("style", "report")

	if cfgFile != "" {
		viper.SetConfigFile(cfgFile)
	} else {
		viper.SetConfigName("config")
		viper.SetConfigType("yaml")
		viper.AddConfigPath(configDir())
	}
	_ = viper.ReadInConfig() // absent config is the normal case, not an error
}

func configDir() string {
	if x := os.Getenv("XDG_CONFIG_HOME"); x != "" {
		return filepath.Join(x, "mdbrand")
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".config", "mdbrand")
}

func defaultBrandsDir() string {
	return filepath.Join(configDir(), "brands")
}

func brandsDir() string { return expand(viper.GetString("brands_dir")) }

func expand(p string) string {
	if p == "~" || len(p) > 1 && p[:2] == "~/" {
		home, _ := os.UserHomeDir()
		return filepath.Join(home, p[1:])
	}
	return os.ExpandEnv(p)
}

func versionCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print the version",
		Run: func(*cobra.Command, []string) {
			fmt.Println("mdbrand " + Version)
		},
	}
}

func configCmd() *cobra.Command {
	c := &cobra.Command{
		Use:   "config",
		Short: "Show where mdbrand reads its settings from",
		Long: `Settings resolve in this order, later wins:
  built-in default  ->  config file  ->  MDBRAND_* environment  ->  flag`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			out := cmd.OutOrStdout()
			fmt.Fprintf(out, "config file : %s\n", orNone(viper.ConfigFileUsed()))
			fmt.Fprintf(out, "brands_dir  : %s\n", brandsDir())
			fmt.Fprintf(out, "brand       : %s\n", orNone(viper.GetString("brand")))
			fmt.Fprintf(out, "style       : %s\n", viper.GetString("style"))
			return nil
		},
	}
	c.AddCommand(&cobra.Command{
		Use:   "init",
		Short: "Write a starter config file",
		RunE: func(cmd *cobra.Command, _ []string) error {
			dir := configDir()
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
