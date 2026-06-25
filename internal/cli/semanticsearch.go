package cli

import (
	"github.com/spf13/cobra"

	"github.com/MsysTechnologiesllc/aziron-atlas/pkg/atlas"
)

// newSemanticSearchCmd runs the OPTIONAL, gated semantic search. It is honest
// about degradation: when vectors are disabled (the default) or the snapshot was
// indexed without --enable-vectors, it falls back to lexical and the result's
// "degraded":true / "mode_used":"lexical" fields say so. Enable vectors with
// ATLAS_ENABLE_VECTORS=1 (and index with `atlas index --enable-vectors`) for a
// real semantic ("mode_used":"semantic") run.
func newSemanticSearchCmd() *cobra.Command {
	var in atlas.SemanticSearchInput
	cmd := &cobra.Command{
		Use:   "semantic-search QUERY",
		Short: "Vector nearest-neighbor search over indexed symbols (optional; degrades to lexical when vectors are off)",
		Long: "Semantic search over the symbol embeddings (OPTIONAL, gated). With vectors\n" +
			"enabled (ATLAS_ENABLE_VECTORS=1, and a repo indexed with --enable-vectors)\n" +
			"it embeds the query and returns the nearest symbols by cosine similarity\n" +
			"(mode_used=semantic). Otherwise it transparently degrades to lexical search\n" +
			"and reports degraded=true / mode_used=lexical — the deterministic core is\n" +
			"always available. By default the embedder is offline (deterministic token\n" +
			"overlap); set ATLAS_EMBED_URL for a real embedding model.",
		Args: cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			in.Query = args[0]
			eng, err := resolveEngine(cmd.Context())
			if err != nil {
				return err
			}
			defer eng.Close()
			res, err := eng.SemanticSearch(cmd.Context(), in)
			if err != nil {
				return err
			}
			return renderJSON(cmd.OutOrStdout(), res)
		},
	}
	f := cmd.Flags()
	f.IntVar(&in.Limit, "limit", 20, "max results")
	f.Float64Var(&in.MinScore, "min-score", 0, "minimum cosine similarity to include a hit")
	return cmd
}
