package cli

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/dominic097/atlas/pkg/atlas"
)

func newExportCmd() *cobra.Command {
	var (
		in     atlas.GraphExportInput
		out    string
		bundle string
	)
	cmd := &cobra.Command{
		Use:   "export",
		Short: "Export the code knowledge graph (a symbol's neighborhood, or the whole repo)",
		Long: "Export the call graph as json, mermaid, dot, or a self-contained interactive\n" +
			"html page. With --symbol, exports the caller/callee neighborhood around that\n" +
			"symbol (bounded by --depth/--max-nodes); with --all, dumps the whole repo graph\n" +
			"(json or html recommended). --out FILE writes instead of stdout.\n\n" +
			"--bundle DIR writes a graphify-out-style directory: DIR/graph.html (the\n" +
			"interactive visualization) AND DIR/report.md (the deterministic graph report).",
		RunE: func(cmd *cobra.Command, _ []string) error {
			in.RepoID = gf.repo
			eng, err := resolveEngine(cmd.Context())
			if err != nil {
				return err
			}
			defer eng.Close()

			// --bundle: write graph.html + report.md into DIR (mirrors graphify-out/).
			if bundle != "" {
				return runBundle(cmd, eng, in, bundle)
			}

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
	f.StringVar(&in.Format, "format", "json", "graph format: json|mermaid|dot|html")
	f.BoolVar(&in.All, "all", false, "export the whole repo graph instead of a subgraph")
	f.StringVarP(&out, "out", "o", "", "write to a file instead of stdout")
	f.StringVar(&bundle, "bundle", "", "write DIR/graph.html (interactive viz) + DIR/report.md (graphify-out style)")
	return cmd
}

// runBundle writes a graphify-out-style directory: graph.html (the interactive,
// self-contained visualization) and report.md (the deterministic graph report).
// It defaults the export to the whole-repo graph + html when the caller didn't
// already select a focus/format, so `atlas export --bundle out` just works.
func runBundle(cmd *cobra.Command, eng atlas.Engine, in atlas.GraphExportInput, dir string) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("bundle: create dir: %w", err)
	}
	// The bundle's viz is always html; default to the whole graph unless a focus
	// symbol was given.
	in.Format = "html"
	if !in.All && in.Symbol == "" {
		in.All = true
	}

	gex, err := eng.GraphExport(cmd.Context(), in)
	if err != nil {
		return err
	}
	htmlPath := filepath.Join(dir, "graph.html")
	if err := os.WriteFile(htmlPath, []byte(gex.Content), 0o644); err != nil {
		return fmt.Errorf("bundle: write graph.html: %w", err)
	}

	rep, err := eng.Report(cmd.Context(), atlas.ReportInput{RepoID: in.RepoID})
	if err != nil {
		return err
	}
	mdPath := filepath.Join(dir, "report.md")
	if err := os.WriteFile(mdPath, []byte(rep.Markdown), 0o644); err != nil {
		return fmt.Errorf("bundle: write report.md: %w", err)
	}

	cmd.Printf("wrote bundle to %s:\n  graph.html  (%d nodes / %d edges)\n  report.md\n",
		dir, gex.Nodes, gex.Edges)
	return nil
}
