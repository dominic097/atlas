package cli

import (
	"github.com/spf13/cobra"

	"github.com/dominic097/atlas/pkg/atlas"
)

func newCrossRepoImpactCmd() *cobra.Command {
	var (
		in    atlas.CrossRepoImpactInput
		paths []string
	)
	cmd := &cobra.Command{
		Use:   "cross-repo-impact",
		Short: "Cross-repo blast radius: which OTHER repos call routes changed handlers serve",
		RunE: func(cmd *cobra.Command, _ []string) error {
			in.ChangedPaths = paths
			if in.Repo == "" {
				in.Repo = gf.repo
			}
			eng, err := resolveEngine(cmd.Context())
			if err != nil {
				return err
			}
			defer eng.Close()
			res, err := eng.CrossRepoImpact(cmd.Context(), in)
			if err != nil {
				return err
			}
			return renderJSON(cmd.OutOrStdout(), res)
		},
	}
	f := cmd.Flags()
	f.StringVar(&in.Repo, "repo", "", "producer repo full_name (defaults to --repo / the single indexed repo)")
	f.StringSliceVar(&paths, "paths", nil, "changed handler file paths (empty = the whole repo's contract)")
	return cmd
}
