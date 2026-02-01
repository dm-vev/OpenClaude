package openai

import (
	"strings"
)

// StreamAccumulator builds a full assistant message from streaming deltas.
type StreamAccumulator struct {
	// contentBuilder accumulates streamed text content.
	contentBuilder strings.Builder
	// toolStates stores tool call data keyed by streaming index.
	toolStates map[int]*toolCallState
	// toolOrder preserves the order tool calls first appeared.
	toolOrder []int
	// finishReason stores the latest finish reason.
	finishReason string
	// usage stores token usage when provided.
	usage Usage
	// hasUsage reports whether usage was supplied.
	hasUsage bool
	// model records the model identifier.
	model string
	// id captures the request id.
	id string
}

// toolCallState accumulates a single tool call delta sequence.
type toolCallState struct {
	// id is the tool call id.
	id string
	// callType is the tool call type.
	callType string
	// name is the tool function name.
	name string
	// argumentsBuilder accumulates the raw JSON arguments.
	argumentsBuilder strings.Builder
}

// NewStreamAccumulator creates a new accumulator for a streaming response.
func NewStreamAccumulator() *StreamAccumulator {
	return &StreamAccumulator{
		toolStates: map[int]*toolCallState{},
	}
}

// Apply ingests a streaming event and updates the accumulator state.
func (acc *StreamAccumulator) Apply(event StreamResponse) error {
	if acc.id == "" && event.ID != "" {
		acc.id = event.ID
	}
	if acc.model == "" && event.Model != "" {
		acc.model = event.Model
	}
	if event.Usage != nil {
		acc.usage = *event.Usage
		acc.hasUsage = true
	}
	for _, choice := range event.Choices {
		if choice.Index != 0 {
			continue
		}
		delta := choice.Delta
		if delta.Content != "" {
			acc.contentBuilder.WriteString(delta.Content)
		}
		for _, toolDelta := range delta.ToolCalls {
			state := acc.toolStates[toolDelta.Index]
			if state == nil {
				state = &toolCallState{}
				acc.toolStates[toolDelta.Index] = state
				acc.toolOrder = append(acc.toolOrder, toolDelta.Index)
			}
			if toolDelta.ID != "" {
				state.id = toolDelta.ID
			}
			if toolDelta.Type != "" {
				state.callType = toolDelta.Type
			}
			if toolDelta.Function.Name != "" {
				state.name = toolDelta.Function.Name
			}
			if toolDelta.Function.Arguments != "" {
				state.argumentsBuilder.WriteString(toolDelta.Function.Arguments)
			}
		}
		if choice.FinishReason != nil {
			acc.finishReason = *choice.FinishReason
		}
	}
	return nil
}

// Message returns the aggregated assistant message.
func (acc *StreamAccumulator) Message() Message {
	message := Message{
		Role: "assistant",
	}
	if content := acc.contentBuilder.String(); content != "" {
		message.Content = content
	}
	message.ToolCalls = acc.ToolCalls()
	return message
}

// ToolCalls returns tool calls in their first-seen order.
func (acc *StreamAccumulator) ToolCalls() []ToolCall {
	calls := make([]ToolCall, 0, len(acc.toolOrder))
	for _, index := range acc.toolOrder {
		state := acc.toolStates[index]
		if state == nil {
			continue
		}
		callType := state.callType
		if callType == "" {
			callType = "function"
		}
		calls = append(calls, ToolCall{
			ID:   state.id,
			Type: callType,
			Function: ToolCallFunction{
				Name:      state.name,
				Arguments: state.argumentsBuilder.String(),
			},
		})
	}
	return calls
}

// FinishReason returns the most recent finish reason.
func (acc *StreamAccumulator) FinishReason() string {
	return acc.finishReason
}

// Usage returns the final usage and whether it was provided.
func (acc *StreamAccumulator) Usage() (Usage, bool) {
	return acc.usage, acc.hasUsage
}

// Model returns the model identifier, if present.
func (acc *StreamAccumulator) Model() string {
	return acc.model
}

// ID returns the stream request id, if present.
func (acc *StreamAccumulator) ID() string {
	return acc.id
}
