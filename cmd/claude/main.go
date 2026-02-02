package main

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"

	"github.com/openclaude/openclaude/internal/agent"
	"github.com/openclaude/openclaude/internal/config"
	"github.com/openclaude/openclaude/internal/llm/openai"
	"github.com/openclaude/openclaude/internal/session"
	"github.com/openclaude/openclaude/internal/streamjson"
	"github.com/openclaude/openclaude/internal/tools"
)

// version tracks the Claude Code compatibility version reported to clients.
// This should match the reference version in docs/compat/claude-latest.md.
const version = "2.1.29"

// defaultTaskMaxDepth caps nested Task executions to prevent runaway recursion.
const defaultTaskMaxDepth = 2

// defaultTaskMaxTurns sets a safe default for Task sub-runs.
const defaultTaskMaxTurns = 4

// options holds all CLI flags for compatibility with Claude Code.
type options struct {
	// AddDirs are extra directories added to the sandbox allowlist.
	AddDirs []string
	// Agent selects a named agent profile when supported.
	Agent string
	// AgentsJSON provides inline JSON agent definitions.
	AgentsJSON string
	// AllowDangerouslySkipPermissions toggles the availability of bypass mode.
	AllowDangerouslySkipPermissions bool
	// AllowedTools restricts tool usage to a whitelist.
	AllowedTools []string
	// AppendSystemPrompt appends extra system instructions.
	AppendSystemPrompt string
	// AppendSystemPromptFile reads system prompt additions from a file.
	AppendSystemPromptFile string
	// Betas adds beta headers in upstream requests.
	Betas []string
	// Chrome enables Claude-in-Chrome integration.
	Chrome bool
	// Continue resumes the most recent session in the current project.
	Continue bool
	// DebugToStderr routes debug output to stderr.
	DebugToStderr bool
	// Debug toggles debug output categories.
	Debug string
	// DebugFile writes debug logs to a file path.
	DebugFile string
	// DisableSlashCommands disables slash-command parsing.
	DisableSlashCommands bool
	// DisallowedTools blocks specific tools even if available.
	DisallowedTools []string
	// EnableAuthStatus emits auth_status events in stream-json output.
	EnableAuthStatus bool
	// FallbackModel is used on retryable errors in print mode.
	FallbackModel string
	// FileSpecs defines preloaded file resources.
	FileSpecs []string
	// ForkSession controls whether resume forks the session id.
	ForkSession bool
	// FromPR resumes a session linked to a PR.
	FromPR string
	// IDE auto-connects to the IDE if supported.
	IDE bool
	// IncludePartialMessages toggles partial message streaming in print mode.
	IncludePartialMessages bool
	// Init triggers setup hooks with the init trigger.
	Init bool
	// InitOnly runs setup hooks and exits.
	InitOnly bool
	// HookConfig stores hook definitions from stream-json control requests.
	HookConfig *streamJSONHookConfig
	// InputFormat controls how prompts are read in print mode.
	InputFormat string
	// JSONSchema provides structured output validation schema.
	JSONSchema string
	// Maintenance triggers setup hooks with maintenance trigger.
	Maintenance bool
	// MCPConfig holds MCP server configuration inputs.
	MCPConfig []string
	// MCPDebug enables deprecated MCP debug mode.
	MCPDebug bool
	// MaxBudgetUSD enforces an estimated spend ceiling.
	MaxBudgetUSD float64
	// MaxTurns caps the number of assistant/tool turns.
	MaxTurns int
	// MaxThinkingTokens configures thinking token budgets for compatible models.
	MaxThinkingTokens int
	// Model overrides the default model selection.
	Model string
	// NoChrome disables Claude-in-Chrome integration.
	NoChrome bool
	// NoSessionPersistence disables saving session history to disk.
	NoSessionPersistence bool
	// OutputFormat controls print mode output encoding.
	OutputFormat string
	// ParentSessionID scopes teammate analytics.
	ParentSessionID string
	// PermissionMode configures tool approval behavior.
	PermissionMode string
	// PermissionPromptTool names the MCP tool used for permission prompts.
	PermissionPromptTool string
	// PluginDir is reserved for future plugin loading.
	PluginDir []string
	// PlanModeRequired forces plan mode before execution.
	PlanModeRequired bool
	// Print enables non-interactive mode.
	Print bool
	// Remote creates a remote session with optional description.
	Remote string
	// ReplayUserMessages echoes user messages in stream-json output.
	ReplayUserMessages bool
	// Resume resumes a specific session id or the interactive picker.
	Resume string
	// ResumeSessionAt limits resume history in print mode.
	ResumeSessionAt string
	// RewindFiles restores files to a user message snapshot and exits.
	RewindFiles string
	// SDKURL points to a remote SDK websocket endpoint.
	SDKURL string
	// SessionID sets a fixed session id.
	SessionID string
	// SettingSources limits Claude settings sources to load.
	SettingSources []string
	// Settings provides a path or inline JSON for settings overrides.
	Settings string
	// StrictMCPConfig enforces MCP-only configs when supported.
	StrictMCPConfig bool
	// SystemPrompt overrides the default system prompt.
	SystemPrompt string
	// SystemPromptFile reads the system prompt from a file.
	SystemPromptFile string
	// TeamName assigns a teammate team name.
	TeamName string
	// TeammateMode configures how teammates are spawned.
	TeammateMode string
	// Teleport resumes a teleport session.
	Teleport string
	// AgentID identifies a teammate agent.
	AgentID string
	// AgentName is the teammate display name.
	AgentName string
	// AgentColor is the teammate UI color.
	AgentColor string
	// AgentType specifies a custom agent type.
	AgentType string
	// Tools defines the available tool set.
	Tools []string
	// Verbose toggles verbose output.
	Verbose bool
	// Version prints the CLI version.
	Version bool
	// DangerouslySkipPermissions bypasses tool permission checks.
	DangerouslySkipPermissions bool
}

// main wires Cobra and executes the CLI.
func main() {
	opts := &options{}
	rootCmd := &cobra.Command{
		Use:   "claude [prompt]",
		Short: "Claude Code - starts an interactive session by default, use -p/--print for non-interactive output",
		RunE: func(cmd *cobra.Command, args []string) error {
			if opts.Version {
				fmt.Printf("%s (Claude Code)\n", version)
				return nil
			}
			return runRoot(cmd, opts, args)
		},
	}
	rootCmd.Args = cobra.ArbitraryArgs

	applyFlags(rootCmd.Flags(), opts)

	rootCmd.AddCommand(doctorCommand())
	rootCmd.AddCommand(installCommand())
	rootCmd.AddCommand(updateCommand())
	rootCmd.AddCommand(mcpCommand())
	rootCmd.AddCommand(pluginCommand())
	rootCmd.AddCommand(setupTokenCommand())

	rootCmd.SetArgs(normalizeArgs(os.Args[1:]))

	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}

// normalizeArgs rewrites shorthand tokens to match Claude Code behavior.
func normalizeArgs(args []string) []string {
	if len(args) == 0 {
		return nil
	}
	normalized := make([]string, 0, len(args))
	for _, arg := range args {
		if arg == "-d2e" {
			normalized = append(normalized, "--debug-to-stderr")
			continue
		}
		normalized = append(normalized, arg)
	}
	return normalized
}

