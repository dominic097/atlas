package cli

import (
	"github.com/spf13/cobra"

	"github.com/MsysTechnologiesllc/aziron-atlas/pkg/atlas"
)

func newImpactCmd() *cobra.Command {
	var (
		in    atlas.ImpactInput
		paths []string
		syms  []string
	)
	cmd := &cobra.Command{
		Use:   "impact",
		Short: "Single-repo blast radius for a change (impacted symbols/files/tests)",
		RunE: func(cmd *cobra.Command, _ []string) error {
			in.ChangedPaths = paths
			in.Symbols = syms
			in.RepoID = gf.repo
			eng, err := resolveEngine(cmd.Context())
			if err != nil {
				return err
			}
			defer eng.Close()
			res, err := eng.Impact(cmd.Context(), in)
			if err != nil {
				return err
			}
			return renderJSON(cmd.OutOrStdout(), res)
		},
	}
	f := cmd.Flags()
	f.StringSliceVar(&paths, "paths", nil, "changed file paths")
	f.StringSliceVar(&syms, "symbols", nil, "changed symbol names")
	f.IntVar(&in.MaxDepth, "max-depth", 3, "max traversal depth")
	f.BoolVar(&in.IncludeTests, "tests", true, "include covering tests")
	return cmd
}
