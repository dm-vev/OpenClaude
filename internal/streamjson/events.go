package streamjson

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"sync"

	"github.com/google/uuid"

	"github.com/openclaude/openclaude/internal/llm/openai"
)

// Message represents the high-level message payload used in stream-json events.
type Message struct {
	// ID is the unique message identifier when provided.
	ID string `json:"id,omitempty"`
	// Container reports any container metadata, or null if unused.
	Container *json.RawMessage `json:"container,omitempty"`
	// Model names the model that generated the message.
	Model string `json:"model,omitempty"`
	// Role is one of user, assistant, or system.
	Role string `json:"role"`
	// StopReason indicates why generation stopped.
	StopReason string `json:"stop_reason,omitempty"`
	// StopSequence holds the stop sequence when applicable.
	StopSequence *string `json:"stop_sequence,omitempty"`
	// Type is always "message" for Claude-style envelopes.
	Type string `json:"type,omitempty"`
	// Usage reports token usage for the message when available.
	Usage *MessageUsage `json:"usage,omitempty"`
	// Content is either a string or a list of content blocks.
	Content any `json:"content"`
	// ContextManagement reports context handling metadata, or null if unused.
	ContextManagement *json.RawMessage `json:"context_management,omitempty"`
}

// ContentBlock represents an Anthropic-style content block.
type ContentBlock struct {
	// Type determines how the content block is interpreted.
	Type string `json:"type"`
	// Text carries plain text content.
	Text string `json:"text,omitempty"`
	// ID identifies a tool call, when Type == tool_use.
	ID string `json:"id,omitempty"`
	// Name specifies the tool name for tool_use blocks.
	Name string `json:"name,omitempty"`
	// Input holds the tool input object for tool_use blocks.
	Input any `json:"input,omitempty"`
	// ToolUseID links tool_result blocks to a tool_use.
	ToolUseID string `json:"tool_use_id,omitempty"`
	// Content carries tool_result output text.
	Content string `json:"content,omitempty"`
	// IsError indicates a tool_result error condition.
	IsError bool `json:"is_error,omitempty"`
}

// AssistantEvent represents a stream-json assistant message event.
type AssistantEvent struct {
	// Type is always "assistant".
	Type string `json:"type"`
	// Message carries the assistant message payload.
	Message Message `json:"message"`
	// SessionID scopes the event to a session.
	SessionID string `json:"session_id"`
	// ParentToolUseID is reserved for nested tool calls.
	ParentToolUseID any `json:"parent_tool_use_id"`
	// UUID uniquely identifies the event.
	UUID string `json:"uuid"`
	// Error optionally carries an error code for synthetic assistant errors.
	Error string `json:"error,omitempty"`
}

// MessageUsage represents Claude-style usage details for assistant messages.
type MessageUsage struct {
	// InputTokens counts prompt tokens.
	InputTokens int `json:"input_tokens"`
	// OutputTokens counts generated tokens.
	OutputTokens int `json:"output_tokens"`
	// CacheCreationInputTokens reports cached creation input tokens.
	CacheCreationInputTokens int `json:"cache_creation_input_tokens"`
	// CacheReadInputTokens reports cached read input tokens.
	CacheReadInputTokens int `json:"cache_read_input_tokens"`
	// ServerToolUse reports tool request counts handled by the service.
	ServerToolUse MessageServerToolUse `json:"server_tool_use"`
	// ServiceTier reports the service tier when available.
	ServiceTier *string `json:"service_tier"`
	// CacheCreation reports cache creation usage breakdowns.
	CacheCreation MessageCacheCreation `json:"cache_creation"`
}

// MessageServerToolUse reports server-side tool request counts.
type MessageServerToolUse struct {
	// WebSearchRequests is the number of web search requests.
	WebSearchRequests int `json:"web_search_requests"`
	// WebFetchRequests is the number of web fetch requests.
	WebFetchRequests int `json:"web_fetch_requests"`
}