// applyFlags defines all CLI flags with Claude Code-compatible names.
func applyFlags(flags *pflag.FlagSet, opts *options) {
	flags.SetNormalizeFunc(normalizeFlagName)

	flags.StringSliceVar(&opts.AddDirs, "add-dir", nil, "Additional directories to allow tool access to")
	flags.StringVar(&opts.Agent, "agent", "", "Agent for the current session. Overrides the 'agent' setting.")
	flags.StringVar(&opts.AgentsJSON, "agents", "", "JSON object defining custom agents (e.g. '{\"reviewer\": {\"description\": \"Reviews code\", \"prompt\": \"You are a code reviewer\"}}')")
	flags.BoolVar(&opts.AllowDangerouslySkipPermissions, "allow-dangerously-skip-permissions", false, "Enable bypassing all permission checks as an option, without it being enabled by default. Recommended only for sandboxes with no internet access.")
	flags.StringSliceVar(&opts.AllowedTools, "allowedTools", nil, "Comma or space-separated list of tool names to allow (e.g. \"Bash(git:*) Edit\")")
	flags.StringVar(&opts.AppendSystemPrompt, "append-system-prompt", "", "Append a system prompt to the default system prompt")
	flags.StringVar(&opts.AppendSystemPromptFile, "append-system-prompt-file", "", "Read system prompt from a file and append to the default system prompt")
	flags.StringSliceVar(&opts.Betas, "betas", nil, "Beta headers to include in API requests (API key users only)")
	flags.BoolVar(&opts.Chrome, "chrome", false, "Enable Claude in Chrome integration")
	flags.BoolVarP(&opts.Continue, "continue", "c", false, "Continue the most recent conversation in the current directory")
	flags.StringVarP(&opts.Debug, "debug", "d", "", "Enable debug mode with optional category filtering (e.g., \"api,hooks\" or \"!statsig,!file\")")
	flags.BoolVar(&opts.DebugToStderr, "debug-to-stderr", false, "Enable debug mode (to stderr)")
	flags.StringVar(&opts.DebugFile, "debug-file", "", "Write debug logs to a specific file path (implicitly enables debug mode)")
	flags.BoolVar(&opts.DisableSlashCommands, "disable-slash-commands", false, "Disable all skills")
	flags.StringSliceVar(&opts.DisallowedTools, "disallowedTools", nil, "Comma or space-separated list of tool names to deny (e.g. \"Bash(git:*) Edit\")")
	flags.BoolVar(&opts.EnableAuthStatus, "enable-auth-status", false, "Enable auth status messages in SDK mode")
	flags.StringVar(&opts.FallbackModel, "fallback-model", "", "Enable automatic fallback to specified model when default model is overloaded (only works with --print)")
	flags.StringSliceVar(&opts.FileSpecs, "file", nil, "File resources to download at startup. Format: file_id:relative_path (e.g., --file file_abc:doc.txt file_def:img.png)")
	flags.BoolVar(&opts.ForkSession, "fork-session", false, "When resuming, create a new session ID instead of reusing the original (use with --resume or --continue)")
	flags.StringVar(&opts.FromPR, "from-pr", "", "Resume a session linked to a PR by PR number/URL, or open interactive picker with optional search term")
	flags.BoolVar(&opts.IDE, "ide", false, "Automatically connect to IDE on startup if exactly one valid IDE is available")
	flags.BoolVar(&opts.IncludePartialMessages, "include-partial-messages", false, "Include partial message chunks as they arrive (only works with --print and --output-format=stream-json)")
	flags.BoolVar(&opts.Init, "init", false, "Run Setup hooks with init trigger, then continue")
	flags.BoolVar(&opts.InitOnly, "init-only", false, "Run Setup and SessionStart:startup hooks, then exit")
	flags.StringVar(&opts.InputFormat, "input-format", "text", "Input format (only works with --print): \"text\" (default), or \"stream-json\" (realtime streaming input)")
	flags.StringVar(&opts.JSONSchema, "json-schema", "", "JSON Schema for structured output validation. Example: {\"type\":\"object\",\"properties\":{\"name\":{\"type\":\"string\"}},\"required\":[\"name\"]}")
	flags.BoolVar(&opts.Maintenance, "maintenance", false, "Run Setup hooks with maintenance trigger, then continue")
	flags.StringSliceVar(&opts.MCPConfig, "mcp-config", nil, "Load MCP servers from JSON files or strings (space-separated)")
	flags.BoolVar(&opts.MCPDebug, "mcp-debug", false, "[DEPRECATED. Use --debug instead] Enable MCP debug mode (shows MCP server errors)")
	flags.Float64Var(&opts.MaxBudgetUSD, "max-budget-usd", 0, "Maximum dollar amount to spend on API calls (only works with --print)")
	flags.IntVar(&opts.MaxThinkingTokens, "max-thinking-tokens", 0, "Maximum number of thinking tokens. (only works with --print)")
	flags.IntVar(&opts.MaxTurns, "max-turns", 0, "Maximum number of agentic turns in non-interactive mode. This will early exit the conversation after the specified number of turns. (only works with --print)")
	flags.StringVar(&opts.Model, "model", "", "Model for the current session. Provide an alias for the latest model (e.g. 'sonnet' or 'opus') or a model's full name (e.g. 'claude-sonnet-4-5-20250929').")
	flags.BoolVar(&opts.NoChrome, "no-chrome", false, "Disable Claude in Chrome integration")
	flags.BoolVar(&opts.NoSessionPersistence, "no-session-persistence", false, "Disable session persistence - sessions will not be saved to disk and cannot be resumed (only works with --print)")
	flags.StringVar(&opts.OutputFormat, "output-format", "text", "Output format (only works with --print): \"text\" (default), \"json\" (single result), or \"stream-json\" (realtime streaming)")
	flags.StringVar(&opts.PermissionMode, "permission-mode", "default", "Permission mode to use for the session")
	flags.StringVar(&opts.PermissionPromptTool, "permission-prompt-tool", "", "MCP tool to use for permission prompts (only works with --print)")
	flags.StringSliceVar(&opts.PluginDir, "plugin-dir", nil, "Load plugins from directories for this session only (repeatable)")
	flags.BoolVarP(&opts.Print, "print", "p", false, "Print response and exit (useful for pipes). Note: The workspace trust dialog is skipped when Claude is run with the -p mode. Only use this flag in directories you trust.")
	flags.StringVar(&opts.Remote, "remote", "", "Create a remote session with the given description")
	flags.BoolVar(&opts.ReplayUserMessages, "replay-user-messages", false, "Re-emit user messages from stdin back on stdout for acknowledgment (only works with --input-format=stream-json and --output-format=stream-json)")
	flags.StringVarP(&opts.Resume, "resume", "r", "", "Resume a conversation by session ID, or open interactive picker with optional search term")
	flags.StringVar(&opts.ResumeSessionAt, "resume-session-at", "", "When resuming, only messages up to and including the assistant message with <message.id> (use with --resume in print mode)")
	flags.StringVar(&opts.RewindFiles, "rewind-files", "", "Restore files to state at the specified user message and exit (requires --resume)")
	flags.StringVar(&opts.SDKURL, "sdk-url", "", "Use remote WebSocket endpoint for SDK I/O streaming (only with -p and stream-json format)")
	flags.StringVar(&opts.SessionID, "session-id", "", "Use a specific session ID for the conversation (must be a valid UUID)")
	flags.StringSliceVar(&opts.SettingSources, "setting-sources", nil, "Comma-separated list of setting sources to load (user, project, local).")
	flags.StringVar(&opts.Settings, "settings", "", "Path to a settings JSON file or a JSON string to load additional settings from")
	flags.BoolVar(&opts.StrictMCPConfig, "strict-mcp-config", false, "Only use MCP servers from --mcp-config, ignoring all other MCP configurations")
	flags.StringVar(&opts.SystemPrompt, "system-prompt", "", "System prompt to use for the session")
	flags.StringVar(&opts.SystemPromptFile, "system-prompt-file", "", "Read system prompt from a file")
	flags.StringVar(&opts.TeamName, "team-name", "", "Team name for swarm coordination")
	flags.StringVar(&opts.TeammateMode, "teammate-mode", "", "How to spawn teammates: \"tmux\", \"in-process\", or \"auto\"")
	flags.StringVar(&opts.Teleport, "teleport", "", "Resume a teleport session, optionally specify session ID")
	flags.StringVar(&opts.AgentID, "agent-id", "", "Teammate agent ID")
	flags.StringVar(&opts.AgentName, "agent-name", "", "Teammate display name")
	flags.StringVar(&opts.AgentColor, "agent-color", "", "Teammate UI color")
	flags.StringVar(&opts.AgentType, "agent-type", "", "Custom agent type for this teammate")
	flags.BoolVar(&opts.PlanModeRequired, "plan-mode-required", false, "Require plan mode before implementation")
	flags.StringVar(&opts.ParentSessionID, "parent-session-id", "", "Parent session ID for analytics correlation")
	flags.StringSliceVar(&opts.Tools, "tools", nil, "Specify the list of available tools from the built-in set. Use \"\" to disable all tools, \"default\" to use all tools, or specify tool names (e.g. \"Bash,Edit,Read\").")
	flags.BoolVar(&opts.Verbose, "verbose", false, "Override verbose mode setting from config")
	flags.BoolVarP(&opts.Version, "version", "v", false, "Output the version number")
	flags.BoolVar(&opts.DangerouslySkipPermissions, "dangerously-skip-permissions", false, "Bypass all permission checks. Recommended only for sandboxes with no internet access.")

	flags.Lookup("debug").NoOptDefVal = "true"
	flags.Lookup("resume").NoOptDefVal = "picker"
	flags.Lookup("from-pr").NoOptDefVal = "picker"
	flags.Lookup("remote").NoOptDefVal = "true"
	flags.Lookup("teleport").NoOptDefVal = "true"

	flags.Lookup("append-system-prompt-file").Hidden = true
	flags.Lookup("debug-to-stderr").Hidden = true
	flags.Lookup("enable-auth-status").Hidden = true
	flags.Lookup("from-pr").Hidden = true
	flags.Lookup("init").Hidden = true
	flags.Lookup("init-only").Hidden = true
	flags.Lookup("maintenance").Hidden = true
	flags.Lookup("max-thinking-tokens").Hidden = true
	flags.Lookup("max-turns").Hidden = true
	flags.Lookup("permission-prompt-tool").Hidden = true
	flags.Lookup("remote").Hidden = true
	flags.Lookup("resume-session-at").Hidden = true
	flags.Lookup("rewind-files").Hidden = true
	flags.Lookup("sdk-url").Hidden = true
	flags.Lookup("system-prompt-file").Hidden = true
	flags.Lookup("teleport").Hidden = true
	flags.Lookup("agent-id").Hidden = true
	flags.Lookup("agent-name").Hidden = true
	flags.Lookup("agent-color").Hidden = true
	flags.Lookup("agent-type").Hidden = true
	flags.Lookup("team-name").Hidden = true
	flags.Lookup("teammate-mode").Hidden = true
	flags.Lookup("plan-mode-required").Hidden = true
	flags.Lookup("parent-session-id").Hidden = true
}

// normalizeFlagName maps dashed flag aliases to camel-case names.
func normalizeFlagName(_ *pflag.FlagSet, name string) pflag.NormalizedName {
	switch name {
	case "allowed-tools":
		return "allowedTools"
	case "disallowed-tools":
		return "disallowedTools"
	default:
		return pflag.NormalizedName(name)
	}
}

// doctorCommand validates provider configuration and permissions.
func doctorCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "doctor",
		Short: "Check the health of your Claude Code auto-updater",
		RunE: func(cmd *cobra.Command, args []string) error {
			path := mustProviderPath()
			info, err := os.Stat(path)
			if err != nil {
				return fmt.Errorf("provider config missing at %s", path)
			}
			mode := info.Mode().Perm()
			if mode&0o077 != 0 {
				return fmt.Errorf("provider config permissions too open: %s", mode)
			}
			if _, err := config.LoadProviderConfig(path); err != nil {
				return fmt.Errorf("provider config invalid: %w", err)
			}
			fmt.Fprintf(os.Stdout, "OK: provider config %s\n", path)
			return nil
		},
	}
}

