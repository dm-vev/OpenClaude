package tools

import (
	"context"
	"encoding/json"
	"os"
	"testing"
)

// TestAskUserQuestionEnvOverride verifies env-based responses.
func TestAskUserQuestionEnvOverride(testingHandle *testing.T) {
	previous := os.Getenv("OPENCLOUDE_ASK_RESPONSE")
	if err := os.Setenv("OPENCLOUDE_ASK_RESPONSE", "yes"); err != nil {
		testingHandle.Fatalf("set env: %v", err)
	}
	defer os.Setenv("OPENCLOUDE_ASK_RESPONSE", previous)

	tool := &AskUserQuestionTool{}
	payload, err := json.Marshal(map[string]any{
		"question": "Proceed?",
	})
	if err != nil {
		testingHandle.Fatalf("marshal payload: %v", err)
	}

	result, runErr := tool.Run(context.Background(), payload, ToolContext{})
	if runErr != nil {
		testingHandle.Fatalf("run tool: %v", runErr)
	}
	if result.IsError {
		testingHandle.Fatalf("unexpected error: %s", result.Content)
	}
	if result.Content != "yes" {
		testingHandle.Fatalf("unexpected response: %s", result.Content)
	}
}

// TestAskUserQuestionNoTTY verifies non-interactive runs error without override.
func TestAskUserQuestionNoTTY(testingHandle *testing.T) {
	previous := os.Getenv("OPENCLOUDE_ASK_RESPONSE")
	if err := os.Setenv("OPENCLOUDE_ASK_RESPONSE", ""); err != nil {
		testingHandle.Fatalf("clear env: %v", err)
	}
	defer os.Setenv("OPENCLOUDE_ASK_RESPONSE", previous)

	if stdinIsTTY() {
		testingHandle.Skip("stdin is a TTY in this environment")
	}

	tool := &AskUserQuestionTool{}
	payload, err := json.Marshal(map[string]any{
		"question": "Proceed?",
	})
	if err != nil {
		testingHandle.Fatalf("marshal payload: %v", err)
	}

	result, runErr := tool.Run(context.Background(), payload, ToolContext{})
	if runErr != nil {
		testingHandle.Fatalf("run tool: %v", runErr)
	}
	if !result.IsError {
		testingHandle.Fatalf("expected error without interactive TTY")
	}
}
