package streamjson

import (
	"bufio"
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/openclaude/openclaude/internal/testutil"
)

// TestStreamJSONSystemEventFixtures verifies hook/auth/keep_alive JSONL ordering and payloads.
func TestStreamJSONSystemEventFixtures(testingHandle *testing.T) {
	// Arrange a stream-json writer with deterministic event payloads.
	var buffer bytes.Buffer
	writer := NewWriter(&buffer)
	events := []any{
		HookStartedEvent{
			Type:      "system",
			Subtype:   "hook_started",
			HookID:    "hook-1",
			HookName:  "preflight",
			HookEvent: "before_prompt",
			UUID:      "<uuid>",
			SessionID: "session-1",
		},
		HookProgressEvent{
			Type:      "system",
			Subtype:   "hook_progress",
			HookID:    "hook-1",
			HookName:  "preflight",
			HookEvent: "before_prompt",
			Stdout:    "running\n",
			Stderr:    "warn\n",
			Output:    "progress",
			UUID:      "<uuid>",
			SessionID: "session-1",
		},
		HookResponseEvent{
			Type:      "system",
			Subtype:   "hook_response",
			HookID:    "hook-1",
			HookName:  "preflight",
			HookEvent: "before_prompt",
			Output:    "done",
			Stdout:    "ok\n",
			Stderr:    "warn\n",
			ExitCode:  1,
			Outcome:   "failed",
			UUID:      "<uuid>",
			SessionID: "session-1",
		},
		AuthStatusEvent{
			Type:             "auth_status",
			IsAuthenticating: true,
			Output:           "Waiting for login",
			Error:            "Missing token",
			UUID:             "<uuid>",
			SessionID:        "session-1",
		},
		KeepAliveEvent{
			Type: "keep_alive",
		},
	}

	for _, event := range events {
		testutil.RequireNoError(testingHandle, writer.Write(event), "write stream-json event")
	}

	gotLines := readJSONLLinesRaw(testingHandle, buffer.Bytes())
	wantLines := readFixtureLinesRaw(testingHandle, "stream_hook_auth_keep_alive.jsonl")

	testutil.RequireEqual(
		testingHandle,
		gotLines,
		wantLines,
		"hook/auth/keep_alive stream-json output mismatch",
	)
}

// readJSONLLinesRaw splits a JSONL payload into trimmed, non-empty lines.
func readJSONLLinesRaw(testingHandle *testing.T, payload []byte) []string {
	testingHandle.Helper()

	var lines []string
	scanner := bufio.NewScanner(bytes.NewReader(payload))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		lines = append(lines, line)
	}
	testutil.RequireNoError(testingHandle, scanner.Err(), "scan output lines")
	return lines
}

// readFixtureLinesRaw reads a JSONL fixture file into trimmed lines.
func readFixtureLinesRaw(testingHandle *testing.T, name string) []string {
	testingHandle.Helper()

	fixturePath := filepath.Join("testdata", name)
	contents, err := os.ReadFile(fixturePath)
	testutil.RequireNoError(testingHandle, err, "read fixture")

	return readJSONLLinesRaw(testingHandle, contents)
}
