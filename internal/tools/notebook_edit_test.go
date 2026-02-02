package tools

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// TestNotebookEditToolAppliesEdit verifies NotebookEdit delegates to Edit.
func TestNotebookEditToolAppliesEdit(testingHandle *testing.T) {
	root := testingHandle.TempDir()
	sandbox := NewSandbox([]string{root})
	toolCtx := ToolContext{Sandbox: sandbox, CWD: root}

	path := filepath.Join(root, "notebook.ipynb")
	original := `{"cells":[{"source":["hello\n"]}],"metadata":{},"nbformat":4,"nbformat_minor":5}`
	if err := os.WriteFile(path, []byte(original), 0o600); err != nil {
		testingHandle.Fatalf("write notebook: %v", err)
	}

	tool := &NotebookEditTool{}
	payload, err := json.Marshal(map[string]any{
		"file_path":  path,
		"old_string": "hello",
		"new_string": "world",
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

	updated, err := os.ReadFile(path)
	if err != nil {
		testingHandle.Fatalf("read notebook: %v", err)
	}
	if string(updated) == original {
		testingHandle.Fatalf("expected notebook to change")
	}
}

// TestNotebookEditToolRejectsNonNotebook verifies extension validation.
func TestNotebookEditToolRejectsNonNotebook(testingHandle *testing.T) {
	root := testingHandle.TempDir()
	sandbox := NewSandbox([]string{root})
	toolCtx := ToolContext{Sandbox: sandbox, CWD: root}

	tool := &NotebookEditTool{}
	payload, err := json.Marshal(map[string]any{
		"file_path":  filepath.Join(root, "notes.txt"),
		"old_string": "a",
		"new_string": "b",
	})
	if err != nil {
		testingHandle.Fatalf("marshal payload: %v", err)
	}

	result, runErr := tool.Run(context.Background(), payload, toolCtx)
	if runErr != nil {
		testingHandle.Fatalf("run tool: %v", runErr)
	}
	if !result.IsError {
		testingHandle.Fatalf("expected error for non-notebook file")
	}
}
