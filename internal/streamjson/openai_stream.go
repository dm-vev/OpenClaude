package streamjson

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/openclaude/openclaude/internal/llm/openai"
)

// OpenAIStreamEmitter converts OpenAI SSE deltas into Claude Code stream-json events.
type OpenAIStreamEmitter struct {
	// writer emits JSONL events to the output stream.
	writer *Writer
	// includePartials controls whether stream_event payloads are emitted.
	includePartials bool
	// sessionID scopes stream_event envelopes.
	sessionID string
	// state tracks the active streaming message.
	state *openAIStreamState
}

// openAIStreamState tracks a single streaming assistant response.
type openAIStreamState struct {
	// writer emits stream-json events.
	writer *Writer
	// includePartials controls stream_event emission.
	includePartials bool
	// sessionID scopes stream_event envelopes.
	sessionID string
	// model is the model identifier for the stream.
	model string
	// messageID is the stream message id.
	messageID string
	// started reports whether message_start was emitted.
	started bool
	// emitted reports whether any stream_event was written.
	emitted bool
	// blocks stores the ordered content blocks.
	blocks []streamBlock
	// textBlockIndex is the index of the text block in blocks.
	textBlockIndex int
	// hasTextBlock reports whether the text block exists.
	hasTextBlock bool
	// textBuilder accumulates streamed text.
	textBuilder strings.Builder
	// toolBlockIndex maps tool call index to block index.
	toolBlockIndex map[int]int
	// toolBlocks stores tool call state keyed by tool index.
	toolBlocks map[int]*toolBlockState
	// finishReason tracks the OpenAI finish reason.
	finishReason string
}

// streamBlock represents a content block in message order.
type streamBlock struct {
	// kind is either "text" or "tool_use".
	kind string
	// toolIndex links tool_use blocks to their tool call index.
	toolIndex int
}

// toolBlockState stores incremental tool call data.
type toolBlockState struct {
	// id is the tool call id.
	id string
	// name is the tool function name.
	name string
	// callType is the tool call type.
	callType string
	// argumentsBuilder accumulates JSON arguments.
	argumentsBuilder strings.Builder
	// started reports whether content_block_start was emitted.
	started bool
	// stopped reports whether content_block_stop was emitted.
	stopped bool
}

// NewOpenAIStreamEmitter constructs a stream emitter.
func NewOpenAIStreamEmitter(writer *Writer, includePartials bool, sessionID string) *OpenAIStreamEmitter {
	return &OpenAIStreamEmitter{
		writer:          writer,
		includePartials: includePartials,
		sessionID:       sessionID,
	}
}

// Begin resets state for a new assistant message stream.
func (emitter *OpenAIStreamEmitter) Begin(model string) {
	emitter.state = &openAIStreamState{
		writer:          emitter.writer,
		includePartials: emitter.includePartials,
		sessionID:       emitter.sessionID,
		model:           model,
		messageID:       NewUUID(),
		textBlockIndex:  -1,
		toolBlockIndex:  map[int]int{},
		toolBlocks:      map[int]*toolBlockState{},
	}
}

// Handle ingests a streaming event and emits stream_event JSON if enabled.
func (emitter *OpenAIStreamEmitter) Handle(event openai.StreamResponse) error {
	if emitter.state == nil {
		model := event.Model
		emitter.Begin(model)
	}
	return emitter.state.Handle(event)
}

// Finalize completes the stream, emitting stop events when needed.
func (emitter *OpenAIStreamEmitter) Finalize() (Message, bool, error) {
	if emitter.state == nil {
		return Message{}, false, errors.New("no stream state available")
	}
	return emitter.state.Finalize()
}

// Streamed reports whether any stream_event was emitted.
func (emitter *OpenAIStreamEmitter) Streamed() bool {
	if emitter.state == nil {
		return false
	}
	return emitter.state.emitted
}

