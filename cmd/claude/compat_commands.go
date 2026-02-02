package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

// unsupportedCommand constructs a command that fails loudly with guidance.
// It always exits with a non-zero status so unsupported commands are never silent.
func unsupportedCommand(use string, short string, hint string) *cobra.Command {
	return &cobra.Command{
		Use:   use,
		Short: short,
		Args:  cobra.ArbitraryArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			message := hint
			if message == "" {
				message = "Use your OpenAI-compatible gateway instead."
			}
			fmt.Fprintf(os.Stderr, "%s is not supported in OpenClaude. %s\n", cmd.CommandPath(), message)
			os.Exit(2)
			return nil
		},
	}
}

// installCommand mirrors the Claude Code install command shape.
// The command remains a stub so users get clear guidance instead of no-op installs.
func installCommand() *cobra.Command {
	cmd := unsupportedCommand(
		"install [target]",
		"Install Claude Code native build. Use [target] to specify version (stable, latest, or specific version)",
		"Use `make build` to build OpenClaude instead.",
	)
	cmd.Flags().Bool("force", false, "Force installation even if already installed")
	return cmd
}

// updateCommand mirrors the Claude Code update command shape.
// OpenClaude does not self-update, so this stays an explicit error path.
func updateCommand() *cobra.Command {
	return unsupportedCommand("update", "Check for updates and install if available", "OpenClaude does not auto-update.")
}

// setupTokenCommand mirrors the Claude Code setup-token command shape.
// OpenClaude uses config files instead of Anthropic-managed tokens.
func setupTokenCommand() *cobra.Command {
	return unsupportedCommand(
		"setup-token",
		"Set up a long-lived authentication token (requires Claude subscription)",
		"OpenClaude uses ~/.openclaude/config.json for credentials.",
	)
}

// mcpCommand mirrors the Claude Code MCP management command tree.
// Subcommands are stubbed to keep CLI compatibility while signaling unsupported features.
func mcpCommand() *cobra.Command {
	cmd := unsupportedCommand("mcp", "Configure and manage MCP servers", "MCP server management is not supported.")

	cmd.AddCommand(mcpAddCommand())
	cmd.AddCommand(mcpAddJSONCommand())
	cmd.AddCommand(mcpAddFromDesktopCommand())
	cmd.AddCommand(mcpGetCommand())
	cmd.AddCommand(mcpListCommand())
	cmd.AddCommand(mcpRemoveCommand())
	cmd.AddCommand(mcpResetProjectChoicesCommand())
	cmd.AddCommand(mcpServeCommand())

	return cmd
}

// mcpAddCommand mirrors claude mcp add options.
// The payload is accepted for parsing parity but execution is unsupported.
func mcpAddCommand() *cobra.Command {
	cmd := unsupportedCommand(
		"add <name> <commandOrUrl> [args...]",
		"Add an MCP server to Claude Code.",
		"MCP server management is not supported.",
	)
	cmd.Flags().StringSliceP("env", "e", nil, "Set environment variables (e.g. -e KEY=value)")
	cmd.Flags().StringSliceP("header", "H", nil, "Set WebSocket headers (e.g. -H \"X-Api-Key: abc123\" -H \"X-Custom: value\")")
	cmd.Flags().StringP("scope", "s", "local", "Configuration scope (local, user, or project)")
	cmd.Flags().StringP("transport", "t", "", "Transport type (stdio, sse, http). Defaults to stdio if not specified.")
	return cmd
}

// mcpAddJSONCommand mirrors claude mcp add-json options.
// This stub keeps flag help output aligned with Claude Code.
func mcpAddJSONCommand() *cobra.Command {
	cmd := unsupportedCommand(
		"add-json <name> <json>",
		"Add an MCP server (stdio or SSE) with a JSON string",
		"MCP server management is not supported.",
	)
	cmd.Flags().StringP("scope", "s", "local", "Configuration scope (local, user, or project)")
	return cmd
}

