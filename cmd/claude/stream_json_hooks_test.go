package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/openclaude/openclaude/internal/streamjson"
)

// TestParseStreamJSONHookConfig verifies hook definitions are parsed from raw input.
func TestParseStreamJSONHookConfig(testingHandle *testing.T) {
	// Arrange a minimal hooks payload with callback ids and timeout.
	raw := map[string]any{
		"PreToolUse": []any{
			map[string]any{
				"matcher":         "read|edit",
				"hookCallbackIds": []any{"cb-1", "cb-2"},
				"timeout":         5.0,
			},
		},
	}

	// Act.
	config := parseStreamJSONHookConfig(raw)

	// Assert.
	if config == nil {
		testingHandle.Fatalf("expected hook config, got nil")
	}
	definitions := config.Events["PreToolUse"]
	if len(definitions) != 1 {
		testingHandle.Fatalf("expected 1 hook definition, got %d", len(definitions))
	}
	if definitions[0].Matcher != "read|edit" {
		testingHandle.Fatalf("unexpected matcher: %s", definitions[0].Matcher)
	}
	if len(definitions[0].CallbackIDs) != 2 {
		testingHandle.Fatalf("expected 2 callback ids, got %d", len(definitions[0].CallbackIDs))
	}
	if definitions[0].TimeoutSeconds != 5 {
		testingHandle.Fatalf("expected timeout 5, got %d", definitions[0].TimeoutSeconds)
	}
}

// TestStreamJSONHookEmitterEmitsEvents ensures hook events are emitted for matching hooks.
func TestStreamJSONHookEmitterEmitsEvents(testingHandle *testing.T) {
	// Arrange a hook emitter with two callback ids.
	config := &streamJSONHookConfig{
		Events: map[string][]streamJSONHookDefinition{
			"PreToolUse": {
				{
					Matcher:     "read",
					CallbackIDs: []string{"cb-1", "cb-2"},
				},
			},
		},
	}
	var buffer bytes.Buffer
	writer := streamjson.NewWriter(&buffer)
	emitter := newStreamJSONHookEmitter(writer, "session-1", config)

	// Act.
	if err := emitter.EmitPreToolUse("read"); err != nil {
		testingHandle.Fatalf("EmitPreToolUse error: %v", err)
	}

	// Assert.
	lines := readJSONLines(testingHandle, buffer.Bytes())
	if len(lines) != 4 {
		testingHandle.Fatalf("expected 4 hook events, got %d", len(lines))
	}
	for index, payload := range lines {
		root, ok := payload.(map[string]any)
		if !ok {
			testingHandle.Fatalf("expected object payload at %d", index)
		}
		if root["type"] != "system" {
			testingHandle.Fatalf("expected system type, got %v", root["type"])
		}
		if root["hook_name"] != "PreToolUse:read" {
			testingHandle.Fatalf("unexpected hook_name: %v", root["hook_name"])
		}
		if root["hook_event"] != "PreToolUse" {
			testingHandle.Fatalf("unexpected hook_event: %v", root["hook_event"])
		}
	}
}

// TestStreamJSONHookEmitterSkipsMismatch ensures hooks are skipped when matcher does not match.
func TestStreamJSONHookEmitterSkipsMismatch(testingHandle *testing.T) {
	// Arrange a hook emitter with a matcher that does not match.
	config := &streamJSONHookConfig{
		Events: map[string][]streamJSONHookDefinition{
			"PreToolUse": {
				{
					Matcher:     "edit",
					CallbackIDs: []string{"cb-1"},
				},
			},
		},
	}
	var buffer bytes.Buffer
	writer := streamjson.NewWriter(&buffer)
	emitter := newStreamJSONHookEmitter(writer, "session-1", config)

	// Act.
	if err := emitter.EmitPreToolUse("read"); err != nil {
		testingHandle.Fatalf("EmitPreToolUse error: %v", err)
	}

	// Assert.
	lines := readJSONLines(testingHandle, buffer.Bytes())
	if len(lines) != 0 {
		testingHandle.Fatalf("expected no hook events, got %d", len(lines))
	}
}

// readJSONLines parses newline-delimited JSON into generic objects.
func readJSONLines(testingHandle *testing.T, data []byte) []any {
	testingHandle.Helper()

	var lines []any
	scanner := bufio.NewScanner(bytes.NewReader(data))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		var payload any
		if err := json.Unmarshal([]byte(line), &payload); err != nil {
			testingHandle.Fatalf("parse json line: %v", err)
		}
		lines = append(lines, payload)
	}
	if err := scanner.Err(); err != nil {
		testingHandle.Fatalf("scan json lines: %v", err)
	}
	return lines
}
