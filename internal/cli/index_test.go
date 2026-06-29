package cli

import (
	"bytes"
	"errors"
	"strings"
	"testing"

	"github.com/dominic097/atlas/pkg/atlas"
)

func TestIndexProgressEnabledProtectsMachineReadableOutput(t *testing.T) {
	cases := []struct {
		name   string
		format string
		json   bool
		flag   bool
		want   bool
	}{
		{name: "default human", flag: true, want: true},
		{name: "plain human", format: "plain", flag: true, want: true},
		{name: "json shorthand", json: true, flag: true, want: false},
		{name: "json format", format: "json", flag: true, want: false},
		{name: "compact format", format: "compact", flag: true, want: false},
		{name: "ndjson format", format: "ndjson", flag: true, want: false},
		{name: "explicit off", flag: false, want: false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			prevFormat, prevJSON := gf.format, gf.json
			gf.format, gf.json = tc.format, tc.json
			defer func() { gf.format, gf.json = prevFormat, prevJSON }()

			if got := indexProgressEnabled(tc.flag); got != tc.want {
				t.Fatalf("indexProgressEnabled()=%v, want %v", got, tc.want)
			}
		})
	}
}

func TestFormatIndexTimingsSorted(t *testing.T) {
	got := formatIndexTimings(map[string]int64{"persist": 7, "parse": 3, "walk": 1})
	want := "parse=3ms persist=7ms walk=1ms"
	if got != want {
		t.Fatalf("formatIndexTimings()=%q, want %q", got, want)
	}
}

func TestStartIndexProgressWritesStats(t *testing.T) {
	prevDB := gf.db
	gf.db = "sqlite:///tmp/atlas-test.db"
	defer func() { gf.db = prevDB }()

	var buf bytes.Buffer
	finish := startIndexProgress(&buf, true, ".")
	finish(&atlas.IndexResult{
		RepoFullName: "repo",
		Mode:         "delta",
		IndexedFiles: 12,
		Symbols:      34,
		Edges:        56,
		Routes:       7,
		DurationMS:   89,
		TimingsMS:    map[string]int64{"parse": 2},
	}, nil)

	out := buf.String()
	for _, want := range []string{
		"atlas index: starting path=. db=sqlite:///tmp/atlas-test.db",
		"atlas index: done repo=repo mode=delta files=12 symbols=34 edges=56 routes=7 duration=89ms",
		"atlas index: timings parse=2ms",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("progress output missing %q:\n%s", want, out)
		}
	}
}

func TestStartIndexProgressWritesFailure(t *testing.T) {
	var buf bytes.Buffer
	finish := startIndexProgress(&buf, true, "svc")
	finish(nil, errors.New("boom"))

	out := buf.String()
	if !strings.Contains(out, "atlas index: starting path=svc") {
		t.Fatalf("progress output missing start line:\n%s", out)
	}
	if !strings.Contains(out, "atlas index: failed after") || !strings.Contains(out, "boom") {
		t.Fatalf("progress output missing failure line:\n%s", out)
	}
}
