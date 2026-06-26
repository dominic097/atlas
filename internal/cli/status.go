package cli

import (
	"github.com/spf13/cobra"

	"github.com/dominic097/atlas/pkg/atlas"
)

func newStatusCmd() *cobra.Command {
	var in atlas.StatusInput
	cmd := &cobra.Command{
		Use:   "status",
		Short: "Engine health: storage driver, tier, per-repo freshness, queue depth",
		RunE: func(cmd *cobra.Command, _ []string) error {
			eng, err := resolveEngine(cmd.Context())
			if err != nil {
				return err
			}
			defer eng.Close()
			res, err := eng.Status(cmd.Context(), in)
			if err != nil {
				return err
			}
			return renderJSON(cmd.OutOrStdout(), res)
		},
	}
	cmd.Flags().BoolVar(&in.Verbose, "verbose", false, "verbose output")
	return cmd
}
