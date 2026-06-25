package install

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestMergeJSONServerIdempotentPreservesOthers is the keystone test: merging the
// atlas entry into a config that already has another server must (a) preserve
// that server, (b) add atlas, and (c) be idempotent — a second merge over the
// first merge's output yields byte-identical output with no duplication.
func TestMergeJSONServerIdempotentPreservesOthers(t *testing.T) {
	pre := []byte(`{
  "mcpServers": {
    "other": {"command": "other-cmd", "args": ["x", "y"]},
    "settingsPreserved": true
  },
  "topLevelKept": 42
}`)

	spec := ServerSpec{Command: "atlas", Args: DefaultArgs("")}

	first, err := MergeJSONServer(pre, "mcpServers", spec)
	if err != nil {
		t.Fatalf("first merge: %v", err)
	}

	// Pre-existing server + top-level keys preserved, atlas added.
	root := decode(t, first)
	servers, ok := root["mcpServers"].(map[string]any)
	if !ok {
		t.Fatalf("mcpServers missing or wrong type: %T", root["mcpServers"])
	}
	if _, ok := servers["other"]; !ok {
		t.Errorf("pre-existing server 'other' was dropped")
	}
	if _, ok := servers["settingsPreserved"]; !ok {
		t.Errorf("non-server key under mcpServers was dropped")
	}
	if _, ok := root["topLevelKept"]; !ok {
		t.Errorf("top-level key 'topLevelKept' was dropped")
	}
	atlas, ok := servers["atlas"].(map[string]any)
	if !ok {
		t.Fatalf("atlas entry missing or wrong type: %T", servers["atlas"])
	}
	if atlas["command"] != "atlas" {
		t.Errorf("atlas.command = %v, want atlas", atlas["command"])
	}

	// Idempotent: merging again over the output is byte-identical.
	second, err := MergeJSONServer(first, "mcpServers", spec)
	if err != nil {
		t.Fatalf("second merge: %v", err)
	}
	if string(first) != string(second) {
		t.Errorf("merge not idempotent:\nfirst:\n%s\nsecond:\n%s", first, second)
	}

	// And exactly one atlas entry, exactly two servers total (other + atlas).
	root2 := decode(t, second)
	servers2 := root2["mcpServers"].(map[string]any)
	if len(servers2) != 3 { // other, settingsPreserved, atlas
		t.Errorf("expected 3 keys under mcpServers, got %d: %v", len(servers2), keys(servers2))
	}
}

func TestMergeJSONServerEmptyInput(t *testing.T) {
	out, err := MergeJSONServer(nil, "servers", ServerSpec{Command: "atlas", Args: DefaultArgs("")})
	if err != nil {
		t.Fatalf("merge empty: %v", err)
	}
	root := decode(t, out)
	servers, ok := root["servers"].(map[string]any)
	if !ok {
		t.Fatalf("servers key not created for empty input")
	}
	if _, ok := servers["atlas"]; !ok {
		t.Errorf("atlas entry not created for empty input")
	}
}

func TestMergeJSONServerThreadsDB(t *testing.T) {
	out, err := MergeJSONServer(nil, "mcpServers", ServerSpec{Command: "atlas", Args: DefaultArgs("sqlite:///tmp/x.db")})
	if err != nil {
		t.Fatalf("merge: %v", err)
	}
	if !strings.Contains(string(out), "--db") || !strings.Contains(string(out), "sqlite:///tmp/x.db") {
		t.Errorf("db not threaded into args:\n%s", out)
	}
}

func TestMergeTOMLServerIdempotentPreservesOthers(t *testing.T) {
	pre := []byte(`model = "gpt-5"

[mcp_servers.github]
command = "gh-mcp"
args = ["serve"]
`)
	spec := ServerSpec{Command: "atlas", Args: DefaultArgs("")}

	first, err := MergeTOMLServer(pre, spec)
	if err != nil {
		t.Fatalf("first toml merge: %v", err)
	}
	s := string(first)
	if !strings.Contains(s, `model = "gpt-5"`) {
		t.Errorf("top-level toml key dropped:\n%s", s)
	}
	if !strings.Contains(s, "[mcp_servers.github]") {
		t.Errorf("pre-existing toml server dropped:\n%s", s)
	}
	if !strings.Contains(s, "[mcp_servers.atlas]") {
		t.Errorf("atlas toml block not added:\n%s", s)
	}

	// Idempotent + no duplicate atlas block.
	second, err := MergeTOMLServer(first, spec)
	if err != nil {
		t.Fatalf("second toml merge: %v", err)
	}
	if string(first) != string(second) {
		t.Errorf("toml merge not idempotent:\nfirst:\n%s\nsecond:\n%s", first, second)
	}
	if n := strings.Count(string(second), "[mcp_servers.atlas]"); n != 1 {
		t.Errorf("expected exactly 1 atlas toml block, got %d", n)
	}
}

