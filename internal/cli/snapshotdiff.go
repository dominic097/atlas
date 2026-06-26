package cli

import (
	"github.com/spf13/cobra"

	"github.com/dominic097/atlas/pkg/atlas"
)

func newSnapshotDiffCmd() *cobra.Command {
	var in atlas.SnapshotDiffInput
	cmd := &cobra.Command{
		Use:     "snapshot-diff",
		Aliases: []string{"diff"},
		Short:   "Structural diff between two snapshots (symbols/edges added/removed/modified)",
		RunE: func(cmd *cobra.Command, _ []string) error {
			in.RepoID = gf.repo
			eng, err := resolveEngine(cmd.Context())
			if err != nil {
				return err
			}
			defer eng.Close()
			res, err := eng.SnapshotDiff(cmd.Context(), in)
			if err != nil {
				return err
			}
			return renderJSON(cmd.OutOrStdout(), res)
		},
	}
	f := cmd.Flags()
	f.StringVar(&in.From, "from", "", "base snapshot: commit sha (prefix ok) or snapshot id (default: the one before --to)")
	f.StringVar(&in.To, "to", "", "head snapshot: commit sha (prefix ok) or snapshot id (default: latest)")
	return cmd
}
