package main

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/openclaude/openclaude/internal/agent"
	"github.com/openclaude/openclaude/internal/config"
	"github.com/openclaude/openclaude/internal/streamjson"
	"github.com/openclaude/openclaude/internal/testutil"
	"github.com/openclaude/openclaude/internal/tools"
)

// TestSystemInitEventFieldsAndOrder verifies required init fields, ordering, and list payloads.
func TestSystemInitEventFieldsAndOrder(testingHandle *testing.T) {
	// Build a tool runner using the canonical Claude Code tool ordering.
	toolRunner := tools.NewRunner(tools.DefaultTools())
	runner := &agent.Runner{
		ToolRunner:  toolRunner,
		Permissions: tools.Permissions{Mode: tools.PermissionDefault},
	}

	// Configure plugins and output style to exercise init payload population paths.
	opts := &options{
		PluginDir: []string{"/plugins/plugin-b", "/plugins/plugin-a"},
	}
	settings := &config.Settings{
		Raw: map[string]any{
			"outputStyle": "default",
		},
		EnabledPlugins: map[string]bool{
			"plugin-z":   true,
			"plugin-c":   true,
			"plugin-a":   true,
			"plugin-off": false,
		},
	}

	// Build the init event with deterministic values where possible.
	initEvent := buildSystemInitEvent(opts, runner, "model-x", "session-1", settings, "config")
	initEvent.UUID = "uuid-init"

	var buffer bytes.Buffer
	writer := streamjson.NewWriter(&buffer)
	testutil.RequireNoError(testingHandle, writer.Write(initEvent), "write init event")

	line := strings.TrimSpace(buffer.String())
	if line == "" {
		testingHandle.Fatalf("expected init output line")
	}

	// Ensure the JSON key ordering matches the Claude Code reference order.
	assertJSONKeyOrder(testingHandle, line, []string{
		"type",
		"subtype",
		"cwd",
		"session_id",
		"tools",
		"mcp_servers",
		"model",
		"permissionMode",
		"slash_commands",
		"apiKeySource",
		"betas",
		"claude_code_version",
		"output_style",
		"agents",
		"skills",
		"plugins",
		"uuid",
	})

	var payload map[string]any
	testutil.RequireNoError(testingHandle, json.Unmarshal([]byte(line), &payload), "parse init JSON")

	// Validate key lists to ensure compatibility-sensitive ordering is preserved.
	testutil.RequireEqual(testingHandle, extractStringSlice(payload["tools"]), expectedToolNames(), "tool list mismatch")
	testutil.RequireEqual(testingHandle, extractStringSlice(payload["slash_commands"]), defaultSlashCommandList(), "slash command list mismatch")
	testutil.RequireEqual(testingHandle, extractStringSlice(payload["agents"]), defaultAgentList(), "agent list mismatch")
	testutil.RequireEqual(testingHandle, extractStringSlice(payload["skills"]), defaultSkillList(), "skill list mismatch")

	// Validate plugin ordering across CLI inputs and settings map entries.
	testutil.RequireEqual(testingHandle, extractPluginList(payload["plugins"]), []map[string]string{
		{"name": "plugin-b", "path": "/plugins/plugin-b"},
		{"name": "plugin-a", "path": "/plugins/plugin-a"},
		{"name": "plugin-c"},
		{"name": "plugin-z"},
	}, "plugin list mismatch")

	// Validate scalar fields that should not regress across compatibility updates.
	if payload["apiKeySource"] != "config" {
		testingHandle.Fatalf("expected apiKeySource config, got %v", payload["apiKeySource"])
	}
	if payload["permissionMode"] != "default" {
		testingHandle.Fatalf("expected permissionMode default, got %v", payload["permissionMode"])
	}
	if payload["claude_code_version"] != version {
		testingHandle.Fatalf("expected claude_code_version %s, got %v", version, payload["claude_code_version"])
	}
	if payload["output_style"] != "default" {
		testingHandle.Fatalf("expected output_style default, got %v", payload["output_style"])
	}
}

