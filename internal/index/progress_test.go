package index

import (
	"context"
	"testing"
)

func TestProgressCountersNilSafe(t *testing.T) {
	var pc *ProgressCounters
	// None of these may panic on a nil receiver.
	pc.AddParsed(3)
	pc.SetFilesTotal(10)
	pc.SetPhase("parse")
	pc.SetRepo("x", 1, 2)
	if s := pc.Snapshot(); s != (ProgressSnapshot{}) {
		t.Fatalf("nil Snapshot = %+v, want zero value", s)
	}
	if ProgressFromContext(context.Background()) != nil {
		t.Fatal("ProgressFromContext on a bare context should be nil")
	}
}

func TestProgressCountersTrackAndReset(t *testing.T) {
	pc := &ProgressCounters{}
	ctx := WithProgress(context.Background(), pc)
	if ProgressFromContext(ctx) != pc {
		t.Fatal("WithProgress/ProgressFromContext roundtrip failed")
	}

	pc.SetRepo("svc-a", 1, 3)
	pc.SetFilesTotal(5)
	pc.SetPhase("parse")
	pc.AddParsed(2)
	pc.AddParsed(4)

	s := pc.Snapshot()
	if s.RepoName != "svc-a" || s.RepoIndex != 1 || s.RepoTotal != 3 {
		t.Errorf("repo fields = %+v", s)
	}
	if s.FilesParsed != 2 || s.FilesTotal != 5 || s.Symbols != 6 || s.Phase != "parse" {
		t.Errorf("counters = %+v, want parsed=2 total=5 symbols=6 phase=parse", s)
	}

	// Switching repos resets the per-repo counters (so a segmented run shows the
	// CURRENT repo's progress, not a monotonic blur).
	pc.SetRepo("svc-b", 2, 3)
	s = pc.Snapshot()
	if s.FilesParsed != 0 || s.Symbols != 0 || s.FilesTotal != 0 {
		t.Errorf("SetRepo should reset per-repo counters, got %+v", s)
	}
	if s.RepoName != "svc-b" || s.RepoIndex != 2 || s.RepoTotal != 3 {
		t.Errorf("repo switch = %+v", s)
	}
}
