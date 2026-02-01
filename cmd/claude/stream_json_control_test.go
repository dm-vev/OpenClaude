package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/openclaude/openclaude/internal/agent"
	"github.com/openclaude/openclaude/internal/config"
	"github.com/openclaude/openclaude/internal/streamjson"
	"github.com/openclaude/openclaude/internal/tools"
)

// TestApplyStreamJSONControlRequestsInitialize validates initialize control responses.
func TestApplyStreamJSONControlRequestsInitialize(testingHandle *testing.T) {
	// Arrange a parsed initialize control request with a deterministic request id.
	parsed := &streamJSONInput{
		ControlRequests: []streamJSONControlRequest{
			{
				RequestID: "req-1",
				Request:   map[string]any{"subtype": "initialize"},
			},
		},
	}
	opts := &options{}
	runner := &agent.Runner{Permissions: tools.Permissions{Mode: tools.PermissionDefault}}
	settings := &config.Settings{}

	var buffer bytes.Buffer
	writer := streamjson.NewWriter(&buffer)

	// Act.
	_, _, err := applyStreamJSONControlRequests(parsed, writer, opts, runner, settings, "session-1", "model-x")

	// Assert.
	if err != nil {
		testingHandle.Fatalf("applyStreamJSONControlRequests error: %v", err)
	}
	scanner := bufio.NewScanner(strings.NewReader(buffer.String()))
	if !scanner.Scan() {
		testingHandle.Fatalf("expected control_response output")
	}
	var payload map[string]any
	if err := json.Unmarshal(scanner.Bytes(), &payload); err != nil {
		testingHandle.Fatalf("parse control_response JSON: %v", err)
	}
	if payload["type"] != "control_response" {
		testingHandle.Fatalf("expected control_response type, got %v", payload["type"])
	}
	response, ok := payload["response"].(map[string]any)
	if !ok {
		testingHandle.Fatalf("expected response payload, got %T", payload["response"])
	}
	if response["request_id"] != "req-1" {
		testingHandle.Fatalf("expected request_id req-1, got %v", response["request_id"])
	}
}
