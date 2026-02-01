package streamjson

import (
	"testing"

	"github.com/openclaude/openclaude/internal/llm/openai"
)

func TestBuildAssistantMessageWithToolUse(t *testing.T) {
	// Arrange an assistant message with text and a tool call.
	msg := openai.Message{
		Role:    "assistant",
		Content: "hello",
		ToolCalls: []openai.ToolCall{
			{
				ID:   "call_1",
				Type: "function",
				Function: openai.ToolCallFunction{
					Name:      "Bash",
					Arguments: `{"command":"ls"}`,
				},
			},
		},
	}

	// Act.
	built := BuildAssistantMessage(msg)

	// Assert.
	blocks, ok := built.Content.([]ContentBlock)
	if !ok {
		t.Fatalf("expected content blocks, got %T", built.Content)
	}
	if len(blocks) != 2 {
		t.Fatalf("expected 2 content blocks, got %d", len(blocks))
	}
	if blocks[0].Type != "text" || blocks[0].Text != "hello" {
		t.Fatalf("expected text block, got %+v", blocks[0])
	}
	if blocks[1].Type != "tool_use" || blocks[1].Name != "Bash" || blocks[1].ID != "call_1" {
		t.Fatalf("expected tool_use block, got %+v", blocks[1])
	}
}

func TestBuildStreamEventsForText(t *testing.T) {
	// Arrange a short text payload.
	events := BuildStreamEventsForText("hello", "model-x")

	// Assert a minimal stream sequence is present.
	if len(events) != 6 {
		t.Fatalf("expected 6 stream events, got %d", len(events))
	}
}