// runRoot orchestrates config loading, session handling, and mode dispatch.
func runRoot(cmd *cobra.Command, opts *options, args []string) error {
	cwd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("get cwd: %w", err)
	}
	if err := validateOptions(opts, cwd); err != nil {
		return err
	}

	providerCfg, err := config.LoadProviderConfig("")
	if err != nil {
		if errors.Is(err, config.ErrProviderConfigMissing) {
			return fmt.Errorf("provider config missing; create %s", mustProviderPath())
		}
		return fmt.Errorf("load provider config: %w", err)
	}
	apiKeySource := "none"
	if providerCfg.APIKey != "" {
		apiKeySource = "config"
	}

	settingSources := splitListArgs(opts.SettingSources)
	settings, err := config.LoadClaudeSettings(cwd, settingSources, opts.Settings)
	if err != nil {
		return fmt.Errorf("load settings: %w", err)
	}

	model := config.ResolveModel(providerCfg, opts.Model, settings.Model)
	if opts.MaxBudgetUSD > 0 {
		if _, ok := providerCfg.Pricing[model]; !ok {
			return fmt.Errorf("pricing missing for model %s; configure pricing to use max-budget-usd", model)
		}
	}

	// Parse permission mode early to determine tool availability.
	permissionMode := parsePermissionMode(opts.PermissionMode)
	if opts.DangerouslySkipPermissions && !opts.AllowDangerouslySkipPermissions {
		return fmt.Errorf("dangerously-skip-permissions requires --allow-dangerously-skip-permissions")
	}
	if opts.DangerouslySkipPermissions {
		permissionMode = tools.PermissionBypass
	}

	store, err := session.NewStore()
	if err != nil {
		return err
	}

	sessionID, history, err := resolveSession(store, cwd, opts)
	if err != nil {
		return err
	}

	rootDirs := append([]string{cwd}, opts.AddDirs...)
	sandbox := tools.NewSandbox(rootDirs)

	availableTools, _, err := buildTools(opts, sandbox, cwd, store, sessionID, permissionMode)
	if err != nil {
		return err
	}

	client := openai.NewClient(providerCfg.APIBaseURL, providerCfg.APIKey, time.Duration(providerCfg.TimeoutMS)*time.Millisecond)
	runner := &agent.Runner{
		Client:       client,
		ToolRunner:   availableTools,
		ToolContext:  tools.ToolContext{Sandbox: sandbox, CWD: cwd, SessionID: sessionID, Store: store},
		Permissions:  tools.Permissions{Mode: permissionMode},
		MaxTurns:     opts.MaxTurns,
		Pricing:      providerCfg.Pricing,
		MaxBudgetUSD: opts.MaxBudgetUSD,
	}

	// Build a base system prompt and apply overrides.
	systemPrompt := resolveSystemPrompt(opts, runner)

	// Configure Task tool execution with a conservative recursion limit.
	runner.ToolContext.TaskMaxDepth = defaultTaskMaxDepth
	runner.ToolContext.TaskExecutor = buildTaskExecutor(runner, opts, model)
	runner.ToolContext.TaskManager = tools.NewTaskManager()

	// Dispatch to print or interactive mode.
	if opts.Print {
		return runPrintMode(cmd, opts, runner, history, systemPrompt, model, sessionID, store, settings, apiKeySource)
	}
	return runInteractive(opts, runner, history, systemPrompt, model, sessionID, store)
}

// mustProviderPath returns the default config path or a fallback placeholder.
func mustProviderPath() string {
	path, err := config.ProviderConfigPath()
	if err != nil {
		return "~/.openclaude/config.json"
	}
	return path
}

// validateOptions enforces Claude Code compatibility constraints and loads file-based flags.
func validateOptions(opts *options, cwd string) error {
	if err := applyPromptFileOverrides(opts, cwd); err != nil {
		return err
	}
	if err := validateFormatOptions(opts); err != nil {
		return err
	}
	if err := validateSessionOptions(opts); err != nil {
		return err
	}
	if err := validateUnsupportedOptions(opts); err != nil {
		return err
	}
	warnNoopOptions(opts)
	return nil
}

// applyPromptFileOverrides reads system prompt content from file flags.
func applyPromptFileOverrides(opts *options, cwd string) error {
	if opts.SystemPromptFile != "" && opts.SystemPrompt != "" {
		return fmt.Errorf("Error: Cannot use both --system-prompt and --system-prompt-file. Please use only one.")
	}
	if opts.SystemPromptFile != "" {
		prompt, err := readPromptFile(cwd, opts.SystemPromptFile, "System prompt")
		if err != nil {
			return err
		}
		opts.SystemPrompt = prompt
	}
	if opts.AppendSystemPromptFile != "" && opts.AppendSystemPrompt != "" {
		return fmt.Errorf("Error: Cannot use both --append-system-prompt and --append-system-prompt-file. Please use only one.")
	}
	if opts.AppendSystemPromptFile != "" {
		prompt, err := readPromptFile(cwd, opts.AppendSystemPromptFile, "Append system prompt")
		if err != nil {
			return err
		}
		opts.AppendSystemPrompt = prompt
	}
	return nil
}

// readPromptFile resolves a prompt path and returns its contents.
func readPromptFile(cwd string, path string, label string) (string, error) {
	resolved, err := resolvePath(cwd, path)
	if err != nil {
		return "", err
	}
	content, err := os.ReadFile(resolved)
	if err != nil {
		return "", fmt.Errorf("Error: %s file not found: %s", label, resolved)
	}
	return string(content), nil
}

// resolvePath expands ~ and resolves relative paths against the current working directory.
func resolvePath(cwd string, path string) (string, error) {
	if strings.HasPrefix(path, "~") {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("resolve home dir: %w", err)
		}
		if path == "~" {
			path = home
		} else if strings.HasPrefix(path, "~/") {
			path = filepath.Join(home, path[2:])
		}
	}
	if !filepath.IsAbs(path) {
		path = filepath.Join(cwd, path)
	}
	return filepath.Clean(path), nil
}

// validateFormatOptions mirrors Claude Code's format validation rules.
func validateFormatOptions(opts *options) error {
	if opts.InputFormat != "text" && opts.InputFormat != "stream-json" {
		return fmt.Errorf("Error: Invalid input format %q.", opts.InputFormat)
	}
	if opts.OutputFormat != "text" && opts.OutputFormat != "json" && opts.OutputFormat != "stream-json" {
		return fmt.Errorf("Error: Invalid output format %q.", opts.OutputFormat)
	}
	if opts.InputFormat == "stream-json" && opts.OutputFormat != "stream-json" {
		return fmt.Errorf("Error: --input-format=stream-json requires output-format=stream-json.")
	}
	if opts.SDKURL != "" && (opts.InputFormat != "stream-json" || opts.OutputFormat != "stream-json") {
		return fmt.Errorf("Error: --sdk-url requires both --input-format=stream-json and --output-format=stream-json.")
	}
	if opts.ReplayUserMessages && (opts.InputFormat != "stream-json" || opts.OutputFormat != "stream-json") {
		return fmt.Errorf("Error: --replay-user-messages requires both --input-format=stream-json and --output-format=stream-json.")
	}
	if opts.IncludePartialMessages && (!opts.Print || opts.OutputFormat != "stream-json") {
		return fmt.Errorf("Error: --include-partial-messages requires --print and --output-format=stream-json.")
	}
	if opts.NoSessionPersistence && !opts.Print {
		return fmt.Errorf("Error: --no-session-persistence can only be used with --print mode.")
	}
	if opts.OutputFormat == "stream-json" && opts.Print && !opts.Verbose {
		return fmt.Errorf("Error: When using --print, --output-format=stream-json requires --verbose")
	}
	if !opts.Print {
		if opts.InputFormat != "text" {
			return fmt.Errorf("Error: --input-format only works with --print.")
		}
		if opts.OutputFormat != "text" {
			return fmt.Errorf("Error: --output-format only works with --print.")
		}
	}
	return nil
}

// validateSessionOptions enforces session-id and resume/continue compatibility.
func validateSessionOptions(opts *options) error {
	if opts.SessionID != "" {
		if _, err := uuid.Parse(opts.SessionID); err != nil {
			return fmt.Errorf("Error: --session-id must be a valid UUID.")
		}
	}
	if opts.SessionID != "" && (opts.Continue || opts.Resume != "") && !opts.ForkSession {
		return fmt.Errorf("Error: --session-id can only be used with --continue or --resume if --fork-session is also specified.")
	}
	return nil
}