func TestInstallSkillWritesAndIsIdempotent(t *testing.T) {
	dir := t.TempDir()
	cfg := filepath.Join(dir, "claude.json")

	opts := SkillOptions{Agent: "claude", ConfigPath: cfg, Home: dir}
	if _, err := InstallSkill(opts); err != nil {
		t.Fatalf("first install: %v", err)
	}
	b1, err := os.ReadFile(cfg)
	if err != nil {
		t.Fatalf("read after first install: %v", err)
	}
	if _, err := InstallSkill(opts); err != nil {
		t.Fatalf("second install: %v", err)
	}
	b2, err := os.ReadFile(cfg)
	if err != nil {
		t.Fatalf("read after second install: %v", err)
	}
	if string(b1) != string(b2) {
		t.Errorf("install skill not idempotent on disk")
	}

	// claude skill markdown was dropped under the home base.
	md := filepath.Join(dir, ".claude", "skills", "atlas", "SKILL.md")
	if _, err := os.Stat(md); err != nil {
		t.Errorf("claude SKILL.md not written: %v", err)
	}
}

func TestInstallHookWritesExecutableAndIsIdempotent(t *testing.T) {
	dir := t.TempDir()
	// Minimal git repo: a .git dir with hooks created on demand.
	if err := os.MkdirAll(filepath.Join(dir, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}

	res, err := InstallHook(HookOptions{Type: "post-merge", Repo: dir})
	if err != nil {
		t.Fatalf("install hook: %v", err)
	}
	info, err := os.Stat(res.Path)
	if err != nil {
		t.Fatalf("hook not written: %v", err)
	}
	if info.Mode().Perm()&0o100 == 0 {
		t.Errorf("hook is not executable: mode %v", info.Mode())
	}
	body, _ := os.ReadFile(res.Path)
	if !strings.Contains(string(body), hookMarker) {
		t.Errorf("hook missing atlas marker:\n%s", body)
	}
	if !strings.Contains(string(body), "atlas index .") {
		t.Errorf("hook does not run atlas index:\n%s", body)
	}

	// Re-running over an atlas-managed hook: no backup created.
	res2, err := InstallHook(HookOptions{Type: "post-merge", Repo: dir})
	if err != nil {
		t.Fatalf("re-install hook: %v", err)
	}
	if res2.BackupPath != "" {
		t.Errorf("re-running over atlas hook should not back up, got %s", res2.BackupPath)
	}
}

func TestInstallHookBacksUpForeignHook(t *testing.T) {
	dir := t.TempDir()
	hooks := filepath.Join(dir, ".git", "hooks")
	if err := os.MkdirAll(hooks, 0o755); err != nil {
		t.Fatal(err)
	}
	foreign := filepath.Join(hooks, "pre-push")
	if err := os.WriteFile(foreign, []byte("#!/bin/sh\necho mine\n"), 0o755); err != nil {
		t.Fatal(err)
	}

	res, err := InstallHook(HookOptions{Type: "pre-push", Repo: dir})
	if err != nil {
		t.Fatalf("install hook: %v", err)
	}
	if res.BackupPath == "" {
		t.Fatalf("expected a backup for foreign hook")
	}
	backup, err := os.ReadFile(res.BackupPath)
	if err != nil {
		t.Fatalf("read backup: %v", err)
	}
	if !strings.Contains(string(backup), "echo mine") {
		t.Errorf("backup does not contain original hook body:\n%s", backup)
	}
}

func TestInstallHookThreadsDB(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	res, err := InstallHook(HookOptions{Type: "post-commit", Repo: dir, DB: "sqlite:///tmp/y.db"})
	if err != nil {
		t.Fatalf("install hook: %v", err)
	}
	body, _ := os.ReadFile(res.Path)
	if !strings.Contains(string(body), "--db 'sqlite:///tmp/y.db'") {
		t.Errorf("db not threaded into hook:\n%s", body)
	}
}

func decode(t *testing.T, b []byte) map[string]any {
	t.Helper()
	var m map[string]any
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatalf("output is not valid JSON: %v\n%s", err, b)
	}
	return m
}

func keys(m map[string]any) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}
