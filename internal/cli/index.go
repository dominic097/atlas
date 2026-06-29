package cli

import (
	"fmt"
	"io"
	"os"
	"runtime"
	"runtime/pprof"
	"sort"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/dominic097/atlas/internal/index"
	"github.com/dominic097/atlas/pkg/atlas"
)

func newIndexCmd() *cobra.Command {
	var (
		in         atlas.IndexInput
		cpuProfile string
		memProfile string
		progress   bool
	)
	cmd := &cobra.Command{
		Use:   "index [PATH]",
		Short: "Index a repo: parse symbols/edges/routes, persist graph + lexical index",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 1 {
				in.ProjectPath = args[0]
			}
			applyIndexDefaults(&in)

			// Optional CPU profiling: wrap the whole index run. StartCPUProfile begins
			// sampling now; stopCPU (deferred) writes and closes the profile. A profiling
			// setup error fails the command early so a bad --cpuprofile path is loud, not
			// silently ignored.
			stopCPU, err := startCPUProfile(cpuProfile)
			if err != nil {
				return err
			}
			defer stopCPU()

			pc := &index.ProgressCounters{}
			finishProgress := startIndexProgress(cmd.ErrOrStderr(), indexProgressEnabled(progress), in.ProjectPath, pc)
			eng, err := resolveEngine(cmd.Context())
			if err != nil {
				finishProgress(nil, err)
				return err
			}
			defer eng.Close()
			res, err := eng.Index(index.WithProgress(cmd.Context(), pc), in)
			if err != nil {
				finishProgress(res, err)
				return err
			}

			// Optional heap profile: written AFTER the run so it reflects the post-index
			// live heap. A GC first makes the "inuse" space accurate.
			if err := writeMemProfile(memProfile); err != nil {
				finishProgress(res, err)
				return err
			}
			finishProgress(res, nil)
			return renderJSON(cmd.OutOrStdout(), res)
		},
	}
	f := cmd.Flags()
	f.StringVar(&in.Ref, "ref", "", "commit or branch to index (default: working tree)")
	f.StringVar(&in.Base, "base", "", "explicit delta base commit")
	f.BoolVar(&in.Reindex, "reindex", false, "force full reindex instead of delta")
	f.BoolVar(&in.EnableVectors, "enable-vectors", false, "run the optional embedding pass")
	f.StringVar(&cpuProfile, "cpuprofile", "", "write a runtime/pprof CPU profile of the index run to this path")
	f.StringVar(&memProfile, "memprofile", "", "write a runtime/pprof heap profile after the index run to this path")
	f.BoolVar(&progress, "progress", true, "print start, periodic progress, and completion stats to stderr for human output")
	return cmd
}

func indexProgressEnabled(flag bool) bool {
	if !flag || gf.json {
		return false
	}
	switch strings.ToLower(strings.TrimSpace(gf.format)) {
	case "json", "compact", "ndjson":
		return false
	default:
		return true
	}
}

func startIndexProgress(w io.Writer, enabled bool, projectPath string, pc *index.ProgressCounters) func(*atlas.IndexResult, error) {
	if !enabled || w == nil {
		return func(*atlas.IndexResult, error) {}
	}
	if strings.TrimSpace(projectPath) == "" {
		projectPath = "."
	}
	start := time.Now()
	done := make(chan struct{})
	stopped := make(chan struct{})
	fmt.Fprintf(w, "atlas index: starting path=%s db=%s\n", projectPath, gf.db)
	go func() {
		defer close(stopped)
		ticker := time.NewTicker(2 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				fmt.Fprintln(w, formatIndexProgress(pc.Snapshot(), time.Since(start)))
			case <-done:
				return
			}
		}
	}()
	return func(res *atlas.IndexResult, err error) {
		close(done)
		<-stopped
		elapsed := roundIndexDuration(time.Since(start))
		if err != nil {
			fmt.Fprintf(w, "atlas index: failed after %s: %v\n", elapsed, err)
			return
		}
		if res == nil {
			fmt.Fprintf(w, "atlas index: done in %s\n", elapsed)
			return
		}
		// Segmented (workspace) run: print the per-repo breakdown above the aggregate.
		if len(res.Repos) > 0 {
			ok, failed := 0, 0
			for _, r := range res.Repos {
				if r.Error != "" {
					failed++
				} else {
					ok++
				}
			}
			fmt.Fprintf(w, "atlas index: segmented %d repos (%d ok, %d failed)\n", len(res.Repos), ok, failed)
			for _, r := range res.Repos {
				if r.Error != "" {
					fmt.Fprintf(w, "atlas index:   %-30s FAILED: %s\n", r.Repo, r.Error)
					continue
				}
				fmt.Fprintf(w, "atlas index:   %-30s files=%d symbols=%d edges=%d routes=%d\n",
					r.Repo, r.Files, r.Symbols, r.Edges, r.Routes)
			}
		}
		fmt.Fprintf(w, "atlas index: done repo=%s mode=%s files=%d symbols=%d edges=%d routes=%d duration=%dms\n",
			res.RepoFullName, res.Mode, res.IndexedFiles, res.Symbols, res.Edges, res.Routes, res.DurationMS)
		if timings := formatIndexTimings(res.TimingsMS); timings != "" {
			fmt.Fprintf(w, "atlas index: timings %s\n", timings)
		}
	}
}

