package cli

import (
	"github.com/spf13/cobra"

	"github.com/dominic097/atlas/pkg/atlas"
)

func newSearchCmd() *cobra.Command {
	var in atlas.SearchInput
	cmd := &cobra.Command{
		Use:   "search QUERY",
		Short: "Code-aware lexical search (BM25 + trigram) over the symbol index",
		Args:  cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			in.Query = args[0]
			if in.Mode == "" {
				in.Mode = "lexical"
			}
			eng, err := resolveEngine(cmd.Context())
			if err != nil {
				return err
			}
			defer eng.Close()
			res, err := eng.Search(cmd.Context(), in)
			if err != nil {
				return err
			}
			return renderJSON(cmd.OutOrStdout(), res)
		},
	}
	f := cmd.Flags()
	f.StringVar(&in.Kind, "kind", "", "symbol-kind filter (function|method|class|...)")
	f.StringVar(&in.PathGlob, "path", "", "path glob filter")
	f.IntVar(&in.Limit, "limit", 20, "max results")
	f.StringVar(&in.Mode, "mode", "lexical", "lexical|semantic|hybrid (semantic/hybrid need vectors)")
	return cmd
}
