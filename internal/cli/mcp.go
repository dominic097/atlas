package cli

import (
	"context"
	"net/http"

	"github.com/spf13/cobra"

	"github.com/MsysTechnologiesllc/aziron-atlas/internal/mcp"
)

func newMCPCmd() *cobra.Command {
	var (
		transport string
		httpAddr  string
	)
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

			// --http wins over --transport: serve MCP over Streamable HTTP.
			if httpAddr != "" || transport == "http" {
				addr := httpAddr
				if addr == "" {
					addr = "127.0.0.1:8765"
				}
				return serveMCPHTTP(cmd, srv, addr)
			}
			return srv.ServeStdio(cmd.Context(), cmd.InOrStdin(), cmd.OutOrStdout())
		},
	}
	cmd.Flags().StringVar(&transport, "transport", "stdio", "transport: stdio|http")
	cmd.Flags().StringVar(&httpAddr, "http", "", "serve MCP over HTTP on this address (e.g. 127.0.0.1:8765); empty = stdio")
	return cmd
}

// serveMCPHTTP runs an http.Server that mounts the MCP Streamable-HTTP handler at
// POST /mcp and shuts down gracefully when the command context is cancelled.
func serveMCPHTTP(cmd *cobra.Command, srv *mcp.Server, addr string) error {
	mux := http.NewServeMux()
	mux.Handle("/mcp", srv.HTTPHandler())
	httpSrv := &http.Server{Addr: addr, Handler: mux}

	ctx := cmd.Context()
	errCh := make(chan error, 1)
	go func() {
		cmd.Printf("atlas mcp listening on http://%s/mcp\n", addr)
		err := httpSrv.ListenAndServe()
		if err == http.ErrServerClosed {
			err = nil
		}
		errCh <- err
	}()

	select {
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithCancel(context.Background())
		defer cancel()
		_ = httpSrv.Shutdown(shutdownCtx)
		return nil
	case err := <-errCh:
		return err
	}
}
