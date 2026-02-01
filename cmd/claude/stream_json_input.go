package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/openclaude/openclaude/internal/llm/openai"
)

// streamJSONInput captures parsed stream-json input for print mode.
type streamJSONInput struct {
	// Messages holds user messages extracted from the input stream.
	Messages []openai.Message
	// UserMessages preserves user message metadata for replay output.
	UserMessages []streamJSONUserMessage
	// ControlRequests stores control requests that must be handled before execution.
	ControlRequests []streamJSONControlRequest
	// ControlResponses stores control responses seen in the input stream.
	ControlResponses []map[string]any
}

// streamJSONUserMessage keeps the user message plus stream-json metadata.
type streamJSONUserMessage struct {
	// Message is the parsed OpenAI-compatible user message.
	Message openai.Message
	// UUID is the stream-json UUID, if provided by the input line.
	UUID string
	// IsReplay reports whether the input explicitly marked the message as a replay.
	IsReplay bool
	// IsSynthetic reports whether the input marked the message as synthetic.
	IsSynthetic bool
}

// streamJSONControlRequest captures a control request envelope.
type streamJSONControlRequest struct {
	// RequestID is the stream-json request identifier.
	RequestID string
	// Request is the control request payload map.
	Request map[string]any
}

// readStreamInputWithControl parses a stream-json input stream into messages and control requests.
func readStreamInputWithControl(reader io.Reader) (*streamJSONInput, error) {
	// Use a buffered scanner to preserve line-based framing of stream-json.
	scanner := bufio.NewScanner(reader)
	parsed := &streamJSONInput{}

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}

		var payload map[string]any
		if err := json.Unmarshal([]byte(line), &payload); err != nil {
			return nil, fmt.Errorf("parse stream input: %w", err)
		}

		if err := handleStreamJSONPayload(payload, parsed); err != nil {
			return nil, err
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("read stream input: %w", err)
	}
	if len(parsed.Messages) == 0 {
		return nil, fmt.Errorf("no user messages found in stream input")
	}
	return parsed, nil
}

// handleStreamJSONPayload routes a single stream-json line into the parsed input structure.
func handleStreamJSONPayload(payload map[string]any, parsed *streamJSONInput) error {
	// Respect explicit type routing for control and system events.
	if typ, ok := payload["type"].(string); ok {
		switch typ {
		case "keep_alive":
			return nil
		case "update_environment_variables":
			applyEnvironmentUpdates(payload)
			return nil
		case "control_request":
			requestID, _ := payload["request_id"].(string)
			request, _ := payload["request"].(map[string]any)
			if requestID == "" || request == nil {
				return fmt.Errorf("invalid control_request payload")
			}
			parsed.ControlRequests = append(parsed.ControlRequests, streamJSONControlRequest{
				RequestID: requestID,
				Request:   request,
			})
			return nil
		case "control_response":
			if response, ok := payload["response"].(map[string]any); ok {
				parsed.ControlResponses = append(parsed.ControlResponses, response)
			}
			return nil
		case "control_cancel_request":
			return nil
		}
	}

	// Fall back to user message parsing for other payloads.
	userMessage, ok := parseStreamMessageWithMetadata(payload)
	if !ok {
		return fmt.Errorf("unsupported stream-json input payload")
	}
	parsed.Messages = append(parsed.Messages, userMessage.Message)
	parsed.UserMessages = append(parsed.UserMessages, userMessage)
	return nil
}

// parseStreamMessageWithMetadata extracts a user message along with stream-json metadata.
func parseStreamMessageWithMetadata(payload map[string]any) (streamJSONUserMessage, bool) {
	message, ok := parseStreamMessage(payload)
	if !ok {
		return streamJSONUserMessage{}, false
	}
	return streamJSONUserMessage{
		Message:     message,
		UUID:        extractStreamUUID(payload),
		IsReplay:    extractBool(payload, "isReplay"),
		IsSynthetic: extractBool(payload, "isSynthetic"),
	}, true
}

// extractStreamUUID pulls a UUID from the stream-json envelope, if present.
func extractStreamUUID(payload map[string]any) string {
	if uuid, ok := payload["uuid"].(string); ok {
		return uuid
	}
	if message, ok := payload["message"].(map[string]any); ok {
		if uuid, ok := message["uuid"].(string); ok {
			return uuid
		}
	}
	return ""
}

// extractBool returns a boolean field if it exists on the payload.
func extractBool(payload map[string]any, key string) bool {
	if value, ok := payload[key].(bool); ok {
		return value
	}
	return false
}

// applyEnvironmentUpdates applies update_environment_variables payloads to the process env.
func applyEnvironmentUpdates(payload map[string]any) {
	raw, ok := payload["variables"].(map[string]any)
	if !ok {
		return
	}
	for key, value := range raw {
		// Use fmt.Sprint to coerce non-string values into a stable representation.
		os.Setenv(key, fmt.Sprint(value))
	}
}
