package cli

import (
	"github.com/spf13/cobra"

	"github.com/MsysTechnologiesllc/aziron-atlas/internal/api"
)

func newServeCmd() *cobra.Command {
	var (
		addr    string
		withMCP bool
	)
	cmd := &cobra.Command{
		Use:   "serve",
		Short: "Run the REST HTTP API (surface S2); --mcp also mounts MCP over HTTP",
		RunE: func(cmd *cobra.Command, _ []string) error {
			eng, err := resolveEngine(cmd.Context())
			if err != nil {
				return err
			}
			defer eng.Close()
			srv := api.NewServer(eng, api.Config{Addr: addr, MountMCP: withMCP})
			cmd.Printf("atlas serve listening on %s\n", addr)
			return srv.ListenAndServe(cmd.Context())
		},
	}
	f := cmd.Flags()
	f.StringVar(&addr, "addr", ":8083", "listen address")
	f.BoolVar(&withMCP, "mcp", false, "also mount MCP over HTTP")
	return cmd
}