// MessageCacheCreation reports cache creation token usage.
type MessageCacheCreation struct {
	// Ephemeral1HInputTokens reports ephemeral 1h cache input tokens.
	Ephemeral1HInputTokens int `json:"ephemeral_1h_input_tokens"`
	// Ephemeral5MInputTokens reports ephemeral 5m cache input tokens.
	Ephemeral5MInputTokens int `json:"ephemeral_5m_input_tokens"`
}

// UserEvent represents a stream-json user message event.
type UserEvent struct {
	// Type is always "user".
	Type string `json:"type"`
	// Message carries the user message payload.
	Message Message `json:"message"`
	// SessionID scopes the event to a session.
	SessionID string `json:"session_id"`
	// ParentToolUseID is reserved for nested tool calls.
	ParentToolUseID any `json:"parent_tool_use_id"`
	// UUID uniquely identifies the event.
	UUID string `json:"uuid"`
	// IsSynthetic marks synthetic or meta messages.
	IsSynthetic bool `json:"isSynthetic,omitempty"`
	// IsReplay marks user messages replayed into the stream.
	IsReplay bool `json:"isReplay,omitempty"`
}

// SystemEvent represents a stream-json system event.
type SystemEvent struct {
	// Type is always "system".
	Type string `json:"type"`
	// Subtype categorizes the system event.
	Subtype string `json:"subtype"`
	// Status carries optional status payloads.
	Status any `json:"status,omitempty"`
	// PermissionMode reflects the active permission mode.
	PermissionMode string `json:"permissionMode,omitempty"`
	// SessionID scopes the event to a session.
	SessionID string `json:"session_id"`
	// UUID uniquely identifies the event.
	UUID string `json:"uuid"`
}

// SystemInitEvent represents the stream-json initialization event.
type SystemInitEvent struct {
	// Type is always "system".
	Type string `json:"type"`
	// Subtype is always "init".
	Subtype string `json:"subtype"`
	// CWD is the active working directory.
	CWD string `json:"cwd"`
	// SessionID scopes the event to a session.
	SessionID string `json:"session_id"`
	// Tools lists available tool names.
	Tools []string `json:"tools"`
	// MCPServers lists connected MCP server descriptors.
	MCPServers []any `json:"mcp_servers"`
	// Model reports the active model identifier.
	Model string `json:"model"`
	// PermissionMode reflects the active permission mode.
	PermissionMode string `json:"permissionMode"`
	// SlashCommands lists available slash commands.
	SlashCommands []string `json:"slash_commands"`
	// APIKeySource reports where the API key was loaded from.
	APIKeySource string `json:"apiKeySource"`
	// Betas lists enabled beta flags.
	Betas []string `json:"betas"`
	// ClaudeCodeVersion reports the CLI version string.
	ClaudeCodeVersion string `json:"claude_code_version"`
	// OutputStyle reports the output style key.
	OutputStyle string `json:"output_style"`
	// Agents lists configured agent profiles.
	Agents []any `json:"agents"`
	// Skills lists available skills.
	Skills []any `json:"skills"`
	// Plugins lists configured plugins.
	Plugins []any `json:"plugins"`
	// UUID uniquely identifies the event.
	UUID string `json:"uuid"`
}

// ProgressEvent represents a stream-json progress event.
type ProgressEvent struct {
	// Type is always "progress".
	Type string `json:"type"`
	// Data carries progress metadata for the event.
	Data ProgressData `json:"data"`
	// SessionID scopes the event to a session.
	SessionID string `json:"session_id"`
	// ParentToolUseID links progress events to a tool use.
	ParentToolUseID string `json:"parent_tool_use_id,omitempty"`
	// UUID uniquely identifies the event.
	UUID string `json:"uuid"`
}

