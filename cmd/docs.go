package cmd

import (
	"embed"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
)

// Assets holds the embedded guides and skills, set from main via SetAssets.
var Assets embed.FS

// SetAssets wires the embedded filesystem from the main package.
func SetAssets(a embed.FS) { Assets = a }

func init() {
	docsCmd := &cobra.Command{Use: "docs", Short: "Built-in guides"}
	show := &cobra.Command{
		Use:   "show <topic>",
		Short: "Print a guide (see `appadscli docs list`)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			b, err := Assets.ReadFile("docs/guides/" + args[0] + ".md")
			if err != nil {
				return fmt.Errorf("unknown topic %q — see `appadscli docs list`", args[0])
			}
			fmt.Print(string(b))
			return nil
		},
	}
	list := &cobra.Command{
		Use:   "list",
		Short: "List available guides",
		RunE: func(cmd *cobra.Command, args []string) error {
			entries, err := fs.ReadDir(Assets, "docs/guides")
			if err != nil {
				return err
			}
			for _, e := range entries {
				fmt.Println(strings.TrimSuffix(e.Name(), ".md"))
			}
			return nil
		},
	}
	docsCmd.AddCommand(show, list)
	rootCmd.AddCommand(docsCmd)

	var global bool
	installSkills := &cobra.Command{
		Use:   "install-skills",
		Short: "Install the agent skills pack (harvest/launch/audit playbooks)",
		RunE: func(cmd *cobra.Command, args []string) error {
			dest := filepath.Join(".", ".claude", "skills")
			if global {
				home, err := os.UserHomeDir()
				if err != nil {
					return err
				}
				dest = filepath.Join(home, ".claude", "skills")
			}
			n := 0
			err := fs.WalkDir(Assets, ".agents/skills", func(path string, d fs.DirEntry, err error) error {
				if err != nil || d.IsDir() {
					return err
				}
				rel := strings.TrimPrefix(path, ".agents/skills/")
				target := filepath.Join(dest, rel)
				if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
					return err
				}
				b, err := Assets.ReadFile(path)
				if err != nil {
					return err
				}
				if err := os.WriteFile(target, b, 0o644); err != nil {
					return err
				}
				n++
				return nil
			})
			if err != nil {
				return err
			}
			fmt.Printf("✓ installed %d skill file(s) to %s\n", n, dest)
			return nil
		},
	}
	installSkills.Flags().BoolVar(&global, "global", false, "install to ~/.claude/skills instead of ./.claude/skills")
	rootCmd.AddCommand(installSkills)
}