// Handle consumes OpenAI stream events for a single response.
func (state *openAIStreamState) Handle(event openai.StreamResponse) error {
	for _, choice := range event.Choices {
		if choice.Index != 0 {
			continue
		}
		delta := choice.Delta
		if delta.Content != "" {
			if err := state.ensureMessageStarted(); err != nil {
				return err
			}
			if err := state.ensureTextBlock(); err != nil {
				return err
			}
			state.textBuilder.WriteString(delta.Content)
			if state.includePartials {
				if err := state.write(StreamEvent{
					Type: "stream_event",
					Event: ContentBlockDeltaEvent{
						Type:  "content_block_delta",
						Index: state.textBlockIndex,
						Delta: StreamDelta{
							Type: "text_delta",
							Text: delta.Content,
						},
					},
				}); err != nil {
					return err
				}
			}
		}
		for _, toolDelta := range delta.ToolCalls {
			if err := state.ensureMessageStarted(); err != nil {
				return err
			}
			blockIndex, blockState, err := state.ensureToolBlock(toolDelta)
			if err != nil {
				return err
			}
			if toolDelta.Function.Arguments != "" {
				blockState.argumentsBuilder.WriteString(toolDelta.Function.Arguments)
				if state.includePartials {
					if err := state.write(StreamEvent{
						Type: "stream_event",
						Event: ContentBlockDeltaEvent{
							Type:  "content_block_delta",
							Index: blockIndex,
							Delta: StreamDelta{
								Type:        "input_json_delta",
								PartialJSON: toolDelta.Function.Arguments,
							},
						},
					}); err != nil {
						return err
					}
				}
			}
		}
		if choice.FinishReason != nil {
			state.finishReason = *choice.FinishReason
		}
	}
	return nil
}

// Finalize completes the stream and builds the assistant message.
func (state *openAIStreamState) Finalize() (Message, bool, error) {
	if state.includePartials && state.started {
		if err := state.stopBlocks(); err != nil {
			return Message{}, false, err
		}
		stopReason := mapFinishReason(state.finishReason)
		if err := state.write(StreamEvent{
			Type: "stream_event",
			Event: MessageDeltaEvent{
				Type: "message_delta",
				Delta: MessageDelta{
					StopReason:   stopReason,
					StopSequence: nil,
				},
			},
		}); err != nil {
			return Message{}, false, err
		}
		if err := state.write(StreamEvent{
			Type:  "stream_event",
			Event: MessageStopEvent{Type: "message_stop"},
		}); err != nil {
			return Message{}, false, err
		}
	}

	message, ok, err := state.buildMessage()
	if err != nil {
		return Message{}, false, err
	}
	return message, ok, nil
}

// ensureMessageStarted emits message_start if needed.
func (state *openAIStreamState) ensureMessageStarted() error {
	if state.started || !state.includePartials {
		state.started = true
		return nil
	}
	state.started = true
	return state.write(StreamEvent{
		Type: "stream_event",
		Event: MessageStartEvent{
			Type: "message_start",
			Message: StreamMessage{
				ID:           state.messageID,
				Type:         "message",
				Role:         "assistant",
				Model:        state.model,
				Content:      []any{},
				StopReason:   nil,
				StopSequence: nil,
			},
		},
	})
}

// ensureTextBlock allocates the text block when streaming text appears.
func (state *openAIStreamState) ensureTextBlock() error {
	if state.hasTextBlock {
		return nil
	}
	state.textBlockIndex = len(state.blocks)
	state.blocks = append(state.blocks, streamBlock{kind: "text"})
	state.hasTextBlock = true
	if !state.includePartials {
		return nil
	}
	return state.write(StreamEvent{
		Type: "stream_event",
		Event: ContentBlockStartEvent{
			Type:  "content_block_start",
			Index: state.textBlockIndex,
			ContentBlock: ContentBlock{
				Type: "text",
				Text: "",
			},
		},
	})
}

