package openai

// StreamOptions configures OpenAI-compatible stream behavior.
type StreamOptions struct {
	// IncludeUsage requests token usage in the final stream payload.
	IncludeUsage bool `json:"include_usage,omitempty"`
}

// StreamResponse is the OpenAI-compatible SSE response payload.
type StreamResponse struct {
	// ID is the provider request id.
	ID string `json:"id,omitempty"`
	// Model is the model identifier for the stream.
	Model string `json:"model,omitempty"`
	// Choices carries incremental delta updates.
	Choices []StreamChoice `json:"choices,omitempty"`
	// Usage reports tokens when stream_options.include_usage is enabled.
	Usage *Usage `json:"usage,omitempty"`
}

// StreamChoice represents a streaming choice delta.
type StreamChoice struct {
	// Index is the choice index.
	Index int `json:"index"`
	// Delta holds the incremental message update.
	Delta StreamDelta `json:"delta"`
	// FinishReason signals why generation stopped.
	FinishReason *string `json:"finish_reason,omitempty"`
}

// StreamDelta represents incremental message content.
type StreamDelta struct {
	// Role sets the assistant role on the first delta.
	Role string `json:"role,omitempty"`
	// Content holds streamed text.
	Content string `json:"content,omitempty"`
	// ToolCalls streams tool call metadata and arguments.
	ToolCalls []StreamToolCallDelta `json:"tool_calls,omitempty"`
}

// StreamToolCallDelta represents incremental tool call data.
type StreamToolCallDelta struct {
	// Index identifies the tool call position.
	Index int `json:"index"`
	// ID is the tool call id.
	ID string `json:"id,omitempty"`
	// Type is the tool call type (typically "function").
	Type string `json:"type,omitempty"`
	// Function contains tool function deltas.
	Function StreamToolCallFunctionDelta `json:"function,omitempty"`
}

// StreamToolCallFunctionDelta contains incremental tool function fields.
type StreamToolCallFunctionDelta struct {
	// Name identifies the tool name.
	Name string `json:"name,omitempty"`
	// Arguments contains partial JSON argument text.
	Arguments string `json:"arguments,omitempty"`
}

// StreamHandler consumes SSE stream responses.
type StreamHandler func(event StreamResponse) error

// StreamSummary captures metadata from a streaming response.
type StreamSummary struct {
	// ID is the stream request id.
	ID string
	// Model is the model identifier.
	Model string
	// Usage reports token usage if available.
	Usage Usage
	// HasUsage reports whether Usage is populated.
	HasUsage bool
}
