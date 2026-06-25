package cli

import (
	"os"

	"github.com/spf13/cobra"

	"github.com/MsysTechnologiesllc/aziron-atlas/pkg/atlas"
)

func newExportCmd() *cobra.Command {
	var (
		in  atlas.GraphExportInput
		out string
	)
	cmd := &cobra.Command{
		Use:   "export",
		Short: "Export the code knowledge graph (a symbol's neighborhood, or the whole repo)",
		Long: "Export the call graph as json, mermaid, or dot. With --symbol, exports the\n" +
			"caller/callee neighborhood around that symbol (bounded by --depth/--max-nodes);\n" +
			"with --all, dumps the whole repo graph (json recommended).",
		RunE: func(cmd *cobra.Command, _ []string) error {
			in.RepoID = gf.repo
			eng, err := resolveEngine(cmd.Context())
			if err != nil {
				return err
			}
			defer eng.Close()
			res, err := eng.GraphExport(cmd.Context(), in)
			if err != nil {
				return err
			}
			if out != "" {
				if err := os.WriteFile(out, []byte(res.Content), 0o644); err != nil {
					return err
				}
				cmd.Printf("wrote %d nodes / %d edges (%s) to %s\n", res.Nodes, res.Edges, res.Format, out)
				return nil
			}
			cmd.Println(res.Content)
			return nil
		},
	}
	f := cmd.Flags()
	f.StringVar(&in.Symbol, "symbol", "", "focus symbol: export its caller/callee neighborhood")
	f.IntVar(&in.Depth, "depth", 2, "neighborhood hops (with --symbol)")
	f.IntVar(&in.MaxNodes, "max-nodes", 200, "subgraph node cap")
	f.StringVar(&in.Format, "format", "json", "graph format: json|mermaid|dot")
	f.BoolVar(&in.All, "all", false, "export the whole repo graph instead of a subgraph")
	f.StringVarP(&out, "out", "o", "", "write to a file instead of stdout")
	return cmd
}
