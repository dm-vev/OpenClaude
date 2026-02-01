package streamjson

import (
	"bufio"
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/openclaude/openclaude/internal/llm/openai"
	"github.com/openclaude/openclaude/internal/testutil"
)

// TestOpenAIStreamEmitterText verifies text streaming events against a JSONL fixture.
func TestOpenAIStreamEmitterText(testingHandle *testing.T) {
	// Arrange a stream emitter with a buffered writer.
	var buffer bytes.Buffer
	writer := NewWriter(&buffer)
	emitter := NewOpenAIStreamEmitter(writer, true, "session-1")
	emitter.Begin("model-x")

	// Emit deterministic text chunks to simulate SSE deltas.
	events := []openai.StreamResponse{
		{
			ID:    "req-1",
			Model: "model-x",
			Choices: []openai.StreamChoice{
				{Index: 0, Delta: openai.StreamDelta{Role: "assistant"}},
			},
		},
		{
			Choices: []openai.StreamChoice{
				{Index: 0, Delta: openai.StreamDelta{Content: "Hello "}},
			},
		},
		{
			Choices: []openai.StreamChoice{
				{Index: 0, Delta: openai.StreamDelta{Content: "world"}},
			},
		},
		{
			Choices: []openai.StreamChoice{
				{Index: 0, Delta: openai.StreamDelta{}, FinishReason: stringPointer("stop")},
			},
		},
	}

	for _, event := range events {
		testutil.RequireNoError(testingHandle, emitter.Handle(event), "emit stream event")
	}

	_, ok, err := emitter.Finalize()
	testutil.RequireNoError(testingHandle, err, "finalize stream emitter")
	testutil.RequireTrue(testingHandle, ok, "expected a finalized message")

	gotLines := normalizeStreamJSONLines(testingHandle, buffer.Bytes())
	wantLines := loadFixtureLines(testingHandle, "stream_text.jsonl")

	testutil.RequireEqual(testingHandle, gotLines, wantLines, "stream text output mismatch")
}

// TestOpenAIStreamEmitterTool verifies tool_use streaming events against a JSONL fixture.
func TestOpenAIStreamEmitterTool(testingHandle *testing.T) {
	// Arrange a stream emitter with a buffered writer.
	var buffer bytes.Buffer
	writer := NewWriter(&buffer)
	emitter := NewOpenAIStreamEmitter(writer, true, "session-1")
	emitter.Begin("model-x")

	// Emit tool call deltas with partial JSON arguments.
	events := []openai.StreamResponse{
		{
			ID:    "req-2",
			Model: "model-x",
			Choices: []openai.StreamChoice{
				{
					Index: 0,
					Delta: openai.StreamDelta{
						ToolCalls: []openai.StreamToolCallDelta{
							{
								Index: 0,
								ID:    "call_1",
								Type:  "function",
								Function: openai.StreamToolCallFunctionDelta{
									Name: "read",
								},
							},
						},
					},
				},
			},
		},
		{
			Choices: []openai.StreamChoice{
				{
					Index: 0,
					Delta: openai.StreamDelta{
						ToolCalls: []openai.StreamToolCallDelta{
							{
								Index: 0,
								Function: openai.StreamToolCallFunctionDelta{
									Arguments: `{"path":"README.md"}`,
								},
							},
						},
					},
				},
			},
		},
		{
			Choices: []openai.StreamChoice{
				{Index: 0, Delta: openai.StreamDelta{}, FinishReason: stringPointer("tool_calls")},
			},
		},
	}

	for _, event := range events {
		testutil.RequireNoError(testingHandle, emitter.Handle(event), "emit tool stream event")
	}

	_, ok, err := emitter.Finalize()
	testutil.RequireNoError(testingHandle, err, "finalize tool stream")
	testutil.RequireTrue(testingHandle, ok, "expected a finalized tool message")

	gotLines := normalizeStreamJSONLines(testingHandle, buffer.Bytes())
	wantLines := loadFixtureLines(testingHandle, "stream_tool.jsonl")

	testutil.RequireEqual(testingHandle, gotLines, wantLines, "stream tool output mismatch")
}

// normalizeStreamJSONLines replaces unstable fields before comparisons.
func normalizeStreamJSONLines(testingHandle *testing.T, output []byte) []any {
	testingHandle.Helper()

	var normalized []any
	scanner := bufio.NewScanner(bytes.NewReader(output))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		var payload any
		testutil.RequireNoError(testingHandle, json.Unmarshal([]byte(line), &payload), "parse output line")
		normalizeStreamEventMetadata(payload)
		normalized = append(normalized, payload)
	}
	testutil.RequireNoError(testingHandle, scanner.Err(), "scan output lines")
	return normalized
}

// loadFixtureLines reads a JSONL fixture file into a slice of generic objects.
func loadFixtureLines(testingHandle *testing.T, name string) []any {
	testingHandle.Helper()

	fixturePath := filepath.Join("testdata", name)
	contents, err := os.ReadFile(fixturePath)
	testutil.RequireNoError(testingHandle, err, "read fixture")

	var lines []any
	scanner := bufio.NewScanner(bytes.NewReader(contents))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		var payload any
		testutil.RequireNoError(testingHandle, json.Unmarshal([]byte(line), &payload), "parse fixture line")
		lines = append(lines, payload)
	}
	testutil.RequireNoError(testingHandle, scanner.Err(), "scan fixture lines")
	return lines
}

// normalizeStreamEventMetadata replaces unstable stream metadata before comparison.
func normalizeStreamEventMetadata(payload any) {
	root, ok := payload.(map[string]any)
	if !ok {
		return
	}
	if root["type"] != "stream_event" {
		return
	}
	root["uuid"] = "<uuid>"
	event, ok := root["event"].(map[string]any)
	if !ok {
		return
	}
	if event["type"] != "message_start" {
		return
	}
	message, ok := event["message"].(map[string]any)
	if !ok {
		return
	}
	message["id"] = "<message_id>"
}

// stringPointer returns a pointer to the provided string.
func stringPointer(value string) *string {
	return &value
}
