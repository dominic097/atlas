package cli

import (
	"github.com/spf13/cobra"

	"github.com/MsysTechnologiesllc/aziron-atlas/pkg/atlas"
)

// newContextCmd exposes the token-budgeted `context` op: given changed/seed
// paths (and an optional retrieval query), it returns a bounded review bundle —
// the changed symbols with body excerpts, retrieval hits, impacted files, and
// the scoped edges between them — so an agent packs spans instead of whole files.
func newContextCmd() *cobra.Command {
	var (
		in    atlas.ContextInput
		paths []string
	)
	cmd := &cobra.Command{
		Use:   "context",
		Short: "Token-budgeted review context for changed paths: changed symbols (with body excerpts), retrieval hits, impacted files, and scoped edges",
		Long: "Build a bounded review-context bundle for the given changed/seed paths.\n\n" +
			"Budgets are configurable at three levels (highest precedence first):\n" +
			"  1. these per-request flags (--limit/--max-files/--max-edges/--max-depth)\n" +
			"  2. the ATLAS_CONTEXT_{LIMIT,MAX_FILES,MAX_EDGES,MAX_DEPTH} env vars\n" +
			"  3. the built-in defaults (80 / 60 / 500 / 3)\n\n" +
			"SDK callers can set the defaults with atlas.WithContextBudget(...).",
		RunE: func(cmd *cobra.Command, _ []string) error {
			in.Paths = paths
			in.RepoID = gf.repo
			eng, err := resolveEngine(cmd.Context())
			if err != nil {
				return err
			}
			defer eng.Close()
			res, err := eng.Context(cmd.Context(), in)
			if err != nil {
				return err
			}
			return renderJSON(cmd.OutOrStdout(), res)
		},
	}
	f := cmd.Flags()
	f.StringSliceVar(&paths, "paths", nil, "changed/seed file paths to build context around")
	f.StringVar(&in.Query, "query", "", "optional retrieval query (auto-derived from the seed paths/symbols when empty)")
	f.IntVar(&in.Limit, "limit", 0, "max symbols packed (default 80)")
	f.IntVar(&in.MaxFiles, "max-files", 0, "max files referenced (default 60)")
	f.IntVar(&in.MaxEdges, "max-edges", 0, "max scoped edges (default 500)")
	f.IntVar(&in.MaxDepth, "max-depth", 0, "impact traversal depth (default 3)")
	return cmd
}
