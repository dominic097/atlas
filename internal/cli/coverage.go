package cli

import (
	"github.com/spf13/cobra"

	"github.com/MsysTechnologiesllc/aziron-atlas/pkg/atlas"
)

func newCoverageCmd() *cobra.Command {
	var in atlas.CoverageInput
	cmd := &cobra.Command{
		Use:   "coverage TARGET",
		Short: "Static call-graph reachability coverage (tests reaching a symbol, or symbols a test exercises)",
		Long: "Static call-graph reachability coverage — NOT runtime coverage.\n" +
			"With --direction tests_for_symbol (default for non-test targets), lists the\n" +
			"transitive test callers that reach TARGET. With --direction symbols_for_test\n" +
			"(default when TARGET is a test), lists the non-test symbols TARGET exercises.",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			in.Target = args[0]
			in.RepoID = gf.repo
			eng, err := resolveEngine(cmd.Context())
			if err != nil {
				return err
			}
			defer eng.Close()
			res, err := eng.Coverage(cmd.Context(), in)
			if err != nil {
				return err
			}
			return renderJSON(cmd.OutOrStdout(), res)
		},
	}
	cmd.Flags().StringVar(&in.Direction, "direction", "", "tests_for_symbol | symbols_for_test (auto-detected from the target by default)")
	return cmd
}
