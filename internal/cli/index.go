package cli

import (
	"github.com/spf13/cobra"

	"github.com/MsysTechnologiesllc/aziron-atlas/pkg/atlas"
)

func newIndexCmd() *cobra.Command {
	var in atlas.IndexInput
	cmd := &cobra.Command{
		Use:   "index [PATH]",
		Short: "Index a repo: parse symbols/edges/routes, persist graph + lexical index",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 1 {
				in.ProjectPath = args[0]
			}
			applyIndexDefaults(&in)
			eng, err := resolveEngine(cmd.Context())
			if err != nil {
				return err
			}
			defer eng.Close()
			res, err := eng.Index(cmd.Context(), in)
			if err != nil {
				return err
			}
			return renderJSON(cmd.OutOrStdout(), res)
		},
	}
	f := cmd.Flags()
	f.StringVar(&in.Ref, "ref", "", "commit or branch to index (default: working tree)")
	f.StringVar(&in.Base, "base", "", "explicit delta base commit")
	f.BoolVar(&in.Reindex, "reindex", false, "force full reindex instead of delta")
	f.BoolVar(&in.EnableVectors, "enable-vectors", false, "run the optional embedding pass")
	return cmd
}

func applyIndexDefaults(in *atlas.IndexInput) {
	if in == nil {
		return
	}
	if in.Repo == "" {
		in.Repo = gf.repo
	}
}
