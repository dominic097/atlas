package cli

import (
	"github.com/spf13/cobra"

	"github.com/dominic097/atlas/pkg/atlas"
)

func newCommunitiesCmd() *cobra.Command {
	var in atlas.CommunitiesInput
	cmd := &cobra.Command{
		Use:   "communities",
		Short: "Detect symbol communities (deterministic clusters of densely-connected symbols)",
		RunE: func(cmd *cobra.Command, _ []string) error {
			in.RepoID = gf.repo
			eng, err := resolveEngine(cmd.Context())
			if err != nil {
				return err
			}
			defer eng.Close()
			res, err := eng.Communities(cmd.Context(), in)
			if err != nil {
				return err
			}
			return renderJSON(cmd.OutOrStdout(), res)
		},
	}
	cmd.Flags().IntVar(&in.Limit, "limit", 20, "max communities to return")
	return cmd
}
