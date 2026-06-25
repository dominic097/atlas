// Package install implements the config-writing logic behind `atlas install`:
// registering Atlas as an MCP server in assistant configs (claude/cursor/gemini/
// codex/copilot) and writing git hooks that keep the local graph fresh.
//
// The logic here is deliberately pure where it can be: MergeJSONServer and
// renderTOMLBlock/mergeTOML operate on bytes, so they are unit-testable without
// touching the filesystem. The thin Install* wrappers do the file IO.
package install

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// ServerSpec is the MCP server entry Atlas registers in an assistant config.
type ServerSpec struct {
	Command string
	Args    []string
}

// DefaultArgs returns the args an Atlas MCP server is launched with. When db is
// non-empty it is threaded through as `mcp --db <db>` so the assistant talks to
// the same graph the user indexed.
func DefaultArgs(db string) []string {
	if strings.TrimSpace(db) == "" {
		return []string{"mcp"}
	}
	return []string{"mcp", "--db", db}
}

// Agent describes one assistant's MCP config layout: where its config file lives
// (relative to home) and which JSON key holds the server map.
type Agent struct {
	// Name is the --agent token (claude, cursor, gemini, codex, copilot, vscode).
	Name string
	// RelPath is the config path relative to the home base (e.g. ".cursor/mcp.json").
	// Empty for project-local agents that default to a repo-relative path.
	RelPath string
	// ProjectRelPath is used when the config lives in the working tree rather than
	// home (copilot/vscode -> .vscode/mcp.json).
	ProjectRelPath string
	// ServersKey is the top-level JSON key holding the server map
	// ("mcpServers" for most, "servers" for vscode/copilot).
	ServersKey string
	// TOML marks codex, whose config is TOML rather than JSON.
	TOML bool
}

// agents is the registry of supported assistants. "all" fans out over every
// entry here.
var agents = map[string]Agent{
	"claude": {Name: "claude", RelPath: ".claude.json", ServersKey: "mcpServers"},
	"cursor": {Name: "cursor", RelPath: ".cursor/mcp.json", ServersKey: "mcpServers"},
	"gemini": {Name: "gemini", RelPath: ".gemini/settings.json", ServersKey: "mcpServers"},
	"codex":  {Name: "codex", RelPath: ".codex/config.toml", TOML: true},
	"copilot": {
		Name: "copilot", ProjectRelPath: ".vscode/mcp.json", ServersKey: "servers",
	},
	"vscode": {
		Name: "vscode", ProjectRelPath: ".vscode/mcp.json", ServersKey: "servers",
	},
}

// AgentNames returns the supported agent tokens (excluding aliases) in a stable
// order for help text.
func AgentNames() []string {
	return []string{"claude", "cursor", "gemini", "codex", "copilot", "vscode", "all"}
}

// LookupAgent resolves an agent token, or returns ok=false if unknown.
func LookupAgent(name string) (Agent, bool) {
	a, ok := agents[strings.ToLower(strings.TrimSpace(name))]
	return a, ok
}

// fanout returns the concrete agents an --agent token expands to.
func fanout(name string) ([]Agent, error) {
	name = strings.ToLower(strings.TrimSpace(name))
	if name == "all" {
		out := make([]Agent, 0, len(agents))
		// Deterministic order; skip the vscode alias to avoid writing the same
		// file twice.
		for _, n := range []string{"claude", "cursor", "gemini", "codex", "copilot"} {
			out = append(out, agents[n])
		}
		return out, nil
	}
	a, ok := agents[name]
	if !ok {
		return nil, fmt.Errorf("unknown agent %q (supported: %s)", name, strings.Join(AgentNames(), ", "))
	}
	return []Agent{a}, nil
}

// SkillOptions configures InstallSkill.
type SkillOptions struct {
	Agent      string // claude|cursor|gemini|codex|copilot|vscode|all
	Command    string // default "atlas"
	DB         string // optional; threaded into args when set
	ConfigPath string // override the target file (for tests/CI); only valid for a single agent
	Home       string // override the home dir base; empty => os.UserHomeDir
	ProjectDir string // base for project-local configs (copilot/vscode); empty => cwd
}

// SkillResult records what InstallSkill wrote, one per agent touched.
type SkillResult struct {
	Agent string
	Path  string
	Entry ServerSpec
}

