package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// EditTool applies edits to files using Claude-style old/new strings.
// It also supports legacy patch/replacement payloads for compatibility.
type EditTool struct{}

func (t *EditTool) Name() string {
	return "Edit"
}

func (t *EditTool) Description() string {
	return "Apply a unified diff patch or string replacements to a file."
}

func (t *EditTool) Schema() map[string]any {
	// Prefer Claude Code's file_path + old_string/new_string fields.
	// While still preserving legacy patch/replacements payloads.
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"file_path": map[string]any{
				"type":        "string",
				"description": "Absolute path to the file to modify.",
			},
			"path": map[string]any{
				"type":        "string",
				"description": "Path to the file to edit (legacy alias for file_path).",
			},
			"old_string": map[string]any{
				"type":        "string",
				"description": "The exact text to replace. Use empty string to create a new file.",
			},
			"new_string": map[string]any{
				"type":        "string",
				"description": "Replacement text. Use empty string to delete old_string.",
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
		"required": []string{"file_path", "old_string", "new_string"},
	}
}

func (t *EditTool) Run(ctx context.Context, input json.RawMessage, toolCtx ToolContext) (ToolResult, error) {
	// The tool is synchronous, so the context is unused by design.
	var payload struct {
		Path         string  `json:"path"`
		FilePath     string  `json:"file_path"`
		Old          *string `json:"old_string"`
		New          *string `json:"new_string"`
		Patch        string  `json:"patch"`
		Replacements []struct {
			Old   string `json:"old"`
			New   string `json:"new"`
			Count *int   `json:"count"`
		} `json:"replacements"`
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

	// Detect whether old_string/new_string were supplied, even if empty.
	usingOldNew := payload.Old != nil || payload.New != nil
	oldValue := ""
	newValue := ""
	if payload.Old != nil {
		oldValue = *payload.Old
	}
	if payload.New != nil {
		newValue = *payload.New
	}

	// Validate the target path in the sandbox.
	requireExisting := true
	if usingOldNew && oldValue == "" {
		requireExisting = false
	}
	path, err := toolCtx.Sandbox.ResolvePath(payload.FilePath, requireExisting)
	if err != nil {
		return ToolResult{IsError: true, Content: err.Error()}, nil
	}

	// Read the original file before applying edits, if required.
	var original []byte
	if requireExisting {
		original, err = os.ReadFile(path)
		if err != nil {
			return ToolResult{IsError: true, Content: err.Error()}, nil
		}
	}

	// Apply either Claude-style old/new edits or legacy patch/replacements.
	updated := string(original)
	switch {
	case usingOldNew:
		// Empty old_string means "create/overwrite" with new_string content.
		if oldValue == "" {
			updated = newValue
		} else {
			// Replace the first matching occurrence to mirror Claude Code behavior.
			if newValue == "" && !strings.HasSuffix(oldValue, "\n") && strings.Contains(updated, oldValue+"\n") {
				updated = strings.Replace(updated, oldValue+"\n", newValue, 1)
			} else {
				updated = strings.Replace(updated, oldValue, newValue, 1)
			}
			if updated == string(original) {
				return ToolResult{IsError: true, Content: "original and edited file match; failed to apply edit"}, nil
			}
		}
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
		return ToolResult{IsError: true, Content: "either old_string/new_string or patch/replacements must be provided"}, nil
	}

	// Backup the original file to the session directory, if available.
	if err := backupFile(toolCtx, path); err != nil {
		return ToolResult{IsError: true, Content: fmt.Sprintf("backup failed: %v", err)}, nil
	}

	// Ensure parent directories exist before writing new files.
	parent := filepath.Dir(path)
	if parent != "" {
		if err := os.MkdirAll(parent, 0o755); err != nil {
			return ToolResult{IsError: true, Content: err.Error()}, nil
		}
	}

	// Write the new file contents atomically.
	mode := os.FileMode(0o644)
	if info, statErr := os.Stat(path); statErr == nil {
		mode = info.Mode().Perm()
	}
	if err := writeAtomic(path, []byte(updated), mode); err != nil {
		return ToolResult{IsError: true, Content: fmt.Sprintf("write failed: %v", err)}, nil
	}

	return ToolResult{Content: "ok"}, nil
}

// writeAtomic writes to a temp file and renames it into place.
// The mode is applied before the rename so the final file has stable permissions.
func writeAtomic(path string, data []byte, mode os.FileMode) error {
	dir := filepath.Dir(path)
	tmpFile, err := os.CreateTemp(dir, ".openclaude-*")
	if err != nil {
		return err
	}
	if err := tmpFile.Chmod(mode); err != nil {
		tmpFile.Close()
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
