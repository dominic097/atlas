// Command atlas is the CLI entrypoint for Aziron Atlas — a live, org-wide code
// knowledge graph that acts. It mounts all five consumption surfaces (CLI, HTTP
// API, MCP, SDK, runtime) over a single shared Engine.
package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/MsysTechnologiesllc/aziron-atlas/internal/cli"
)

func main() {
	if err := cli.NewRootCmd().Execute(); err != nil {
		// Many engine errors already carry an "atlas:" prefix; trim it so the
		// message reads "atlas: <msg>" rather than "atlas: atlas: <msg>".
		msg := strings.TrimPrefix(err.Error(), "atlas: ")
		fmt.Fprintln(os.Stderr, "atlas:", msg)
		os.Exit(1)
	}
}