// validateUnsupportedOptions rejects flags that OpenClaude cannot emulate yet.
func validateUnsupportedOptions(opts *options) error {
	if opts.Chrome || opts.NoChrome {
		return unsupportedFlagError("--chrome/--no-chrome", "Claude in Chrome integration is not supported.")
	}
	if opts.IDE {
		return unsupportedFlagError("--ide", "IDE integration is not supported.")
	}
	if opts.FromPR != "" {
		return unsupportedFlagError("--from-pr", "PR-linked sessions are not supported.")
	}
	if opts.Remote != "" {
		return unsupportedFlagError("--remote", "Remote sessions are not supported.")
	}
	if opts.Teleport != "" {
		return unsupportedFlagError("--teleport", "Teleport sessions are not supported.")
	}
	if opts.PermissionPromptTool != "" {
		return unsupportedFlagError("--permission-prompt-tool", "Permission prompt tools are not supported.")
	}
	if len(opts.PluginDir) > 0 {
		return unsupportedFlagError("--plugin-dir", "Plugin loading is not supported.")
	}
	if len(opts.MCPConfig) > 0 || opts.StrictMCPConfig {
		return unsupportedFlagError("--mcp-config/--strict-mcp-config", "MCP configuration is not supported.")
	}
	if opts.JSONSchema != "" {
		return unsupportedFlagError("--json-schema", "Structured output validation is not supported.")
	}
	if opts.AgentsJSON != "" || opts.Agent != "" {
		return unsupportedFlagError("--agent/--agents", "Custom agents are not supported.")
	}
	if opts.AgentID != "" || opts.AgentName != "" || opts.TeamName != "" || opts.AgentColor != "" || opts.AgentType != "" || opts.TeammateMode != "" || opts.PlanModeRequired || opts.ParentSessionID != "" {
		return unsupportedFlagError("--agent-id/--team-name/--teammate-mode", "Teammate coordination flags are not supported.")
	}
	if len(opts.FileSpecs) > 0 {
		return unsupportedFlagError("--file", "File resource downloads are not supported.")
	}
	if opts.Init || opts.InitOnly || opts.Maintenance {
		return unsupportedFlagError("--init/--init-only/--maintenance", "Setup hook triggers are not supported.")
	}
	if opts.ResumeSessionAt != "" {
		return unsupportedFlagError("--resume-session-at", "Partial resume is not supported.")
	}
	if opts.RewindFiles != "" {
		return unsupportedFlagError("--rewind-files", "Rewind files is not supported.")
	}
	if opts.SDKURL != "" {
		return unsupportedFlagError("--sdk-url", "Remote SDK streaming is not supported.")
	}
	return nil
}

// unsupportedFlagError formats a consistent unsupported-flag error message.
func unsupportedFlagError(flag string, hint string) error {
	if hint == "" {
		return fmt.Errorf("Error: %s is not supported in OpenClaude.", flag)
	}
	return fmt.Errorf("Error: %s is not supported in OpenClaude. %s", flag, hint)
}

// warnNoopOptions surfaces no-op flags without failing execution.
func warnNoopOptions(opts *options) {
	if opts.MCPDebug {
		fmt.Fprintln(os.Stderr, "Warning: --mcp-debug is deprecated and has no effect in OpenClaude.")
	}
	if opts.Debug != "" || opts.DebugFile != "" || opts.DebugToStderr {
		fmt.Fprintln(os.Stderr, "Warning: Debug flags are accepted but not yet implemented in OpenClaude.")
	}
}

// resolveSession determines session id and loads history, if any.
func resolveSession(store *session.Store, cwd string, opts *options) (string, []openai.Message, error) {
	var (
		baseSessionID string
		history       []openai.Message
	)
	projectHash := session.ProjectHash(cwd)
	if opts.Resume != "" {
		if opts.Resume == "picker" {
			picked, err := pickSession(store)
			if err != nil {
				return "", nil, err
			}
			if picked == "" {
				return "", nil, errors.New("no session selected")
			}
			baseSessionID = picked
		} else {
			baseSessionID = opts.Resume
		}
	} else if opts.Continue {
		lastID, err := store.LoadLastSession(projectHash)
		if err == nil && lastID != "" {
			baseSessionID = lastID
		}
	}

	if baseSessionID != "" {
		var err error
		history, err = loadSessionMessages(store, baseSessionID)
		if err != nil {
			return "", nil, err
		}
	}

	targetSessionID := opts.SessionID
	if targetSessionID == "" {
		if baseSessionID != "" && !opts.ForkSession {
			targetSessionID = baseSessionID
		} else {
			targetSessionID = uuid.New().String()
		}
	}

	if baseSessionID != "" && targetSessionID != baseSessionID {
		if err := store.CloneSession(baseSessionID, targetSessionID); err != nil {
			return "", nil, err
		}
	}

	return targetSessionID, history, nil
}

// pickSession shows a small interactive chooser for recent sessions.
func pickSession(store *session.Store) (string, error) {
	ids, err := store.ListSessions(10)
	if err != nil {
		return "", err
	}
	if len(ids) == 0 {
		return "", errors.New("no sessions available")
	}
	fmt.Fprintln(os.Stdout, "Select a session:")
	for i, id := range ids {
		fmt.Fprintf(os.Stdout, "%d) %s\n", i+1, id)
	}
	fmt.Fprint(os.Stdout, "Enter number: ")
	reader := bufio.NewReader(os.Stdin)
	line, err := reader.ReadString('\n')
	if err != nil {
		return "", err
	}
	line = strings.TrimSpace(line)
	if line == "" {
		return "", nil
	}
	var index int
	if _, err := fmt.Sscanf(line, "%d", &index); err != nil {
		return "", fmt.Errorf("invalid selection")
	}
	if index < 1 || index > len(ids) {
		return "", fmt.Errorf("selection out of range")
	}
	return ids[index-1], nil
}

// buildTools constructs the tool runner based on CLI filters.
func buildTools(
	opts *options,
	sandbox *tools.Sandbox,
	cwd string,
	store *session.Store,
	sessionID string,
	mode tools.PermissionMode,
) (*tools.Runner, []string, error) {
	if mode == tools.PermissionPlan {
		return nil, nil, nil
	}

	toolSet := tools.DefaultTools()

	// Handle explicit tool set selection.
	toolsArg := splitListArgs(opts.Tools)
	if len(opts.Tools) == 0 {
		toolsArg = []string{"default"}
	}
	if len(toolsArg) == 1 && strings.TrimSpace(toolsArg[0]) == "" {
		return nil, nil, nil
	}
	if len(toolsArg) != 1 || strings.ToLower(strings.TrimSpace(toolsArg[0])) != "default" {
		allowed := normalizeToolList(toolsArg)
		filtered, err := tools.FilterTools(toolSet, allowed, nil)
		if err != nil {
			return nil, nil, err
		}
		toolSet = filtered
	}

	allowedTools := normalizeToolList(splitListArgs(opts.AllowedTools))
	disallowedTools := normalizeToolList(splitListArgs(opts.DisallowedTools))
	filtered, err := tools.FilterTools(toolSet, allowedTools, disallowedTools)
	if err != nil {
		return nil, nil, err
	}

	runner := tools.NewRunner(filtered)
	names := make([]string, 0, len(runner.Tools))
	for name := range runner.Tools {
		names = append(names, name)
	}
	return runner, names, nil
}

// runPrintMode handles one-shot requests and prints output to stdout.
func runPrintMode(
	cmd *cobra.Command,
	opts *options,
	runner *agent.Runner,
	history []openai.Message,
	systemPrompt string,
	model string,
	sessionID string,
	store *session.Store,
	settings *config.Settings,
	apiKeySource string,
) error {
	if opts.OutputFormat == "stream-json" {
		return runPrintModeStreamJSON(cmd, opts, runner, history, systemPrompt, model, sessionID, store, settings, apiKeySource)
	}

	inputMessages, err := readInputMessages(cmd, opts)
	if err != nil {
		return err
	}

	messages := append(history, inputMessages...)
	messages = ensureSystem(messages, systemPrompt)
	runner.AuthorizeTool = func(name string, args json.RawMessage) (bool, error) {
		return false, fmt.Errorf("tool %s requires confirmation in print mode", name)
	}

	startTime := time.Now()
	modelUsed := model
	result, err := runner.Run(context.Background(), messages, "", model, runner.ToolRunner != nil)
	if err != nil {
		if opts.FallbackModel != "" && isRetryableError(err) {
			modelUsed = opts.FallbackModel
			result, err = runner.Run(context.Background(), messages, "", opts.FallbackModel, runner.ToolRunner != nil)
		}
	}
	if err != nil {
		if opts.OutputFormat == "stream-json" {
			return writeStreamJSONError(err, opts, inputMessages, sessionID, modelUsed, time.Since(startTime))
		}
		return err
	}

	if !opts.NoSessionPersistence {
		newMessages := result.Messages
		if len(history) > 0 && len(result.Messages) >= len(history) {
			newMessages = result.Messages[len(history):]
		}
		if err := persistSession(store, sessionID, newMessages, result.Events); err != nil {
			return err
		}
		_ = store.SaveLastSession(session.ProjectHash(mustCwd()), sessionID)
	}

	return writeOutput(
		opts.OutputFormat,
		result,
		opts.ReplayUserMessages,
		opts.IncludePartialMessages,
		string(runner.Permissions.Mode),
		sessionID,
		modelUsed,
		opts,
		runner,
		settings,
		apiKeySource,
	)
}

