package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/openclaude/openclaude/internal/agent"
	"github.com/openclaude/openclaude/internal/streamjson"
)

// TestExtractPermissionDenialsToolDenied verifies denied tool errors are surfaced.
func TestExtractPermissionDenialsToolDenied(testingHandle *testing.T) {
	err := fmt.Errorf("%w: %s", agent.ErrToolDenied, "Bash")
	denials := extractPermissionDenials(err)
	if len(denials) != 1 {
		testingHandle.Fatalf("expected 1 denial, got %d", len(denials))
	}
	denial, ok := denials[0].(permissionDenial)
	if !ok {
		testingHandle.Fatalf("expected permissionDenial, got %T", denials[0])
	}
	if denial.ToolName != "Bash" {
		testingHandle.Fatalf("expected tool name Bash, got %s", denial.ToolName)
	}
	if denial.Reason != "user_denied" {
		testingHandle.Fatalf("expected reason user_denied, got %s", denial.Reason)
	}
}

// TestExtractPermissionDenialsPlanMode verifies plan mode denials are surfaced.
func TestExtractPermissionDenialsPlanMode(testingHandle *testing.T) {
	denials := extractPermissionDenials(agent.ErrPlanMode)
	if len(denials) != 1 {
		testingHandle.Fatalf("expected 1 denial, got %d", len(denials))
	}
	denial, ok := denials[0].(permissionDenial)
	if !ok {
		testingHandle.Fatalf("expected permissionDenial, got %T", denials[0])
	}
	if denial.ToolName != "" {
		testingHandle.Fatalf("expected empty tool name, got %s", denial.ToolName)
	}
	if denial.Reason != "plan_mode" {
		testingHandle.Fatalf("expected reason plan_mode, got %s", denial.Reason)
	}
}

// TestExtractPermissionDenialsDefault verifies unrelated errors produce no denials.
func TestExtractPermissionDenialsDefault(testingHandle *testing.T) {
	denials := extractPermissionDenials(fmt.Errorf("other error"))
	if len(denials) != 0 {
		testingHandle.Fatalf("expected no denials, got %d", len(denials))
	}
}

// TestWriteStreamJSONErrorResultIncludesDenials ensures result payload includes denials.
func TestWriteStreamJSONErrorResultIncludesDenials(testingHandle *testing.T) {
	err := fmt.Errorf("%w: %s", agent.ErrToolDenied, "Write")

	var buffer bytes.Buffer
	writer := streamjson.NewWriter(&buffer)

	if writeErr := writeStreamJSONErrorResult(writer, err, "session-1", "model-x", 5*time.Millisecond); writeErr != nil {
		testingHandle.Fatalf("writeStreamJSONErrorResult error: %v", writeErr)
	}

	line := bytes.TrimSpace(buffer.Bytes())
	if len(line) == 0 {
		testingHandle.Fatalf("expected result JSON line")
	}

	var payload map[string]any
	if unmarshalErr := json.Unmarshal(line, &payload); unmarshalErr != nil {
		testingHandle.Fatalf("parse result JSON: %v", unmarshalErr)
	}

	denials, ok := payload["permission_denials"].([]any)
	if !ok || len(denials) != 1 {
		testingHandle.Fatalf("expected one permission denial, got %v", payload["permission_denials"])
	}
	entry, ok := denials[0].(map[string]any)
	if !ok {
		testingHandle.Fatalf("expected permission denial object, got %T", denials[0])
	}
	if entry["tool_name"] != "Write" || entry["reason"] != "user_denied" {
		testingHandle.Fatalf("unexpected denial payload: %v", entry)
	}
}
