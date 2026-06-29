// Package runtimecfg centralizes process-wide Go runtime tuning (the soft memory
// limit and GC aggressiveness) so every Atlas entrypoint applies it the same way.
//
// Motivation: the indexer had zero GC/memory tuning. Under heavy churn the
// runtime spends measurable CPU in madvise/usleep returning freed pages to the
// OS, and a long-lived warm process (atlas watch / serve --watch / mcp --watch)
// can hold a high steady RSS because the default GOGC=100 lets the heap double
// before each collection. This package lets an operator set a soft memory limit
// (debug.SetMemoryLimit) and/or a GC percentage (debug.SetGCPercent) via env,
// and lets the warm daemons opt into a slightly lower GOGC default to trade a
// little CPU for lower steady-state memory.
//
// Design rules:
//   - Respect what Go already did. If the user exported GOMEMLIMIT/GOGC, Go has
//     already applied them at startup; we only override when the ATLAS_* knobs
//     are explicitly set.
//   - Never hard-cap by default. A too-low memory limit causes GC thrash or OOM,
//     so the only default we apply is for the warm daemons, and only the gentle
//     GOGC nudge — never a memory limit.
//   - The one-shot CLI (atlas index) must not lower GOGC; a more aggressive GC
//     would slow the index. Apply (called by every entrypoint) therefore only
//     honors explicit ATLAS_* knobs; the warm-daemon default lives in
//     ApplyWarmDaemonDefaults, which the long-lived paths call in addition.
package runtimecfg

import (
	"fmt"
	"io"
	"math"
	"os"
	"runtime/debug"
	"strconv"
	"strings"
)

// Env var names. Kept here so they are the single source of truth.
const (
	// EnvMemoryLimit sets a soft heap limit, forwarded to debug.SetMemoryLimit.
	// Accepts a plain byte count ("800000000") or a size with a unit suffix
	// ("512MiB", "1GiB", "512MB", "1GB", "512M", "1G"). Binary (MiB/GiB/Ki) and
	// decimal (MB/GB/K) suffixes are both accepted; bare "M"/"G"/"K" are treated
	// as binary (1024-based), matching GOMEMLIMIT's own grammar.
	EnvMemoryLimit = "ATLAS_MEMORY_LIMIT"

	// EnvGOGC sets the GC target percentage, forwarded to debug.SetGCPercent.
	// A lower value collects more often (lower RSS, more CPU); -1 disables GC.
	EnvGOGC = "ATLAS_GOGC"
)

// warmDaemonGOGC is the soft default GC percentage for the long-lived warm
// processes. It is below the runtime default of 100 so a daemon idling between
// bursts of file-change re-indexing keeps a lower steady RSS, at the cost of a
// little extra CPU. It is intentionally conservative: too low would thrash.
const warmDaemonGOGC = 75

// Apply reads the ATLAS_* runtime knobs and applies any that are explicitly set.
// It is a no-op when neither is set (so a process started with only GOMEMLIMIT/
// GOGC in the environment keeps exactly what Go already applied). Safe to call
// once at process start. logw receives a single human-readable line per knob
// applied; pass nil (or io.Discard) to stay silent.
//
// Apply never lowers GOGC on its own — it only honors an explicit ATLAS_GOGC.
// The warm-daemon GOGC nudge lives in ApplyWarmDaemonDefaults so the one-shot
// `atlas index` path (which calls only Apply) is never slowed by a more
// aggressive GC.
func Apply(logw io.Writer) {
	apply(logw, os.Getenv)
}

// apply is the testable core: getenv is injected so tests can drive it without
// mutating the real process environment.
func apply(logw io.Writer, getenv func(string) string) {
	if raw := strings.TrimSpace(getenv(EnvMemoryLimit)); raw != "" {
		bytes, err := ParseBytes(raw)
		switch {
		case err != nil:
			logf(logw, "atlas: ignoring %s=%q: %v\n", EnvMemoryLimit, raw, err)
		case bytes <= 0:
			logf(logw, "atlas: ignoring %s=%q: must be > 0\n", EnvMemoryLimit, raw)
		default:
			debug.SetMemoryLimit(bytes)
			logf(logw, "atlas: soft memory limit set to %s (%d bytes) from %s\n",
				humanBytes(bytes), bytes, EnvMemoryLimit)
		}
	}

	if raw := strings.TrimSpace(getenv(EnvGOGC)); raw != "" {
		pct, err := strconv.Atoi(raw)
		if err != nil {
			logf(logw, "atlas: ignoring %s=%q: not an integer\n", EnvGOGC, raw)
		} else {
			debug.SetGCPercent(pct)
			logf(logw, "atlas: GOGC set to %d from %s\n", pct, EnvGOGC)
		}
	}
}

