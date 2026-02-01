package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
)

// ListDirTool lists directory contents.
type ListDirTool struct{}

func (t *ListDirTool) Name() string {
	return "ListDir"
}

func (t *ListDirTool) Description() string {
	return "List entries in a directory."
}

func (t *ListDirTool) Schema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"path": map[string]any{
				"type":        "string",
				"description": "Directory path to list.",
			},
		},
		"required": []string{"path"},
	}
}

func (t *ListDirTool) Run(ctx context.Context, input json.RawMessage, toolCtx ToolContext) (ToolResult, error) {
	var payload struct {
		Path string `json:"path"`
	}
	if err := json.Unmarshal(input, &payload); err != nil {
		return ToolResult{IsError: true, Content: fmt.Sprintf("invalid input: %v", err)}, nil
	}

	// Validate path against sandbox rules.
	path, err := toolCtx.Sandbox.ResolvePath(payload.Path, true)
	if err != nil {
		return ToolResult{IsError: true, Content: err.Error()}, nil
	}

	// Read directory entries.
	entries, err := os.ReadDir(path)
	if err != nil {
		return ToolResult{IsError: true, Content: err.Error()}, nil
	}

	type entry struct {
		Name string
		Info string
	}

	var list []entry
	for _, item := range entries {
		info, err := item.Info()
		if err != nil {
			continue
		}
		kind := "file"
		if item.IsDir() {
			kind = "dir"
		} else if info.Mode()&os.ModeSymlink != 0 {
			kind = "symlink"
		}
		list = append(list, entry{
			Name: filepath.Join(path, item.Name()),
			Info: fmt.Sprintf("%s %d", kind, info.Size()),
		})
	}

	// Sort output for deterministic results.
	sort.Slice(list, func(i, j int) bool {
		return list[i].Name < list[j].Name
	})

	// Emit a tab-separated listing to keep parsing simple.
	var output string
	for _, item := range list {
		output += fmt.Sprintf("%s\t%s\n", item.Info, item.Name)
	}

	return ToolResult{Content: output}, nil
}