// runPrintModeStreamJSON handles print mode with streaming JSON output.
func runPrintModeStreamJSON(
	cmd *cobra.Command,
	opts *options,
	runner *agent.Runner,
	history []openai.Message,
	systemPrompt string,
	model string,
	sessionID string,
	store *session.Store,
	settings *config.Settings,
	apiKeySource string,
) (returnErr error) {
	// Claude Code requires --verbose when streaming JSON in print mode.
	if !opts.Verbose {
		return fmt.Errorf("Error: When using --print, --output-format=stream-json requires --verbose")
	}

	var (
		inputMessages []openai.Message
		streamInput   *streamJSONInput
		err           error
	)
	// Parse stream-json input when requested to capture control requests and UUIDs.
	if opts.InputFormat == "stream-json" {
		streamInput, err = readStreamInputWithControl(os.Stdin)
		if err != nil {
			return err
		}
		inputMessages = streamInput.Messages
	} else {
		inputMessages, err = readInputMessages(cmd, opts)
		if err != nil {
			return err
		}
	}

	// Use a recorder in print mode so replay-user-messages can emit exact JSON lines.
	outputWriter := io.Writer(os.Stdout)
	if !opts.NoSessionPersistence && store != nil {
		outputWriter = newStreamJSONRecorder(os.Stdout, store, sessionID)
	}
	writer := streamjson.NewWriter(outputWriter)
	streamed := false
	modelUsed := model
	authStatusEmitted := false
	hookEmitter := newStreamJSONHookEmitter(writer, sessionID, opts.HookConfig)
	var keepAlive *keepAliveEmitter

	// Ensure keep-alive emissions are stopped before returning.
	defer func() {
		if keepAlive == nil {
			return
		}
		if err := keepAlive.Stop(); err != nil && returnErr == nil {
			returnErr = err
		}
	}()

	// Replay input control responses before initialization when requested.
	if opts.ReplayUserMessages && streamInput != nil {
		for _, response := range streamInput.ControlResponses {
			controlEvent := streamjson.ControlResponseEvent{
				Type:     "control_response",
				Response: response,
			}
			if err := writer.Write(controlEvent); err != nil {
				return err
			}
		}
	}

	// Apply control requests before building the system:init event.
	if streamInput != nil {
		modelUsed, authStatusEmitted, err = applyStreamJSONControlRequests(streamInput, writer, opts, runner, settings, sessionID, modelUsed)
		if err != nil {
			return err
		}
	}

	// Recompute the system prompt after any control-request overrides.
	systemPrompt = resolveSystemPrompt(opts, runner)
	messages := append(history, inputMessages...)
	messages = ensureSystem(messages, systemPrompt)
	runner.AuthorizeTool = func(name string, args json.RawMessage) (bool, error) {
		return false, fmt.Errorf("tool %s requires confirmation in print mode", name)
	}

	initEvent := buildSystemInitEvent(opts, runner, modelUsed, sessionID, settings, apiKeySource)
	if err := writer.Write(initEvent); err != nil {
		return err
	}
	if opts.EnableAuthStatus && !authStatusEmitted {
		if err := emitAuthStatus(writer, sessionID, false, "", ""); err != nil {
			return err
		}
		authStatusEmitted = true
	}

	replayedStoredUsers := false
	if opts.ReplayUserMessages && !opts.NoSessionPersistence {
		replayedStoredUsers, err = replayStoredStreamJSON(store, sessionID, outputWriter)
		if err != nil {
			return err
		}
	}

	// Start keep-alive emissions after initialization output and replay are sent.
	keepAlive = startKeepAlive(writer, time.Second)

	// Replay user messages using stable UUIDs for stream-json consumers.
	if opts.ReplayUserMessages {
		if !replayedStoredUsers {
			for _, msg := range history {
				if msg.Role != "user" {
					continue
				}
				userEvent := streamjson.UserEvent{
					Type:            "user",
					Message:         streamjson.BuildUserMessage(msg),
					SessionID:       sessionID,
					ParentToolUseID: nil,
					UUID:            streamjson.NewUUID(),
					IsReplay:        true,
					IsSynthetic:     false,
				}
				if err := writer.Write(userEvent); err != nil {
					return err
				}
			}
		}
		if streamInput != nil {
			for _, user := range streamInput.UserMessages {
				uuid := user.UUID
				if uuid == "" {
					uuid = streamjson.NewUUID()
				}
				userEvent := streamjson.UserEvent{
					Type:            "user",
					Message:         streamjson.BuildUserMessage(user.Message),
					SessionID:       sessionID,
					ParentToolUseID: nil,
					UUID:            uuid,
					IsReplay:        true,
					IsSynthetic:     user.IsSynthetic,
				}
				if err := writer.Write(userEvent); err != nil {
					return err
				}
			}
		} else {
			for _, msg := range inputMessages {
				if msg.Role != "user" {
					continue
				}
				userEvent := streamjson.UserEvent{
					Type:            "user",
					Message:         streamjson.BuildUserMessage(msg),
					SessionID:       sessionID,
					ParentToolUseID: nil,
					UUID:            streamjson.NewUUID(),
					IsReplay:        true,
					IsSynthetic:     false,
				}
				if err := writer.Write(userEvent); err != nil {
					return err
				}
			}
		}
	}

	startTime := time.Now()

	emitter := streamjson.NewOpenAIStreamEmitter(writer, opts.IncludePartialMessages, sessionID)
	callbacks := buildStreamCallbacks(emitter, writer, sessionID, &streamed, hookEmitter)

	result, err := runner.RunStream(context.Background(), messages, "", modelUsed, runner.ToolRunner != nil, callbacks)
	if err != nil && opts.FallbackModel != "" && isRetryableError(err) && !streamed {
		modelUsed = opts.FallbackModel
		emitter = streamjson.NewOpenAIStreamEmitter(writer, opts.IncludePartialMessages, sessionID)
		callbacks = buildStreamCallbacks(emitter, writer, sessionID, &streamed, hookEmitter)
		result, err = runner.RunStream(
			context.Background(),
			messages,
			"",
			opts.FallbackModel,
			runner.ToolRunner != nil,
			callbacks,
		)
	}
	if err != nil {
		return writeStreamJSONErrorResult(writer, err, sessionID, modelUsed, time.Since(startTime))
	}

	if !opts.NoSessionPersistence {
		newMessages := result.Messages
		if len(history) > 0 && len(result.Messages) >= len(history) {
			newMessages = result.Messages[len(history):]
		}
		if err := persistSession(store, sessionID, newMessages, result.Events); err != nil {
			return err
		}
		_ = store.SaveLastSession(session.ProjectHash(mustCwd()), sessionID)
	}

	return writeStreamJSONResult(writer, result, sessionID, modelUsed)
}

// runInteractive reads prompts from stdin and maintains a live session.
func runInteractive(
	opts *options,
	runner *agent.Runner,
	history []openai.Message,
	systemPrompt string,
	model string,
	sessionID string,
	store *session.Store,
) error {
	return runInteractiveTUI(opts, runner, history, systemPrompt, model, sessionID, store)
}

// buildStreamCallbacks wires stream-json emission into the streaming agent loop.
func buildStreamCallbacks(
	emitter *streamjson.OpenAIStreamEmitter,
	writer *streamjson.Writer,
	sessionID string,
	streamed *bool,
	hookEmitter *streamJSONHookEmitter,
) *agent.StreamCallbacks {
	toolUseIDs := []string{}
	return &agent.StreamCallbacks{
		OnStreamStart: func(model string) error {
			emitter.Begin(model)
			return nil
		},
		OnStreamEvent: func(event openai.StreamResponse) error {
			if err := emitter.Handle(event); err != nil {
				return err
			}
			if emitter.Streamed() {
				*streamed = true
			}
			return nil
		},
		OnToolCall: func(event agent.ToolEvent) error {
			// Emit pre-tool hook events before tool progress starts.
			if hookEmitter != nil {
				if err := hookEmitter.EmitPreToolUse(event.ToolName); err != nil {
					return err
				}
			}
			progressEvent := streamjson.ProgressEvent{
				Type: "progress",
				Data: streamjson.ProgressData{
					Type:     "tool_progress",
					ToolName: event.ToolName,
					Status:   "started",
					Message:  fmt.Sprintf("Starting tool %s", event.ToolName),
				},
				SessionID:       sessionID,
				ParentToolUseID: event.ToolID,
				UUID:            streamjson.NewUUID(),
			}
			if err := writer.Write(progressEvent); err != nil {
				return err
			}
			if event.ToolID != "" {
				toolUseIDs = append(toolUseIDs, event.ToolID)
			}
			*streamed = true
			return nil
		},
		OnStreamComplete: func(summary agent.StreamSummary) error {
			message, ok, err := emitter.Finalize()
			if err != nil {
				return err
			}
			if !ok {
				message = streamjson.BuildAssistantMessage(summary.Message)
			}
			stopReason := mapFinishReasonToStopReason(summary.FinishReason)
			usage := streamjson.NewEmptyMessageUsage("")
			if summary.HasUsage {
				usage = streamjson.NewMessageUsageFromOpenAI(summary.Usage, "")
			}
			message = buildAssistantMessageEnvelope(message, summary.Model, stopReason, usage)
			assistantEvent := streamjson.AssistantEvent{
				Type:            "assistant",
				Message:         message,
				SessionID:       sessionID,
				ParentToolUseID: nil,
				UUID:            streamjson.NewUUID(),
				Error:           "",
			}
			if err := writer.Write(assistantEvent); err != nil {
				return err
			}
			*streamed = true
			return nil
		},
		OnToolResult: func(event agent.ToolEvent, _ openai.Message) error {
			// Emit post-tool hook events after execution finishes.
			if hookEmitter != nil {
				var err error
				if event.IsError {
					err = hookEmitter.EmitPostToolUseFailure(event.ToolName)
				} else {
					err = hookEmitter.EmitPostToolUse(event.ToolName)
				}
				if err != nil {
					return err
				}
			}
			progressEvent := streamjson.ProgressEvent{
				Type: "progress",
				Data: streamjson.ProgressData{
					Type:     "tool_progress",
					ToolName: event.ToolName,
					Status:   "completed",
					Message:  fmt.Sprintf("Completed tool %s", event.ToolName),
				},
				SessionID:       sessionID,
				ParentToolUseID: event.ToolID,
				UUID:            streamjson.NewUUID(),
			}
			if err := writer.Write(progressEvent); err != nil {
				return err
			}
			userEvent := streamjson.UserEvent{
				Type: "user",
				Message: streamjson.BuildToolResultMessage(
					event.ToolID,
					event.Result,
					event.IsError,
				),
				SessionID:       sessionID,
				ParentToolUseID: nil,
				UUID:            streamjson.NewUUID(),
				IsReplay:        false,
				IsSynthetic:     false,
			}
			if err := writer.Write(userEvent); err != nil {
				return err
			}
			if event.ToolID != "" {
				summaryEvent := streamjson.ToolUseSummaryEvent{
					Type:                "tool_use_summary",
					Summary:             buildToolUseSummary(event),
					PrecedingToolUseIDs: append([]string(nil), toolUseIDs...),
					SessionID:           sessionID,
					UUID:                streamjson.NewUUID(),
				}
				if err := writer.Write(summaryEvent); err != nil {
					return err
				}
			}
			*streamed = true
			return nil
		},
	}
}

