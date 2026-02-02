package tools

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// TestSkillToolLoadsFile verifies skill lookup within project directories.
func TestSkillToolLoadsFile(testingHandle *testing.T) {
	root := testingHandle.TempDir()
	sandbox := NewSandbox([]string{root})
	toolCtx := ToolContext{Sandbox: sandbox, CWD: root}

	skillPath := filepath.Join(root, ".openclaude", "skills", "demo", "SKILL.md")
	if err := os.MkdirAll(filepath.Dir(skillPath), 0o755); err != nil {
		testingHandle.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(skillPath, []byte("demo skill"), 0o600); err != nil {
		testingHandle.Fatalf("write skill: %v", err)
	}

	tool := &SkillTool{}
	payload, err := json.Marshal(map[string]any{
		"name": "demo",
	})
	if err != nil {
		testingHandle.Fatalf("marshal payload: %v", err)
	}

	result, runErr := tool.Run(context.Background(), payload, toolCtx)
	if runErr != nil {
		testingHandle.Fatalf("run tool: %v", runErr)
	}
	if result.IsError {
		testingHandle.Fatalf("unexpected error: %s", result.Content)
	}
	if result.Content != "demo skill" {
		testingHandle.Fatalf("unexpected content: %s", result.Content)
	}
}

// TestSkillToolMissing verifies missing skills return an error.
func TestSkillToolMissing(testingHandle *testing.T) {
	root := testingHandle.TempDir()
	sandbox := NewSandbox([]string{root})
	toolCtx := ToolContext{Sandbox: sandbox, CWD: root}

	tool := &SkillTool{}
	payload, err := json.Marshal(map[string]any{
		"name": "missing",
	})
	if err != nil {
		testingHandle.Fatalf("marshal payload: %v", err)
	}

	result, runErr := tool.Run(context.Background(), payload, toolCtx)
	if runErr != nil {
		testingHandle.Fatalf("run tool: %v", runErr)
	}
	if !result.IsError {
		testingHandle.Fatalf("expected error for missing skill")
	}
}