// ensureToolBlock allocates a tool_use block for the given delta.
func (state *openAIStreamState) ensureToolBlock(
	delta openai.StreamToolCallDelta,
) (int, *toolBlockState, error) {
	blockIndex, ok := state.toolBlockIndex[delta.Index]
	blockState := state.toolBlocks[delta.Index]
	if !ok {
		blockIndex = len(state.blocks)
		state.toolBlockIndex[delta.Index] = blockIndex
		state.blocks = append(state.blocks, streamBlock{kind: "tool_use", toolIndex: delta.Index})
		blockState = &toolBlockState{}
		state.toolBlocks[delta.Index] = blockState
	}
	if delta.ID != "" {
		blockState.id = delta.ID
	}
	if delta.Type != "" {
		blockState.callType = delta.Type
	}
	if delta.Function.Name != "" {
		blockState.name = delta.Function.Name
	}
	if state.includePartials && !blockState.started {
		blockState.started = true
		if err := state.write(StreamEvent{
			Type: "stream_event",
			Event: ContentBlockStartEvent{
				Type:  "content_block_start",
				Index: blockIndex,
				ContentBlock: ContentBlock{
					Type:  "tool_use",
					ID:    blockState.id,
					Name:  blockState.name,
					Input: map[string]any{},
				},
			},
		}); err != nil {
			return 0, nil, err
		}
	}
	return blockIndex, blockState, nil
}

// stopBlocks emits content_block_stop events for active blocks.
// Blocks are stopped in the same order they were started to keep output deterministic.
func (state *openAIStreamState) stopBlocks() error {
	for blockIndex, block := range state.blocks {
		switch block.kind {
		case "text":
			if !state.hasTextBlock {
				continue
			}
			if err := state.write(StreamEvent{
				Type: "stream_event",
				Event: ContentBlockStopEvent{
					Type:  "content_block_stop",
					Index: blockIndex,
				},
			}); err != nil {
				return err
			}
		case "tool_use":
			blockState := state.toolBlocks[block.toolIndex]
			if blockState == nil || blockState.stopped {
				continue
			}
			blockState.stopped = true
			if err := state.write(StreamEvent{
				Type: "stream_event",
				Event: ContentBlockStopEvent{
					Type:  "content_block_stop",
					Index: blockIndex,
				},
			}); err != nil {
				return err
			}
		}
	}
	return nil
}

// buildMessage assembles the assistant message content blocks.
func (state *openAIStreamState) buildMessage() (Message, bool, error) {
	var blocks []ContentBlock
	for _, block := range state.blocks {
		switch block.kind {
		case "text":
			text := state.textBuilder.String()
			if text == "" {
				continue
			}
			blocks = append(blocks, ContentBlock{
				Type: "text",
				Text: text,
			})
		case "tool_use":
			toolState := state.toolBlocks[block.toolIndex]
			if toolState == nil {
				continue
			}
			input, err := parseToolInput(toolState.argumentsBuilder.String())
			if err != nil {
				return Message{}, false, err
			}
			blocks = append(blocks, ContentBlock{
				Type:  "tool_use",
				ID:    toolState.id,
				Name:  toolState.name,
				Input: input,
			})
		}
	}
	if len(blocks) == 0 {
		return Message{}, false, nil
	}
	stopReason := mapFinishReason(state.finishReason)
	var stopSequence *string
	if stopReason == "stop_sequence" {
		stopSequence = StringPointer("")
	}
	return Message{
		ID:           state.messageID,
		Type:         "message",
		Model:        state.model,
		Role:         "assistant",
		StopReason:   stopReason,
		StopSequence: stopSequence,
		Content:      blocks,
	}, true, nil
}

// parseToolInput converts a JSON argument string into a map or raw wrapper.
func parseToolInput(raw string) (any, error) {
	if raw == "" {
		return map[string]any{}, nil
	}
	var input any
	if err := json.Unmarshal([]byte(raw), &input); err != nil {
		return map[string]any{"raw": raw}, nil
	}
	return input, nil
}

// mapFinishReason converts OpenAI finish reasons into Claude Code stop reasons.
func mapFinishReason(reason string) string {
	switch reason {
	case "stop":
		return "end_turn"
	case "length":
		return "max_tokens"
	case "tool_calls":
		return "tool_use"
	default:
		if reason == "" {
			return "end_turn"
		}
		return reason
	}
}

// write emits a stream-json event and tracks output.
func (state *openAIStreamState) write(event StreamEvent) error {
	if state.writer == nil {
		return fmt.Errorf("stream-json writer is required")
	}
	if event.SessionID == "" {
		event.SessionID = state.sessionID
	}
	if event.UUID == "" {
		event.UUID = NewUUID()
	}
	if event.ParentToolUseID == nil {
		event.ParentToolUseID = nil
	}
	state.emitted = true
	return state.writer.Write(event)
}
