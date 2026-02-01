package main

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/openclaude/openclaude/internal/streamjson"
)

// streamJSONHookConfig stores hook definitions keyed by event name.
type streamJSONHookConfig struct {
	// Events maps hook event names to hook definitions.
	Events map[string][]streamJSONHookDefinition
}

// streamJSONHookDefinition represents a single hook entry from stream-json input.
type streamJSONHookDefinition struct {
	// Matcher filters which tools should trigger the hook.
	Matcher string
	// CallbackIDs references callback identifiers supplied by the client.
	CallbackIDs []string
	// TimeoutSeconds captures the hook timeout, when provided.
	TimeoutSeconds int
}

// parseStreamJSONHookConfig converts a stream-json hooks payload into structured config.
func parseStreamJSONHookConfig(raw any) *streamJSONHookConfig {
	rawMap, ok := raw.(map[string]any)
	if !ok || len(rawMap) == 0 {
		return nil
	}

	config := &streamJSONHookConfig{
		Events: map[string][]streamJSONHookDefinition{},
	}

	for eventName, entry := range rawMap {
		hooks := parseStreamJSONHookEntries(entry)
		if len(hooks) == 0 {
			continue
		}
		config.Events[eventName] = hooks
	}

	if len(config.Events) == 0 {
		return nil
	}
	return config
}

// parseStreamJSONHookEntries parses a hook entry list into hook definitions.
func parseStreamJSONHookEntries(raw any) []streamJSONHookDefinition {
	rawList, ok := raw.([]any)
	if !ok {
		return nil
	}

	definitions := make([]streamJSONHookDefinition, 0, len(rawList))
	for _, item := range rawList {
		entry, ok := item.(map[string]any)
		if !ok {
			continue
		}

		// Accept camelCase and snake_case callback ID fields for compatibility.
		callbackIDs := parseHookCallbackIDs(entry["hookCallbackIds"])
		if len(callbackIDs) == 0 {
			callbackIDs = parseHookCallbackIDs(entry["hook_callback_ids"])
		}
		// Allow multiple timeout key spellings so stream-json control requests are flexible.
		timeoutSeconds, _ := numberField(entry, "timeout", "timeoutSeconds", "timeout_seconds")

		definition := streamJSONHookDefinition{
			Matcher:        stringField(entry, "matcher"),
			CallbackIDs:    callbackIDs,
			TimeoutSeconds: int(timeoutSeconds),
		}
		definitions = append(definitions, definition)
	}

	return definitions
}

// parseHookCallbackIDs extracts hook callback identifiers from a raw list.
func parseHookCallbackIDs(raw any) []string {
	rawList, ok := raw.([]any)
	if !ok {
		return nil
	}
	ids := make([]string, 0, len(rawList))
	for _, value := range rawList {
		id, ok := value.(string)
		if !ok || id == "" {
			continue
		}
		ids = append(ids, id)
	}
	return ids
}

// streamJSONHookEmitter emits hook lifecycle events when configured.
type streamJSONHookEmitter struct {
	// writer sends stream-json events to the output stream.
	writer *streamjson.Writer
	// sessionID scopes hook events to the session.
	sessionID string
	// config defines which hooks should emit events.
	config *streamJSONHookConfig
}

// newStreamJSONHookEmitter constructs a hook emitter when configuration is available.
func newStreamJSONHookEmitter(
	writer *streamjson.Writer,
	sessionID string,
	config *streamJSONHookConfig,
) *streamJSONHookEmitter {
	if writer == nil || config == nil || len(config.Events) == 0 {
		return nil
	}
	return &streamJSONHookEmitter{
		writer:    writer,
		sessionID: sessionID,
		config:    config,
	}
}

// EmitPreToolUse reports pre-tool hook events for the provided tool name.
func (e *streamJSONHookEmitter) EmitPreToolUse(toolName string) error {
	return e.emitHookEvents("PreToolUse", toolName, "")
}

// EmitPostToolUse reports post-tool hook events for the provided tool name.
func (e *streamJSONHookEmitter) EmitPostToolUse(toolName string) error {
	return e.emitHookEvents("PostToolUse", toolName, "success")
}

// EmitPostToolUseFailure reports failed post-tool hook events for the provided tool name.
func (e *streamJSONHookEmitter) EmitPostToolUseFailure(toolName string) error {
	return e.emitHookEvents("PostToolUseFailure", toolName, "error")
}

// emitHookEvents emits hook_started/response pairs for matching hooks.
func (e *streamJSONHookEmitter) emitHookEvents(hookEvent string, matchQuery string, outcome string) error {
	if e == nil {
		return nil
	}
	hooks := e.config.Events[hookEvent]
	if len(hooks) == 0 {
		return nil
	}

	hookName := hookEvent
	if matchQuery != "" {
		// Include the matched query so consumers can disambiguate hook outputs.
		hookName = fmt.Sprintf("%s:%s", hookEvent, matchQuery)
	}

	for _, hook := range hooks {
		if !hookMatcherMatches(hook.Matcher, matchQuery) {
			continue
		}
		count := len(hook.CallbackIDs)
		if count == 0 {
			count = 1
		}
		// Emit once per callback ID to mirror Claude Code hook fan-out behavior.
		for index := 0; index < count; index++ {
			if err := e.emitHookLifecycle(hookName, hookEvent, outcome); err != nil {
				return err
			}
		}
	}

	return nil
}

// emitHookLifecycle emits a hook_started followed by hook_response event.
func (e *streamJSONHookEmitter) emitHookLifecycle(hookName string, hookEvent string, outcome string) error {
	hookID := streamjson.NewUUID()

	started := streamjson.HookStartedEvent{
		Type:      "system",
		Subtype:   "hook_started",
		HookID:    hookID,
		HookName:  hookName,
		HookEvent: hookEvent,
		UUID:      streamjson.NewUUID(),
		SessionID: e.sessionID,
	}
	if err := e.writer.Write(started); err != nil {
		return err
	}

	response := streamjson.HookResponseEvent{
		Type:      "system",
		Subtype:   "hook_response",
		HookID:    hookID,
		HookName:  hookName,
		HookEvent: hookEvent,
		Output:    "",
		Stdout:    "",
		Stderr:    "",
		ExitCode:  0,
		Outcome:   outcome,
		UUID:      streamjson.NewUUID(),
		SessionID: e.sessionID,
	}
	if err := e.writer.Write(response); err != nil {
		return err
	}

	return nil
}

// hookMatcherMatches reports whether a hook matcher applies to a candidate value.
func hookMatcherMatches(matcher string, candidate string) bool {
	if matcher == "" || candidate == "" {
		return matcher == "" || candidate != ""
	}

	// Simple tokens or pipes indicate exact matching.
	if simpleMatcherPattern.MatchString(matcher) {
		if strings.Contains(matcher, "|") {
			for _, part := range strings.Split(matcher, "|") {
				if strings.TrimSpace(part) == candidate {
					return true
				}
			}
			return false
		}
		return matcher == candidate
	}

	// Fall back to regex matching when matcher contains special characters.
	regex, err := regexp.Compile(matcher)
	if err != nil {
		return false
	}
	return regex.MatchString(candidate)
}

// simpleMatcherPattern matches Claude Code-style literal matchers.
var simpleMatcherPattern = regexp.MustCompile(`^[a-zA-Z0-9_|]+$`)