// ApplyWarmDaemonDefaults applies the warm-daemon GC nudge for a long-lived
// process (atlas watch, serve --watch, mcp --watch). It is additive to Apply and
// must be called AFTER it. It only lowers GOGC when the operator has expressed
// no preference at all — i.e. neither ATLAS_GOGC nor the standard GOGC env is
// set, and ATLAS_MEMORY_LIMIT / GOMEMLIMIT are also unset (a memory limit is a
// stronger, explicit RSS control, so we don't second-guess it with a GOGC
// change). This keeps the default conservative and fully opt-out: any explicit
// knob wins. Safe to call once at daemon start.
func ApplyWarmDaemonDefaults(logw io.Writer) {
	applyWarmDaemonDefaults(logw, os.Getenv)
}

func applyWarmDaemonDefaults(logw io.Writer, getenv func(string) string) {
	// Any explicit operator preference disables the soft default.
	if strings.TrimSpace(getenv(EnvGOGC)) != "" ||
		strings.TrimSpace(getenv("GOGC")) != "" ||
		strings.TrimSpace(getenv(EnvMemoryLimit)) != "" ||
		strings.TrimSpace(getenv("GOMEMLIMIT")) != "" {
		return
	}
	debug.SetGCPercent(warmDaemonGOGC)
	logf(logw, "atlas: warm-daemon GC tuning: GOGC=%d (lower steady memory; set %s/%s to override)\n",
		warmDaemonGOGC, EnvGOGC, EnvMemoryLimit)
}

// ParseBytes parses a byte size that is either a plain integer ("800000000") or
// an integer/decimal with a unit suffix. Supported suffixes (case-insensitive):
//
//	B
//	KiB MiB GiB TiB   (binary, 1024-based)
//	KB  MB  GB  TB    (decimal, 1000-based)
//	K   M   G   T     (binary, 1024-based — matches GOMEMLIMIT)
//
// Whitespace around the value and between the number and suffix is tolerated.
// It returns an error for empty, malformed, negative, or overflowing input.
func ParseBytes(s string) (int64, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, fmt.Errorf("empty size")
	}

	// Split the leading numeric run (digits and at most one dot) from the suffix.
	i := 0
	for i < len(s) && (s[i] == '.' || (s[i] >= '0' && s[i] <= '9')) {
		i++
	}
	numPart := s[:i]
	unitPart := strings.TrimSpace(s[i:])
	if numPart == "" {
		return 0, fmt.Errorf("no numeric value in %q", s)
	}

	num, err := strconv.ParseFloat(numPart, 64)
	if err != nil {
		return 0, fmt.Errorf("invalid number %q: %w", numPart, err)
	}
	if num < 0 {
		return 0, fmt.Errorf("size must be non-negative, got %q", s)
	}

	var mult float64 = 1
	switch strings.ToLower(unitPart) {
	case "", "b":
		mult = 1
	case "k", "kib":
		mult = 1 << 10
	case "m", "mib":
		mult = 1 << 20
	case "g", "gib":
		mult = 1 << 30
	case "t", "tib":
		mult = 1 << 40
	case "kb":
		mult = 1e3
	case "mb":
		mult = 1e6
	case "gb":
		mult = 1e9
	case "tb":
		mult = 1e12
	default:
		return 0, fmt.Errorf("unknown size unit %q", unitPart)
	}

	total := num * mult
	if total > math.MaxInt64 {
		return 0, fmt.Errorf("size %q overflows int64", s)
	}
	return int64(total), nil
}

// humanBytes renders a byte count with the largest exact-ish binary unit for
// readable startup logs. It is best-effort cosmetic; the exact byte count is
// always logged alongside it.
func humanBytes(b int64) string {
	const unit = 1024
	if b < unit {
		return fmt.Sprintf("%dB", b)
	}
	div, exp := int64(unit), 0
	for n := b / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f%ciB", float64(b)/float64(div), "KMGT"[exp])
}

func logf(w io.Writer, format string, args ...any) {
	if w == nil {
		return
	}
	fmt.Fprintf(w, format, args...)
}
