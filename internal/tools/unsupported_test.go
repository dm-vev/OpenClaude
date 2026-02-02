package tools

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
)

// TestDefaultToolsOrder validates that the Claude Code tool ordering is stable.
func TestDefaultToolsOrder(testingHandle *testing.T) {
	// Collect the default tool names in order for comparison.
	tools := DefaultTools()
	names := make([]string, 0, len(tools))
	for _, tool := range tools {
		if tool == nil {
			continue
		}
		names = append(names, tool.Name())
	}

	expected := []string{
		"Task",
		"TaskOutput",
		"Bash",
		"Glob",
		"Grep",
		"ExitPlanMode",
		"Read",
		"Edit",
		"Write",
		"NotebookEdit",
		"WebFetch",
		"TodoWrite",
		"WebSearch",
		"TaskStop",
		"AskUserQuestion",
		"Skill",
		"EnterPlanMode",
	}

	if len(names) != len(expected) {
		testingHandle.Fatalf("expected %d tools, got %d", len(expected), len(names))
	}
	for index, name := range expected {
		if names[index] != name {
			testingHandle.Fatalf("tool order mismatch at %d: expected %s, got %s", index, name, names[index])
		}
	}
}

// TestUnsupportedToolRun ensures stub tools return consistent errors with guidance.
func TestUnsupportedToolRun(testingHandle *testing.T) {
	// Build an unsupported tool with deterministic naming and guidance.
	stub := newUnsupportedTool("StubTool", "Stub description.", "Use a real tool instead.")

	// Execute with valid JSON input to exercise the normal error path.
	result, err := stub.Run(context.Background(), json.RawMessage(`{"key":"value"}`), ToolContext{})
	if err != nil {
		testingHandle.Fatalf("unexpected error: %v", err)
	}
	if !result.IsError {
		testingHandle.Fatalf("expected IsError to be true")
	}
	if !strings.Contains(result.Content, "StubTool is not supported in OpenClaude yet.") {
		testingHandle.Fatalf("unexpected error content: %s", result.Content)
	}
	if !strings.Contains(result.Content, "Use a real tool instead.") {
		testingHandle.Fatalf("missing guidance text: %s", result.Content)
	}
}

// TestUnsupportedToolInvalidInput verifies malformed JSON is surfaced clearly.
func TestUnsupportedToolInvalidInput(testingHandle *testing.T) {
	// Execute with malformed JSON to confirm the error path is deterministic.
	stub := newUnsupportedTool("StubTool", "Stub description.", "Use a real tool instead.")
	result, err := stub.Run(context.Background(), json.RawMessage(`{invalid`), ToolContext{})
	if err != nil {
		testingHandle.Fatalf("unexpected error: %v", err)
	}
	if !result.IsError {
		testingHandle.Fatalf("expected IsError to be true")
	}
	if !strings.Contains(result.Content, "invalid input:") {
		testingHandle.Fatalf("expected invalid input error, got %s", result.Content)
	}
}