// buildToolUseSummary returns a compact summary for tool usage events.
func buildToolUseSummary(event agent.ToolEvent) string {
	if event.ToolName == "" {
		return "Tool completed"
	}
	if event.IsError {
		return fmt.Sprintf("Tool %s failed", event.ToolName)
	}
	return fmt.Sprintf("Tool %s completed", event.ToolName)
}

// buildSystemInitEvent constructs the initial stream-json system event.
func buildSystemInitEvent(opts *options, runner *agent.Runner, model string, sessionID string, settings *config.Settings, apiKeySource string) streamjson.SystemInitEvent {
	betas := opts.Betas
	if betas == nil {
		betas = []string{}
	}
	return streamjson.SystemInitEvent{
		Type:              "system",
		Subtype:           "init",
		CWD:               mustCwd(),
		SessionID:         sessionID,
		Tools:             listToolNames(runner),
		MCPServers:        []any{},
		Model:             model,
		PermissionMode:    string(runner.Permissions.Mode),
		SlashCommands:     listSlashCommands(opts),
		APIKeySource:      apiKeySource,
		Betas:             betas,
		ClaudeCodeVersion: version,
		OutputStyle:       resolveOutputStyle(settings),
		Agents:            listAgentNames(opts),
		Skills:            listSkillNames(opts, settings),
		Plugins:           listPluginDescriptors(opts, settings),
		UUID:              streamjson.NewUUID(),
	}
}

// listToolNames returns tool names in the configured ordering.
func listToolNames(runner *agent.Runner) []string {
	if runner == nil || runner.ToolRunner == nil {
		return []string{}
	}
	return runner.ToolRunner.ToolNames()
}

// readInputMessages parses prompt input for print mode.
func readInputMessages(cmd *cobra.Command, opts *options) ([]openai.Message, error) {
	if opts.InputFormat == "stream-json" {
		return readStreamInput(os.Stdin)
	}

	prompt := strings.TrimSpace(strings.Join(cmd.Flags().Args(), " "))
	if prompt == "" {
		input, err := io.ReadAll(os.Stdin)
		if err != nil {
			return nil, fmt.Errorf("read stdin: %w", err)
		}
		prompt = strings.TrimSpace(string(input))
	}
	if prompt == "" {
		return nil, errors.New("prompt is required")
	}
	return []openai.Message{{Role: "user", Content: prompt}}, nil
}

// readStreamInput consumes stream-json input into user messages.
func readStreamInput(reader io.Reader) ([]openai.Message, error) {
	parsed, err := readStreamInputWithControl(reader)
	if err != nil {
		return nil, err
	}
	return parsed.Messages, nil
}

// parseStreamMessage extracts a user message from stream-json events.
func parseStreamMessage(payload map[string]any) (openai.Message, bool) {
	// Support direct role/content payloads.
	if role, ok := payload["role"].(string); ok {
		if role == "user" {
			content := streamjson.ExtractText(payload["content"])
			return openai.Message{Role: "user", Content: content}, true
		}
	}

	// Support stream-json envelope with a message field.
	if msg, ok := payload["message"].(map[string]any); ok {
		role, _ := msg["role"].(string)
		if role == "user" {
			content := streamjson.ExtractText(msg["content"])
			return openai.Message{Role: "user", Content: content}, true
		}
	}

	// Support explicit user event envelopes.
	if typ, ok := payload["type"].(string); ok {
		switch typ {
		case "user":
			if msg, ok := payload["message"].(map[string]any); ok {
				content := streamjson.ExtractText(msg["content"])
				return openai.Message{Role: "user", Content: content}, true
			}
		case "user_message":
			content := streamjson.ExtractText(payload["content"])
			return openai.Message{Role: "user", Content: content}, true
		}
	}

	return openai.Message{}, false
}

// persistSession writes new messages and tool events to disk.
func persistSession(store *session.Store, sessionID string, messages []openai.Message, events []agent.ToolEvent) error {
	for _, message := range messages {
		event := map[string]any{
			"type":    "message",
			"message": message,
		}
		if err := store.AppendEvent(sessionID, event); err != nil {
			return err
		}
	}
	for _, event := range events {
		if err := store.AppendEvent(sessionID, event); err != nil {
			return err
		}
	}
	return nil
}

// loadSessionMessages returns previously stored messages for a session.
func loadSessionMessages(store *session.Store, sessionID string) ([]openai.Message, error) {
	events, err := store.LoadEvents(sessionID)
	if err != nil {
		return nil, err
	}
	var messages []openai.Message
	for _, raw := range events {
		var payload struct {
			Type    string         `json:"type"`
			Message openai.Message `json:"message"`
		}
		if err := json.Unmarshal(raw, &payload); err != nil {
			continue
		}
		if payload.Type == "message" && payload.Message.Role != "" {
			messages = append(messages, payload.Message)
		}
	}
	return messages, nil
}

// writeOutput formats the final response according to the selected format.
func writeOutput(
	format string,
	result *agent.RunResult,
	replayUser bool,
	includePartial bool,
	permissionMode string,
	sessionID string,
	model string,
	opts *options,
	runner *agent.Runner,
	settings *config.Settings,
	apiKeySource string,
) error {
	switch format {
	case "text":
		fmt.Println(formatContent(result.Final.Content))
	case "json":
		payload := map[string]any{
			"session_id": sessionID,
			"model":      model,
			"final":      result.Final.Content,
			"usage":      result.TotalUsage,
			"cost_usd":   result.CostUSD,
		}
		return writeJSON(payload)
	case "stream-json":
		return writeStreamJSON(result, replayUser, includePartial, permissionMode, sessionID, model, opts, runner, settings, apiKeySource)
	default:
		return fmt.Errorf("unsupported output format: %s", format)
	}
	return nil
}

// writeStreamJSON emits stream-json events that mirror Claude Code output.
func writeStreamJSON(
	result *agent.RunResult,
	replayUser bool,
	includePartial bool,
	permissionMode string,
	sessionID string,
	model string,
	opts *options,
	runner *agent.Runner,
	settings *config.Settings,
	apiKeySource string,
) error {
	writer := streamjson.NewWriter(os.Stdout)

	// Emit the init event first to mirror Claude Code's stream-json ordering.
	if opts != nil {
		initEvent := buildSystemInitEvent(opts, runner, model, sessionID, settings, apiKeySource)
		if err := writer.Write(initEvent); err != nil {
			return err
		}
		if opts.EnableAuthStatus {
			if err := emitAuthStatus(writer, sessionID, false, "", ""); err != nil {
				return err
			}
		}
	}

	// Emit an initial status event to communicate the permission mode.
	statusEvent := streamjson.SystemEvent{
		Type:           "system",
		Subtype:        "status",
		Status:         nil,
		PermissionMode: permissionMode,
		SessionID:      sessionID,
		UUID:           streamjson.NewUUID(),
	}
	if err := writer.Write(statusEvent); err != nil {
		return err
	}

	// Build a tool result lookup for error flags.
	toolErrors := make(map[string]bool)
	for _, event := range result.Events {
		if event.Type == "tool_result" && event.ToolID != "" {
			toolErrors[event.ToolID] = event.IsError
		}
	}

	// Emit message events in order.
	for _, msg := range result.Messages {
		switch msg.Role {
		case "system":
			// System messages are not emitted unless explicitly required.
			continue
		case "user":
			if !replayUser {
				// Skip user messages unless replay was requested.
				continue
			}
			userEvent := streamjson.UserEvent{
				Type:            "user",
				Message:         streamjson.BuildUserMessage(msg),
				SessionID:       sessionID,
				ParentToolUseID: nil,
				UUID:            streamjson.NewUUID(),
				IsReplay:        true,
				IsSynthetic:     false,
			}
			if err := writer.Write(userEvent); err != nil {
				return err
			}
		case "assistant":
			// Stream partial text chunks before emitting the final assistant event.
			text := extractMessageText(msg)
			if includePartial {
				for _, event := range streamjson.BuildStreamEventsForText(text, model, sessionID) {
					if err := writer.Write(event); err != nil {
						return err
					}
				}
			}
			// Emit the full assistant message as an Anthropic-style payload.
			stopReason := deriveStopReason(msg)
			usage := streamjson.NewMessageUsageFromOpenAI(result.TotalUsage, "")
			assistantEvent := streamjson.AssistantEvent{
				Type:            "assistant",
				Message:         buildAssistantMessageEnvelope(streamjson.BuildAssistantMessage(msg), model, stopReason, usage),
				SessionID:       sessionID,
				ParentToolUseID: nil,
				UUID:            streamjson.NewUUID(),
				Error:           "",
			}
			if err := writer.Write(assistantEvent); err != nil {
				return err
			}
		case "tool":
			// Tool results are emitted as synthetic user messages with tool_result blocks.
			toolText := formatContent(msg.Content)
			userEvent := streamjson.UserEvent{
				Type: "user",
				Message: streamjson.BuildToolResultMessage(
					msg.ToolCallID,
					toolText,
					toolErrors[msg.ToolCallID],
				),
				SessionID:       sessionID,
				ParentToolUseID: nil,
				UUID:            streamjson.NewUUID(),
				IsReplay:        false,
				IsSynthetic:     false,
			}
			if err := writer.Write(userEvent); err != nil {
				return err
			}
		}
	}

	// Emit the final result event.
	resultEvent := streamjson.ResultEvent{
		Type:              "result",
		Subtype:           "success",
		IsError:           false,
		DurationMS:        result.Duration.Milliseconds(),
		DurationAPIMS:     result.APIDuration.Milliseconds(),
		NumTurns:          result.NumTurns,
		Result:            formatContent(result.Final.Content),
		SessionID:         sessionID,
		TotalCostUSD:      result.CostUSD,
		Usage:             streamjson.NewMessageUsageFromOpenAI(result.TotalUsage, streamjson.StandardServiceTier),
		ModelUsage:        convertModelUsage(model, result.ModelUsage, result.TotalUsage, streamjson.StandardServiceTier),
		PermissionDenials: []any{},
		UUID:              streamjson.NewUUID(),
	}
	return writer.Write(resultEvent)
}

