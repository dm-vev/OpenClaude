package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// TodoWriteTool persists a structured todo list for the current session.
type TodoWriteTool struct{}

// Name returns the tool identifier used in tool calls.
func (t *TodoWriteTool) Name() string {
	return "TodoWrite"
}

// Description summarizes the todo write behavior for the model.
func (t *TodoWriteTool) Description() string {
	return "Persist a structured todo list for the current session."
}

// Schema describes the expected todo payload.
func (t *TodoWriteTool) Schema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"todos": map[string]any{
				"type":        "array",
				"description": "List of todo items to store.",
				"items": map[string]any{
					"type": "object",
					"properties": map[string]any{
						"id":        map[string]any{"type": "string"},
						"text":      map[string]any{"type": "string"},
						"completed": map[string]any{"type": "boolean"},
					},
					"required": []string{"text"},
				},
			},
		},
		"required": []string{"todos"},
	}
}

// Run validates the payload and persists it under the session directory.
func (t *TodoWriteTool) Run(ctx context.Context, input json.RawMessage, toolCtx ToolContext) (ToolResult, error) {
	// The tool is synchronous, so the context is unused by design.
	_ = ctx

	var payload map[string]any
	if err := json.Unmarshal(input, &payload); err != nil {
		return ToolResult{IsError: true, Content: fmt.Sprintf("invalid input: %v", err)}, nil
	}
	if _, ok := payload["todos"]; !ok {
		return ToolResult{IsError: true, Content: "todos is required"}, nil
	}

	result := map[string]any{
		"status":    "ok",
		"persisted": false,
		"todos":     payload["todos"],
	}

	if toolCtx.Store == nil || toolCtx.SessionID == "" {
		encoded, _ := json.Marshal(result)
		return ToolResult{Content: string(encoded)}, nil
	}

	path := filepath.Join(toolCtx.Store.BaseDir, "session-env", toolCtx.SessionID, "todo.json")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return ToolResult{IsError: true, Content: fmt.Sprintf("persist todo list: %v", err)}, nil
	}
	encoded, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		return ToolResult{IsError: true, Content: fmt.Sprintf("encode todo list: %v", err)}, nil
	}
	if err := os.WriteFile(path, encoded, 0o600); err != nil {
		return ToolResult{IsError: true, Content: fmt.Sprintf("write todo list: %v", err)}, nil
	}

	result["persisted"] = true
	result["path"] = path
	encoded, _ = json.Marshal(result)
	return ToolResult{Content: string(encoded)}, nil
}
