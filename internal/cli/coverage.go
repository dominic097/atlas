package cli

import (
	"github.com/spf13/cobra"

	"github.com/MsysTechnologiesllc/aziron-atlas/pkg/atlas"
)

func newCoverageCmd() *cobra.Command {
	var in atlas.CoverageInput
	cmd := &cobra.Command{
		Use:   "coverage TARGET",
		Short: "Coverage for a symbol — runtime (if imported) else static call-graph reachability",
		Long: "Reports coverage for TARGET. If a coverage profile was imported for TARGET\n" +
			"(see `atlas coverage import`), reports the REAL covered/total line ratio\n" +
			"(mode=runtime). Otherwise falls back to static call-graph reachability\n" +
			"(mode=static): with --direction tests_for_symbol (default for non-test\n" +
			"targets) it lists the transitive test callers that reach TARGET; with\n" +
			"--direction symbols_for_test (default when TARGET is a test) it lists the\n" +
			"non-test symbols TARGET exercises.",
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
	cmd.AddCommand(newCoverageImportCmd())
	return cmd
}

// newCoverageImportCmd ingests a runtime coverage profile (Go coverprofile or
// LCOV), maps covered lines onto indexed symbols, and persists per-symbol runtime
// coverage facts so subsequent `atlas coverage SYMBOL` reports mode=runtime.
func newCoverageImportCmd() *cobra.Command {
	var repo string
	cmd := &cobra.Command{
		Use:   "import PROFILE",
		Short: "Ingest a runtime coverage profile (Go coverprofile or LCOV) into the graph",
		Long: "Parses PROFILE (auto-detecting Go coverprofile vs LCOV), maps the covered\n" +
			"line sets onto the indexed symbols of the latest snapshot, and stores one\n" +
			"runtime coverage fact per symbol (covered/total lines in the symbol's span).\n" +
			"After import, `atlas coverage <symbol>` reports real RUNTIME coverage. Re-\n" +
			"importing replaces the snapshot's prior runtime facts.",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if repo == "" {
				repo = gf.repo
			}
			eng, err := resolveEngine(cmd.Context())
			if err != nil {
				return err
			}
			defer eng.Close()
			res, err := eng.CoverageImport(cmd.Context(), atlas.CoverageImportInput{
				Path:   args[0],
				RepoID: repo,
			})
			if err != nil {
				return err
			}
			return renderJSON(cmd.OutOrStdout(), res)
		},
	}
	cmd.Flags().StringVar(&repo, "repo", "", "repo to attach the coverage to (path, org/name, or repo_id; default: --repo / most-recent)")
	return cmd
}
