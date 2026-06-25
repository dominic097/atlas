// Command atlas is the CLI entrypoint for Aziron Atlas — a live, org-wide code
// knowledge graph that acts. It mounts all five consumption surfaces (CLI, HTTP
// API, MCP, SDK, runtime) over a single shared Engine.
package main

import (
	"fmt"
	"os"

	"github.com/MsysTechnologiesllc/aziron-atlas/internal/cli"
)

func main() {
	if err := cli.NewRootCmd().Execute(); err != nil {
		fmt.Fprintln(os.Stderr, "atlas:", err)
		os.Exit(1)
	}
}
