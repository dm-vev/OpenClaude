package tools

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

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
	Tools map[string]Tool
}

// NewRunner constructs a tool runner.
func NewRunner(tools []Tool) *Runner {
	toolMap := make(map[string]Tool)
	for _, tool := range tools {
		toolMap[tool.Name()] = tool
	}
	return &Runner{Tools: toolMap}
}

// ToolSpecs returns OpenAI-compatible tool definitions.
func (r *Runner) ToolSpecs() []openai.Tool {
	specs := make([]openai.Tool, 0, len(r.Tools))
	for _, tool := range r.Tools {
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

// DefaultTools returns the built-in tool set.
func DefaultTools() []Tool {
	return []Tool{
		&ReadTool{},
		&EditTool{},
		&BashTool{},
		&GlobTool{},
		&GrepTool{},
		&ListDirTool{},
	}
}
