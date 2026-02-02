package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"sync"

	"github.com/openclaude/openclaude/internal/session"
)

// streamJSONRecorder captures emitted stream-json lines while forwarding output.
// It preserves the exact bytes written to stdout so replay stays bit-for-bit.
// identical to the original JSON lines (including escape rules and field order).
type streamJSONRecorder struct {
	// target is the underlying output destination (usually stdout).
	target io.Writer
	// store persists stream-json lines for replay.
	store *session.Store
	// sessionID scopes the persisted stream-json lines.
	sessionID string
	// enabled toggles whether new stream-json lines should be persisted.
	enabled bool
	// buffer accumulates partial writes until full JSON lines are available.
	buffer bytes.Buffer
	// mu guards access to enabled and buffer state.
	mu sync.Mutex
}

// streamJSONEnvelope captures the minimum fields needed to filter replayable events.
type streamJSONEnvelope struct {
	// Type identifies the stream-json envelope type.
	Type string `json:"type"`
	// Subtype differentiates system event subtypes.
	Subtype string `json:"subtype,omitempty"`
}

// newStreamJSONRecorder constructs a recorder that forwards to the target writer.
// The recorder does not assume the target implements any extra interfaces.
func newStreamJSONRecorder(target io.Writer, store *session.Store, sessionID string) *streamJSONRecorder {
	return &streamJSONRecorder{
		target:    target,
		store:     store,
		sessionID: sessionID,
		enabled:   true,
	}
}

// SetRecording toggles whether the recorder should persist stream-json lines.
// This is used to avoid re-recording replayed output.
func (r *streamJSONRecorder) SetRecording(enabled bool) {
	if r == nil {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.enabled = enabled
}

// WithRecordingDisabled runs a callback while disabling persistence.
// The callback still writes to the target writer so output remains intact.
func (r *streamJSONRecorder) WithRecordingDisabled(fn func(io.Writer) error) error {
	if r == nil {
		if fn == nil {
			return nil
		}
		return fn(nil)
	}
	r.mu.Lock()
	previous := r.enabled
	r.enabled = false
	r.mu.Unlock()

	var err error
	if fn != nil {
		err = fn(r)
	}

	r.mu.Lock()
	r.enabled = previous
	r.mu.Unlock()
	return err
}

// Write forwards bytes to the target writer and captures full JSON lines.
// It assumes the upstream writer emits newline-delimited JSON.
func (r *streamJSONRecorder) Write(payload []byte) (int, error) {
	if r == nil {
		return 0, fmt.Errorf("stream-json recorder is required")
	}
	if r.target == nil {
		return 0, fmt.Errorf("stream-json output target is required")
	}
	written, err := r.target.Write(payload)
	if err != nil {
		return written, err
	}
	if written != len(payload) {
		return written, io.ErrShortWrite
	}

	r.mu.Lock()
	defer r.mu.Unlock()
	if !r.enabled || r.store == nil {
		return written, nil
	}

	// Accumulate bytes so we can split on newline boundaries.
	_, _ = r.buffer.Write(payload)
	for {
		line, ok := r.nextLineLocked()
		if !ok {
			break
		}
		if err := r.persistLineLocked(line); err != nil {
			return written, err
		}
	}
	return written, nil
}

// nextLineLocked extracts the next newline-delimited JSON line.
func (r *streamJSONRecorder) nextLineLocked() (string, bool) {
	data := r.buffer.Bytes()
	index := bytes.IndexByte(data, '\n')
	if index < 0 {
		return "", false
	}
	line := string(data[:index])
	_ = r.buffer.Next(index + 1)
	return line, true
}

// persistLineLocked persists a JSON line when it matches replay criteria.
// This keeps the persisted payload in its original JSON text form.
func (r *streamJSONRecorder) persistLineLocked(line string) error {
	trimmed := strings.TrimSpace(line)
	if trimmed == "" {
		return nil
	}
	envelope, err := parseStreamJSONEnvelope(trimmed)
	if err != nil {
		// Ignore malformed lines so output is not interrupted by parser failures.
		return nil
	}
	if !shouldPersistStreamJSONEnvelope(envelope) {
		return nil
	}
	if err := r.store.AppendStreamJSONLine(r.sessionID, trimmed); err != nil {
		return fmt.Errorf("persist stream-json event: %w", err)
	}
	return nil
}

// parseStreamJSONEnvelope decodes the minimal envelope needed for filtering.
func parseStreamJSONEnvelope(line string) (streamJSONEnvelope, error) {
	var envelope streamJSONEnvelope
	if err := json.Unmarshal([]byte(line), &envelope); err != nil {
		return streamJSONEnvelope{}, err
	}
	return envelope, nil
}

// shouldPersistStreamJSONEnvelope reports whether an event should be replayed.
// We only persist user events because --replay-user-messages only replays them.
func shouldPersistStreamJSONEnvelope(envelope streamJSONEnvelope) bool {
	return shouldReplayStreamJSONEnvelope(envelope)
}

// shouldReplayStreamJSONEnvelope filters stored lines during replay.
// This protects against older sessions that stored non-user lines.
func shouldReplayStreamJSONEnvelope(envelope streamJSONEnvelope) bool {
	switch envelope.Type {
	case "user", "user_message":
		return true
	default:
		return false
	}
}

// replayStoredStreamJSON replays stored user stream-json events before new output.
func replayStoredStreamJSON(store *session.Store, sessionID string, writer io.Writer) (bool, error) {
	if store == nil {
		return false, nil
	}
	if sessionID == "" {
		return false, fmt.Errorf("session id is required for stream-json replay")
	}
	lines, err := store.LoadStreamJSONLines(sessionID)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return false, nil
		}
		return false, fmt.Errorf("load stream-json replay: %w", err)
	}
	if len(lines) == 0 {
		return false, nil
	}
	replayed := false
	writeLines := func(target io.Writer) error {
		for _, line := range lines {
			trimmed := strings.TrimSpace(line)
			if trimmed == "" {
				continue
			}
			envelope, err := parseStreamJSONEnvelope(trimmed)
			if err != nil {
				continue
			}
			if !shouldReplayStreamJSONEnvelope(envelope) {
				continue
			}
			if _, err := io.WriteString(target, trimmed+"\n"); err != nil {
				return fmt.Errorf("write stream-json replay: %w", err)
			}
			replayed = true
		}
		return nil
	}
	if recorder, ok := writer.(*streamJSONRecorder); ok && recorder != nil {
		if err := recorder.WithRecordingDisabled(writeLines); err != nil {
			return false, err
		}
		return replayed, nil
	}
	if err := writeLines(writer); err != nil {
		return false, err
	}
	return replayed, nil
}
