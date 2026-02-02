package main

import (
	"bytes"
	"io"
	"strings"
	"testing"

	"github.com/openclaude/openclaude/internal/session"
)

// TestStreamJSONRecorderPersistsReplayableLines verifies recorder filtering and forwarding.
func TestStreamJSONRecorderPersistsReplayableLines(testingHandle *testing.T) {
	// Arrange a recorder with an in-memory session store.
	store := &session.Store{BaseDir: testingHandle.TempDir()}
	var buffer bytes.Buffer
	recorder := newStreamJSONRecorder(&buffer, store, "session-1")

	lines := []string{
		`{"type":"system","subtype":"init","uuid":"uuid-init","session_id":"session-1"}`,
		`{"type":"user","uuid":"uuid-user","session_id":"session-1","message":{"role":"user","content":[{"type":"text","text":"hi"}]}}`,
		`{"type":"keep_alive"}`,
		`{"type":"result","uuid":"uuid-result","session_id":"session-1"}`,
		`{"type":"assistant","uuid":"uuid-assistant","session_id":"session-1","message":{"role":"assistant","content":[{"type":"text","text":"ok"}]}}`,
	}

	// Act: write each line as a newline-delimited JSON entry.
	for _, line := range lines {
		if _, err := io.WriteString(recorder, line+"\n"); err != nil {
			testingHandle.Fatalf("write stream-json line: %v", err)
		}
	}

	// Assert: only user lines are persisted for replay-user-messages.
	stored, err := store.LoadStreamJSONLines("session-1")
	if err != nil {
		testingHandle.Fatalf("load stream-json lines: %v", err)
	}
	expected := []string{lines[1]}
	if len(stored) != len(expected) {
		testingHandle.Fatalf("expected %d stored lines, got %d", len(expected), len(stored))
	}
	for index, line := range expected {
		if stored[index] != line {
			testingHandle.Fatalf("expected stored line %q, got %q", line, stored[index])
		}
	}

	// Assert: output is forwarded unchanged to the target buffer.
	expectedOutput := strings.Join(lines, "\n") + "\n"
	if buffer.String() != expectedOutput {
		testingHandle.Fatalf("unexpected output buffer: %q", buffer.String())
	}
}

// TestReplayStoredStreamJSONSkipsRecording ensures replay does not duplicate persistence.
func TestReplayStoredStreamJSONSkipsRecording(testingHandle *testing.T) {
	// Arrange a stored stream-json line and a recorder that will replay it.
	store := &session.Store{BaseDir: testingHandle.TempDir()}
	line := `{"type":"user","uuid":"uuid-user","session_id":"session-1","message":{"role":"user","content":[{"type":"text","text":"hi"}]}}`
	if err := store.AppendStreamJSONLine("session-1", line); err != nil {
		testingHandle.Fatalf("append stream-json line: %v", err)
	}

	var buffer bytes.Buffer
	recorder := newStreamJSONRecorder(&buffer, store, "session-1")
	recorder.SetRecording(true)

	// Act: replay stored history through the recorder.
	replayed, err := replayStoredStreamJSON(store, "session-1", recorder)
	if err != nil {
		testingHandle.Fatalf("replay stored stream-json: %v", err)
	}
	if !replayed {
		testingHandle.Fatalf("expected replayed history to be true")
	}

	// Assert: replay output is written once and persistence is not duplicated.
	if buffer.String() != line+"\n" {
		testingHandle.Fatalf("unexpected replay output: %q", buffer.String())
	}
	stored, err := store.LoadStreamJSONLines("session-1")
	if err != nil {
		testingHandle.Fatalf("load stream-json lines: %v", err)
	}
	if len(stored) != 1 {
		testingHandle.Fatalf("expected 1 stored line, got %d", len(stored))
	}
}

// TestReplayStoredStreamJSONFiltersNonUser ensures non-user lines are ignored on replay.
func TestReplayStoredStreamJSONFiltersNonUser(testingHandle *testing.T) {
	store := &session.Store{BaseDir: testingHandle.TempDir()}
	userLine := `{"type":"user","uuid":"uuid-user","session_id":"session-1","message":{"role":"user","content":[{"type":"text","text":"hi"}]}}`
	assistantLine := `{"type":"assistant","uuid":"uuid-assistant","session_id":"session-1","message":{"role":"assistant","content":[{"type":"text","text":"ok"}]}}`
	if err := store.AppendStreamJSONLine("session-1", userLine); err != nil {
		testingHandle.Fatalf("append user line: %v", err)
	}
	if err := store.AppendStreamJSONLine("session-1", assistantLine); err != nil {
		testingHandle.Fatalf("append assistant line: %v", err)
	}

	var buffer bytes.Buffer
	replayed, err := replayStoredStreamJSON(store, "session-1", &buffer)
	if err != nil {
		testingHandle.Fatalf("replay stored stream-json: %v", err)
	}
	if !replayed {
		testingHandle.Fatalf("expected replay to be true")
	}
	if buffer.String() != userLine+"\n" {
		testingHandle.Fatalf("unexpected replay output: %q", buffer.String())
	}
}
