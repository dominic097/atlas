// Package cli implements consumption surface S1: the `atlas` command tree for
// humans, scripts, and CI. Every command resolves a shared Engine and calls
// into it, so CLI/HTTP/MCP/SDK all share one code path.
package cli

import (
	"context"

	"github.com/spf13/cobra"

	"github.com/MsysTechnologiesllc/aziron-atlas/pkg/atlas"
)

// persistent flags shared by every command.
type globalFlags struct {
	db     string
	repo   string
	tenant string
	format string
	json   bool
}

var gf globalFlags

// NewRootCmd builds the full command tree.
func NewRootCmd() *cobra.Command {
	root := &cobra.Command{
		Use:   "atlas",
		Short: "Atlas — a live, org-wide code knowledge graph that acts",
		Long: "Atlas indexes code into a knowledge graph (symbols, calls, routes, " +
			"coverage), then answers impact/search/cross-repo/test queries and acts on " +
			"them. Local-first (embedded SQLite, zero infra) or hosted (org-wide).",
		SilenceUsage:  true,
		SilenceErrors: true,
	}

	pf := root.PersistentFlags()
	pf.StringVar(&gf.db, "db", "sqlite://./.atlas/atlas.db", "storage DSN (sqlite://PATH or postgres://...)")
	pf.StringVar(&gf.repo, "repo", "", "default repo (path, org/name, or repo_id)")
	pf.StringVar(&gf.tenant, "tenant", "", "tenant/org scope to isolate repos to (hosted multi-tenant; empty = all repos)")
	pf.StringVar(&gf.format, "format", "", "output format: table|plain|json|ndjson (auto by default)")
	pf.BoolVar(&gf.json, "json", false, "shorthand for --format json")

	root.AddCommand(
		newIndexCmd(),
		newSearchCmd(),
		newImpactCmd(),
		newCallersCmd(),
		newSymbolCmd(),
		newNeighborsCmd(),
		newPathCmd(),
		newRefsCmd(),
		newExplainCmd(),
		newCoverageCmd(),
		newExportCmd(),
		newHistoryCmd(),
		newSnapshotDiffCmd(),
		newRouteContractsCmd(),
		newConsumersCmd(),
		newCrossRepoImpactCmd(),
		newStatusCmd(),
		newServeCmd(),
		newMCPCmd(),
		newInstallCmd(),
		newVersionCmd(),
	)
	return root
}

// resolveEngine opens the shared Engine from the global --db flag. The DSN
// scheme picks the tier (the keystone swap).
func resolveEngine(ctx context.Context) (atlas.Engine, error) {
	opt := atlas.WithSQLite("./.atlas/atlas.db")
	switch {
	case len(gf.db) > len("postgres://") && gf.db[:len("postgres://")] == "postgres://":
		opt = atlas.WithPostgres(gf.db)
	case len(gf.db) > len("sqlite://") && gf.db[:len("sqlite://")] == "sqlite://":
		opt = atlas.WithSQLite(gf.db[len("sqlite://"):])
	}
	opts := []atlas.Option{opt}
	if gf.tenant != "" {
		opts = append(opts, atlas.WithScope(gf.tenant))
	}
	return atlas.New(ctx, opts...)
}

func newVersionCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print the Atlas version",
		RunE: func(cmd *cobra.Command, _ []string) error {
			cmd.Println("atlas (scaffold)")
			return nil
		},
	}
}