// ProgressData describes the progress payload.
type ProgressData struct {
	// Type identifies the progress payload type.
	Type string `json:"type"`
	// ToolName identifies the tool being executed.
	ToolName string `json:"tool_name,omitempty"`
	// Status summarizes the progress status.
	Status string `json:"status,omitempty"`
	// Message provides a human-readable description.
	Message string `json:"message,omitempty"`
}

// ToolUseSummaryEvent summarizes completed tool usage.
type ToolUseSummaryEvent struct {
	// Type is always "tool_use_summary".
	Type string `json:"type"`
	// Summary provides a human-readable summary.
	Summary string `json:"summary"`
	// PrecedingToolUseIDs lists tool use ids in order.
	PrecedingToolUseIDs []string `json:"preceding_tool_use_ids"`
	// SessionID scopes the event to a session.
	SessionID string `json:"session_id"`
	// UUID uniquely identifies the event.
	UUID string `json:"uuid"`
}

// AuthStatusEvent reports authentication status in stream-json output.
type AuthStatusEvent struct {
	// Type is always "auth_status".
	Type string `json:"type"`
	// IsAuthenticating reports whether authentication is in progress.
	IsAuthenticating bool `json:"isAuthenticating"`
	// Output carries optional status output.
	Output string `json:"output,omitempty"`
	// Error carries optional error details.
	Error string `json:"error,omitempty"`
	// UUID uniquely identifies the event.
	UUID string `json:"uuid"`
	// SessionID scopes the event to a session.
	SessionID string `json:"session_id"`
}

// HookStartedEvent reports a hook start in stream-json output.
type HookStartedEvent struct {
	// Type is always "system".
	Type string `json:"type"`
	// Subtype is always "hook_started".
	Subtype string `json:"subtype"`
	// HookID identifies the hook invocation.
	HookID string `json:"hook_id"`
	// HookName names the hook.
	HookName string `json:"hook_name"`
	// HookEvent identifies the hook lifecycle event.
	HookEvent string `json:"hook_event"`
	// UUID uniquely identifies the event.
	UUID string `json:"uuid"`
	// SessionID scopes the event to a session.
	SessionID string `json:"session_id"`
}

// HookProgressEvent reports incremental hook output.
type HookProgressEvent struct {
	// Type is always "system".
	Type string `json:"type"`
	// Subtype is always "hook_progress".
	Subtype string `json:"subtype"`
	// HookID identifies the hook invocation.
	HookID string `json:"hook_id"`
	// HookName names the hook.
	HookName string `json:"hook_name"`
	// HookEvent identifies the hook lifecycle event.
	HookEvent string `json:"hook_event"`
	// Stdout captures stdout output.
	Stdout string `json:"stdout,omitempty"`
	// Stderr captures stderr output.
	Stderr string `json:"stderr,omitempty"`
	// Output carries any aggregated output.
	Output string `json:"output,omitempty"`
	// UUID uniquely identifies the event.
	UUID string `json:"uuid"`
	// SessionID scopes the event to a session.
	SessionID string `json:"session_id"`
}

// HookResponseEvent reports hook completion output.
type HookResponseEvent struct {
	// Type is always "system".
	Type string `json:"type"`
	// Subtype is always "hook_response".
	Subtype string `json:"subtype"`
	// HookID identifies the hook invocation.
	HookID string `json:"hook_id"`
	// HookName names the hook.
	HookName string `json:"hook_name"`
	// HookEvent identifies the hook lifecycle event.
	HookEvent string `json:"hook_event"`
	// Output carries hook output.
	Output string `json:"output,omitempty"`
	// Stdout captures stdout output.
	Stdout string `json:"stdout,omitempty"`
	// Stderr captures stderr output.
	Stderr string `json:"stderr,omitempty"`
	// ExitCode reports the hook process exit code.
	ExitCode int `json:"exit_code,omitempty"`
	// Outcome reports the hook outcome string.
	Outcome string `json:"outcome,omitempty"`
	// UUID uniquely identifies the event.
	UUID string `json:"uuid"`
	// SessionID scopes the event to a session.
	SessionID string `json:"session_id"`
}

