package runtimecfg

import (
	"bytes"
	"math"
	"runtime/debug"
	"strings"
	"testing"
)

func TestParseBytes(t *testing.T) {
	cases := []struct {
		in   string
		want int64
		err  bool
	}{
		// Raw byte counts.
		{"800000000", 800000000, false},
		{"0", 0, false},
		{" 1024 ", 1024, false},
		{"512B", 512, false},
		// Binary suffixes.
		{"512MiB", 512 << 20, false},
		{"1GiB", 1 << 30, false},
		{"2KiB", 2 << 10, false},
		{"1TiB", 1 << 40, false},
		// Bare single-letter suffixes are binary (GOMEMLIMIT grammar).
		{"512M", 512 << 20, false},
		{"1G", 1 << 30, false},
		{"4K", 4 << 10, false},
		// Decimal suffixes.
		{"512MB", 512_000_000, false},
		{"1GB", 1_000_000_000, false},
		{"2KB", 2_000, false},
		// Case-insensitive + spacing.
		{"1gib", 1 << 30, false},
		{"512 mib", 512 << 20, false},
		// Fractional.
		{"1.5GiB", int64(1.5 * (1 << 30)), false},
		{"0.5MiB", 512 << 10, false},
		// Errors.
		{"", 0, true},
		{"   ", 0, true},
		{"abc", 0, true},
		{"MiB", 0, true},
		{"512XB", 0, true},
		{"-1", 0, true},
		{"-5MiB", 0, true},
	}
	for _, c := range cases {
		got, err := ParseBytes(c.in)
		if c.err {
			if err == nil {
				t.Errorf("ParseBytes(%q) = %d, want error", c.in, got)
			}
			continue
		}
		if err != nil {
			t.Errorf("ParseBytes(%q) unexpected error: %v", c.in, err)
			continue
		}
		if got != c.want {
			t.Errorf("ParseBytes(%q) = %d, want %d", c.in, got, c.want)
		}
	}
}

func TestParseBytesOverflow(t *testing.T) {
	if _, err := ParseBytes("99999999999TiB"); err == nil {
		t.Fatal("expected overflow error for absurdly large size")
	}
}

// setMemoryLimitForTest sets a known limit and restores it after the test,
// returning the prior limit so callers can assert against it. debug.SetMemoryLimit
// returns the PREVIOUS limit, which is how we observe what Apply did.
func restoreMemLimit(t *testing.T) {
	t.Helper()
	// Read current by setting to a sentinel that returns prior, then restore it.
	prior := debug.SetMemoryLimit(-1) // -1 reads without changing.
	t.Cleanup(func() { debug.SetMemoryLimit(prior) })
}

func restoreGCPercent(t *testing.T) {
	t.Helper()
	prior := debug.SetGCPercent(100)
	t.Cleanup(func() { debug.SetGCPercent(prior) })
}

func TestApplySetsMemoryLimit(t *testing.T) {
	restoreMemLimit(t)
	restoreGCPercent(t)

	env := map[string]string{EnvMemoryLimit: "512MiB"}
	getenv := func(k string) string { return env[k] }

	var buf bytes.Buffer
	apply(&buf, getenv)

	// debug.SetMemoryLimit(-1) reads the CURRENT limit without changing it.
	// After Apply with 512MiB, the current limit must be exactly 512MiB.
	cur := debug.SetMemoryLimit(-1)
	want := int64(512 << 20)
	if cur != want {
		t.Fatalf("memory limit after Apply = %d, want %d", cur, want)
	}
	if !strings.Contains(buf.String(), "soft memory limit set to") {
		t.Errorf("expected a log line about the memory limit, got: %q", buf.String())
	}
}

func TestApplyRawByteMemoryLimit(t *testing.T) {
	restoreMemLimit(t)

	env := map[string]string{EnvMemoryLimit: "800000000"}
	apply(nil, func(k string) string { return env[k] })

	cur := debug.SetMemoryLimit(-1)
	if cur != 800000000 {
		t.Fatalf("memory limit after Apply = %d, want 800000000", cur)
	}
}

func TestApplySetsGOGC(t *testing.T) {
	restoreGCPercent(t)

	env := map[string]string{EnvGOGC: "60"}
	var buf bytes.Buffer
	apply(&buf, func(k string) string { return env[k] })

	// SetGCPercent returns the prior value; calling it again with the same value
	// must report 60, proving Apply installed it.
	prior := debug.SetGCPercent(60)
	if prior != 60 {
		t.Fatalf("GOGC after Apply = %d, want 60", prior)
	}
	if !strings.Contains(buf.String(), "GOGC set to 60") {
		t.Errorf("expected a GOGC log line, got: %q", buf.String())
	}
}