// TestStreamJSONAuthErrorOrdering verifies init/assistant/result ordering for auth failures.
func TestStreamJSONAuthErrorOrdering(testingHandle *testing.T) {
	// Build a minimal init event so the error sequence matches Claude Code ordering.
	toolRunner := tools.NewRunner(tools.DefaultTools())
	runner := &agent.Runner{
		ToolRunner:  toolRunner,
		Permissions: tools.Permissions{Mode: tools.PermissionDefault},
	}
	initEvent := buildSystemInitEvent(&options{}, runner, "model-x", "session-1", &config.Settings{Raw: map[string]any{}}, "config")
	initEvent.UUID = "uuid-init"

	var buffer bytes.Buffer
	writer := streamjson.NewWriter(&buffer)
	testutil.RequireNoError(testingHandle, writer.Write(initEvent), "write init event")

	// Emit the auth error events and capture their ordering.
	testutil.RequireNoError(testingHandle, emitAuthErrorEvents(writer, "session-1", "Invalid API key · Please run /login", 12), "emit auth error events")

	rawLines := readJSONLines(testingHandle, buffer.Bytes())
	lines := coerceJSONMaps(testingHandle, rawLines)
	if len(lines) != 3 {
		testingHandle.Fatalf("expected 3 JSON lines, got %d", len(lines))
	}

	// Validate ordering: init, assistant error, result error.
	assertJSONFieldValue(testingHandle, lines[0], "type", "system")
	assertJSONFieldValue(testingHandle, lines[0], "subtype", "init")
	assertJSONFieldValue(testingHandle, lines[1], "type", "assistant")
	assertJSONFieldValue(testingHandle, lines[2], "type", "result")

	assistant := lines[1]
	if assistant["error"] != "authentication_failed" {
		testingHandle.Fatalf("expected authentication_failed, got %v", assistant["error"])
	}
	message, ok := assistant["message"].(map[string]any)
	if !ok {
		testingHandle.Fatalf("expected assistant message payload")
	}
	requireMessageFields(testingHandle, message)
	assertMessageText(testingHandle, message, "Invalid API key · Please run /login")

	result := lines[2]
	if result["is_error"] != true {
		testingHandle.Fatalf("expected is_error true, got %v", result["is_error"])
	}
	if result["result"] != "Invalid API key · Please run /login" {
		testingHandle.Fatalf("unexpected result text: %v", result["result"])
	}
	if len(extractAnySlice(result["permission_denials"])) != 0 {
		testingHandle.Fatalf("expected empty permission_denials")
	}
	if usage, ok := result["usage"].(map[string]any); !ok || usage["service_tier"] != "standard" {
		testingHandle.Fatalf("expected standard service_tier, got %v", result["usage"])
	}
	if modelUsage, ok := result["modelUsage"].(map[string]any); !ok || len(modelUsage) != 0 {
		testingHandle.Fatalf("expected empty modelUsage, got %v", result["modelUsage"])
	}
}

// TestSystemInitEventDisablesSlashCommands verifies the disable flag removes lists.
func TestSystemInitEventDisablesSlashCommands(testingHandle *testing.T) {
	toolRunner := tools.NewRunner(tools.DefaultTools())
	runner := &agent.Runner{
		ToolRunner:  toolRunner,
		Permissions: tools.Permissions{Mode: tools.PermissionDefault},
	}
	opts := &options{DisableSlashCommands: true}
	settings := &config.Settings{Raw: map[string]any{}}

	initEvent := buildSystemInitEvent(opts, runner, "model-x", "session-1", settings, "config")
	initEvent.UUID = "uuid-init"

	var buffer bytes.Buffer
	writer := streamjson.NewWriter(&buffer)
	testutil.RequireNoError(testingHandle, writer.Write(initEvent), "write init event")

	line := strings.TrimSpace(buffer.String())
	if line == "" {
		testingHandle.Fatalf("expected init output line")
	}
	var payload map[string]any
	testutil.RequireNoError(testingHandle, json.Unmarshal([]byte(line), &payload), "parse init JSON")

	if len(extractStringSlice(payload["slash_commands"])) != 0 {
		testingHandle.Fatalf("expected no slash commands when disabled")
	}
	if len(extractAnySlice(payload["skills"])) != 0 {
		testingHandle.Fatalf("expected no skills when disabled")
	}
}