// ControlRequestEvent represents a stream-json control request.
type ControlRequestEvent struct {
	// Type is always "control_request".
	Type string `json:"type"`
	// RequestID correlates responses to the request.
	RequestID string `json:"request_id"`
	// Request carries the control request payload.
	Request any `json:"request"`
}

// ControlResponseEvent represents a stream-json control response.
type ControlResponseEvent struct {
	// Type is always "control_response".
	Type string `json:"type"`
	// Response carries the control response payload.
	Response any `json:"response"`
}

// ControlCancelRequestEvent represents a stream-json control cancel request.
type ControlCancelRequestEvent struct {
	// Type is always "control_cancel_request".
	Type string `json:"type"`
	// RequestID identifies the request being cancelled.
	RequestID string `json:"request_id"`
}

// KeepAliveEvent represents a stream-json keep-alive event.
type KeepAliveEvent struct {
	// Type is always "keep_alive".
	Type string `json:"type"`
}

// ResultEvent represents the terminal stream-json result.
type ResultEvent struct {
	// Type is always "result".
	Type string `json:"type"`
	// Subtype describes success or error conditions.
	Subtype string `json:"subtype"`
	// IsError reports whether the result indicates an error.
	IsError bool `json:"is_error"`
	// DurationMS is the total runtime in milliseconds.
	DurationMS int64 `json:"duration_ms"`
	// DurationAPIMS is the total API time in milliseconds.
	DurationAPIMS int64 `json:"duration_api_ms"`
	// NumTurns is the number of assistant turns processed.
	NumTurns int `json:"num_turns"`
	// Result contains the final assistant text.
	Result string `json:"result"`
	// SessionID scopes the event to a session.
	SessionID string `json:"session_id"`
	// TotalCostUSD reports the estimated total cost.
	TotalCostUSD float64 `json:"total_cost_usd"`
	// Usage contains aggregated usage stats.
	Usage any `json:"usage"`
	// ModelUsage contains per-model usage stats.
	ModelUsage any `json:"modelUsage"`
	// PermissionDenials lists denied tool uses.
	PermissionDenials []any `json:"permission_denials"`
	// UUID uniquely identifies the event.
	UUID string `json:"uuid"`
	// Errors holds error messages for error subtypes.
	Errors []string `json:"errors,omitempty"`
}

// StreamEvent wraps a low-level streaming event.
type StreamEvent struct {
	// Type is always "stream_event".
	Type string `json:"type"`
	// Event contains the streaming payload.
	Event any `json:"event"`
	// SessionID scopes the event to a session.
	SessionID string `json:"session_id"`
	// ParentToolUseID is reserved for nested tool calls.
	ParentToolUseID any `json:"parent_tool_use_id"`
	// UUID uniquely identifies the event.
	UUID string `json:"uuid"`
}

// MessageStartEvent represents the start of a streaming message.
type MessageStartEvent struct {
	// Type identifies the stream event type.
	Type string `json:"type"`
	// Message describes the streaming message.
	Message StreamMessage `json:"message"`
}

// StreamMessage represents a streaming assistant message.
type StreamMessage struct {
	// ID is the message identifier.
	ID string `json:"id"`
	// Type is always "message".
	Type string `json:"type"`
	// Role is "assistant" for streaming output.
	Role string `json:"role"`
	// Model is the model identifier.
	Model string `json:"model"`
	// Content is initially empty for streaming.
	Content []any `json:"content"`
	// StopReason indicates why generation stopped.
	StopReason any `json:"stop_reason"`
	// StopSequence indicates a stop sequence if used.
	StopSequence any `json:"stop_sequence"`
}

// ContentBlockStartEvent represents the start of a streaming content block.
type ContentBlockStartEvent struct {
	// Type identifies the stream event type.
	Type string `json:"type"`
	// Index is the content block index.
	Index int `json:"index"`
	// ContentBlock contains the block metadata.
	ContentBlock ContentBlock `json:"content_block"`
}