// writeStreamJSONError emits a stream-json error result event.
func writeStreamJSONError(
	err error,
	opts *options,
	inputMessages []openai.Message,
	sessionID string,
	model string,
	duration time.Duration,
) error {
	writer := streamjson.NewWriter(os.Stdout)

	// Emit an initial status event to communicate the permission mode.
	statusEvent := streamjson.SystemEvent{
		Type:           "system",
		Subtype:        "status",
		Status:         nil,
		PermissionMode: opts.PermissionMode,
		SessionID:      sessionID,
		UUID:           streamjson.NewUUID(),
	}
	if writeErr := writer.Write(statusEvent); writeErr != nil {
		return writeErr
	}

	// Optionally replay user messages for stream-json compatibility.
	if opts.ReplayUserMessages {
		for _, msg := range inputMessages {
			if msg.Role != "user" {
				continue
			}
			userEvent := streamjson.UserEvent{
				Type:            "user",
				Message:         streamjson.BuildUserMessage(msg),
				SessionID:       sessionID,
				ParentToolUseID: nil,
				UUID:            streamjson.NewUUID(),
				IsReplay:        true,
				IsSynthetic:     false,
			}
			if writeErr := writer.Write(userEvent); writeErr != nil {
				return writeErr
			}
		}
	}

	if message, ok := authErrorInfo(err); ok {
		return emitAuthErrorEvents(writer, sessionID, message, duration)
	}

	// Translate the error into a Claude Code result subtype.
	subtype, isError, errorsList := mapStreamJSONError(err)
	resultText := ""
	if len(errorsList) > 0 {
		resultText = errorsList[0]
	}
	permissionDenials := extractPermissionDenials(err)
	resultEvent := streamjson.ResultEvent{
		Type:              "result",
		Subtype:           subtype,
		IsError:           isError,
		DurationMS:        duration.Milliseconds(),
		DurationAPIMS:     0,
		NumTurns:          0,
		Result:            resultText,
		SessionID:         sessionID,
		TotalCostUSD:      0,
		Usage:             streamjson.NewEmptyMessageUsage(streamjson.StandardServiceTier),
		ModelUsage:        map[string]streamjson.MessageUsage{model: *streamjson.NewEmptyMessageUsage(streamjson.StandardServiceTier)},
		PermissionDenials: permissionDenials,
		UUID:              streamjson.NewUUID(),
		Errors:            errorsList,
	}
	return writer.Write(resultEvent)
}

// writeStreamJSONResult emits only the terminal stream-json result event.
func writeStreamJSONResult(
	writer *streamjson.Writer,
	result *agent.RunResult,
	sessionID string,
	model string,
) error {
	if writer == nil {
		return fmt.Errorf("stream-json writer is required")
	}
	modelUsage := convertModelUsage(model, result.ModelUsage, result.TotalUsage, streamjson.StandardServiceTier)
	usage := streamjson.NewMessageUsageFromOpenAI(result.TotalUsage, streamjson.StandardServiceTier)
	resultEvent := streamjson.ResultEvent{
		Type:              "result",
		Subtype:           "success",
		IsError:           false,
		DurationMS:        result.Duration.Milliseconds(),
		DurationAPIMS:     result.APIDuration.Milliseconds(),
		NumTurns:          result.NumTurns,
		Result:            formatContent(result.Final.Content),
		SessionID:         sessionID,
		TotalCostUSD:      result.CostUSD,
		Usage:             usage,
		ModelUsage:        modelUsage,
		PermissionDenials: []any{},
		UUID:              streamjson.NewUUID(),
	}
	return writer.Write(resultEvent)
}

// writeStreamJSONErrorResult emits a stream-json error result event without status.
func writeStreamJSONErrorResult(
	writer *streamjson.Writer,
	err error,
	sessionID string,
	model string,
	duration time.Duration,
) error {
	if writer == nil {
		return fmt.Errorf("stream-json writer is required")
	}
	if message, ok := authErrorInfo(err); ok {
		return emitAuthErrorEvents(writer, sessionID, message, duration)
	}
	subtype, isError, errorsList := mapStreamJSONError(err)
	resultText := ""
	if len(errorsList) > 0 {
		resultText = errorsList[0]
	}
	permissionDenials := extractPermissionDenials(err)
	resultEvent := streamjson.ResultEvent{
		Type:              "result",
		Subtype:           subtype,
		IsError:           isError,
		DurationMS:        duration.Milliseconds(),
		DurationAPIMS:     0,
		NumTurns:          0,
		Result:            resultText,
		SessionID:         sessionID,
		TotalCostUSD:      0,
		Usage:             streamjson.NewEmptyMessageUsage(streamjson.StandardServiceTier),
		ModelUsage:        map[string]streamjson.MessageUsage{model: *streamjson.NewEmptyMessageUsage(streamjson.StandardServiceTier)},
		PermissionDenials: permissionDenials,
		UUID:              streamjson.NewUUID(),
		Errors:            errorsList,
	}
	return writer.Write(resultEvent)
}

// buildAssistantMessageEnvelope ensures assistant messages match Claude Code envelopes.
// It fills required fields, adds null container/context fields, and ensures usage is present.
func buildAssistantMessageEnvelope(
	message streamjson.Message,
	model string,
	stopReason string,
	usage *streamjson.MessageUsage,
) streamjson.Message {
	if message.ID == "" {
		message.ID = streamjson.NewUUID()
	}
	if message.Type == "" {
		message.Type = "message"
	}
	if message.Role == "" {
		message.Role = "assistant"
	}
	if message.Model == "" {
		message.Model = model
	}
	if message.StopReason == "" {
		message.StopReason = stopReason
	}
	if message.StopSequence == nil && message.StopReason == "stop_sequence" {
		message.StopSequence = streamjson.StringPointer("")
	}
	if message.Usage == nil {
		if usage == nil {
			message.Usage = streamjson.NewEmptyMessageUsage("")
		} else {
			message.Usage = usage
		}
	}
	if message.Container == nil {
		message.Container = streamjson.NewNullRawMessage()
	}
	if message.ContextManagement == nil {
		message.ContextManagement = streamjson.NewNullRawMessage()
	}
	return message
}

// mapFinishReasonToStopReason converts OpenAI finish reasons into Claude Code stop reasons.
// The mapping normalizes OpenAI "stop" to Claude "end_turn" semantics.
func mapFinishReasonToStopReason(reason string) string {
	switch reason {
	case "stop":
		return "end_turn"
	case "length":
		return "max_tokens"
	case "tool_calls":
		return "tool_use"
	case "":
		return "end_turn"
	default:
		return reason
	}
}

// deriveStopReason picks a best-effort stop reason for non-streaming messages.
// Without stream metadata, tool calls are the only signal of a tool_use stop.
func deriveStopReason(message openai.Message) string {
	if len(message.ToolCalls) > 0 {
		return "tool_use"
	}
	return "end_turn"
}

// convertModelUsage maps OpenAI usage into Claude-style per-model usage.
// The fallback usage is used when the gateway does not provide per-model breakdowns.
func convertModelUsage(
	model string,
	usageMap map[string]openai.Usage,
	fallback openai.Usage,
	serviceTier string,
) map[string]streamjson.MessageUsage {
	if len(usageMap) == 0 {
		if model == "" {
			return map[string]streamjson.MessageUsage{}
		}
		return map[string]streamjson.MessageUsage{model: *streamjson.NewMessageUsageFromOpenAI(fallback, serviceTier)}
	}
	converted := make(map[string]streamjson.MessageUsage, len(usageMap))
	for key, usage := range usageMap {
		converted[key] = *streamjson.NewMessageUsageFromOpenAI(usage, serviceTier)
	}
	return converted
}

// authErrorInfo detects authentication failures and returns the Claude message.
// It recognizes common 401/403 API errors and returns a user-facing prompt.
func authErrorInfo(err error) (string, bool) {
	var apiErr *openai.APIError
	if errors.As(err, &apiErr) && (apiErr.StatusCode == 401 || apiErr.StatusCode == 403) {
		return "Invalid API key · Please run /login", true
	}
	return "", false
}

