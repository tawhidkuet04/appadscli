// Package cmd wires the asacli command tree.
package cmd

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"
	"github.com/tawhidkuet04/asacli/internal/api"
	"github.com/tawhidkuet04/asacli/internal/config"
	"github.com/tawhidkuet04/asacli/internal/output"
)

// Version is stamped by goreleaser via -ldflags.
var Version = "dev"

var (
	flagOutput  string
	flagAccount string
)

var rootCmd = &cobra.Command{
	Use:   "asacli",
	Short: "Your App Store growth stack in one binary — ASO + Apple Ads CLI",
	Long: `asacli — the complete ASO + Apple Ads CLI for indie developers.

Apple Ads Platform API v1 management (campaigns, keywords, bids, reports,
insights) fused with organic ASO (rank tracking, keyword difficulty,
competitor intel) and revenue attribution via RevenueCat. JSON-first,
agent-native, --dry-run everywhere.`,
	Version:       Version,
	SilenceUsage:  true,
	SilenceErrors: true,
}

// Execute runs the CLI.
func Execute() error { return rootCmd.Execute() }

func init() {
	rootCmd.PersistentFlags().StringVarP(&flagOutput, "output", "o", "", "output format: table|json|csv|markdown (default: TTY→table, pipe→json)")
	rootCmd.PersistentFlags().StringVar(&flagAccount, "account", "", "ad account id (default: `asacli accounts use` selection)")
}

// cfg loads config, tolerating a missing file.
func cfg() *config.Config {
	c, err := config.Load()
	if err != nil {
		fmt.Fprintln(os.Stderr, "warning: config unreadable:", err)
		return &config.Config{}
	}
	return c
}

// client builds an API client honoring --account and config defaults.
func client() *api.Client { return api.New(cfg(), flagAccount) }

// render builds the output renderer honoring --output and config defaults.
func render() *output.Renderer { return output.New(flagOutput, cfg().DefaultOutput) }

// confirmOrAbort enforces the mutation safety contract: --dry-run prints the
// would-be action and returns false; otherwise --confirm (or --yes) or an
// interactive "y" is required.
func confirmOrAbort(cmd *cobra.Command, action string) (bool, error) {
	dry, _ := cmd.Flags().GetBool("dry-run")
	if dry {
		fmt.Fprintln(os.Stderr, "[dry-run] would "+action)
		return false, nil
	}
	confirmed, _ := cmd.Flags().GetBool("confirm")
	if confirmed {
		return true, nil
	}
	fi, err := os.Stdin.Stat()
	if err == nil && (fi.Mode()&os.ModeCharDevice) != 0 {
		fmt.Fprintf(os.Stderr, "About to %s. Continue? [y/N] ", action)
		line, _ := bufio.NewReader(os.Stdin).ReadString('\n')
		if strings.EqualFold(strings.TrimSpace(line), "y") {
			return true, nil
		}
		return false, fmt.Errorf("aborted")
	}
	return false, fmt.Errorf("refusing to %s without --confirm (non-interactive)", action)
}

// addMutationFlags attaches the standard safety flags.
func addMutationFlags(cmd *cobra.Command) {
	cmd.Flags().Bool("dry-run", false, "print what would happen without doing it")
	cmd.Flags().Bool("confirm", false, "skip the interactive confirmation prompt")
}

// readLines reads a file of newline-separated values, skipping blanks and #comments.
func readLines(path string) ([]string, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var out []string
	for _, l := range strings.Split(string(b), "\n") {
		l = strings.TrimSpace(l)
		if l == "" || strings.HasPrefix(l, "#") {
			continue
		}
		out = append(out, l)
	}
	return out, nil
}
