package main

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"sort"
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

// version is the CLI build version.
const version = "0.1.0"

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
	AllowedTools string
	// AppendSystemPrompt appends extra system instructions.
	AppendSystemPrompt string
	// Betas adds beta headers in upstream requests.
	Betas []string
	// Continue resumes the most recent session in the current project.
	Continue bool
	// Debug toggles debug output categories.
	Debug string
	// DebugFile writes debug logs to a file path.
	DebugFile string
	// DisableSlashCommands disables slash-command parsing.
	DisableSlashCommands bool
	// DisallowedTools blocks specific tools even if available.
	DisallowedTools string
	// EnableAuthStatus emits auth_status events in stream-json output.
	EnableAuthStatus bool
	// FallbackModel is used on retryable errors in print mode.
	FallbackModel string
	// FileSpecs defines preloaded file resources.
	FileSpecs []string
	// ForkSession controls whether resume forks the session id.
	ForkSession bool
	// IncludePartialMessages toggles partial message streaming in print mode.
	IncludePartialMessages bool
	// HookConfig stores hook definitions from stream-json control requests.
	HookConfig *streamJSONHookConfig
	// InputFormat controls how prompts are read in print mode.
	InputFormat string
	// JSONSchema provides structured output validation schema.
	JSONSchema string
	// MaxBudgetUSD enforces an estimated spend ceiling.
	MaxBudgetUSD float64
	// MaxTurns caps the number of assistant/tool turns.
	MaxTurns int
	// MaxThinkingTokens configures thinking token budgets for compatible models.
	MaxThinkingTokens int
	// Model overrides the default model selection.
	Model string
	// NoSessionPersistence disables saving session history to disk.
	NoSessionPersistence bool
	// OutputFormat controls print mode output encoding.
	OutputFormat string
	// PermissionMode configures tool approval behavior.
	PermissionMode string
	// PluginDir is reserved for future plugin loading.
	PluginDir []string
	// Print enables non-interactive mode.
	Print bool
	// ReplayUserMessages echoes user messages in stream-json output.
	ReplayUserMessages bool
	// Resume resumes a specific session id or the interactive picker.
	Resume string
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
	// Tools defines the available tool set.
	Tools string
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
		Short: "OpenClaude - drop-in CLI replacement for Claude Code",
		RunE: func(cmd *cobra.Command, args []string) error {
			if opts.Version {
				fmt.Println(version)
				return nil
			}
			return runRoot(cmd, opts, args)
		},
	}
	rootCmd.Args = cobra.ArbitraryArgs

	applyFlags(rootCmd.Flags(), opts)

	rootCmd.AddCommand(doctorCommand())
	rootCmd.AddCommand(stubCommand("install"))
	rootCmd.AddCommand(stubCommand("update"))
	rootCmd.AddCommand(stubCommand("mcp"))
	rootCmd.AddCommand(stubCommand("plugin"))
	rootCmd.AddCommand(stubCommand("setup-token"))

	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}

