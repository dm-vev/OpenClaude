package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
)

// maxReadBytes limits file reads to avoid huge responses.
const maxReadBytes = 1024 * 1024

// ReadTool reads a file from disk.
type ReadTool struct{}

func (t *ReadTool) Name() string {
	return "Read"
}

func (t *ReadTool) Description() string {
	return "Read the contents of a file from disk."
}

func (t *ReadTool) Schema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"path": map[string]any{
				"type":        "string",
				"description": "Path to the file to read.",
			},
		},
		"required": []string{"path"},
	}
}

func (t *ReadTool) Run(ctx context.Context, input json.RawMessage, toolCtx ToolContext) (ToolResult, error) {
	var payload struct {
		Path string `json:"path"`
	}
	if err := json.Unmarshal(input, &payload); err != nil {
		return ToolResult{IsError: true, Content: fmt.Sprintf("invalid input: %v", err)}, nil
	}

	// Enforce sandbox policies before touching the filesystem.
	path, err := toolCtx.Sandbox.ResolvePath(payload.Path, true)
	if err != nil {
		return ToolResult{IsError: true, Content: err.Error()}, nil
	}

	// Reject oversized files for safety and performance.
	info, err := os.Stat(path)
	if err != nil {
		return ToolResult{IsError: true, Content: err.Error()}, nil
	}
	if info.Size() > maxReadBytes {
		return ToolResult{IsError: true, Content: fmt.Sprintf("file too large: %d bytes", info.Size())}, nil
	}

	// Read and validate file content.
	data, err := os.ReadFile(path)
	if err != nil {
		return ToolResult{IsError: true, Content: err.Error()}, nil
	}

	// Quick binary detection to avoid dumping binary blobs.
	for _, b := range data {
		if b == 0 {
			return ToolResult{IsError: true, Content: "binary file detected"}, nil
		}
	}

	return ToolResult{Content: string(data)}, nil
}
