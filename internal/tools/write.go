package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// WriteTool writes full file contents to disk.
// It enforces sandbox rules, backs up existing content, and writes atomically.
type WriteTool struct{}

// Name returns the tool identifier used in tool calls.
func (t *WriteTool) Name() string {
	return "Write"
}

// Description summarizes the write behavior for the model.
func (t *WriteTool) Description() string {
	return "Write content to a file, creating it if needed."
}

// Schema describes the required write payload.
func (t *WriteTool) Schema() map[string]any {
	// Prefer file_path to match Claude Code, but keep path as a legacy alias.
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"file_path": map[string]any{
				"type":        "string",
				"description": "Absolute path to the file to write.",
			},
			"path": map[string]any{
				"type":        "string",
				"description": "Path to the file to write (legacy alias for file_path).",
			},
			"content": map[string]any{
				"type":        "string",
				"description": "Full file contents to write.",
			},
		},
		"required": []string{"file_path", "content"},
	}
}

// Run validates the payload, backs up existing files, and writes atomically.
func (t *WriteTool) Run(ctx context.Context, input json.RawMessage, toolCtx ToolContext) (ToolResult, error) {
	// The tool is synchronous, so the context is unused by design.
	_ = ctx

	var payload struct {
		Path     string `json:"path"`
		FilePath string `json:"file_path"`
		Content  string `json:"content"`
	}
	if err := json.Unmarshal(input, &payload); err != nil {
		return ToolResult{IsError: true, Content: fmt.Sprintf("invalid input: %v", err)}, nil
	}
	// Accept legacy "path" while standardizing on file_path.
	if payload.FilePath == "" {
		payload.FilePath = payload.Path
	}
	if payload.FilePath == "" {
		return ToolResult{IsError: true, Content: "file_path is required"}, nil
	}

	// Validate the write path against the sandbox rules.
	path, err := toolCtx.Sandbox.ResolvePath(payload.FilePath, false)
	if err != nil {
		return ToolResult{IsError: true, Content: err.Error()}, nil
	}

	// Ensure parent directories exist before writing.
	parent := filepath.Dir(path)
	if parent != "" {
		if err := os.MkdirAll(parent, 0o755); err != nil {
			return ToolResult{IsError: true, Content: err.Error()}, nil
		}
	}

	// Backup existing content to the session store when applicable.
	mode := os.FileMode(0o644)
	info, err := os.Stat(path)
	switch {
	case err == nil:
		if info.IsDir() {
			return ToolResult{IsError: true, Content: "path is a directory"}, nil
		}
		mode = info.Mode().Perm()
		if err := backupFile(toolCtx, path); err != nil {
			return ToolResult{IsError: true, Content: fmt.Sprintf("backup failed: %v", err)}, nil
		}
	case os.IsNotExist(err):
		// Use default file mode for newly created files.
	default:
		return ToolResult{IsError: true, Content: err.Error()}, nil
	}

	// Write the new file contents atomically.
	if err := writeAtomic(path, []byte(payload.Content), mode); err != nil {
		return ToolResult{IsError: true, Content: fmt.Sprintf("write failed: %v", err)}, nil
	}

	return ToolResult{Content: "ok"}, nil
}