// InstallSkill registers Atlas as an MCP server for one or more assistants,
// idempotently. Re-running over an existing config replaces only the atlas
// entry, preserving every other server.
func InstallSkill(opts SkillOptions) ([]SkillResult, error) {
	command := opts.Command
	if command == "" {
		command = "atlas"
	}
	spec := ServerSpec{Command: command, Args: DefaultArgs(opts.DB)}

	targets, err := fanout(opts.Agent)
	if err != nil {
		return nil, err
	}
	if opts.ConfigPath != "" && len(targets) != 1 {
		return nil, fmt.Errorf("--config can only be used with a single --agent, not %q", opts.Agent)
	}

	home := opts.Home
	if home == "" {
		home, err = os.UserHomeDir()
		if err != nil {
			return nil, fmt.Errorf("resolve home dir: %w", err)
		}
	}
	projectDir := opts.ProjectDir
	if projectDir == "" {
		projectDir, _ = os.Getwd()
	}

	var results []SkillResult
	for _, a := range targets {
		path := opts.ConfigPath
		if path == "" {
			path = configPathFor(a, home, projectDir)
		}
		if err := writeServerEntry(a, path, spec); err != nil {
			return nil, fmt.Errorf("install %s: %w", a.Name, err)
		}
		results = append(results, SkillResult{Agent: a.Name, Path: path, Entry: spec})
	}

	// Drop the claude skill markdown when claude (or all) was a target and a
	// home base is available.
	if skillMarkdownWanted(opts.Agent) {
		if err := writeClaudeSkillMarkdown(home); err != nil {
			return nil, fmt.Errorf("write claude skill markdown: %w", err)
		}
	}
	return results, nil
}

func skillMarkdownWanted(agent string) bool {
	a := strings.ToLower(strings.TrimSpace(agent))
	return a == "claude" || a == "all"
}

// configPathFor resolves the on-disk config path for an agent.
func configPathFor(a Agent, home, projectDir string) string {
	if a.ProjectRelPath != "" {
		return filepath.Join(projectDir, a.ProjectRelPath)
	}
	return filepath.Join(home, a.RelPath)
}

// writeServerEntry merges the atlas server entry into an agent config file,
// creating the file (and parent dirs) when missing.
func writeServerEntry(a Agent, path string, spec ServerSpec) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	existing, err := os.ReadFile(path)
	if err != nil && !os.IsNotExist(err) {
		return err
	}

	var merged []byte
	if a.TOML {
		merged, err = MergeTOMLServer(existing, spec)
	} else {
		merged, err = MergeJSONServer(existing, a.ServersKey, spec)
	}
	if err != nil {
		return err
	}
	return os.WriteFile(path, merged, 0o644)
}

// MergeJSONServer merges (or replaces) the "atlas" entry under serversKey in a
// JSON document, preserving every other key and server. It is a pure function
// over bytes: empty/blank input is treated as an empty object.
func MergeJSONServer(existing []byte, serversKey string, spec ServerSpec) ([]byte, error) {
	root := map[string]any{}
	if trimmed := strings.TrimSpace(string(existing)); trimmed != "" {
		if err := json.Unmarshal(existing, &root); err != nil {
			return nil, fmt.Errorf("parse existing config: %w", err)
		}
	}

	servers, ok := root[serversKey].(map[string]any)
	if !ok {
		servers = map[string]any{}
	}
	entry := map[string]any{"command": spec.Command}
	if len(spec.Args) > 0 {
		args := make([]any, len(spec.Args))
		for i, s := range spec.Args {
			args[i] = s
		}
		entry["args"] = args
	}
	servers["atlas"] = entry
	root[serversKey] = servers

	out, err := json.MarshalIndent(root, "", "  ")
	if err != nil {
		return nil, err
	}
	return append(out, '\n'), nil
}