// ContentBlockDeltaEvent represents a streaming content delta.
type ContentBlockDeltaEvent struct {
	// Type identifies the stream event type.
	Type string `json:"type"`
	// Index is the content block index.
	Index int `json:"index"`
	// Delta contains the incremental update.
	Delta StreamDelta `json:"delta"`
}

// StreamDelta represents a delta payload for streaming text.
type StreamDelta struct {
	// Type is the delta type, typically "text_delta".
	Type string `json:"type"`
	// Text is the streamed text chunk.
	Text string `json:"text,omitempty"`
	// PartialJSON carries incremental JSON for tool inputs.
	PartialJSON string `json:"partial_json,omitempty"`
}

// ContentBlockStopEvent represents the end of a content block.
type ContentBlockStopEvent struct {
	// Type identifies the stream event type.
	Type string `json:"type"`
	// Index is the content block index.
	Index int `json:"index"`
}

// MessageDeltaEvent represents message-level stream metadata updates.
type MessageDeltaEvent struct {
	// Type identifies the stream event type.
	Type string `json:"type"`
	// Delta reports stop reasons.
	Delta MessageDelta `json:"delta"`
}

// MessageDelta contains message-level metadata changes.
type MessageDelta struct {
	// StopReason reports why generation stopped.
	StopReason string `json:"stop_reason,omitempty"`
	// StopSequence reports the stop sequence if applicable.
	StopSequence any `json:"stop_sequence,omitempty"`
}

// MessageStopEvent represents the end of a streaming message.
type MessageStopEvent struct {
	// Type identifies the stream event type.
	Type string `json:"type"`
}

// Writer emits stream-json events as JSON Lines.
// The writer guarantees each call produces exactly one newline-delimited JSON object.
type Writer struct {
	// mu serializes writes to prevent JSON line interleaving.
	mu sync.Mutex
	// writer is the underlying output destination.
	writer io.Writer
	// afterWrite runs after a JSON line is written when set.
	afterWrite func(event any) error
}

// NewWriter constructs a stream-json writer.
func NewWriter(writer io.Writer) *Writer {
	return &Writer{writer: writer}
}

// SetAfterWrite registers a hook invoked after each event is written.
// The hook is invoked under the write lock so persisted ordering is preserved.
func (w *Writer) SetAfterWrite(afterWrite func(event any) error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.afterWrite = afterWrite
}

