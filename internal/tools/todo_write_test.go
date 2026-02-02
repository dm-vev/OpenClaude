package tools

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/openclaude/openclaude/internal/session"
)

// TestTodoWriteToolPersists verifies todo payload persistence to the session store.
func TestTodoWriteToolPersists(testingHandle *testing.T) {
	store := &session.Store{BaseDir: testingHandle.TempDir()}
	toolCtx := ToolContext{Store: store, SessionID: "session-1"}

	tool := &TodoWriteTool{}
	payload, err := json.Marshal(map[string]any{
		"todos": []map[string]any{
			{"text": "ship it", "completed": false},
		},
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

	path := filepath.Join(store.BaseDir, "session-env", "session-1", "todo.json")
	if _, err := os.Stat(path); err != nil {
		testingHandle.Fatalf("expected todo file: %v", err)
	}
	if !json.Valid([]byte(result.Content)) {
		testingHandle.Fatalf("expected JSON response, got: %s", result.Content)
	}
}
