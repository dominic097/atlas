package cli

import (
	"context"
	"net/http"

	"github.com/spf13/cobra"

	"github.com/dominic097/atlas/internal/mcp"
)

func newMCPCmd() *cobra.Command {
	var (
		transport string
		httpAddr  string
		sseAddr   string
		withWatch bool
		watchPath string
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

			// --watch: the SAME warm process that serves the agent also keeps the
			// graph fresh in the background, so every MCP query hits a fresh graph
			// with NO manual `atlas index`. Opt-in to avoid surprising existing users.
			if withWatch {
				path := watchPath
				if path == "" {
					path = gf.repo
				}
				stop := startBackgroundWatch(cmd.Context(), cmd, eng, path)
				defer stop()
			}

			srv := mcp.NewServer(eng)

			// --sse: serve the legacy HTTP+SSE transport for older clients.
			if sseAddr != "" || transport == "sse" {
				addr := sseAddr
				if addr == "" {
					addr = "127.0.0.1:8766"
				}
				return serveMCPSSE(cmd, srv, addr)
			}
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
	cmd.Flags().StringVar(&transport, "transport", "stdio", "transport: stdio|http|sse")
	cmd.Flags().StringVar(&httpAddr, "http", "", "serve MCP over Streamable HTTP on this address (e.g. 127.0.0.1:8765); empty = stdio")
	cmd.Flags().StringVar(&sseAddr, "sse", "", "serve the legacy MCP HTTP+SSE transport on this address (e.g. 127.0.0.1:8766); empty = stdio")
	cmd.Flags().BoolVar(&withWatch, "watch", false, "also keep the graph fresh in the background by watching the repo (opt-in; auto-refresh on change)")
	cmd.Flags().StringVar(&watchPath, "watch-path", "", "repo path to watch when --watch is set (default: --repo, else current dir)")
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

// serveMCPSSE runs an http.Server that mounts the legacy MCP HTTP+SSE transport
// (GET /sse opens the event-stream; POST /message delivers JSON-RPC requests)
// and shuts down gracefully when the command context is cancelled.
func serveMCPSSE(cmd *cobra.Command, srv *mcp.Server, addr string) error {
	handler := srv.SSEHandler()
	mux := http.NewServeMux()
	mux.Handle("/sse", handler)
	mux.Handle("/message", handler)
	httpSrv := &http.Server{Addr: addr, Handler: mux}

	ctx := cmd.Context()
	errCh := make(chan error, 1)
	go func() {
		cmd.Printf("atlas mcp (legacy SSE) listening on http://%s/sse\n", addr)
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