// mcpAddFromDesktopCommand mirrors claude mcp add-from-claude-desktop options.
// It intentionally fails so users know the feature is unavailable.
func mcpAddFromDesktopCommand() *cobra.Command {
	cmd := unsupportedCommand(
		"add-from-claude-desktop",
		"Import MCP servers from Claude Desktop (Mac and WSL only)",
		"MCP server management is not supported.",
	)
	cmd.Flags().StringP("scope", "s", "local", "Configuration scope (local, user, or project)")
	return cmd
}

// mcpGetCommand mirrors claude mcp get options.
// The stub communicates unsupported MCP management.
func mcpGetCommand() *cobra.Command {
	return unsupportedCommand("get <name>", "Get details about an MCP server", "MCP server management is not supported.")
}

// mcpListCommand mirrors claude mcp list options.
// The stub exists for CLI parity only.
func mcpListCommand() *cobra.Command {
	return unsupportedCommand("list", "List configured MCP servers", "MCP server management is not supported.")
}

// mcpRemoveCommand mirrors claude mcp remove options.
// This remains a stub because MCP persistence is not implemented.
func mcpRemoveCommand() *cobra.Command {
	cmd := unsupportedCommand("remove <name>", "Remove an MCP server", "MCP server management is not supported.")
	cmd.Flags().StringP("scope", "s", "", "Configuration scope (local, user, or project) - if not specified, removes from whichever scope it exists in")
	return cmd
}

// mcpResetProjectChoicesCommand mirrors claude mcp reset-project-choices.
// The stub warns users that project-scoped MCP choices are not tracked.
func mcpResetProjectChoicesCommand() *cobra.Command {
	return unsupportedCommand(
		"reset-project-choices",
		"Reset all approved and rejected project-scoped (.mcp.json) servers within this project",
		"MCP server management is not supported.",
	)
}

// mcpServeCommand mirrors claude mcp serve options.
// OpenClaude does not ship an MCP server, so this always fails.
func mcpServeCommand() *cobra.Command {
	cmd := unsupportedCommand("serve", "Start the Claude Code MCP server", "MCP server management is not supported.")
	cmd.Flags().BoolP("debug", "d", false, "Enable debug mode")
	cmd.Flags().Bool("verbose", false, "Override verbose mode setting from config")
	return cmd
}

// pluginCommand mirrors the Claude Code plugin management tree.
// Subcommands are stubbed to keep CLI parity without implementing plugins.
func pluginCommand() *cobra.Command {
	cmd := unsupportedCommand("plugin", "Manage Claude Code plugins", "Plugins are not supported.")

	cmd.AddCommand(pluginListCommand())
	cmd.AddCommand(pluginInstallCommand())
	cmd.AddCommand(pluginUninstallCommand())
	cmd.AddCommand(pluginUpdateCommand())
	cmd.AddCommand(pluginEnableCommand())
	cmd.AddCommand(pluginDisableCommand())
	cmd.AddCommand(pluginValidateCommand())
	cmd.AddCommand(pluginMarketplaceCommand())

	return cmd
}

// pluginListCommand mirrors claude plugin list options.
// The stub accepts flags but always reports unsupported.
func pluginListCommand() *cobra.Command {
	cmd := unsupportedCommand("list", "List installed plugins", "Plugins are not supported.")
	cmd.Flags().Bool("available", false, "Include available plugins from marketplaces (requires --json)")
	cmd.Flags().Bool("json", false, "Output as JSON")
	return cmd
}

// pluginInstallCommand mirrors claude plugin install options.
// The stub keeps command shape while informing users plugins are unavailable.
func pluginInstallCommand() *cobra.Command {
	cmd := unsupportedCommand(
		"install <plugin>",
		"Install a plugin from available marketplaces (use plugin@marketplace for specific marketplace)",
		"Plugins are not supported.",
	)
	cmd.Aliases = []string{"i"}
	cmd.Flags().StringP("scope", "s", "user", "Installation scope: user, project, or local")
	return cmd
}

