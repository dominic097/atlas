package cli

import (
	"github.com/spf13/cobra"

	"github.com/dominic097/atlas/pkg/atlas"
)

// newLinkCmd registers a repo into the graph WITHOUT indexing it, so it
// participates in cross-repo and shows in `atlas status`. REPO may be a
// filesystem path, a git remote URL (git@host:Org/Repo.git or
// https://host/Org/Repo(.git)), or a bare org/name.
func newLinkCmd() *cobra.Command {
	var in atlas.LinkInput
	cmd := &cobra.Command{
		Use:   "link REPO",
		Short: "Register a repo into the graph without indexing it (declarative repo registration)",
		Long: "Registers REPO into the graph WITHOUT indexing it, so it participates in\n" +
			"cross-repo queries and shows up in `atlas status`. REPO may be a filesystem\n" +
			"path, a git remote URL (git@host:Org/Repo.git or https://host/Org/Repo(.git)),\n" +
			"or a bare org/name. link is idempotent: re-linking the same repo updates its\n" +
			"registration and reports created=false. Indexing (and webhooks) are Pulse's\n" +
			"job — run `atlas index` to populate the graph.",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			in.Repo = args[0]
			eng, err := resolveEngine(cmd.Context())
			if err != nil {
				return err
			}
			defer eng.Close()
			res, err := eng.Link(cmd.Context(), in)
			if err != nil {
				return err
			}
			return renderJSON(cmd.OutOrStdout(), res)
		},
	}
	cmd.Flags().StringVar(&in.Branch, "branch", "main", "default branch to register for the repo")
	return cmd
}
