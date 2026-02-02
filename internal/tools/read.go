package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
)

// maxReadBytes caps file reads so tool output stays bounded and predictable.
// Claude Code truncates large files, so we fail fast with a clear error instead.
const maxReadBytes = 1024 * 1024

// ReadTool reads a file from disk with sandbox and size protections.
// It also supports line-window reads to mirror Claude Code's offset/limit behavior.
type ReadTool struct{}

func (t *ReadTool) Name() string {
	return "Read"
}

func (t *ReadTool) Description() string {
	return "Read the contents of a file from disk."
}

func (t *ReadTool) Schema() map[string]any {
	// Prefer file_path to match Claude Code, but keep path as a legacy alias.
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"file_path": map[string]any{
				"type":        "string",
				"description": "Absolute path to the file to read.",
			},
			"path": map[string]any{
				"type":        "string",
				"description": "Path to the file to read (legacy alias for file_path).",
			},
			"offset": map[string]any{
				"type":        "integer",
				"description": "Line number to start reading from (1-indexed).",
			},
			"limit": map[string]any{
				"type":        "integer",
				"description": "Maximum number of lines to read.",
			},
		},
		"required": []string{"file_path"},
	}
}

func (t *ReadTool) Run(ctx context.Context, input json.RawMessage, toolCtx ToolContext) (ToolResult, error) {
	// The tool is synchronous, so the context is unused by design.
	var payload struct {
		Path     string `json:"path"`
		FilePath string `json:"file_path"`
		Offset   *int   `json:"offset"`
		Limit    *int   `json:"limit"`
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

	// Enforce sandbox policies before touching the filesystem.
	path, err := toolCtx.Sandbox.ResolvePath(payload.FilePath, true)
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

	content := string(data)
	if payload.Offset != nil || payload.Limit != nil {
		// Offset is 1-indexed to match Claude Code's line numbering.
		lines := strings.Split(content, "\n")
		start := 0
		if payload.Offset != nil && *payload.Offset > 0 {
			start = *payload.Offset - 1
		}
		if start < 0 {
			start = 0
		}
		if start > len(lines) {
			return ToolResult{IsError: true, Content: "offset exceeds file length"}, nil
		}
		end := len(lines)
		if payload.Limit != nil && *payload.Limit >= 0 {
			limit := *payload.Limit
			if limit < 0 {
				limit = 0
			}
			if start+limit < end {
				end = start + limit
			}
		}
		content = strings.Join(lines[start:end], "\n")
	}

	return ToolResult{Content: content}, nil
}
