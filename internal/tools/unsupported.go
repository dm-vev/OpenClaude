package tools

import (
	"context"
	"encoding/json"
	"fmt"
)

// unsupportedTool implements Tool for features that are not yet available.
// It preserves Claude Code compatibility by exposing the name while failing loudly.
type unsupportedTool struct {
	// name is the tool identifier reported to the model.
	name string
	// description describes the intended capability for compatibility prompts.
	description string
	// reason provides user-facing guidance for the unsupported response.
	reason string
}

// newUnsupportedTool constructs a stub tool with consistent messaging.
// The reason string should include a user-actionable hint when possible.
func newUnsupportedTool(name string, description string, reason string) *unsupportedTool {
	return &unsupportedTool{
		name:        name,
		description: description,
		reason:      reason,
	}
}

// Name returns the tool identifier used in tool calls.
func (t *unsupportedTool) Name() string {
	return t.name
}

// Description reports the intended tool behavior, noting the stubbed status.
func (t *unsupportedTool) Description() string {
	return t.description
}

// Schema accepts arbitrary JSON to avoid rejecting upstream payloads.
// This keeps the stub tolerant of evolving tool schemas.
func (t *unsupportedTool) Schema() map[string]any {
	return map[string]any{
		"type":                 "object",
		"additionalProperties": true,
	}
}

// Run returns a deterministic error payload with guidance.
// The error is delivered inside the tool result to keep the tool contract intact.
func (t *unsupportedTool) Run(ctx context.Context, input json.RawMessage, toolCtx ToolContext) (ToolResult, error) {
	_ = ctx
	_ = toolCtx

	if len(input) > 0 {
		var payload any
		if err := json.Unmarshal(input, &payload); err != nil {
			return ToolResult{IsError: true, Content: fmt.Sprintf("invalid input: %v", err)}, nil
		}
	}

	message := fmt.Sprintf("%s is not supported in OpenClaude yet.", t.name)
	if t.reason != "" {
		message = fmt.Sprintf("%s %s", message, t.reason)
	}

	return ToolResult{IsError: true, Content: message}, nil
}
