package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
)

// NotebookEditTool applies text edits to a Jupyter notebook file.
// It mirrors the Edit tool behavior but restricts edits to .ipynb files.
type NotebookEditTool struct{}

// Name returns the tool identifier used in tool calls.
func (t *NotebookEditTool) Name() string {
	return "NotebookEdit"
}

// Description summarizes the notebook edit behavior for the model.
func (t *NotebookEditTool) Description() string {
	return "Apply text edits to a Jupyter notebook file."
}

// Schema reuses the Edit tool schema to keep compatibility with Claude-style edits.
func (t *NotebookEditTool) Schema() map[string]any {
	edit := &EditTool{}
	return edit.Schema()
}

// Run validates the target path and delegates to the Edit tool implementation.
func (t *NotebookEditTool) Run(ctx context.Context, input json.RawMessage, toolCtx ToolContext) (ToolResult, error) {
	// Decode the path so we can enforce the .ipynb constraint before editing.
	var payload struct {
		Path     string `json:"path"`
		FilePath string `json:"file_path"`
	}
	if err := json.Unmarshal(input, &payload); err != nil {
		return ToolResult{IsError: true, Content: fmt.Sprintf("invalid input: %v", err)}, nil
	}
	if payload.FilePath == "" {
		payload.FilePath = payload.Path
	}
	if payload.FilePath == "" {
		return ToolResult{IsError: true, Content: "file_path is required"}, nil
	}

	if strings.ToLower(filepath.Ext(payload.FilePath)) != ".ipynb" {
		return ToolResult{IsError: true, Content: "NotebookEdit requires a .ipynb file"}, nil
	}

	// Delegate to EditTool for the actual edit semantics.
	edit := &EditTool{}
	return edit.Run(ctx, input, toolCtx)
}
