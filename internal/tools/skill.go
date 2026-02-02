package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// SkillTool loads a local skill definition file for the requested skill name.
type SkillTool struct{}

// Name returns the tool identifier used in tool calls.
func (t *SkillTool) Name() string {
	return "Skill"
}

// Description summarizes the skill loading behavior.
func (t *SkillTool) Description() string {
	return "Load a local skill definition file."
}

// Schema describes the expected skill payload.
func (t *SkillTool) Schema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"name": map[string]any{
				"type":        "string",
				"description": "Skill identifier to load.",
			},
			"path": map[string]any{
				"type":        "string",
				"description": "Optional path to a specific skill file.",
			},
		},
		"required": []string{"name"},
	}
}

// Run locates and returns the skill contents from local files.
func (t *SkillTool) Run(ctx context.Context, input json.RawMessage, toolCtx ToolContext) (ToolResult, error) {
	// The tool is synchronous, so the context is unused by design.
	_ = ctx

	var payload struct {
		Name string `json:"name"`
		Path string `json:"path"`
	}
	if err := json.Unmarshal(input, &payload); err != nil {
		return ToolResult{IsError: true, Content: fmt.Sprintf("invalid input: %v", err)}, nil
	}
	payload.Name = strings.TrimSpace(payload.Name)
	payload.Path = strings.TrimSpace(payload.Path)
	if payload.Name == "" && payload.Path == "" {
		return ToolResult{IsError: true, Content: "name is required"}, nil
	}

	candidates := buildSkillCandidates(toolCtx, payload.Name, payload.Path)
	for _, candidate := range candidates {
		path, err := toolCtx.Sandbox.ResolvePath(candidate, true)
		if err != nil {
			continue
		}
		contents, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		return ToolResult{Content: string(contents)}, nil
	}

	return ToolResult{IsError: true, Content: "skill not found"}, nil
}

// buildSkillCandidates builds potential file paths for the skill lookup.
func buildSkillCandidates(toolCtx ToolContext, name string, path string) []string {
	if path != "" {
		return []string{path}
	}

	baseDirs := []string{
		filepath.Join(toolCtx.CWD, ".openclaude", "skills"),
		filepath.Join(toolCtx.CWD, "skills"),
	}

	var candidates []string
	for _, base := range baseDirs {
		candidates = append(candidates,
			filepath.Join(base, name, "SKILL.md"),
			filepath.Join(base, name+".md"),
		)
	}
	return candidates
}
