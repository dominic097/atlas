package cli

import (
	"github.com/spf13/cobra"

	"github.com/dominic097/atlas/pkg/atlas"
)

func newRouteContractsCmd() *cobra.Command {
	var in atlas.RouteContractsInput
	cmd := &cobra.Command{
		Use:   "route-contracts",
		Short: "List the producer HTTP routes a repo serves (its public contract)",
		RunE: func(cmd *cobra.Command, _ []string) error {
			if in.Repo == "" {
				in.Repo = gf.repo
			}
			eng, err := resolveEngine(cmd.Context())
			if err != nil {
				return err
			}
			defer eng.Close()
			res, err := eng.RouteContracts(cmd.Context(), in)
			if err != nil {
				return err
			}
			return renderJSON(cmd.OutOrStdout(), res)
		},
	}
	cmd.Flags().StringVar(&in.Repo, "repo", "", "repo full_name (defaults to --repo / the single indexed repo)")
	return cmd
}
