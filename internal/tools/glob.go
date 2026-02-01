package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"sort"
	"strings"
)

// GlobTool performs glob searches.
type GlobTool struct{}

func (t *GlobTool) Name() string {
	return "Glob"
}

func (t *GlobTool) Description() string {
	return "Find files matching a glob pattern."
}

func (t *GlobTool) Schema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"pattern": map[string]any{
				"type":        "string",
				"description": "Glob pattern to match files.",
			},
		},
		"required": []string{"pattern"},
	}
}

func (t *GlobTool) Run(ctx context.Context, input json.RawMessage, toolCtx ToolContext) (ToolResult, error) {
	var payload struct {
		Pattern string `json:"pattern"`
	}
	if err := json.Unmarshal(input, &payload); err != nil {
		return ToolResult{IsError: true, Content: fmt.Sprintf("invalid input: %v", err)}, nil
	}

	if payload.Pattern == "" {
		return ToolResult{IsError: true, Content: "pattern is required"}, nil
	}

	// Use filepath.Glob to expand patterns from the current process context.
	matches, err := filepath.Glob(payload.Pattern)
	if err != nil {
		return ToolResult{IsError: true, Content: err.Error()}, nil
	}

	// Enforce sandbox constraints on each match.
	var filtered []string
	for _, match := range matches {
		resolved, err := toolCtx.Sandbox.ResolvePath(match, true)
		if err != nil {
			continue
		}
		filtered = append(filtered, resolved)
	}

	// Sort for deterministic output.
	sort.Strings(filtered)
	return ToolResult{Content: strings.Join(filtered, "\n")}, nil
}
