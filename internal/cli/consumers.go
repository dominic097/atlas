package cli

import (
	"github.com/spf13/cobra"

	"github.com/MsysTechnologiesllc/aziron-atlas/pkg/atlas"
)

func newConsumersCmd() *cobra.Command {
	var in atlas.ConsumersInput
	cmd := &cobra.Command{
		Use:   "consumers",
		Short: "List OTHER repos that call any route this repo serves (cross-repo)",
		RunE: func(cmd *cobra.Command, _ []string) error {
			if in.Repo == "" {
				in.Repo = gf.repo
			}
			eng, err := resolveEngine(cmd.Context())
			if err != nil {
				return err
			}
			defer eng.Close()
			res, err := eng.Consumers(cmd.Context(), in)
			if err != nil {
				return err
			}
			return renderJSON(cmd.OutOrStdout(), res)
		},
	}
	cmd.Flags().StringVar(&in.Repo, "repo", "", "producer repo full_name (defaults to --repo / the single indexed repo)")
	return cmd
}
