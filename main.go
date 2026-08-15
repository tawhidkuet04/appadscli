package main

import (
	"embed"
	"fmt"
	"os"

	"github.com/tawhidkuet04/asacli/cmd"
)

//go:embed all:docs/guides all:.agents/skills
var assets embed.FS

func main() {
	cmd.SetAssets(assets)
	if err := cmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, "Error:", err)
		os.Exit(1)
	}
}