// emitAuthErrorEvents writes assistant + result events for authentication failures.
// Claude Code surfaces auth failures as a synthetic assistant message plus result event.
func emitAuthErrorEvents(
	writer *streamjson.Writer,
	sessionID string,
	message string,
	duration time.Duration,
) error {
	assistantMessage := streamjson.BuildTextMessage("assistant", message)
	assistantMessage = buildAssistantMessageEnvelope(assistantMessage, "<synthetic>", "stop_sequence", streamjson.NewEmptyMessageUsage(""))
	assistantMessage.StopSequence = streamjson.StringPointer("")
	assistantEvent := streamjson.AssistantEvent{
		Type:            "assistant",
		Message:         assistantMessage,
		SessionID:       sessionID,
		ParentToolUseID: nil,
		UUID:            streamjson.NewUUID(),
		Error:           "authentication_failed",
	}
	if err := writer.Write(assistantEvent); err != nil {
		return err
	}
	resultEvent := streamjson.ResultEvent{
		Type:              "result",
		Subtype:           "success",
		IsError:           true,
		DurationMS:        duration.Milliseconds(),
		DurationAPIMS:     0,
		NumTurns:          1,
		Result:            message,
		SessionID:         sessionID,
		TotalCostUSD:      0,
		Usage:             streamjson.NewEmptyMessageUsage(streamjson.StandardServiceTier),
		ModelUsage:        map[string]streamjson.MessageUsage{},
		PermissionDenials: []any{},
		UUID:              streamjson.NewUUID(),
	}
	return writer.Write(resultEvent)
}

// mapStreamJSONError maps errors into Claude Code result subtypes.
func mapStreamJSONError(err error) (string, bool, []string) {
	switch {
	case errors.Is(err, agent.ErrMaxTurns):
		return "error_max_turns", false, []string{}
	case errors.Is(err, agent.ErrMaxBudget):
		return "error_max_budget_usd", false, []string{}
	default:
		return "error_during_execution", false, []string{err.Error()}
	}
}

// permissionDenial describes a denied tool request for stream-json output.
type permissionDenial struct {
	// ToolName identifies the tool that was denied, when available.
	ToolName string `json:"tool_name,omitempty"`
	// Reason summarizes why the request was denied.
	Reason string `json:"reason"`
}

// extractPermissionDenials builds stream-json permission_denials from an error.
func extractPermissionDenials(err error) []any {
	if err == nil {
		return []any{}
	}
	if errors.Is(err, agent.ErrToolDenied) {
		return []any{permissionDenial{
			ToolName: extractDeniedToolName(err),
			Reason:   "user_denied",
		}}
	}
	if errors.Is(err, agent.ErrPlanMode) {
		return []any{permissionDenial{
			Reason: "plan_mode",
		}}
	}
	return []any{}
}

// extractDeniedToolName pulls the tool identifier from the error text when possible.
func extractDeniedToolName(err error) string {
	if err == nil {
		return ""
	}
	message := err.Error()
	parts := strings.SplitN(message, ":", 2)
	if len(parts) < 2 {
		return ""
	}
	return strings.TrimSpace(parts[1])
}

// extractMessageText returns the message text for partial streaming.
func extractMessageText(message openai.Message) string {
	if text, ok := message.Content.(string); ok {
		return text
	}
	return streamjson.ExtractText(message.Content)
}

// writeJSON writes a single JSON line to stdout.
func writeJSON(payload any) error {
	data, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	fmt.Println(string(data))
	return nil
}

// formatContent normalizes assistant content into a string.
func formatContent(content any) string {
	if text, ok := content.(string); ok {
		return text
	}
	data, err := json.Marshal(content)
	if err != nil {
		return fmt.Sprintf("%v", content)
	}
	return string(data)
}

// ensureSystem injects a system prompt if one is not present.
func ensureSystem(messages []openai.Message, prompt string) []openai.Message {
	if prompt == "" {
		return messages
	}
	if len(messages) > 0 && messages[0].Role == "system" {
		return messages
	}
	system := openai.Message{Role: "system", Content: prompt}
	return append([]openai.Message{system}, messages...)
}

// splitList parses comma/space-separated lists.
func splitList(value string) []string {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}
	parts := strings.FieldsFunc(value, func(r rune) bool {
		return r == ',' || r == ' '
	})
	var list []string
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part != "" {
			list = append(list, part)
		}
	}
	return list
}

// splitListArgs flattens multiple list arguments into a single normalized list.
func splitListArgs(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	var combined []string
	for _, value := range values {
		combined = append(combined, splitList(value)...)
	}
	return combined
}

// buildTaskExecutor wires Task tool execution to a new agent run.
func buildTaskExecutor(runner *agent.Runner, opts *options, baseModel string) tools.TaskExecutor {
	if runner == nil {
		return nil
	}
	return tools.TaskExecutorFunc(func(ctx context.Context, request tools.TaskRequest) (tools.TaskResult, error) {
		if runner.Client == nil {
			return tools.TaskResult{}, fmt.Errorf("task executor requires a client")
		}

		model := resolveTaskModel(request.Model, opts, baseModel)
		if model == "" {
			return tools.TaskResult{}, fmt.Errorf("task model is required")
		}

		systemPrompt := strings.TrimSpace(request.SystemPrompt)
		if systemPrompt == "" {
			systemPrompt = resolveSystemPrompt(opts, runner)
		}

		messages := request.Messages
		if len(messages) == 0 {
			messages = []openai.Message{{Role: "user", Content: request.Prompt}}
		}
		if len(messages) > 0 && messages[0].Role == "system" {
			systemPrompt = ""
		}

		taskRunner := *runner
		taskRunner.ToolContext = runner.ToolContext
		taskRunner.ToolContext.TaskDepth = runner.ToolContext.TaskDepth + 1
		taskRunner.ToolContext.TaskExecutor = runner.ToolContext.TaskExecutor

		if request.MaxTurns > 0 {
			taskRunner.MaxTurns = request.MaxTurns
		} else if taskRunner.MaxTurns <= 0 {
			taskRunner.MaxTurns = defaultTaskMaxTurns
		}

		result, err := taskRunner.Run(ctx, messages, systemPrompt, model, taskRunner.ToolRunner != nil)
		if err != nil {
			return tools.TaskResult{}, err
		}

		meta := map[string]any{
			"num_turns": result.NumTurns,
			"cost_usd":  result.CostUSD,
		}
		return tools.TaskResult{
			Output:   formatContent(result.Final.Content),
			Metadata: meta,
		}, nil
	})
}

// resolveTaskModel picks a model for Task execution.
func resolveTaskModel(requested string, opts *options, baseModel string) string {
	requested = strings.TrimSpace(requested)
	if requested != "" && requested != "default" {
		return requested
	}
	if opts != nil {
		model := strings.TrimSpace(opts.Model)
		if model != "" && model != "default" {
			return model
		}
	}
	return baseModel
}

// normalizeToolList maps CLI tool names to canonical tool identifiers.
// This keeps legacy aliases working while aligning with Claude Code tool names.
func normalizeToolList(names []string) []string {
	var normalized []string
	for _, name := range names {
		switch strings.ToLower(name) {
		case "read":
			normalized = append(normalized, "Read")
		case "view":
			normalized = append(normalized, "Read")
		case "edit":
			normalized = append(normalized, "Edit")
		case "write":
			normalized = append(normalized, "Write")
		case "replace":
			normalized = append(normalized, "Write")
		case "notebookedit", "notebook-edit", "notebook_edit":
			normalized = append(normalized, "NotebookEdit")
		case "bash":
			normalized = append(normalized, "Bash")
		case "search", "websearch", "web-search", "web_search":
			normalized = append(normalized, "WebSearch")
		case "webfetch", "web-fetch", "web_fetch":
			normalized = append(normalized, "WebFetch")
		case "glob":
			normalized = append(normalized, "Glob")
		case "grep":
			normalized = append(normalized, "Grep")
		case "task":
			normalized = append(normalized, "Task")
		case "taskoutput", "task-output", "task_output":
			normalized = append(normalized, "TaskOutput")
		case "taskstop", "task-stop", "task_stop":
			normalized = append(normalized, "TaskStop")
		case "enterplanmode", "enter-plan-mode", "enter_plan_mode":
			normalized = append(normalized, "EnterPlanMode")
		case "exitplanmode", "exit-plan-mode", "exit_plan_mode":
			normalized = append(normalized, "ExitPlanMode")
		case "askuserquestion", "ask-user-question", "ask_user_question":
			normalized = append(normalized, "AskUserQuestion")
		case "skill":
			normalized = append(normalized, "Skill")
		case "todowrite", "todo-write", "todo_write", "todo":
			normalized = append(normalized, "TodoWrite")
		default:
			normalized = append(normalized, name)
		}
	}
	return normalized
}

// parsePermissionMode translates CLI values into internal modes.
func parsePermissionMode(value string) tools.PermissionMode {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "default":
		return tools.PermissionDefault
	case "acceptedits":
		return tools.PermissionAcceptEdits
	case "dontask":
		return tools.PermissionDontAsk
	case "delegate":
		return tools.PermissionDelegate
	case "bypasspermissions":
		return tools.PermissionBypass
	case "plan":
		return tools.PermissionPlan
	default:
		return tools.PermissionDefault
	}
}

// isRetryableError reports whether an error should trigger fallback.
func isRetryableError(err error) bool {
	var apiErr *openai.APIError
	if errors.As(err, &apiErr) {
		return apiErr.StatusCode == 429 || apiErr.StatusCode >= 500
	}
	return false
}

// mustCwd returns cwd or "." if unavailable.
func mustCwd() string {
	cwd, err := os.Getwd()
	if err != nil {
		return "."
	}
	return cwd
}
