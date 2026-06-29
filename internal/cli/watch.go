package cli

import (
	"context"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/spf13/cobra"

	"github.com/dominic097/atlas/internal/watch"
	"github.com/dominic097/atlas/pkg/atlas"
)

// newWatchCmd builds `atlas watch [PATH]`: a foreground daemon that indexes
// once, then keeps the graph fresh by re-indexing (incrementally) on every file
// change until interrupted. This is what a dev or agent runs once to keep the
// local graph live with no manual `atlas index` after each edit.
func newWatchCmd() *cobra.Command {
	var (
		debounceMS    int
		enableVectors bool
	)
	cmd := &cobra.Command{
		Use:   "watch [PATH]",
		Short: "Watch a repo and auto-refresh the graph on every change (Ctrl-C to stop)",
		Long: "Index the repo once, then watch its working tree and run an incremental, " +
			"working-tree-aware update on every file change so the graph stays fresh with " +
			"no manual `atlas index`. A burst of edits is coalesced into one update. " +
			"Runs in the foreground until interrupted.",
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			path := "."
			if len(args) == 1 {
				path = args[0]
			}

			eng, err := resolveEngine(cmd.Context())
			if err != nil {
				return err
			}
			defer eng.Close()

			// Initial index so the graph exists (and so the watcher's deltas have a
			// base snapshot to diff against). Render it like `atlas index` would.
			in := atlas.IndexInput{ProjectPath: path, Repo: gf.repo, EnableVectors: enableVectors}
			res, err := eng.Index(cmd.Context(), in)
			if err != nil {
				return err
			}
			if err := renderJSON(cmd.OutOrStdout(), res); err != nil {
				return err
			}

			w, err := watch.New(eng, path, watch.Options{
				Repo:          gf.repo,
				Debounce:      time.Duration(debounceMS) * time.Millisecond,
				Logger:        cmd.OutOrStdout(),
				EnableVectors: enableVectors,
			})
			if err != nil {
				return err
			}

			// Own signal handling: the root uses Execute() (not ExecuteContext), so
			// cmd.Context() is never cancelled. Translate SIGINT/SIGTERM into a clean
			// Stop so the foreground daemon shuts down gracefully on Ctrl-C.
			ctx, stop := signal.NotifyContext(cmd.Context(), os.Interrupt, syscall.SIGTERM)
			defer stop()

			if err := w.Start(ctx); err != nil {
				return err
			}
			cmd.Printf("atlas watch: live on %s (Ctrl-C to stop)\n", res.RepoFullName)

			<-ctx.Done()
			w.Stop()
			cmd.Println("atlas watch: stopped")
			return nil
		},
	}
	f := cmd.Flags()
	f.IntVar(&debounceMS, "debounce-ms", int(watch.DefaultDebounce/time.Millisecond), "coalesce a burst of edits within this window (ms) into one update")
	f.BoolVar(&enableVectors, "enable-vectors", false, "keep the optional embedding layer fresh on each update")
	return cmd
}

// startBackgroundWatch starts a Watcher bound to eng for the long-lived `serve`
// and `mcp` processes so the SAME warm process that answers an agent's queries
// also keeps the graph fresh in the background. It is opt-in (the caller gates
// on a --watch flag) to avoid surprising existing users. The watcher runs until
// ctx is cancelled; the returned stop func blocks until it has fully exited and
// must be deferred by the caller. Watch-setup failures are reported to the
// command's stderr and degrade gracefully — the server still serves, just
// without auto-refresh — so a watcher problem never takes the API down.
func startBackgroundWatch(ctx context.Context, cmd *cobra.Command, eng atlas.Engine, path string) func() {
	if path == "" {
		path = "."
	}
	w, err := watch.New(eng, path, watch.Options{
		Repo:   gf.repo,
		Logger: cmd.ErrOrStderr(),
	})
	if err != nil {
		cmd.PrintErrf("atlas: --watch disabled: %v\n", err)
		return func() {}
	}
	if err := w.Start(ctx); err != nil {
		cmd.PrintErrf("atlas: --watch disabled: %v\n", err)
		return func() {}
	}
	cmd.PrintErrf("atlas: background watch live on %s (auto-refresh on change)\n", path)
	return w.Stop
}