// pluginUninstallCommand mirrors claude plugin uninstall/remove options.
// This stub avoids implying plugins exist in OpenClaude.
func pluginUninstallCommand() *cobra.Command {
	cmd := unsupportedCommand("uninstall <plugin>", "Uninstall an installed plugin", "Plugins are not supported.")
	cmd.Aliases = []string{"remove"}
	cmd.Flags().StringP("scope", "s", "user", "Uninstall from scope: user, project, or local")
	return cmd
}

// pluginUpdateCommand mirrors claude plugin update options.
// OpenClaude does not manage plugins, so the command always errors.
func pluginUpdateCommand() *cobra.Command {
	cmd := unsupportedCommand(
		"update <plugin>",
		"Update a plugin to the latest version (restart required to apply)",
		"Plugins are not supported.",
	)
	cmd.Flags().StringP("scope", "s", "user", "Installation scope: user, project, local, managed")
	return cmd
}

// pluginEnableCommand mirrors claude plugin enable options.
// Keeping this stub preserves CLI compatibility with upstream docs.
func pluginEnableCommand() *cobra.Command {
	cmd := unsupportedCommand("enable <plugin>", "Enable a disabled plugin", "Plugins are not supported.")
	cmd.Flags().StringP("scope", "s", "user", "Installation scope: user, project, local")
	return cmd
}

// pluginDisableCommand mirrors claude plugin disable options.
// The stub exists to signal unsupported plugin toggles.
func pluginDisableCommand() *cobra.Command {
	cmd := unsupportedCommand("disable [plugin]", "Disable an enabled plugin", "Plugins are not supported.")
	cmd.Flags().BoolP("all", "a", false, "Disable all enabled plugins")
	cmd.Flags().StringP("scope", "s", "user", "Installation scope: user, project, local")
	return cmd
}

// pluginValidateCommand mirrors claude plugin validate options.
// This stub clarifies plugin validation is unavailable in OpenClaude.
func pluginValidateCommand() *cobra.Command {
	return unsupportedCommand("validate <path>", "Validate a plugin or marketplace manifest", "Plugins are not supported.")
}

// pluginMarketplaceCommand mirrors claude plugin marketplace subcommands.
// Subcommands are stubbed for compatibility only.
func pluginMarketplaceCommand() *cobra.Command {
	cmd := unsupportedCommand("marketplace", "Manage Claude Code marketplaces", "Plugins are not supported.")

	cmd.AddCommand(pluginMarketplaceAddCommand())
	cmd.AddCommand(pluginMarketplaceListCommand())
	cmd.AddCommand(pluginMarketplaceRemoveCommand())
	cmd.AddCommand(pluginMarketplaceUpdateCommand())

	return cmd
}

// pluginMarketplaceAddCommand mirrors claude plugin marketplace add options.
// The stub preserves flag help output without enabling marketplaces.
func pluginMarketplaceAddCommand() *cobra.Command {
	return unsupportedCommand("add <source>", "Add a marketplace from a URL, path, or GitHub repo", "Plugins are not supported.")
}

// pluginMarketplaceListCommand mirrors claude plugin marketplace list options.
// The stub keeps JSON output flags visible for parity.
func pluginMarketplaceListCommand() *cobra.Command {
	cmd := unsupportedCommand("list", "List all configured marketplaces", "Plugins are not supported.")
	cmd.Flags().Bool("json", false, "Output as JSON")
	return cmd
}

// pluginMarketplaceRemoveCommand mirrors claude plugin marketplace remove options.
// This stub fails loudly so users know marketplace config is unsupported.
func pluginMarketplaceRemoveCommand() *cobra.Command {
	cmd := unsupportedCommand("remove <name>", "Remove a configured marketplace", "Plugins are not supported.")
	cmd.Aliases = []string{"rm"}
	return cmd
}

// pluginMarketplaceUpdateCommand mirrors claude plugin marketplace update options.
// The stub exists to keep command structure aligned with Claude Code.
func pluginMarketplaceUpdateCommand() *cobra.Command {
	return unsupportedCommand(
		"update [name]",
		"Update marketplace(s) from their source - updates all if no name specified",
		"Plugins are not supported.",
	)
}
