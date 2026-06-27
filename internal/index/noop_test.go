package index

import (
	"context"
	"os/exec"
	"testing"
)

func TestRunNoopsWhenHeadAlreadyIndexed(t *testing.T) {
	git, err := exec.LookPath("git")
	if err != nil {
		t.Skip("git not installed")
	}
	ctx := context.Background()
	repo := writeGoRepo(t)
	gitCmd(t, git, repo, "init", "-q")
	gitCmd(t, git, repo, "add", ".")
	gitCmd(t, git, repo, "-c", "user.name=Atlas Test", "-c", "user.email=atlas@example.invalid", "commit", "-q", "--no-gpg-sign", "-m", "init")

	drv := openTestStore(t)
	first, firstStats, err := Run(ctx, drv, nil, "", "svc", repo, Options{})
	if err != nil {
		t.Fatalf("first Run: %v", err)
	}
	second, secondStats, err := Run(ctx, drv, nil, "", "svc", repo, Options{})
	if err != nil {
		t.Fatalf("second Run: %v", err)
	}
	if secondStats.Mode != "noop" {
		t.Fatalf("second Run mode = %q, want noop (stats=%+v)", secondStats.Mode, secondStats)
	}
	if second.ID != first.ID {
		t.Fatalf("noop Run returned snapshot %q, want existing %q", second.ID, first.ID)
	}
	if secondStats.Files != firstStats.Files || secondStats.Symbols != firstStats.Symbols || secondStats.Edges != firstStats.Edges {
		t.Fatalf("noop counts = files:%d symbols:%d edges:%d, want files:%d symbols:%d edges:%d",
			secondStats.Files, secondStats.Symbols, secondStats.Edges,
			firstStats.Files, firstStats.Symbols, firstStats.Edges)
	}
	if secondStats.TimingsMS["parse"] != 0 || secondStats.TimingsMS["persist"] != 0 || secondStats.TimingsMS["lexical"] != 0 {
		t.Fatalf("noop should not parse/persist/build lexical; timings=%v", secondStats.TimingsMS)
	}
}

func gitCmd(t *testing.T, git, repo string, args ...string) {
	t.Helper()
	cmd := exec.Command(git, append([]string{"-C", repo}, args...)...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
}