// MergeTOMLServer appends or replaces the [mcp_servers.atlas] block in a codex
// config.toml. Pure text merge: it drops any prior atlas block (whole table,
// up to the next table header or EOF) then appends a fresh one. Every other line
// is preserved verbatim.
func MergeTOMLServer(existing []byte, spec ServerSpec) ([]byte, error) {
	lines := splitLinesKeep(string(existing))
	var out []string
	skipping := false
	for _, ln := range lines {
		trimmed := strings.TrimSpace(ln)
		if isTOMLTableHeader(trimmed) {
			if trimmed == "[mcp_servers.atlas]" {
				skipping = true
				continue
			}
			// A different table header ends any atlas block we were skipping.
			skipping = false
		}
		if skipping {
			continue
		}
		out = append(out, ln)
	}

	// Trim trailing blank lines so the appended block sits cleanly.
	for len(out) > 0 && strings.TrimSpace(out[len(out)-1]) == "" {
		out = out[:len(out)-1]
	}

	block := renderTOMLBlock(spec)
	body := strings.Join(out, "\n")
	if body != "" {
		body += "\n\n"
	}
	body += block
	if !strings.HasSuffix(body, "\n") {
		body += "\n"
	}
	return []byte(body), nil
}

// renderTOMLBlock renders the [mcp_servers.atlas] table for a spec.
func renderTOMLBlock(spec ServerSpec) string {
	var b strings.Builder
	b.WriteString("[mcp_servers.atlas]\n")
	b.WriteString(fmt.Sprintf("command = %s\n", tomlQuote(spec.Command)))
	b.WriteString("args = [")
	for i, a := range spec.Args {
		if i > 0 {
			b.WriteString(", ")
		}
		b.WriteString(tomlQuote(a))
	}
	b.WriteString("]\n")
	return b.String()
}

func tomlQuote(s string) string {
	s = strings.ReplaceAll(s, `\`, `\\`)
	s = strings.ReplaceAll(s, `"`, `\"`)
	return `"` + s + `"`
}

func isTOMLTableHeader(trimmed string) bool {
	return strings.HasPrefix(trimmed, "[") && strings.HasSuffix(trimmed, "]")
}

// splitLinesKeep splits on \n without keeping a phantom trailing empty element
// for a final newline.
func splitLinesKeep(s string) []string {
	if s == "" {
		return nil
	}
	s = strings.ReplaceAll(s, "\r\n", "\n")
	parts := strings.Split(s, "\n")
	// Split leaves a trailing "" when s ends in \n; drop it.
	if len(parts) > 0 && parts[len(parts)-1] == "" {
		parts = parts[:len(parts)-1]
	}
	return parts
}

const claudeSkillMarkdown = `# Atlas code-intelligence skill

Atlas is a deterministic, local-first code knowledge graph (symbols, calls,
routes, coverage, cross-repo edges). When this skill's MCP server is connected,
prefer the ` + "`atlas`" + ` tools over grepping or guessing.

## When to call the atlas tools

- **Before editing an unfamiliar symbol** — call ` + "`symbols`/`explain`" + ` to see
  its definition, then ` + "`callers`/`refs`" + ` to find who depends on it.
- **Sizing a change / blast radius** — call ` + "`impact`" + ` with the changed files or
  symbols to get the affected set, and ` + "`coverage`" + ` to see which tests guard it.
- **"What calls / what reaches X"** — use ` + "`callers`, `refs`, `neighbors`, `path`" + `
  instead of full-text search; they walk the real graph.
- **Finding code by intent** — ` + "`search`" + ` runs BM25 lexical search over indexed
  symbols; faster and more precise than reading files blind.
- **HTTP route / API questions** — ` + "`route_contracts`" + ` lists a repo's routes and
  ` + "`consumers`" + ` shows who calls them.
- **Cross-repo work** — ` + "`cross_repo_impact`" + ` propagates a change across linked
  repos; ` + "`repos`" + ` lists what's indexed.
- **Staleness / drift** — ` + "`status`" + ` reports index freshness; ` + "`history`" + ` and
  ` + "`snapshot_diff`" + ` compare graph snapshots over time. Run ` + "`index`" + ` to refresh.

## Notes

- All answers are deterministic graph reads — cite the ` + "`file:line`" + ` they return.
- If a tool reports the graph is stale or empty, run ` + "`atlas index .`" + ` (or the
  ` + "`index`" + ` tool) first, then retry.
`

// writeClaudeSkillMarkdown drops a small SKILL.md describing when to call the
// atlas tools, under <home>/.claude/skills/atlas/SKILL.md.
func writeClaudeSkillMarkdown(home string) error {
	dir := filepath.Join(home, ".claude", "skills", "atlas")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte(claudeSkillMarkdown), 0o644)
}

// ---- git hooks ----

// hookTypes is the set of supported hook events.
var hookTypes = map[string]bool{
	"post-merge":  true,
	"post-commit": true,
	"pre-push":    true,
}

