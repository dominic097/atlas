package cli

import (
	"github.com/spf13/cobra"

	"github.com/dominic097/atlas/pkg/atlas"
)

func newStatsCmd() *cobra.Command {
	var in atlas.StatsInput
	cmd := &cobra.Command{
		Use:   "stats",
		Short: "Show graph and index telemetry statistics for an indexed repo",
		RunE: func(cmd *cobra.Command, _ []string) error {
			in.RepoID = gf.repo
			eng, err := resolveEngine(cmd.Context())
			if err != nil {
				return err
			}
			defer eng.Close()
			res, err := eng.Stats(cmd.Context(), in)
			if err != nil {
				return err
			}
			return renderJSON(cmd.OutOrStdout(), res)
		},
	}
	cmd.Flags().IntVar(&in.Limit, "limit", 20, "number of recent snapshot telemetry rows to return")
	return cmd
}
