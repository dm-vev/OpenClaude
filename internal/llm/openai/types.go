package openai

// ChatRequest matches the OpenAI-compatible chat/completions request.
type ChatRequest struct {
	// Model is the provider model identifier.
	Model string `json:"model"`
	// Messages is the ordered conversation history.
	Messages []Message `json:"messages"`
	// Tools advertises available tool functions.
	Tools []Tool `json:"tools,omitempty"`
	// ToolChoice directs tool usage (e.g., "auto").
	ToolChoice any `json:"tool_choice,omitempty"`
	// Stream toggles server-sent events in the response.
	Stream bool `json:"stream,omitempty"`
	// Temperature controls randomness, if supported by the backend.
	Temperature *float64 `json:"temperature,omitempty"`
	// MaxTokens limits the model output, if supported by the backend.
	MaxTokens *int `json:"max_tokens,omitempty"`
}

// Message represents a chat message.
type Message struct {
	// Role is one of system, user, assistant, or tool.
	Role string `json:"role"`
	// Content carries message text or structured payloads.
	Content any `json:"content,omitempty"`
	// ToolCalls lists tool invocations requested by the assistant.
	ToolCalls []ToolCall `json:"tool_calls,omitempty"`
	// ToolCallID associates a tool response to a prior call.
	ToolCallID string `json:"tool_call_id,omitempty"`
	// Name optionally identifies a function or assistant.
	Name string `json:"name,omitempty"`
}

// Tool describes a callable function for the model.
type Tool struct {
	// Type must be "function" for OpenAI-compatible tools.
	Type string `json:"type"`
	// Function describes the callable function contract.
	Function ToolFunction `json:"function"`
}

// ToolFunction defines a function for tool calling.
type ToolFunction struct {
	// Name is the unique identifier for the function.
	Name string `json:"name"`
	// Description provides a natural language summary.
	Description string `json:"description,omitempty"`
	// Parameters is a JSON Schema object describing inputs.
	Parameters map[string]any `json:"parameters,omitempty"`
}

// ToolCall represents a tool invocation requested by the model.
type ToolCall struct {
	// ID is the unique tool call id.
	ID string `json:"id"`
	// Type is the tool type, typically "function".
	Type string `json:"type"`
	// Function includes the name and serialized arguments.
	Function ToolCallFunction `json:"function"`
}

// ToolCallFunction is the function call payload.
type ToolCallFunction struct {
	// Name identifies which tool to invoke.
	Name string `json:"name"`
	// Arguments contains a JSON string to be parsed by the tool.
	Arguments string `json:"arguments"`
}

// ChatResponse matches the OpenAI-compatible chat/completions response.
type ChatResponse struct {
	// ID is the request id from the provider.
	ID string `json:"id"`
	// Choices contains the assistant messages.
	Choices []ChatChoice `json:"choices"`
	// Usage reports token counts.
	Usage Usage `json:"usage"`
}

// ChatChoice represents a single completion choice.
type ChatChoice struct {
	// Index is the choice index.
	Index int `json:"index"`
	// Message is the assistant response.
	Message Message `json:"message"`
	// FinishReason indicates why generation stopped.
	FinishReason string `json:"finish_reason"`
}

// Usage represents token usage info.
type Usage struct {
	// PromptTokens counts input tokens.
	PromptTokens int `json:"prompt_tokens"`
	// CompletionTokens counts output tokens.
	CompletionTokens int `json:"completion_tokens"`
	// TotalTokens is the sum of prompt and completion tokens.
	TotalTokens int `json:"total_tokens"`
}
