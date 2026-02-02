package session

import (
	"bufio"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// Store manages session persistence under ~/.openclaude.
type Store struct {
	// BaseDir is the root for all persisted data.
	BaseDir string
}

// streamJSONRecordType marks stream-json line records stored in session JSONL.
// Keeping a distinct type avoids mixing with message/tool events.
const streamJSONRecordType = "stream_json"

// StreamJSONRecord wraps a stream-json line for session persistence.
// Lines are stored verbatim so replay can emit identical JSON text.
type StreamJSONRecord struct {
	// Type tags the record as stream-json so loaders can filter it.
	Type string `json:"type"`
	// Line holds the raw JSON line without a trailing newline.
	Line string `json:"line"`
}

// NewStore constructs a Store using the default base directory.
func NewStore() (*Store, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("resolve home dir: %w", err)
	}
	return &Store{BaseDir: filepath.Join(home, ".openclaude")}, nil
}

// ProjectHash returns a stable hash for the current workspace path.
func ProjectHash(path string) string {
	clean := filepath.Clean(path)
	sum := sha256.Sum256([]byte(clean))
	return hex.EncodeToString(sum[:8])
}

// SessionPath returns the JSONL path for a session.
func (s *Store) SessionPath(sessionID string) string {
	return filepath.Join(s.BaseDir, "sessions", sessionID+".jsonl")
}

// AppendEvent writes a JSONL event for the session.
func (s *Store) AppendEvent(sessionID string, event any) error {
	if sessionID == "" {
		return errors.New("session id required")
	}
	path := s.SessionPath(sessionID)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create session dir: %w", err)
	}
	file, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return fmt.Errorf("open session file: %w", err)
	}
	defer file.Close()

	data, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("marshal session event: %w", err)
	}

	if _, err := file.Write(append(data, '\n')); err != nil {
		return fmt.Errorf("write session event: %w", err)
	}

	return nil
}

// AppendStreamJSONLine stores a stream-json line for later replay.
// It trims surrounding whitespace so empty lines do not pollute the session log.
func (s *Store) AppendStreamJSONLine(sessionID string, line string) error {
	// Persist only non-empty lines to avoid cluttering the session log.
	trimmed := strings.TrimSpace(line)
	if trimmed == "" {
		return nil
	}
	record := StreamJSONRecord{
		Type: streamJSONRecordType,
		Line: trimmed,
	}
	return s.AppendEvent(sessionID, record)
}

// LoadEvents reads all JSONL events from a session file.
func (s *Store) LoadEvents(sessionID string) ([]json.RawMessage, error) {
	path := s.SessionPath(sessionID)
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	var events []json.RawMessage
	scanner := bufio.NewScanner(file)
	// Increase the scanner buffer so large stream-json lines are not dropped.
	// The cap is intentionally generous to handle tool outputs without truncation.
	const maxEventSize = 10 * 1024 * 1024
	scanner.Buffer(make([]byte, 0, 64*1024), maxEventSize)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		events = append(events, json.RawMessage(line))
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("read session file: %w", err)
	}
	return events, nil
}

// LoadStreamJSONLines returns stored stream-json lines in session order.
// It skips malformed entries so replay is resilient to partial writes.
func (s *Store) LoadStreamJSONLines(sessionID string) ([]string, error) {
	events, err := s.LoadEvents(sessionID)
	if err != nil {
		return nil, err
	}
	lines := make([]string, 0, len(events))
	for _, raw := range events {
		var record StreamJSONRecord
		// Ignore malformed entries to keep replay resilient.
		if err := json.Unmarshal(raw, &record); err != nil {
			continue
		}
		if record.Type != streamJSONRecordType || record.Line == "" {
			continue
		}
		lines = append(lines, record.Line)
	}
	return lines, nil
}

// CloneSession copies events from one session id to another.
func (s *Store) CloneSession(fromSessionID string, toSessionID string) error {
	if fromSessionID == "" || toSessionID == "" {
		return errors.New("session id required")
	}
	if fromSessionID == toSessionID {
		return nil
	}
	events, err := s.LoadEvents(fromSessionID)
	if err != nil {
		return err
	}
	for _, event := range events {
		if err := s.AppendEvent(toSessionID, event); err != nil {
			return err
		}
	}
	return nil
}

// SaveLastSession stores the last session id for a project hash.
func (s *Store) SaveLastSession(projectHash string, sessionID string) error {
	path := filepath.Join(s.BaseDir, "projects", projectHash, "last_session")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create project dir: %w", err)
	}
	if err := os.WriteFile(path, []byte(sessionID), 0o600); err != nil {
		return fmt.Errorf("write last session: %w", err)
	}
	return nil
}

// LoadLastSession returns the last session id for a project hash.
func (s *Store) LoadLastSession(projectHash string) (string, error) {
	path := filepath.Join(s.BaseDir, "projects", projectHash, "last_session")
	raw, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(raw)), nil
}

// ListSessions returns recent session ids sorted by modification time desc.
func (s *Store) ListSessions(limit int) ([]string, error) {
	dir := filepath.Join(s.BaseDir, "sessions")
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}

	type entry struct {
		Name string
		Time time.Time
	}

	var list []entry
	for _, item := range entries {
		if item.IsDir() {
			continue
		}
		info, err := item.Info()
		if err != nil {
			continue
		}
		name := strings.TrimSuffix(item.Name(), filepath.Ext(item.Name()))
		list = append(list, entry{Name: name, Time: info.ModTime()})
	}

	sort.Slice(list, func(i, j int) bool {
		return list[i].Time.After(list[j].Time)
	})

	if limit > 0 && len(list) > limit {
		list = list[:limit]
	}

	result := make([]string, 0, len(list))
	for _, item := range list {
		result = append(result, item.Name)
	}
	return result, nil
}
