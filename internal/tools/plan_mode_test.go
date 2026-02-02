package tools

import (
	"testing"

	"github.com/openclaude/openclaude/internal/session"
)

// TestPlanModeToggle verifies plan mode marker handling.
func TestPlanModeToggle(testingHandle *testing.T) {
	store := &session.Store{BaseDir: testingHandle.TempDir()}
	sessionID := "session-1"

	if IsPlanMode(store, sessionID) {
		testingHandle.Fatalf("expected plan mode to be false initially")
	}
	if err := SetPlanMode(store, sessionID, true); err != nil {
		testingHandle.Fatalf("enable plan mode: %v", err)
	}
	if !IsPlanMode(store, sessionID) {
		testingHandle.Fatalf("expected plan mode to be true")
	}
	if err := SetPlanMode(store, sessionID, false); err != nil {
		testingHandle.Fatalf("disable plan mode: %v", err)
	}
	if IsPlanMode(store, sessionID) {
		testingHandle.Fatalf("expected plan mode to be false after disable")
	}
}
