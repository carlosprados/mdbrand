package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/spf13/cobra"
)

// SkillDoc is SKILL.md, embedded into the binary by main. Carrying it inside
// means a person who installed a release binary can install the skill too: it
// used to require a checkout of this repository, which is exactly the wrong
// dependency for the audience that most needs the skill.
var SkillDoc string

// stampRe matches the marker appended on install, so an existing file can be
// compared against the embedded one without the version line getting in the way.
var stampRe = regexp.MustCompile(`(?m)\n<!-- installed by mdbrand [^>]* -->\n?$`)

func skillDir() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".claude", "skills", "mdbrand")
}

func skillCmd() *cobra.Command {
	c := &cobra.Command{
		Use:   "skill",
		Short: "Install the agent skill that teaches an AI to drive mdbrand",
		Long: `mdbrand ships with a skill document for coding agents (Claude Code and
anything else that reads a skills directory). It states the front matter, the
diagram rules and the traps, so an agent uses the CLI correctly instead of
improvising a pandoc command line.

The document is embedded in this binary, so installing it needs nothing else:

  mdbrand skill install          into ~/.claude/skills/mdbrand/SKILL.md
  mdbrand skill install --project   into ./.claude/skills/mdbrand/SKILL.md
  mdbrand skill show             print it, to pipe wherever you keep such things
  mdbrand skill path             where install would write`,
	}
	c.AddCommand(skillInstallCmd(), skillShowCmd(), skillPathCmd())
	return c
}

func skillShowCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "show",
		Short: "Print the embedded skill document",
		RunE: func(cmd *cobra.Command, _ []string) error {
			fmt.Fprint(cmd.OutOrStdout(), SkillDoc)
			return nil
		},
	}
}

func skillPathCmd() *cobra.Command {
	var project bool
	c := &cobra.Command{
		Use:   "path",
		Short: "Print where the skill would be installed",
		RunE: func(cmd *cobra.Command, _ []string) error {
			dir := skillDir()
			if project {
				dir = filepath.Join(".claude", "skills", "mdbrand")
			}
			fmt.Fprintln(cmd.OutOrStdout(), filepath.Join(dir, "SKILL.md"))
			return nil
		},
	}
	c.Flags().BoolVar(&project, "project", false, "the project-local directory instead of your home")
	return c
}

func skillInstallCmd() *cobra.Command {
	var dir string
	var project, force bool

	c := &cobra.Command{
		Use:   "install",
		Short: "Write the embedded skill where an agent will find it",
		Long: `Write the embedded skill document into a skills directory.

Refuses to overwrite a file that differs from the embedded one unless --force,
because that file may be a symlink into a checkout — the arrangement used when
developing mdbrand itself, where overwriting would edit the repository through
the link.`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if strings.TrimSpace(SkillDoc) == "" {
				return fmt.Errorf("this build carries no embedded skill document")
			}
			target := dir
			switch {
			case target != "":
				// as given
			case project:
				target = filepath.Join(".claude", "skills", "mdbrand")
			default:
				target = skillDir()
			}
			file := filepath.Join(target, "SKILL.md")
			body := stampRe.ReplaceAllString(strings.TrimRight(SkillDoc, "\n"), "") +
				fmt.Sprintf("\n\n<!-- installed by mdbrand %s -->\n", Version)

			out := cmd.OutOrStdout()
			// Lstat, not Stat: a symlink must be seen as a symlink, or writing
			// follows it into whatever it points at.
			if st, err := os.Lstat(file); err == nil {
				link := st.Mode()&os.ModeSymlink != 0
				current, _ := os.ReadFile(file)
				same := stampRe.ReplaceAllString(string(current), "") ==
					stampRe.ReplaceAllString(body, "")
				switch {
				case same && !link:
					fmt.Fprintf(out, "%s is already up to date\n", file)
					return nil
				case !force && link:
					dest, _ := os.Readlink(file)
					return fmt.Errorf(`%s is a symlink to %s
  Overwriting would write through it and edit that file. If this is a checkout
  of mdbrand, the symlink is what you want and there is nothing to do; pass
  --force to replace it with a copy`, file, dest)
				case !force:
					return fmt.Errorf("%s exists and differs from this build's copy; pass --force to replace it", file)
				}
				if link {
					if err := os.Remove(file); err != nil {
						return err
					}
				}
			}

			if err := os.MkdirAll(target, 0o755); err != nil {
				return err
			}
			if err := os.WriteFile(file, []byte(body), 0o644); err != nil {
				return err
			}
			fmt.Fprintf(out, `installed %s

An agent picks it up from that directory; in Claude Code it loads by name,
"mdbrand". Re-run this after upgrading the binary to keep the two in step.
`, file)
			return nil
		},
	}
	c.Flags().StringVar(&dir, "dir", "", "install into this directory instead")
	c.Flags().BoolVar(&project, "project", false, "install into ./.claude/skills/mdbrand")
	c.Flags().BoolVar(&force, "force", false, "replace an existing or symlinked SKILL.md")
	return c
}