// HookTypeNames returns the supported hook tokens in a stable order.
func HookTypeNames() []string {
	return []string{"post-merge", "post-commit", "pre-push"}
}

// HookOptions configures InstallHook.
type HookOptions struct {
	Type string // post-merge|post-commit|pre-push
	Repo string // git repo dir; empty => cwd
	DB   string // optional; threaded into `atlas index . --db <db>`
}

// HookResult records what InstallHook wrote.
type HookResult struct {
	Type       string
	Path       string
	BackupPath string // non-empty when a pre-existing non-atlas hook was backed up
}

const hookMarker = "# atlas-managed"

// InstallHook writes an executable .git/hooks/<type> that runs `atlas index .`
// so the local graph stays fresh. It is idempotent: an existing atlas-managed
// hook is replaced; an existing non-atlas hook is backed up to
// <type>.atlas-backup before being overwritten.
func InstallHook(opts HookOptions) (HookResult, error) {
	htype := strings.ToLower(strings.TrimSpace(opts.Type))
	if !hookTypes[htype] {
		return HookResult{}, fmt.Errorf("unknown hook type %q (supported: %s)", opts.Type, strings.Join(HookTypeNames(), ", "))
	}
	repo := opts.Repo
	if repo == "" {
		repo, _ = os.Getwd()
	}

	gitDir, err := resolveGitDir(repo)
	if err != nil {
		return HookResult{}, err
	}
	hooksDir := filepath.Join(gitDir, "hooks")
	if err := os.MkdirAll(hooksDir, 0o755); err != nil {
		return HookResult{}, err
	}
	path := filepath.Join(hooksDir, htype)

	res := HookResult{Type: htype, Path: path}

	// Back up an existing non-atlas hook so we never silently clobber user work.
	if existing, err := os.ReadFile(path); err == nil {
		if !strings.Contains(string(existing), hookMarker) {
			backup := path + ".atlas-backup"
			if err := os.WriteFile(backup, existing, 0o755); err != nil {
				return HookResult{}, fmt.Errorf("back up existing hook: %w", err)
			}
			res.BackupPath = backup
		}
	} else if !os.IsNotExist(err) {
		return HookResult{}, err
	}

	if err := os.WriteFile(path, []byte(renderHook(opts.DB)), 0o755); err != nil {
		return HookResult{}, err
	}
	// WriteFile honors the mode only on create; force it for the replace case.
	if err := os.Chmod(path, 0o755); err != nil {
		return HookResult{}, err
	}
	return res, nil
}

// renderHook builds the hook script body. The marker line lets re-runs detect an
// atlas-managed hook and replace it cleanly.
func renderHook(db string) string {
	cmd := "atlas index ."
	if strings.TrimSpace(db) != "" {
		cmd = "atlas index . --db " + shellQuote(db)
	}
	return "#!/bin/sh\n" +
		hookMarker + "\n" +
		"# Keeps the local Atlas code graph fresh. Managed by `atlas install hook`.\n" +
		"# Re-running `atlas install hook` replaces this block.\n" +
		"command -v atlas >/dev/null 2>&1 || exit 0\n" +
		cmd + " >/dev/null 2>&1 || true\n"
}

func shellQuote(s string) string {
	if s == "" {
		return "''"
	}
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

// resolveGitDir finds the .git directory for a repo. It handles both a normal
// repo (.git is a directory) and a worktree/submodule (.git is a file pointing
// at "gitdir: <path>").
func resolveGitDir(repo string) (string, error) {
	dotGit := filepath.Join(repo, ".git")
	info, err := os.Stat(dotGit)
	if err != nil {
		return "", fmt.Errorf("%s is not a git repo (no .git): %w", repo, err)
	}
	if info.IsDir() {
		return dotGit, nil
	}
	// .git is a file: "gitdir: <path>".
	data, err := os.ReadFile(dotGit)
	if err != nil {
		return "", err
	}
	line := strings.TrimSpace(string(data))
	const prefix = "gitdir:"
	if !strings.HasPrefix(line, prefix) {
		return "", fmt.Errorf("malformed .git file in %s", repo)
	}
	gitDir := strings.TrimSpace(strings.TrimPrefix(line, prefix))
	if !filepath.IsAbs(gitDir) {
		gitDir = filepath.Join(repo, gitDir)
	}
	return gitDir, nil
}
