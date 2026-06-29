package cli

import (
	"context"
	"errors"
	"net/http"

	"github.com/spf13/cobra"

	"github.com/dominic097/atlas/internal/api"
	"github.com/dominic097/atlas/internal/mcp"
)

func newServeCmd() *cobra.Command {
	var (
		addr      string
		withMCP   bool
		withWatch bool
		watchPath string
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

			// --watch: the same long-lived server that answers REST/MCP queries also
			// keeps the graph fresh in the background. Opt-in; off by default.
			if withWatch {
				path := watchPath
				if path == "" {
					path = gf.repo
				}
				stop := startBackgroundWatch(cmd.Context(), cmd, eng, path)
				defer stop()
			}

			srv := api.NewServer(eng, api.Config{Addr: addr, MountMCP: withMCP})

			if !withMCP {
				cmd.Printf("atlas serve listening on %s\n", addr)
				return srv.ListenAndServe(cmd.Context())
			}

			// --mcp: mount MCP over Streamable HTTP on the SAME listener as REST.
			// We wrap rather than edit the api package: a parent mux routes
			// POST /mcp to the MCP handler and delegates everything else to the
			// fully-wired REST handler (auth middleware + /api/v1 routes).
			mcpSrv := mcp.NewServer(eng)
			parent := http.NewServeMux()
			parent.Handle("/mcp", mcpSrv.HTTPHandler())
			parent.Handle("/", srv.Handler())

			cmd.Printf("atlas serve listening on %s (REST /api/v1 + MCP POST /mcp)\n", addr)
			return runHTTP(cmd.Context(), addr, parent)
		},
	}
	f := cmd.Flags()
	f.StringVar(&addr, "addr", ":8083", "listen address")
	f.BoolVar(&withMCP, "mcp", false, "also mount MCP over HTTP at POST /mcp")
	f.BoolVar(&withWatch, "watch", false, "also keep the graph fresh in the background by watching the repo (opt-in; auto-refresh on change)")
	f.StringVar(&watchPath, "watch-path", "", "repo path to watch when --watch is set (default: --repo, else current dir)")
	return cmd
}

// runHTTP serves handler on addr and shuts down gracefully on ctx cancel.
func runHTTP(ctx context.Context, addr string, handler http.Handler) error {
	httpSrv := &http.Server{Addr: addr, Handler: handler}
	go func() {
		<-ctx.Done()
		_ = httpSrv.Shutdown(context.Background())
	}()
	if err := httpSrv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}
