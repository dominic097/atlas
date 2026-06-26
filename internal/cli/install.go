package cli

import (
	"strings"

	"github.com/spf13/cobra"

	"github.com/dominic097/atlas/internal/install"
)

func newInstallCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "install",
		Short: "Install Atlas integration glue (skills, MCP config, git hooks)",
	}
	cmd.AddCommand(newInstallSkillCmd(), newInstallHookCmd())
	return cmd
}

func newInstallSkillCmd() *cobra.Command {
	var (
		agent      string
		command    string
		db         string
		configPath string
		home       string
	)
	cmd := &cobra.Command{
		Use:   "skill",
		Short: "Register Atlas as an MCP server / skill for an AI assistant",
		Long: "Writes (and idempotently merges) Atlas as an MCP server into an " +
			"assistant config so it can call the atlas tools. Re-running replaces " +
			"only the atlas entry and preserves any other configured servers.",
		RunE: func(cmd *cobra.Command, _ []string) error {
			results, err := install.InstallSkill(install.SkillOptions{
				Agent:      agent,
				Command:    command,
				DB:         db,
				ConfigPath: configPath,
				Home:       home,
			})
			if err != nil {
				return err
			}
			for _, r := range results {
				cmd.Printf("wrote %s\n", r.Path)
				cmd.Printf("  %s.atlas = {command: %q, args: [%s]}\n",
					serversKeyFor(r.Agent), r.Entry.Command, quoteJoin(r.Entry.Args))
			}
			return nil
		},
	}
	f := cmd.Flags()
	f.StringVar(&agent, "agent", "claude", strings.Join(install.AgentNames(), "|"))
	f.StringVar(&command, "command", "atlas", "command the assistant launches for the MCP server")
	f.StringVar(&db, "db", "", "storage DSN to pass through as `mcp --db <db>` (default: server default)")
	f.StringVar(&configPath, "config", "", "override the target config file path (single --agent only; for tests/CI)")
	f.StringVar(&home, "home", "", "override the home dir base (default: $HOME)")
	return cmd
}

// serversKeyFor mirrors the agent's JSON key for the printed summary.
func serversKeyFor(agent string) string {
	if a, ok := install.LookupAgent(agent); ok && a.ServersKey != "" {
		return a.ServersKey
	}
	if a, ok := install.LookupAgent(agent); ok && a.TOML {
		return "mcp_servers"
	}
	return "mcpServers"
}

func quoteJoin(args []string) string {
	parts := make([]string, len(args))
	for i, a := range args {
		parts[i] = `"` + a + `"`
	}
	return strings.Join(parts, ", ")
}

func newInstallHookCmd() *cobra.Command {
	var (
		hookType string
		repo     string
		db       string
	)
	cmd := &cobra.Command{
		Use:   "hook",
		Short: "Install a local git hook that keeps the Atlas graph fresh",
		Long: "Writes an executable .git/hooks/<type> that runs `atlas index .` so " +
			"the local graph stays current. Idempotent: an existing atlas-managed " +
			"hook is replaced, and a pre-existing non-atlas hook is backed up to " +
			"<type>.atlas-backup first.",
		RunE: func(cmd *cobra.Command, _ []string) error {
			res, err := install.InstallHook(install.HookOptions{
				Type: hookType,
				Repo: repo,
				DB:   db,
			})
			if err != nil {
				return err
			}
			if res.BackupPath != "" {
				cmd.Printf("backed up existing hook to %s\n", res.BackupPath)
			}
			cmd.Printf("wrote executable %s hook %s\n", res.Type, res.Path)
			return nil
		},
	}
	f := cmd.Flags()
	f.StringVar(&hookType, "type", "post-merge", strings.Join(install.HookTypeNames(), "|"))
	f.StringVar(&repo, "repo", "", "git repo dir (default: cwd)")
	f.StringVar(&db, "db", "", "storage DSN to pass through as `atlas index . --db <db>`")
	return cmd
}