// applyFlags defines all CLI flags with Claude Code-compatible names.
func applyFlags(flags *pflag.FlagSet, opts *options) {
	flags.SetNormalizeFunc(normalizeFlagName)

	flags.StringSliceVar(&opts.AddDirs, "add-dir", nil, "Additional directories to allow tool access to")
	flags.StringVar(&opts.Agent, "agent", "", "Agent for the current session")
	flags.StringVar(&opts.AgentsJSON, "agents", "", "JSON object defining custom agents")
	flags.BoolVar(&opts.AllowDangerouslySkipPermissions, "allow-dangerously-skip-permissions", false, "Allow bypassing permissions")
	flags.StringVar(&opts.AllowedTools, "allowedTools", "", "Allowed tools list")
	flags.StringVar(&opts.AppendSystemPrompt, "append-system-prompt", "", "Append a system prompt")
	flags.StringSliceVar(&opts.Betas, "betas", nil, "Beta headers")
	flags.BoolVarP(&opts.Continue, "continue", "c", false, "Continue the most recent conversation")
	flags.StringVar(&opts.Debug, "debug", "", "Enable debug mode")
	flags.StringVar(&opts.DebugFile, "debug-file", "", "Write debug logs to a file")
	flags.BoolVar(&opts.DisableSlashCommands, "disable-slash-commands", false, "Disable slash commands")
	flags.StringVar(&opts.DisallowedTools, "disallowedTools", "", "Disallowed tools list")
	flags.BoolVar(&opts.EnableAuthStatus, "enable-auth-status", false, "Emit auth_status events in stream-json output")
	flags.StringVar(&opts.FallbackModel, "fallback-model", "", "Fallback model")
	flags.StringSliceVar(&opts.FileSpecs, "file", nil, "File resources to download at startup")
	flags.BoolVar(&opts.ForkSession, "fork-session", false, "Fork session on resume")
	flags.BoolVar(&opts.IncludePartialMessages, "include-partial-messages", false, "Include partial message chunks")
	flags.StringVar(&opts.InputFormat, "input-format", "text", "Input format (text|stream-json)")
	flags.StringVar(&opts.JSONSchema, "json-schema", "", "JSON schema for structured output")
	flags.Float64Var(&opts.MaxBudgetUSD, "max-budget-usd", 0, "Maximum budget in USD")
	flags.IntVar(&opts.MaxTurns, "max-turns", 0, "Maximum number of turns")
	flags.IntVar(&opts.MaxThinkingTokens, "max-thinking-tokens", 0, "Maximum thinking tokens")
	flags.StringVar(&opts.Model, "model", "", "Model for the current session")
	flags.BoolVar(&opts.NoSessionPersistence, "no-session-persistence", false, "Disable session persistence")
	flags.StringVar(&opts.OutputFormat, "output-format", "text", "Output format (text|json|stream-json)")
	flags.StringVar(&opts.PermissionMode, "permission-mode", "default", "Permission mode")
	flags.StringSliceVar(&opts.PluginDir, "plugin-dir", nil, "Load plugins from directories")
	flags.BoolVarP(&opts.Print, "print", "p", false, "Print response and exit")
	flags.BoolVar(&opts.ReplayUserMessages, "replay-user-messages", false, "Replay user messages from stdin")
	flags.StringVarP(&opts.Resume, "resume", "r", "", "Resume a conversation by session ID")
	flags.Lookup("resume").NoOptDefVal = "picker"
	flags.StringVar(&opts.SessionID, "session-id", "", "Use a specific session ID")
	flags.StringSliceVar(&opts.SettingSources, "setting-sources", nil, "Setting sources (user,project,local)")
	flags.StringVar(&opts.Settings, "settings", "", "Settings file path or JSON")
	flags.BoolVar(&opts.StrictMCPConfig, "strict-mcp-config", false, "Strict MCP config")
	flags.StringVar(&opts.SystemPrompt, "system-prompt", "", "System prompt")
	flags.StringVar(&opts.Tools, "tools", "default", "Available tools list")
	flags.BoolVar(&opts.Verbose, "verbose", false, "Verbose output")
	flags.BoolVarP(&opts.Version, "version", "v", false, "Output the version number")
	flags.BoolVar(&opts.DangerouslySkipPermissions, "dangerously-skip-permissions", false, "Bypass permissions")
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

// stubCommand provides a placeholder for unsupported commands.
func stubCommand(name string) *cobra.Command {
	return &cobra.Command{
		Use:   name,
		Short: "Not supported in OpenClaude",
		RunE: func(cmd *cobra.Command, args []string) error {
			fmt.Fprintf(os.Stderr, "%s is not supported in OpenClaude. Use your OpenAI-compatible gateway instead.\n", name)
			os.Exit(2)
			return nil
		},
	}
}

// doctorCommand validates provider configuration and permissions.
func doctorCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "doctor",
		Short: "Check OpenClaude configuration",
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

	providerCfg, err := config.LoadProviderConfig("")
	if err != nil {
		if errors.Is(err, config.ErrProviderConfigMissing) {
			return fmt.Errorf("provider config missing; create %s", mustProviderPath())
		}
		return fmt.Errorf("load provider config: %w", err)
	}

	settingSources := splitList(strings.Join(opts.SettingSources, ","))
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

	// Dispatch to print or interactive mode.
	if opts.Print {
		return runPrintMode(cmd, opts, runner, history, systemPrompt, model, sessionID, store, settings)
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

// resolveSession determines session id and loads history, if any.
func resolveSession(store *session.Store, cwd string, opts *options) (string, []openai.Message, error) {
	if opts.SessionID != "" {
		messages, err := loadSessionMessages(store, opts.SessionID)
		return opts.SessionID, messages, err
	}

	projectHash := session.ProjectHash(cwd)
	if opts.Continue {
		lastID, err := store.LoadLastSession(projectHash)
		if err == nil && lastID != "" {
			messages, err := loadSessionMessages(store, lastID)
			return lastID, messages, err
		}
	}

	if opts.Resume != "" {
		if opts.Resume == "picker" {
			picked, err := pickSession(store)
			if err != nil {
				return "", nil, err
			}
			if picked == "" {
				return "", nil, errors.New("no session selected")
			}
			messages, err := loadSessionMessages(store, picked)
			return picked, messages, err
		}
		messages, err := loadSessionMessages(store, opts.Resume)
		return opts.Resume, messages, err
	}

	id := uuid.New().String()
	return id, nil, nil
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
	toolsArg := strings.TrimSpace(opts.Tools)
	if toolsArg == "" {
		return nil, nil, nil
	}
	if strings.ToLower(toolsArg) != "default" {
		allowed := normalizeToolList(splitList(toolsArg))
		filtered, err := tools.FilterTools(toolSet, allowed, nil)
		if err != nil {
			return nil, nil, err
		}
		toolSet = filtered
	}

	allowedTools := normalizeToolList(splitList(opts.AllowedTools))
	disallowedTools := normalizeToolList(splitList(opts.DisallowedTools))
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
) error {
	if opts.OutputFormat == "stream-json" {
		return runPrintModeStreamJSON(cmd, opts, runner, history, systemPrompt, model, sessionID, store, settings)
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
) (returnErr error) {
	// Claude Code requires --verbose when streaming JSON in print mode.
	if !opts.Verbose {
		return fmt.Errorf("when using --print, --output-format=stream-json requires --verbose")
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

	writer := streamjson.NewWriter(os.Stdout)
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

	initEvent := buildSystemInitEvent(opts, runner, modelUsed, sessionID, settings)
	if err := writer.Write(initEvent); err != nil {
		return err
	}
	if opts.EnableAuthStatus && !authStatusEmitted {
		if err := emitAuthStatus(writer, sessionID, false, "", ""); err != nil {
			return err
		}
		authStatusEmitted = true
	}

	// Start keep-alive emissions after initialization output is sent.
	keepAlive = startKeepAlive(writer, time.Second)

	// Replay user messages using stable UUIDs for stream-json consumers.
	if opts.ReplayUserMessages {
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
	reader := bufio.NewScanner(os.Stdin)
	messages := ensureSystem(history, systemPrompt)

	for {
		fmt.Fprint(os.Stdout, "\n> ")
		if !reader.Scan() {
			break
		}
		line := strings.TrimSpace(reader.Text())
		if line == "" {
			continue
		}
		previousLen := len(messages)
		userMsg := openai.Message{Role: "user", Content: line}
		messages = append(messages, userMsg)
		runner.AuthorizeTool = func(name string, args json.RawMessage) (bool, error) {
			if !runner.Permissions.ShouldPrompt(name) {
				return true, nil
			}
			fmt.Fprintf(os.Stdout, "Allow tool %s? [y/N]: ", name)
			resp, _ := bufio.NewReader(os.Stdin).ReadString('\n')
			resp = strings.TrimSpace(strings.ToLower(resp))
			return resp == "y" || resp == "yes", nil
		}

		result, err := runner.Run(context.Background(), messages, "", model, runner.ToolRunner != nil)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			continue
		}
		messages = result.Messages
		fmt.Fprintln(os.Stdout, formatContent(result.Final.Content))

		if !opts.NoSessionPersistence {
			newMessages := result.Messages
			if previousLen > 0 && len(result.Messages) >= previousLen {
				newMessages = result.Messages[previousLen:]
			}
			if err := persistSession(store, sessionID, newMessages, result.Events); err != nil {
				fmt.Fprintln(os.Stderr, err)
			}
			_ = store.SaveLastSession(session.ProjectHash(mustCwd()), sessionID)
		}
	}
	return nil
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
			assistantEvent := streamjson.AssistantEvent{
				Type:            "assistant",
				Message:         message,
				SessionID:       sessionID,
				ParentToolUseID: nil,
				UUID:            streamjson.NewUUID(),
				Error:           false,
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
func buildSystemInitEvent(opts *options, runner *agent.Runner, model string, sessionID string, settings *config.Settings) streamjson.SystemInitEvent {
	return streamjson.SystemInitEvent{
		Type:              "system",
		Subtype:           "init",
		CWD:               mustCwd(),
		SessionID:         sessionID,
		Tools:             listToolNames(runner),
		MCPServers:        []any{},
		Model:             model,
		PermissionMode:    string(runner.Permissions.Mode),
		SlashCommands:     []string{},
		APIKeySource:      "config",
		Betas:             opts.Betas,
		ClaudeCodeVersion: version,
		OutputStyle:       resolveOutputStyle(settings),
		Agents:            listAgentNames(opts),
		Skills:            listSkillNames(settings),
		Plugins:           listPluginDescriptors(opts, settings),
		UUID:              streamjson.NewUUID(),
	}
}

// listToolNames returns a sorted list of tool names from the runner.
func listToolNames(runner *agent.Runner) []string {
	if runner == nil || runner.ToolRunner == nil {
		return nil
	}
	names := make([]string, 0, len(runner.ToolRunner.Tools))
	for name := range runner.ToolRunner.Tools {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
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
		return nil, nil
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
		return writeStreamJSON(result, replayUser, includePartial, permissionMode, sessionID, model)
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
) error {
	writer := streamjson.NewWriter(os.Stdout)

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
			assistantEvent := streamjson.AssistantEvent{
				Type:            "assistant",
				Message:         streamjson.BuildAssistantMessage(msg),
				SessionID:       sessionID,
				ParentToolUseID: nil,
				UUID:            streamjson.NewUUID(),
				Error:           false,
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
		Usage:             result.TotalUsage,
		ModelUsage:        result.ModelUsage,
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

	// Translate the error into a Claude Code result subtype.
	subtype, isError, errorsList := mapStreamJSONError(err)
	resultEvent := streamjson.ResultEvent{
		Type:              "result",
		Subtype:           subtype,
		IsError:           isError,
		DurationMS:        duration.Milliseconds(),
		DurationAPIMS:     0,
		NumTurns:          0,
		SessionID:         sessionID,
		TotalCostUSD:      0,
		Usage:             openai.Usage{},
		ModelUsage:        map[string]openai.Usage{model: openai.Usage{}},
		PermissionDenials: []any{},
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
	modelUsage := result.ModelUsage
	if modelUsage == nil {
		modelUsage = map[string]openai.Usage{model: result.TotalUsage}
	}
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
		Usage:             result.TotalUsage,
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
	subtype, isError, errorsList := mapStreamJSONError(err)
	resultEvent := streamjson.ResultEvent{
		Type:              "result",
		Subtype:           subtype,
		IsError:           isError,
		DurationMS:        duration.Milliseconds(),
		DurationAPIMS:     0,
		NumTurns:          0,
		SessionID:         sessionID,
		TotalCostUSD:      0,
		Usage:             openai.Usage{},
		ModelUsage:        map[string]openai.Usage{model: openai.Usage{}},
		PermissionDenials: []any{},
		UUID:              streamjson.NewUUID(),
		Errors:            errorsList,
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

// normalizeToolList maps CLI tool names to canonical tool identifiers.
func normalizeToolList(names []string) []string {
	var normalized []string
	for _, name := range names {
		switch strings.ToLower(name) {
		case "read":
			normalized = append(normalized, "Read")
		case "edit":
			normalized = append(normalized, "Edit")
		case "bash":
			normalized = append(normalized, "Bash")
		case "glob":
			normalized = append(normalized, "Glob")
		case "grep":
			normalized = append(normalized, "Grep")
		case "listdir":
			normalized = append(normalized, "ListDir")
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