func TestApplyNoOpWhenUnset(t *testing.T) {
	restoreMemLimit(t)
	restoreGCPercent(t)

	// Capture the limit before Apply with an empty environment.
	before := debug.SetMemoryLimit(-1)
	beforeGC := debug.SetGCPercent(100)
	debug.SetGCPercent(beforeGC) // restore what we just read

	var buf bytes.Buffer
	apply(&buf, func(string) string { return "" })

	after := debug.SetMemoryLimit(-1)
	if after != before {
		t.Errorf("memory limit changed by no-op Apply: before=%d after=%d", before, after)
	}
	if buf.Len() != 0 {
		t.Errorf("expected no log output for unset env, got: %q", buf.String())
	}
}

func TestApplyIgnoresInvalidValues(t *testing.T) {
	restoreMemLimit(t)
	restoreGCPercent(t)

	before := debug.SetMemoryLimit(-1)

	env := map[string]string{
		EnvMemoryLimit: "not-a-size",
		EnvGOGC:        "not-an-int",
	}
	var buf bytes.Buffer
	apply(&buf, func(k string) string { return env[k] })

	after := debug.SetMemoryLimit(-1)
	if after != before {
		t.Errorf("invalid memory limit should not change the limit: before=%d after=%d", before, after)
	}
	out := buf.String()
	if !strings.Contains(out, "ignoring "+EnvMemoryLimit) {
		t.Errorf("expected an ignore line for the bad memory limit, got: %q", out)
	}
	if !strings.Contains(out, "ignoring "+EnvGOGC) {
		t.Errorf("expected an ignore line for the bad GOGC, got: %q", out)
	}
}

func TestApplyIgnoresNonPositiveMemoryLimit(t *testing.T) {
	restoreMemLimit(t)

	before := debug.SetMemoryLimit(-1)
	env := map[string]string{EnvMemoryLimit: "0"}
	var buf bytes.Buffer
	apply(&buf, func(k string) string { return env[k] })

	after := debug.SetMemoryLimit(-1)
	if after != before {
		t.Errorf("zero memory limit should be ignored: before=%d after=%d", before, after)
	}
	if !strings.Contains(buf.String(), "must be > 0") {
		t.Errorf("expected a >0 ignore line, got: %q", buf.String())
	}
}

func TestApplyWarmDaemonDefaults(t *testing.T) {
	restoreGCPercent(t)

	// No operator knobs set: the warm-daemon nudge must lower GOGC to the default.
	var buf bytes.Buffer
	applyWarmDaemonDefaults(&buf, func(string) string { return "" })

	prior := debug.SetGCPercent(100)
	if prior != warmDaemonGOGC {
		t.Fatalf("warm-daemon GOGC = %d, want %d", prior, warmDaemonGOGC)
	}
	if !strings.Contains(buf.String(), "warm-daemon GC tuning") {
		t.Errorf("expected a warm-daemon log line, got: %q", buf.String())
	}
}

func TestApplyWarmDaemonDefaultsRespectsExplicitKnobs(t *testing.T) {
	for _, key := range []string{EnvGOGC, "GOGC", EnvMemoryLimit, "GOMEMLIMIT"} {
		t.Run(key, func(t *testing.T) {
			restoreGCPercent(t)

			// Pin a known GOGC, then run the warm-daemon defaults with the knob set.
			debug.SetGCPercent(100)
			env := map[string]string{key: "1GiB"} // any non-empty value
			var buf bytes.Buffer
			applyWarmDaemonDefaults(&buf, func(k string) string { return env[k] })

			prior := debug.SetGCPercent(100)
			if prior != 100 {
				t.Fatalf("warm-daemon default should not override explicit %s; GOGC=%d", key, prior)
			}
			if buf.Len() != 0 {
				t.Errorf("expected no warm-daemon log when %s is set, got: %q", key, buf.String())
			}
		})
	}
}

func TestHumanBytes(t *testing.T) {
	cases := map[int64]string{
		512:        "512B",
		1 << 10:    "1.0KiB",
		512 << 20:  "512.0MiB",
		1 << 30:    "1.0GiB",
		math.MaxInt32: "2.0GiB",
	}
	for in, want := range cases {
		if got := humanBytes(in); got != want {
			t.Errorf("humanBytes(%d) = %q, want %q", in, got, want)
		}
	}
}
