package cli

import (
	"github.com/spf13/cobra"

	"github.com/MsysTechnologiesllc/aziron-atlas/pkg/atlas"
)

func newHistoryCmd() *cobra.Command {
	var in atlas.HistoryInput
	cmd := &cobra.Command{
		Use:   "history",
		Short: "Per-commit snapshot timeline for a repo (the temporal moat)",
		RunE: func(cmd *cobra.Command, _ []string) error {
			in.RepoID = gf.repo
			eng, err := resolveEngine(cmd.Context())
			if err != nil {
				return err
			}
			defer eng.Close()
			res, err := eng.History(cmd.Context(), in)
			if err != nil {
				return err
			}
			return renderJSON(cmd.OutOrStdout(), res)
		},
	}
	cmd.Flags().IntVar(&in.Limit, "limit", 50, "max snapshots to list")
	return cmd
}
