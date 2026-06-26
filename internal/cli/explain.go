package cli

import (
	"github.com/spf13/cobra"

	"github.com/dominic097/atlas/pkg/atlas"
)

func newExplainCmd() *cobra.Command {
	var in atlas.ExplainInput
	cmd := &cobra.Command{
		Use:   "explain SYMBOL",
		Short: "Deterministic context bundle for a symbol (defs, callers/callees, imports, served routes, cross-repo consumers)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			in.Name = args[0]
			in.RepoID = gf.repo
			eng, err := resolveEngine(cmd.Context())
			if err != nil {
				return err
			}
			defer eng.Close()
			res, err := eng.Explain(cmd.Context(), in)
			if err != nil {
				return err
			}
			return renderJSON(cmd.OutOrStdout(), res)
		},
	}
	return cmd
}
