package tools

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// GrepTool searches for a string within files.
type GrepTool struct{}

func (t *GrepTool) Name() string {
	return "Grep"
}

func (t *GrepTool) Description() string {
	return "Search for a string in files under a path."
}

func (t *GrepTool) Schema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"query": map[string]any{
				"type":        "string",
				"description": "Search string.",
			},
			"path": map[string]any{
				"type":        "string",
				"description": "Path to search (file or directory).",
			},
		},
		"required": []string{"query"},
	}
}

func (t *GrepTool) Run(ctx context.Context, input json.RawMessage, toolCtx ToolContext) (ToolResult, error) {
	var payload struct {
		Query string `json:"query"`
		Path  string `json:"path"`
	}
	if err := json.Unmarshal(input, &payload); err != nil {
		return ToolResult{IsError: true, Content: fmt.Sprintf("invalid input: %v", err)}, nil
	}

	if payload.Query == "" {
		return ToolResult{IsError: true, Content: "query is required"}, nil
	}

	// Default to the current working directory.
	root := payload.Path
	if root == "" {
		root = toolCtx.CWD
	}

	// Validate search path against sandbox rules.
	root, err := toolCtx.Sandbox.ResolvePath(root, true)
	if err != nil {
		return ToolResult{IsError: true, Content: err.Error()}, nil
	}

	// Walk the tree and scan files line by line.
	var matches []string
	err = filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if entry.IsDir() {
			return nil
		}
		info, err := entry.Info()
		if err != nil {
			return nil
		}
		if info.Size() > maxReadBytes {
			return nil
		}
		file, err := os.Open(path)
		if err != nil {
			return nil
		}
		defer file.Close()

		scanner := bufio.NewScanner(file)
		lineNumber := 1
		for scanner.Scan() {
			line := scanner.Text()
			if strings.Contains(line, payload.Query) {
				matches = append(matches, fmt.Sprintf("%s:%d:%s", path, lineNumber, line))
			}
			lineNumber++
		}
		return nil
	})
	if err != nil {
		return ToolResult{IsError: true, Content: err.Error()}, nil
	}

	return ToolResult{Content: strings.Join(matches, "\n")}, nil
}
