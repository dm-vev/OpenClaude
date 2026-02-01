package main

import (
	"encoding/json"
	"fmt"

	"github.com/openclaude/openclaude/internal/agent"
	"github.com/openclaude/openclaude/internal/config"
	"github.com/openclaude/openclaude/internal/streamjson"
	"github.com/openclaude/openclaude/internal/tools"
)

// applyStreamJSONControlRequests processes control requests and emits responses.
func applyStreamJSONControlRequests(
	parsed *streamJSONInput,
	writer *streamjson.Writer,
	opts *options,
	runner *agent.Runner,
	settings *config.Settings,
	sessionID string,
	model string,
) (string, bool, error) {
	if parsed == nil || len(parsed.ControlRequests) == 0 {
		return model, false, nil
	}
	if writer == nil {
		return model, false, fmt.Errorf("stream-json writer is required")
	}
	if runner == nil {
		return model, false, fmt.Errorf("runner is required")
	}

	initialized := false
	authStatusEmitted := false
	resolvedModel := model

	for _, request := range parsed.ControlRequests {
		subtype := stringField(request.Request, "subtype")
		switch subtype {
		case "initialize":
			if initialized {
				if err := writeControlResponseError(writer, request.RequestID, "Already initialized"); err != nil {
					return resolvedModel, authStatusEmitted, err
				}
				continue
			}
			initialized = true
			applyInitializeRequest(request.Request, opts, &resolvedModel, model)
			response := buildInitializeControlResponse(opts, settings, resolvedModel)
			if err := writeControlResponseSuccess(writer, request.RequestID, response); err != nil {
				return resolvedModel, authStatusEmitted, err
			}
			if opts.EnableAuthStatus && !authStatusEmitted {
				if err := emitAuthStatus(writer, sessionID, false, "", ""); err != nil {
					return resolvedModel, authStatusEmitted, err
				}
				authStatusEmitted = true
			}
		case "set_permission_mode":
			mode := stringField(request.Request, "mode", "permissionMode", "permission_mode")
			permissionMode, err := parsePermissionModeStrict(mode)
			if err != nil {
				if writeErr := writeControlResponseError(writer, request.RequestID, err.Error()); writeErr != nil {
					return resolvedModel, authStatusEmitted, writeErr
				}
				continue
			}
			opts.PermissionMode = string(permissionMode)
			runner.Permissions.Mode = permissionMode
			if err := writeControlResponseSuccess(writer, request.RequestID, map[string]any{"mode": opts.PermissionMode}); err != nil {
				return resolvedModel, authStatusEmitted, err
			}
			if err := emitSystemStatus(writer, sessionID, opts.PermissionMode); err != nil {
				return resolvedModel, authStatusEmitted, err
			}
		case "set_model":
			requestedModel := stringField(request.Request, "model")
			if requestedModel == "" {
				if err := writeControlResponseError(writer, request.RequestID, "Missing model"); err != nil {
					return resolvedModel, authStatusEmitted, err
				}
				continue
			}
			if requestedModel == "default" {
				resolvedModel = model
			} else {
				resolvedModel = requestedModel
			}
			opts.Model = requestedModel
			if err := writeControlResponseSuccess(writer, request.RequestID, map[string]any{"model": resolvedModel}); err != nil {
				return resolvedModel, authStatusEmitted, err
			}
		case "set_max_thinking_tokens":
			value, ok := numberField(request.Request, "max_thinking_tokens", "maxThinkingTokens")
			if !ok {
				if err := writeControlResponseError(writer, request.RequestID, "Missing max_thinking_tokens"); err != nil {
					return resolvedModel, authStatusEmitted, err
				}
				continue
			}
			opts.MaxThinkingTokens = int(value)
			if err := writeControlResponseSuccess(writer, request.RequestID, map[string]any{"max_thinking_tokens": opts.MaxThinkingTokens}); err != nil {
				return resolvedModel, authStatusEmitted, err
			}
		case "interrupt":
			if err := writeControlResponseSuccess(writer, request.RequestID, map[string]any{}); err != nil {
				return resolvedModel, authStatusEmitted, err
			}
		default:
			if err := writeControlResponseError(writer, request.RequestID, fmt.Sprintf("Unsupported control request subtype: %s", subtype)); err != nil {
				return resolvedModel, authStatusEmitted, err
			}
		}
	}

	return resolvedModel, authStatusEmitted, nil
}