// Write emits a single event as a JSON line.
// If the after-write hook fails, the write is treated as failed for callers.
func (w *Writer) Write(event any) error {
	var buffer bytes.Buffer
	encoder := json.NewEncoder(&buffer)
	// Disable HTML escaping to match Claude Code's JSON.stringify output.
	encoder.SetEscapeHTML(false)
	if err := encoder.Encode(event); err != nil {
		return fmt.Errorf("encode stream-json event: %w", err)
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	if _, err := w.writer.Write(buffer.Bytes()); err != nil {
		return fmt.Errorf("write stream-json event: %w", err)
	}
	if w.afterWrite != nil {
		if err := w.afterWrite(event); err != nil {
			return fmt.Errorf("after-write hook: %w", err)
		}
	}
	return nil
}

// NewUUID returns a new UUID string for stream-json events.
func NewUUID() string {
	return uuid.NewString()
}

// StandardServiceTier is the default service tier label in Claude Code output.
const StandardServiceTier = "standard"

// BuildTextMessage constructs a message containing a single text block.
func BuildTextMessage(role string, text string) Message {
	return Message{
		Type: "message",
		Role: role,
		Content: []ContentBlock{
			{Type: "text", Text: text},
		},
	}
}

// BuildToolUseMessage constructs an assistant message containing tool_use blocks.
func BuildToolUseMessage(toolCalls []openai.ToolCall) Message {
	blocks := make([]ContentBlock, 0, len(toolCalls))
	for _, call := range toolCalls {
		input := map[string]any{}
		if err := json.Unmarshal([]byte(call.Function.Arguments), &input); err != nil {
			input["raw"] = call.Function.Arguments
		}
		blocks = append(blocks, ContentBlock{
			Type:  "tool_use",
			ID:    call.ID,
			Name:  call.Function.Name,
			Input: input,
		})
	}
	return Message{
		Type:    "message",
		Role:    "assistant",
		Content: blocks,
	}
}

// BuildToolResultMessage constructs a user message containing tool_result blocks.
func BuildToolResultMessage(toolCallID string, content string, isError bool) Message {
	return Message{
		Type: "message",
		Role: "user",
		Content: []ContentBlock{
			{
				Type:      "tool_result",
				ToolUseID: toolCallID,
				Content:   content,
				IsError:   isError,
			},
		},
	}
}

// BuildAssistantMessage builds an assistant message from an OpenAI message.
func BuildAssistantMessage(message openai.Message) Message {
	var blocks []ContentBlock
	if text, ok := message.Content.(string); ok && text != "" {
		blocks = append(blocks, ContentBlock{Type: "text", Text: text})
	}
	if len(message.ToolCalls) > 0 {
		for _, call := range message.ToolCalls {
			input := map[string]any{}
			if err := json.Unmarshal([]byte(call.Function.Arguments), &input); err != nil {
				input["raw"] = call.Function.Arguments
			}
			blocks = append(blocks, ContentBlock{
				Type:  "tool_use",
				ID:    call.ID,
				Name:  call.Function.Name,
				Input: input,
			})
		}
	}
	if len(blocks) > 0 {
		return Message{Type: "message", Role: "assistant", Content: blocks}
	}
	raw, err := json.Marshal(message.Content)
	if err != nil {
		return BuildTextMessage("assistant", fmt.Sprintf("%v", message.Content))
	}
	return BuildTextMessage("assistant", string(raw))
}

// BuildUserMessage builds a user message from an OpenAI message.
func BuildUserMessage(message openai.Message) Message {
	if text, ok := message.Content.(string); ok {
		return BuildTextMessage("user", text)
	}
	raw, err := json.Marshal(message.Content)
	if err != nil {
		return BuildTextMessage("user", fmt.Sprintf("%v", message.Content))
	}
	return BuildTextMessage("user", string(raw))
}

// NewMessageUsageFromOpenAI converts OpenAI usage data into a Claude-style usage payload.
// Cache and server tool usage fields are zeroed when the gateway does not provide them.
func NewMessageUsageFromOpenAI(usage openai.Usage, serviceTier string) *MessageUsage {
	var tier *string
	if serviceTier != "" {
		tier = StringPointer(serviceTier)
	}
	return &MessageUsage{
		InputTokens:              usage.PromptTokens,
		OutputTokens:             usage.CompletionTokens,
		CacheCreationInputTokens: 0,
		CacheReadInputTokens:     0,
		ServerToolUse: MessageServerToolUse{
			WebSearchRequests: 0,
			WebFetchRequests:  0,
		},
		ServiceTier: tier,
		CacheCreation: MessageCacheCreation{
			Ephemeral1HInputTokens: 0,
			Ephemeral5MInputTokens: 0,
		},
	}
}

// NewEmptyMessageUsage returns a zeroed usage payload.
// This is used for synthetic assistant/result events that must still carry usage fields.
func NewEmptyMessageUsage(serviceTier string) *MessageUsage {
	var tier *string
	if serviceTier != "" {
		tier = StringPointer(serviceTier)
	}
	return &MessageUsage{
		InputTokens:              0,
		OutputTokens:             0,
		CacheCreationInputTokens: 0,
		CacheReadInputTokens:     0,
		ServerToolUse: MessageServerToolUse{
			WebSearchRequests: 0,
			WebFetchRequests:  0,
		},
		ServiceTier: tier,
		CacheCreation: MessageCacheCreation{
			Ephemeral1HInputTokens: 0,
			Ephemeral5MInputTokens: 0,
		},
	}
}

// NewNullRawMessage returns a JSON "null" payload pointer.
// Use this to force a null value to be emitted when omitempty is present.
func NewNullRawMessage() *json.RawMessage {
	value := json.RawMessage("null")
	return &value
}

// StringPointer returns a pointer to the provided string.
// It avoids allocating temporary variables at call sites.
func StringPointer(value string) *string {
	return &value
}

// ExtractText extracts text content from an Anthropic-style content array.
func ExtractText(content any) string {
	switch typed := content.(type) {
	case string:
		return typed
	case []any:
		var builder strings.Builder
		for _, item := range typed {
			block, ok := item.(map[string]any)
			if !ok {
				continue
			}
			if block["type"] == "text" {
				if text, ok := block["text"].(string); ok {
					builder.WriteString(text)
				}
			}
		}
		return builder.String()
	default:
		return ""
	}
}

// BuildStreamEventsForText synthesizes stream_event messages for text output.
func BuildStreamEventsForText(text string, model string, sessionID string) []StreamEvent {
	if text == "" {
		return nil
	}

	messageID := NewUUID()
	events := []StreamEvent{
		{
			Type:            "stream_event",
			SessionID:       sessionID,
			ParentToolUseID: nil,
			UUID:            NewUUID(),
			Event: MessageStartEvent{
				Type: "message_start",
				Message: StreamMessage{
					ID:           messageID,
					Type:         "message",
					Role:         "assistant",
					Model:        model,
					Content:      []any{},
					StopReason:   nil,
					StopSequence: nil,
				},
			},
		},
		{
			Type:            "stream_event",
			SessionID:       sessionID,
			ParentToolUseID: nil,
			UUID:            NewUUID(),
			Event: ContentBlockStartEvent{
				Type:  "content_block_start",
				Index: 0,
				ContentBlock: ContentBlock{
					Type: "text",
					Text: "",
				},
			},
		},
	}

	for _, chunk := range splitText(text, 60) {
		events = append(events, StreamEvent{
			Type:            "stream_event",
			SessionID:       sessionID,
			ParentToolUseID: nil,
			UUID:            NewUUID(),
			Event: ContentBlockDeltaEvent{
				Type:  "content_block_delta",
				Index: 0,
				Delta: StreamDelta{
					Type: "text_delta",
					Text: chunk,
				},
			},
		})
	}

	events = append(events,
		StreamEvent{
			Type:            "stream_event",
			SessionID:       sessionID,
			ParentToolUseID: nil,
			UUID:            NewUUID(),
			Event: ContentBlockStopEvent{
				Type:  "content_block_stop",
				Index: 0,
			},
		},
		StreamEvent{
			Type:            "stream_event",
			SessionID:       sessionID,
			ParentToolUseID: nil,
			UUID:            NewUUID(),
			Event: MessageDeltaEvent{
				Type: "message_delta",
				Delta: MessageDelta{
					StopReason:   "end_turn",
					StopSequence: nil,
				},
			},
		},
		StreamEvent{
			Type:            "stream_event",
			SessionID:       sessionID,
			ParentToolUseID: nil,
			UUID:            NewUUID(),
			Event:           MessageStopEvent{Type: "message_stop"},
		},
	)

	return events
}

// splitText chunks a string by rune length.
func splitText(text string, chunkSize int) []string {
	if chunkSize <= 0 {
		return []string{text}
	}
	runes := []rune(text)
	if len(runes) <= chunkSize {
		return []string{text}
	}
	var chunks []string
	for i := 0; i < len(runes); i += chunkSize {
		end := i + chunkSize
		if end > len(runes) {
			end = len(runes)
		}
		chunks = append(chunks, string(runes[i:end]))
	}
	return chunks
}
