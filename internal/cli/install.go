package cli

import (
	"github.com/spf13/cobra"
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
	var agent string
	cmd := &cobra.Command{
		Use:   "skill",
		Short: "Register Atlas as an MCP server / skill for an AI assistant",
		RunE: func(cmd *cobra.Command, _ []string) error {
			// TODO(install): write the client's mcp.json (claude/cursor/codex/
			// gemini/copilot) pointing at `atlas mcp --transport stdio --repo .`,
			// idempotently merging into any existing config.
			cmd.Printf("would install Atlas MCP skill for agent %q (not implemented)\n", agent)
			return nil
		},
	}
	cmd.Flags().StringVar(&agent, "agent", "claude", "claude|cursor|codex|gemini|copilot|all")
	return cmd
}

func newInstallHookCmd() *cobra.Command {
	var hookType string
	cmd := &cobra.Command{
		Use:   "hook",
		Short: "Install a local git blast-radius gate (pre-push)",
		RunE: func(cmd *cobra.Command, _ []string) error {
			// TODO(install): write .git/hooks/<type> running `atlas impact --gate`.
			cmd.Printf("would install %q git hook (not implemented)\n", hookType)
			return nil
		},
	}
	cmd.Flags().StringVar(&hookType, "type", "pre-push", "pre-push|pre-commit|post-merge")
	return cmd
}