// formatIndexProgress renders one live progress line from a counters snapshot —
// the current repo (in a segmented run), phase, and parsed/total file + symbol
// counts — so the user sees real movement instead of a bare elapsed timer.
func formatIndexProgress(s index.ProgressSnapshot, elapsed time.Duration) string {
	var b strings.Builder
	b.WriteString("atlas index: ")
	if s.RepoTotal > 1 {
		fmt.Fprintf(&b, "[%d/%d] %s ", s.RepoIndex, s.RepoTotal, s.RepoName)
	}
	switch s.Phase {
	case "parse":
		if s.FilesTotal > 0 {
			fmt.Fprintf(&b, "parsing %d/%d files, %d symbols", s.FilesParsed, s.FilesTotal, s.Symbols)
		} else {
			b.WriteString("parsing files")
		}
	case "go_types":
		fmt.Fprintf(&b, "analyzing types (%d files, %d symbols)", s.FilesParsed, s.Symbols)
	case "persist":
		fmt.Fprintf(&b, "persisting %d symbols", s.Symbols)
	case "discover", "":
		b.WriteString("scanning files")
	case "done":
		b.WriteString("finishing up")
	default:
		b.WriteString(s.Phase)
	}
	fmt.Fprintf(&b, " elapsed=%s", roundIndexDuration(elapsed))
	return b.String()
}

func roundIndexDuration(d time.Duration) time.Duration {
	if d < time.Second {
		return d.Round(time.Millisecond)
	}
	return d.Round(time.Second)
}

func formatIndexTimings(timings map[string]int64) string {
	if len(timings) == 0 {
		return ""
	}
	keys := make([]string, 0, len(timings))
	for k := range timings {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(keys))
	for _, k := range keys {
		parts = append(parts, fmt.Sprintf("%s=%dms", k, timings[k]))
	}
	return strings.Join(parts, " ")
}

// startCPUProfile begins CPU profiling to path and returns a stop function that
// writes+closes the profile. When path is empty it is a no-op (the returned stop
// is safe to call). pprof allows only one active CPU profile process-wide, which
// is fine for the single-shot `atlas index` command.
func startCPUProfile(path string) (func(), error) {
	if path == "" {
		return func() {}, nil
	}
	fh, err := os.Create(path)
	if err != nil {
		return nil, fmt.Errorf("index: create cpu profile %q: %w", path, err)
	}
	if err := pprof.StartCPUProfile(fh); err != nil {
		_ = fh.Close()
		return nil, fmt.Errorf("index: start cpu profile: %w", err)
	}
	return func() {
		pprof.StopCPUProfile()
		_ = fh.Close()
	}, nil
}

// writeMemProfile writes a heap profile to path (no-op when empty). It runs a GC
// first so the heap profile's inuse_space reflects reachable memory, matching the
// standard runtime/pprof heap-profile recipe.
func writeMemProfile(path string) error {
	if path == "" {
		return nil
	}
	fh, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("index: create mem profile %q: %w", path, err)
	}
	defer fh.Close()
	runtime.GC()
	if err := pprof.WriteHeapProfile(fh); err != nil {
		return fmt.Errorf("index: write heap profile: %w", err)
	}
	return nil
}

func applyIndexDefaults(in *atlas.IndexInput) {
	if in == nil {
		return
	}
	if in.Repo == "" {
		in.Repo = gf.repo
	}
}
