package cli

import (
	"github.com/spf13/cobra"

	"github.com/MsysTechnologiesllc/aziron-atlas/pkg/atlas"
)

func newPathCmd() *cobra.Command {
	var in atlas.PathInput
	cmd := &cobra.Command{
		Use:   "path FROM TO",
		Short: "Find the shortest forward call path from one symbol to another",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			in.From = args[0]
			in.To = args[1]
			in.RepoID = gf.repo
			eng, err := resolveEngine(cmd.Context())
			if err != nil {
				return err
			}
			defer eng.Close()
			res, err := eng.Path(cmd.Context(), in)
			if err != nil {
				return err
			}
			return renderJSON(cmd.OutOrStdout(), res)
		},
	}
	cmd.Flags().IntVar(&in.MaxDepth, "max-depth", 6, "max call-graph hops to search")
	return cmd
}
