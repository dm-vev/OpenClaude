package main

import (
	"os"
	"strings"
	"testing"
)

// TestReadStreamInputWithControlParsesUserAndControl verifies control and user parsing.
func TestReadStreamInputWithControlParsesUserAndControl(testingHandle *testing.T) {
	// Arrange a stream-json input with control, env update, and user lines.
	payload := strings.Join([]string{
		`{"type":"control_request","request_id":"req-1","request":{"subtype":"initialize","systemPrompt":"hello"}}`,
		`{"type":"update_environment_variables","variables":{"OPENCLAUDE_TEST_ENV":"1"}}`,
		`{"type":"user","message":{"role":"user","content":"hi"},"uuid":"user-1","isSynthetic":true}`,
	}, "\n")

	// Clean up environment mutation after the test.
	previous := os.Getenv("OPENCLAUDE_TEST_ENV")
	testingHandle.Cleanup(func() {
		if previous == "" {
			os.Unsetenv("OPENCLAUDE_TEST_ENV")
		} else {
			os.Setenv("OPENCLAUDE_TEST_ENV", previous)
		}
	})

	// Act.
	parsed, err := readStreamInputWithControl(strings.NewReader(payload))

	// Assert.
	if err != nil {
		testingHandle.Fatalf("readStreamInputWithControl error: %v", err)
	}
	if len(parsed.ControlRequests) != 1 {
		testingHandle.Fatalf("expected 1 control request, got %d", len(parsed.ControlRequests))
	}
	if parsed.ControlRequests[0].RequestID != "req-1" {
		testingHandle.Fatalf("expected control request_id req-1, got %s", parsed.ControlRequests[0].RequestID)
	}
	if len(parsed.Messages) != 1 || parsed.Messages[0].Role != "user" {
		testingHandle.Fatalf("expected one user message, got %+v", parsed.Messages)
	}
	if len(parsed.UserMessages) != 1 || parsed.UserMessages[0].UUID != "user-1" {
		testingHandle.Fatalf("expected user UUID user-1, got %+v", parsed.UserMessages)
	}
	if !parsed.UserMessages[0].IsSynthetic {
		testingHandle.Fatalf("expected synthetic user message flag to be true")
	}
	if got := os.Getenv("OPENCLAUDE_TEST_ENV"); got != "1" {
		testingHandle.Fatalf("expected env var set, got %q", got)
	}
}

// TestReadStreamInputWithControlRejectsUnknownType ensures strict input validation.
func TestReadStreamInputWithControlRejectsUnknownType(testingHandle *testing.T) {
	// Arrange a payload with an unsupported top-level type.
	payload := `{"type":"assistant","message":{"role":"assistant","content":"hi"}}`

	// Act.
	_, err := readStreamInputWithControl(strings.NewReader(payload))

	// Assert.
	if err == nil {
		testingHandle.Fatalf("expected error for unsupported payload type")
	}
}
