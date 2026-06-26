package cli

import (
	"github.com/spf13/cobra"

	"github.com/dominic097/atlas/pkg/atlas"
)

func newCallersCmd() *cobra.Command {
	var in atlas.CallersInput
	cmd := &cobra.Command{
		Use:   "callers SYMBOL",
		Short: "List the symbols that directly call a given symbol",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			in.Name = args[0]
			in.RepoID = gf.repo
			eng, err := resolveEngine(cmd.Context())
			if err != nil {
				return err
			}
			defer eng.Close()
			res, err := eng.Callers(cmd.Context(), in)
			if err != nil {
				return err
			}
			return renderJSON(cmd.OutOrStdout(), res)
		},
	}
	cmd.Flags().IntVar(&in.Limit, "limit", 50, "max callers to return")
	return cmd
}
