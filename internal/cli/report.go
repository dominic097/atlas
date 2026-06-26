package cli

import (
	"github.com/spf13/cobra"

	"github.com/dominic097/atlas/pkg/atlas"
)

func newReportCmd() *cobra.Command {
	var in atlas.ReportInput
	cmd := &cobra.Command{
		Use:   "report",
		Short: "Render a deterministic graph report: summary stats, top hubs (god nodes), and communities",
		Long: "report composes the snapshot's graph stats, top hubs, and top communities. " +
			"The default JSON carries a ready-to-render Markdown document; use --format plain " +
			"to print the Markdown report directly.",
		RunE: func(cmd *cobra.Command, _ []string) error {
			in.RepoID = gf.repo
			eng, err := resolveEngine(cmd.Context())
			if err != nil {
				return err
			}
			defer eng.Close()
			res, err := eng.Report(cmd.Context(), in)
			if err != nil {
				return err
			}
			return renderJSON(cmd.OutOrStdout(), res)
		},
	}
	return cmd
}
