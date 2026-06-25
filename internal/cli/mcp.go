package cli

import (
	"github.com/spf13/cobra"

	"github.com/MsysTechnologiesllc/aziron-atlas/internal/mcp"
)

func newMCPCmd() *cobra.Command {
	var transport string
	cmd := &cobra.Command{
		Use:   "mcp",
		Short: "Expose graph/search/impact as MCP tools to LLM agents (surface S3)",
		RunE: func(cmd *cobra.Command, _ []string) error {
			eng, err := resolveEngine(cmd.Context())
			if err != nil {
				return err
			}
			defer eng.Close()
			srv := mcp.NewServer(eng)
			switch transport {
			case "stdio", "":
				return srv.ServeStdio(cmd.Context(), cmd.InOrStdin(), cmd.OutOrStdout())
			default:
				// TODO(mcp): http + legacy-SSE transports.
				return srv.ServeStdio(cmd.Context(), cmd.InOrStdin(), cmd.OutOrStdout())
			}
		},
	}
	cmd.Flags().StringVar(&transport, "transport", "stdio", "transport: stdio|http")
	return cmd
}
