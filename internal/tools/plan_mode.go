package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/openclaude/openclaude/internal/session"
)

// planModeFilename stores a marker file that toggles plan-only behavior.
const planModeFilename = "plan_mode"

// EnterPlanModeTool enables plan-only mode for the current session.
type EnterPlanModeTool struct{}

// Name returns the tool identifier used in tool calls.
func (t *EnterPlanModeTool) Name() string {
	return "EnterPlanMode"
}

// Description summarizes the plan mode toggle behavior.
func (t *EnterPlanModeTool) Description() string {
	return "Enable plan-only mode for the current session."
}

// Schema accepts arbitrary JSON so upstream payloads remain compatible.
func (t *EnterPlanModeTool) Schema() map[string]any {
	return map[string]any{
		"type":                 "object",
		"additionalProperties": true,
	}
}

// Run enables plan-only mode for the session.
func (t *EnterPlanModeTool) Run(ctx context.Context, input json.RawMessage, toolCtx ToolContext) (ToolResult, error) {
	// The tool is synchronous, so the context is unused by design.
	_ = ctx
	_ = input

	if err := SetPlanMode(toolCtx.Store, toolCtx.SessionID, true); err != nil {
		return ToolResult{IsError: true, Content: fmt.Sprintf("enter plan mode: %v", err)}, nil
	}
	return ToolResult{Content: "ok"}, nil
}

// ExitPlanModeTool disables plan-only mode for the current session.
type ExitPlanModeTool struct{}

// Name returns the tool identifier used in tool calls.
func (t *ExitPlanModeTool) Name() string {
	return "ExitPlanMode"
}

// Description summarizes the plan mode toggle behavior.
func (t *ExitPlanModeTool) Description() string {
	return "Disable plan-only mode for the current session."
}

// Schema accepts arbitrary JSON so upstream payloads remain compatible.
func (t *ExitPlanModeTool) Schema() map[string]any {
	return map[string]any{
		"type":                 "object",
		"additionalProperties": true,
	}
}

// Run disables plan-only mode for the session.
func (t *ExitPlanModeTool) Run(ctx context.Context, input json.RawMessage, toolCtx ToolContext) (ToolResult, error) {
	// The tool is synchronous, so the context is unused by design.
	_ = ctx
	_ = input

	if err := SetPlanMode(toolCtx.Store, toolCtx.SessionID, false); err != nil {
		return ToolResult{IsError: true, Content: fmt.Sprintf("exit plan mode: %v", err)}, nil
	}
	return ToolResult{Content: "ok"}, nil
}

// SetPlanMode toggles plan-only mode for a session by writing a marker file.
func SetPlanMode(store *session.Store, sessionID string, enabled bool) error {
	if store == nil || sessionID == "" {
		return fmt.Errorf("session store unavailable")
	}
	path := planModePath(store, sessionID)
	if enabled {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(path, []byte("1"), 0o600); err != nil {
			return err
		}
		return nil
	}
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

// IsPlanMode reports whether plan-only mode is enabled for the session.
func IsPlanMode(store *session.Store, sessionID string) bool {
	if store == nil || sessionID == "" {
		return false
	}
	_, err := os.Stat(planModePath(store, sessionID))
	return err == nil
}

// planModePath returns the marker file path for a session.
func planModePath(store *session.Store, sessionID string) string {
	return filepath.Join(store.BaseDir, "session-env", sessionID, planModeFilename)
}
