// Command atlas is the CLI entrypoint for a local code knowledge graph. It
// mounts the CLI, HTTP API, MCP, and SDK surfaces over a shared Engine.
package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/dominic097/atlas/internal/cli"
	"github.com/dominic097/atlas/internal/runtimecfg"
)

func main() {
	// Apply any operator-set runtime knobs (ATLAS_MEMORY_LIMIT / ATLAS_GOGC)
	// before anything allocates. No-op unless explicitly set; the warm daemons
	// add their own soft GC default on top (see internal/cli watch/serve/mcp).
	runtimecfg.Apply(os.Stderr)

	cli.SetBuildInfo(Version, Commit, Date)
	if err := cli.NewRootCmd().Execute(); err != nil {
		// Many engine errors already carry an "atlas:" prefix; trim it so the
		// message reads "atlas: <msg>" rather than "atlas: atlas: <msg>".
		msg := strings.TrimPrefix(err.Error(), "atlas: ")
		fmt.Fprintln(os.Stderr, "atlas:", msg)
		os.Exit(1)
	}
}
