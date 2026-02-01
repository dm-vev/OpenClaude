package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// EditTool applies patches or replacements to files.
type EditTool struct{}

func (t *EditTool) Name() string {
	return "Edit"
}

func (t *EditTool) Description() string {
	return "Apply a unified diff patch or string replacements to a file."
}

func (t *EditTool) Schema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"path": map[string]any{
				"type":        "string",
				"description": "Path to the file to edit.",
			},
			"patch": map[string]any{
				"type":        "string",
				"description": "Unified diff patch to apply.",
			},
			"replacements": map[string]any{
				"type": "array",
				"items": map[string]any{
					"type": "object",
					"properties": map[string]any{
						"old":   map[string]any{"type": "string"},
						"new":   map[string]any{"type": "string"},
						"count": map[string]any{"type": "integer"},
					},
					"required": []string{"old", "new"},
				},
			},
		},
		"required": []string{"path"},
	}
}

func (t *EditTool) Run(ctx context.Context, input json.RawMessage, toolCtx ToolContext) (ToolResult, error) {
	var payload struct {
		Path         string `json:"path"`
		Patch        string `json:"patch"`
		Replacements []struct {
			Old   string `json:"old"`
			New   string `json:"new"`
			Count *int   `json:"count"`
		} `json:"replacements"`
	}
	if err := json.Unmarshal(input, &payload); err != nil {
		return ToolResult{IsError: true, Content: fmt.Sprintf("invalid input: %v", err)}, nil
	}
	if payload.Path == "" {
		return ToolResult{IsError: true, Content: "path is required"}, nil
	}

	// Validate the target path in the sandbox.
	path, err := toolCtx.Sandbox.ResolvePath(payload.Path, true)
	if err != nil {
		return ToolResult{IsError: true, Content: err.Error()}, nil
	}

	// Read the original file before applying edits.
	original, err := os.ReadFile(path)
	if err != nil {
		return ToolResult{IsError: true, Content: err.Error()}, nil
	}

	// Apply either a unified diff patch or replacements.
	updated := string(original)
	switch {
	case strings.TrimSpace(payload.Patch) != "":
		updated, err = applyUnifiedPatch(updated, payload.Patch)
		if err != nil {
			return ToolResult{IsError: true, Content: err.Error()}, nil
		}
	case len(payload.Replacements) > 0:
		for _, rep := range payload.Replacements {
			count := 1
			if rep.Count != nil {
				count = *rep.Count
			}
			updated = strings.Replace(updated, rep.Old, rep.New, count)
		}
	default:
		return ToolResult{IsError: true, Content: "either patch or replacements must be provided"}, nil
	}

	// Backup the original file to the session directory, if available.
	if err := t.backupFile(toolCtx, path); err != nil {
		return ToolResult{IsError: true, Content: fmt.Sprintf("backup failed: %v", err)}, nil
	}

	// Write the new file contents atomically.
	if err := writeAtomic(path, []byte(updated)); err != nil {
		return ToolResult{IsError: true, Content: fmt.Sprintf("write failed: %v", err)}, nil
	}

	return ToolResult{Content: "ok"}, nil
}

func (t *EditTool) backupFile(toolCtx ToolContext, path string) error {
	if toolCtx.Store == nil || toolCtx.SessionID == "" {
		return nil
	}
	backupDir := filepath.Join(toolCtx.Store.BaseDir, "session-env", toolCtx.SessionID, "backup")
	if err := os.MkdirAll(backupDir, 0o755); err != nil {
		return err
	}
	backupPath := filepath.Join(backupDir, filepath.Base(path))
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	return os.WriteFile(backupPath, data, 0o600)
}

// writeAtomic writes to a temp file and renames it into place.
func writeAtomic(path string, data []byte) error {
	dir := filepath.Dir(path)
	tmpFile, err := os.CreateTemp(dir, ".openclaude-*")
	if err != nil {
		return err
	}
	tmpName := tmpFile.Name()
	if _, err := tmpFile.Write(data); err != nil {
		tmpFile.Close()
		return err
	}
	if err := tmpFile.Close(); err != nil {
		return err
	}
	return os.Rename(tmpName, path)
}

// applyUnifiedPatch applies a minimal unified diff patch to a string.
func applyUnifiedPatch(original string, patch string) (string, error) {
	lines := strings.Split(original, "\n")
	patchLines := strings.Split(patch, "\n")

	var output []string
	index := 0

	for i := 0; i < len(patchLines); i++ {
		line := patchLines[i]
		if strings.HasPrefix(line, "---") || strings.HasPrefix(line, "+++") {
			continue
		}
		if strings.HasPrefix(line, "@@") {
			// Parse the old starting line index from the hunk header.
			var oldStart int
			_, err := fmt.Sscanf(line, "@@ -%d", &oldStart)
			if err != nil {
				return "", fmt.Errorf("invalid hunk header: %s", line)
			}
			oldStart--
			if oldStart < 0 {
				oldStart = 0
			}
			if oldStart > len(lines) {
				return "", fmt.Errorf("hunk out of range: %s", line)
			}
			output = append(output, lines[index:oldStart]...)
			index = oldStart

			for i+1 < len(patchLines) {
				next := patchLines[i+1]
				if strings.HasPrefix(next, "@@") {
					break
				}
				i++
				if strings.HasPrefix(next, "\\ No newline at end of file") {
					continue
				}
				if next == "" && i == len(patchLines)-1 {
					break
				}
				if next == "" {
					next = " "
				}
				// Apply additions, deletions, and context lines.
				prefix := next[:1]
				content := ""
				if len(next) > 1 {
					content = next[1:]
				}
				switch prefix {
				case " ":
					if index >= len(lines) || lines[index] != content {
						return "", fmt.Errorf("context mismatch at line %d", index+1)
					}
					output = append(output, content)
					index++
				case "-":
					if index >= len(lines) || lines[index] != content {
						return "", fmt.Errorf("delete mismatch at line %d", index+1)
					}
					index++
				case "+":
					output = append(output, content)
				default:
					return "", fmt.Errorf("invalid patch line: %s", next)
				}
			}
		}
	}

	output = append(output, lines[index:]...)
	return strings.Join(output, "\n"), nil
}
