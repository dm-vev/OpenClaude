package main

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/openclaude/openclaude/internal/streamjson"
	"github.com/openclaude/openclaude/internal/testutil"
)

// TestResultEventFieldOrder verifies the result event key ordering matches cli.js.
func TestResultEventFieldOrder(testingHandle *testing.T) {
	// Build a deterministic result event with all required fields populated.
	resultEvent := streamjson.ResultEvent{
		Type:              "result",
		Subtype:           "success",
		IsError:           false,
		DurationMS:        123,
		DurationAPIMS:     45,
		NumTurns:          2,
		Result:            "ok",
		SessionID:         "session-1",
		TotalCostUSD:      0.12,
		Usage:             streamjson.NewEmptyMessageUsage(streamjson.StandardServiceTier),
		ModelUsage:        map[string]streamjson.MessageUsage{"model-x": *streamjson.NewEmptyMessageUsage(streamjson.StandardServiceTier)},
		PermissionDenials: []any{},
		UUID:              "uuid-result",
	}

	var buffer bytes.Buffer
	writer := streamjson.NewWriter(&buffer)
	testutil.RequireNoError(testingHandle, writer.Write(resultEvent), "write result event")

	line := strings.TrimSpace(buffer.String())
	if line == "" {
		testingHandle.Fatalf("expected result output line")
	}

	assertJSONKeyOrderResult(testingHandle, line, []string{
		"type",
		"subtype",
		"is_error",
		"duration_ms",
		"duration_api_ms",
		"num_turns",
		"result",
		"session_id",
		"total_cost_usd",
		"usage",
		"modelUsage",
		"permission_denials",
		"uuid",
	})

	var payload map[string]any
	testutil.RequireNoError(testingHandle, json.Unmarshal([]byte(line), &payload), "parse result JSON")
	if payload["type"] != "result" || payload["subtype"] != "success" {
		testingHandle.Fatalf("unexpected result payload: %v", payload)
	}
}

// assertJSONKeyOrderResult ensures keys appear in the expected order within the JSON line.
func assertJSONKeyOrderResult(testingHandle *testing.T, line string, keys []string) {
	testingHandle.Helper()
	lastIndex := -1
	for _, key := range keys {
		target := `"` + key + `":`
		index := strings.Index(line, target)
		if index == -1 {
			testingHandle.Fatalf("expected key %q in JSON line", key)
		}
		if index <= lastIndex {
			testingHandle.Fatalf("key %q appears out of order", key)
		}
		lastIndex = index
	}
}
