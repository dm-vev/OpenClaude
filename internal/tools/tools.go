package tools

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"

	"github.com/openclaude/openclaude/internal/llm/openai"
	"github.com/openclaude/openclaude/internal/session"
)

// ToolContext provides shared context to tool implementations.
type ToolContext struct {
	// Sandbox enforces path allow/deny rules.
	Sandbox *Sandbox
	// CWD is the working directory for command tools.
	CWD string
	// SessionID identifies the current session for backups.
	SessionID string
	// Store persists session artifacts when available.
	Store *session.Store
	// TaskExecutor runs Task tool subtasks when configured.
	TaskExecutor TaskExecutor
	// TaskDepth tracks nested task execution depth.
	TaskDepth int
	// TaskMaxDepth caps nested task execution depth (0 disables nesting).
	TaskMaxDepth int
	// TaskManager tracks async task execution state.
	TaskManager *TaskManager
}

// TaskRequest describes a subtask request issued via the Task tool.
type TaskRequest struct {
	// Prompt holds a single user prompt for the task.
	Prompt string
	// Messages optionally provide a full message history for the task.
	Messages []openai.Message
	// SystemPrompt optionally overrides the default system prompt.
	SystemPrompt string
	// Model overrides the default model when provided.
	Model string
	// MaxTurns overrides the default turn limit for the task.
	MaxTurns int
	// Metadata stores raw task payload fields for auditing.
	Metadata map[string]any
}

// TaskResult captures the output of a subtask execution.
type TaskResult struct {
	// Output is the final assistant text for the task.
	Output string
	// Metadata carries any extra metadata from execution.
	Metadata map[string]any
}

// TaskExecutor runs subtasks for the Task tool.
type TaskExecutor interface {
	ExecuteTask(ctx context.Context, request TaskRequest) (TaskResult, error)
}

// TaskExecutorFunc is a helper to build TaskExecutor instances from functions.
type TaskExecutorFunc func(ctx context.Context, request TaskRequest) (TaskResult, error)

// ExecuteTask calls the wrapped function.
func (fn TaskExecutorFunc) ExecuteTask(ctx context.Context, request TaskRequest) (TaskResult, error) {
	return fn(ctx, request)
}

// ToolResult is the result of a tool invocation.
type ToolResult struct {
	// Content holds the tool output payload.
	Content string
	// IsError reports whether the tool failed.
	IsError bool
}

// Tool defines a callable tool.
type Tool interface {
	Name() string
	Description() string
	Schema() map[string]any
	Run(ctx context.Context, input json.RawMessage, toolCtx ToolContext) (ToolResult, error)
}

// Runner executes tools with validation.
type Runner struct {
	// Tools stores tool implementations keyed by name.
	Tools map[string]Tool
	// Order preserves the deterministic tool ordering for output payloads.
	Order []string
}

// NewRunner constructs a tool runner.
func NewRunner(tools []Tool) *Runner {
	toolMap := make(map[string]Tool, len(tools))
	order := make([]string, 0, len(tools))
	for _, tool := range tools {
		if tool == nil {
			continue
		}
		name := tool.Name()
		if name == "" {
			continue
		}
		if _, exists := toolMap[name]; exists {
			continue
		}
		// Preserve input order while de-duplicating tool names.
		toolMap[name] = tool
		order = append(order, name)
	}
	return &Runner{Tools: toolMap, Order: order}
}

// ToolSpecs returns OpenAI-compatible tool definitions.
func (r *Runner) ToolSpecs() []openai.Tool {
	specs := make([]openai.Tool, 0, len(r.Tools))
	names := r.ToolNames()
	if len(names) == 0 {
		return specs
	}
	// Emit tool specs in the configured order for deterministic payloads.
	for _, name := range names {
		tool, ok := r.Tools[name]
		if !ok {
			continue
		}
		specs = append(specs, openai.Tool{
			Type: "function",
			Function: openai.ToolFunction{
				Name:        tool.Name(),
				Description: tool.Description(),
				Parameters:  tool.Schema(),
			},
		})
	}
	return specs
}

// ToolNames returns the configured tool names in deterministic order.
func (r *Runner) ToolNames() []string {
	if r == nil {
		return nil
	}
	if len(r.Order) > 0 {
		// Copy to avoid mutating the runner's internal order.
		names := make([]string, 0, len(r.Order))
		names = append(names, r.Order...)
		return names
	}
	if len(r.Tools) == 0 {
		return nil
	}
	names := make([]string, 0, len(r.Tools))
	for name := range r.Tools {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// Run executes a tool by name.
func (r *Runner) Run(ctx context.Context, name string, args json.RawMessage, toolCtx ToolContext) (ToolResult, error) {
	tool, ok := r.Tools[name]
	if !ok {
		return ToolResult{IsError: true, Content: fmt.Sprintf("tool not found: %s", name)}, nil
	}
	return tool.Run(ctx, args, toolCtx)
}

// FilterTools applies allow/deny constraints.
func FilterTools(tools []Tool, allowed []string, disallowed []string) ([]Tool, error) {
	allowedSet := toNameSet(allowed)
	disallowedSet := toNameSet(disallowed)

	var filtered []Tool
	for _, tool := range tools {
		name := tool.Name()
		if len(allowedSet) > 0 && !allowedSet[name] {
			continue
		}
		if disallowedSet[name] {
			continue
		}
		filtered = append(filtered, tool)
	}

	if len(filtered) == 0 {
		return nil, errors.New("no tools available after filtering")
	}
	return filtered, nil
}

// toNameSet converts a list of names to a lookup set.
func toNameSet(names []string) map[string]bool {
	set := make(map[string]bool)
	for _, name := range names {
		if name == "" {
			continue
		}
		set[name] = true
	}
	return set
}

// DefaultTools returns the built-in tool set in Claude Code order.
// Unsupported tools are represented as stubs so the system prompt stays compatible.
func DefaultTools() []Tool {
	return []Tool{
		&TaskTool{},
		&TaskOutputTool{},
		&BashTool{},
		&GlobTool{},
		&GrepTool{},
		&ExitPlanModeTool{},
		&ReadTool{},
		&EditTool{},
		&WriteTool{},
		&NotebookEditTool{},
		&WebFetchTool{},
		&TodoWriteTool{},
		&WebSearchTool{},
		&TaskStopTool{},
		&AskUserQuestionTool{},
		&SkillTool{},
		&EnterPlanModeTool{},
	}
}