// applyInitializeRequest updates option values based on an initialize control request.
func applyInitializeRequest(request map[string]any, opts *options, resolvedModel *string, fallbackModel string) {
	if value := stringField(request, "systemPrompt", "system_prompt"); value != "" {
		opts.SystemPrompt = value
	}
	if value := stringField(request, "appendSystemPrompt", "append_system_prompt"); value != "" {
		opts.AppendSystemPrompt = value
	}
	if value := stringField(request, "agent"); value != "" {
		opts.Agent = value
	}
	if value := stringField(request, "model"); value != "" {
		if value == "default" {
			*resolvedModel = fallbackModel
		} else {
			*resolvedModel = value
		}
		opts.Model = value
	}
	if rawAgents, ok := request["agents"]; ok {
		if encoded, err := json.Marshal(rawAgents); err == nil {
			opts.AgentsJSON = string(encoded)
		}
	}
	if schema, ok := request["jsonSchema"]; ok {
		if encoded, err := json.Marshal(schema); err == nil {
			opts.JSONSchema = string(encoded)
		}
	}
	if schema, ok := request["json_schema"]; ok {
		if encoded, err := json.Marshal(schema); err == nil {
			opts.JSONSchema = string(encoded)
		}
	}
	if hooks, ok := request["hooks"]; ok {
		opts.HookConfig = parseStreamJSONHookConfig(hooks)
	}
}

// buildInitializeControlResponse assembles the initialize response payload.
func buildInitializeControlResponse(opts *options, settings *config.Settings, model string) map[string]any {
	return map[string]any{
		"commands":                []map[string]string{},
		"output_style":            resolveOutputStyle(settings),
		"available_output_styles": []string{"default"},
		"models":                  buildModelOptions(model, opts.FallbackModel),
		"account": map[string]any{
			"email":            nil,
			"organization":     nil,
			"subscriptionType": nil,
			"tokenSource":      nil,
			"apiKeySource":     "config",
		},
	}
}

// buildModelOptions produces a Claude Code compatible model list.
func buildModelOptions(model string, fallback string) []map[string]string {
	seen := map[string]bool{}
	var options []map[string]string

	appendModel := func(value string) {
		if value == "" || seen[value] {
			return
		}
		seen[value] = true
		options = append(options, map[string]string{
			"value":       value,
			"displayName": value,
			"description": "",
		})
	}

	appendModel("default")
	appendModel(model)
	appendModel(fallback)

	return options
}

// writeControlResponseSuccess emits a successful control_response envelope.
func writeControlResponseSuccess(writer *streamjson.Writer, requestID string, response any) error {
	return writer.Write(streamjson.ControlResponseEvent{
		Type: "control_response",
		Response: map[string]any{
			"subtype":    "success",
			"request_id": requestID,
			"response":   response,
		},
	})
}

// writeControlResponseError emits an error control_response envelope.
func writeControlResponseError(writer *streamjson.Writer, requestID string, message string) error {
	return writer.Write(streamjson.ControlResponseEvent{
		Type: "control_response",
		Response: map[string]any{
			"subtype":    "error",
			"request_id": requestID,
			"error":      message,
		},
	})
}

// emitAuthStatus writes an auth_status event to the stream-json output.
func emitAuthStatus(writer *streamjson.Writer, sessionID string, authenticating bool, output string, errText string) error {
	event := streamjson.AuthStatusEvent{
		Type:             "auth_status",
		IsAuthenticating: authenticating,
		Output:           output,
		Error:            errText,
		UUID:             streamjson.NewUUID(),
		SessionID:        sessionID,
	}
	return writer.Write(event)
}

// emitSystemStatus emits a system status event when permission mode changes.
func emitSystemStatus(writer *streamjson.Writer, sessionID string, permissionMode string) error {
	event := streamjson.SystemEvent{
		Type:           "system",
		Subtype:        "status",
		Status:         nil,
		PermissionMode: permissionMode,
		SessionID:      sessionID,
		UUID:           streamjson.NewUUID(),
	}
	return writer.Write(event)
}

// parsePermissionMode validates and converts a permission mode string.
func parsePermissionModeStrict(mode string) (tools.PermissionMode, error) {
	switch mode {
	case string(tools.PermissionDefault),
		string(tools.PermissionAcceptEdits),
		string(tools.PermissionDontAsk),
		string(tools.PermissionDelegate),
		string(tools.PermissionBypass),
		string(tools.PermissionPlan):
		return tools.PermissionMode(mode), nil
	default:
		if mode == "" {
			return "", fmt.Errorf("Missing permission mode")
		}
		return "", fmt.Errorf("Unsupported permission mode: %s", mode)
	}
}

// stringField extracts the first matching string field from a map.
func stringField(payload map[string]any, keys ...string) string {
	for _, key := range keys {
		if value, ok := payload[key].(string); ok {
			return value
		}
	}
	return ""
}

// numberField extracts a numeric field from a map.
func numberField(payload map[string]any, keys ...string) (float64, bool) {
	for _, key := range keys {
		if value, ok := payload[key]; ok {
			if value == nil {
				return 0, true
			}
			if number, ok := value.(float64); ok {
				return number, true
			}
		}
	}
	return 0, false
}
