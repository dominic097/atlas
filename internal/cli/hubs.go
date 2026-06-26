package cli

import (
	"github.com/spf13/cobra"

	"github.com/dominic097/atlas/pkg/atlas"
)

func newHubsCmd() *cobra.Command {
	var in atlas.HubsInput
	cmd := &cobra.Command{
		Use:   "hubs",
		Short: "Rank the graph's hubs (\"god nodes\") by call-graph degree centrality",
		RunE: func(cmd *cobra.Command, _ []string) error {
			in.RepoID = gf.repo
			eng, err := resolveEngine(cmd.Context())
			if err != nil {
				return err
			}
			defer eng.Close()
			res, err := eng.Hubs(cmd.Context(), in)
			if err != nil {
				return err
			}
			return renderJSON(cmd.OutOrStdout(), res)
		},
	}
	cmd.Flags().IntVar(&in.Limit, "limit", 20, "max hubs to return")
	return cmd
}