// expectedToolNames returns the canonical Claude Code tool ordering for init validation.
func expectedToolNames() []string {
	return []string{
		"Task",
		"TaskOutput",
		"Bash",
		"Glob",
		"Grep",
		"ExitPlanMode",
		"Read",
		"Edit",
		"Write",
		"NotebookEdit",
		"WebFetch",
		"TodoWrite",
		"WebSearch",
		"TaskStop",
		"AskUserQuestion",
		"Skill",
		"EnterPlanMode",
	}
}

// assertJSONKeyOrder ensures the provided keys appear in the JSON line in order.
func assertJSONKeyOrder(testingHandle *testing.T, line string, keys []string) {
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

// extractStringSlice converts a JSON array of strings into a Go slice.
func extractStringSlice(raw any) []string {
	values := extractAnySlice(raw)
	out := make([]string, 0, len(values))
	for _, value := range values {
		if text, ok := value.(string); ok {
			out = append(out, text)
		}
	}
	return out
}

// extractPluginList converts the plugin list payload into a deterministic slice of maps.
func extractPluginList(raw any) []map[string]string {
	values := extractAnySlice(raw)
	out := make([]map[string]string, 0, len(values))
	for _, value := range values {
		entry, ok := value.(map[string]any)
		if !ok {
			continue
		}
		plugin := map[string]string{}
		if name, ok := entry["name"].(string); ok {
			plugin["name"] = name
		}
		if path, ok := entry["path"].(string); ok {
			plugin["path"] = path
		}
		out = append(out, plugin)
	}
	return out
}

// extractAnySlice normalizes JSON arrays into a simple slice for inspection.
func extractAnySlice(raw any) []any {
	if raw == nil {
		return nil
	}
	if values, ok := raw.([]any); ok {
		return values
	}
	return nil
}

// coerceJSONMaps asserts the JSON lines are object payloads for downstream checks.
func coerceJSONMaps(testingHandle *testing.T, raw []any) []map[string]any {
	testingHandle.Helper()

	objects := make([]map[string]any, 0, len(raw))
	for _, item := range raw {
		payload, ok := item.(map[string]any)
		if !ok {
			testingHandle.Fatalf("expected JSON object, got %T", item)
		}
		objects = append(objects, payload)
	}
	return objects
}

// assertJSONFieldValue validates a scalar field in a decoded JSON object.
func assertJSONFieldValue(testingHandle *testing.T, payload map[string]any, key string, expected any) {
	testingHandle.Helper()
	if payload[key] != expected {
		testingHandle.Fatalf("expected %s %v, got %v", key, expected, payload[key])
	}
}

// requireMessageFields checks that Claude Code message envelope keys are present.
func requireMessageFields(testingHandle *testing.T, message map[string]any) {
	testingHandle.Helper()
	required := []string{
		"id",
		"container",
		"model",
		"role",
		"stop_reason",
		"stop_sequence",
		"type",
		"usage",
		"content",
		"context_management",
	}
	for _, key := range required {
		if _, ok := message[key]; !ok {
			testingHandle.Fatalf("missing message field %q", key)
		}
	}
}

// assertMessageText extracts the first text block and compares it to the expected content.
func assertMessageText(testingHandle *testing.T, message map[string]any, expected string) {
	testingHandle.Helper()

	blocks, ok := message["content"].([]any)
	if !ok || len(blocks) == 0 {
		testingHandle.Fatalf("expected content blocks")
	}
	block, ok := blocks[0].(map[string]any)
	if !ok {
		testingHandle.Fatalf("expected content block object")
	}
	if block["text"] != expected {
		testingHandle.Fatalf("expected text %q, got %v", expected, block["text"])
	}
}
