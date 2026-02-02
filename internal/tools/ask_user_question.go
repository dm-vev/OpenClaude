package tools

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
)

// AskUserQuestionTool prompts the user for input during interactive runs.
type AskUserQuestionTool struct{}

// Name returns the tool identifier used in tool calls.
func (t *AskUserQuestionTool) Name() string {
	return "AskUserQuestion"
}

// Description summarizes the interactive question behavior.
func (t *AskUserQuestionTool) Description() string {
	return "Ask the user a question and return their response."
}

// Schema describes the expected question payload.
func (t *AskUserQuestionTool) Schema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"question": map[string]any{
				"type":        "string",
				"description": "Question text to present to the user.",
			},
			"options": map[string]any{
				"type":        "array",
				"description": "Optional list of suggested responses.",
				"items": map[string]any{
					"type": "string",
				},
			},
			"default": map[string]any{
				"type":        "string",
				"description": "Default response if the user submits an empty line.",
			},
			"allow_multiple": map[string]any{
				"type":        "boolean",
				"description": "Whether multiple selections are allowed.",
			},
		},
		"required": []string{"question"},
	}
}

// Run prompts the user for input when a TTY is available.
func (t *AskUserQuestionTool) Run(ctx context.Context, input json.RawMessage, toolCtx ToolContext) (ToolResult, error) {
	// The tool is synchronous, so the context is unused by design.
	_ = ctx
	_ = toolCtx

	var payload struct {
		Question      string   `json:"question"`
		Options       []string `json:"options"`
		Default       string   `json:"default"`
		AllowMultiple bool     `json:"allow_multiple"`
	}
	if err := json.Unmarshal(input, &payload); err != nil {
		return ToolResult{IsError: true, Content: fmt.Sprintf("invalid input: %v", err)}, nil
	}
	payload.Question = strings.TrimSpace(payload.Question)
	if payload.Question == "" {
		return ToolResult{IsError: true, Content: "question is required"}, nil
	}

	// Allow automated runs to inject an answer via environment variable.
	if response := strings.TrimSpace(os.Getenv("OPENCLOUDE_ASK_RESPONSE")); response != "" {
		return ToolResult{Content: response}, nil
	}

	if !stdinIsTTY() {
		return ToolResult{IsError: true, Content: "AskUserQuestion requires an interactive TTY"}, nil
	}

	fmt.Fprintln(os.Stderr, payload.Question)
	if len(payload.Options) > 0 {
		for index, option := range payload.Options {
			fmt.Fprintf(os.Stderr, "  %d) %s\n", index+1, option)
		}
	}
	if payload.Default != "" {
		fmt.Fprintf(os.Stderr, "[default: %s] ", payload.Default)
	}

	reader := bufio.NewReader(os.Stdin)
	line, _ := reader.ReadString('\n')
	answer := strings.TrimSpace(line)
	if answer == "" {
		answer = payload.Default
	}

	if payload.AllowMultiple {
		answer = normalizeMultiAnswer(answer)
	}
	return ToolResult{Content: answer}, nil
}

// stdinIsTTY reports whether stdin is connected to a terminal.
func stdinIsTTY() bool {
	info, err := os.Stdin.Stat()
	if err != nil {
		return false
	}
	return info.Mode()&os.ModeCharDevice != 0
}

// normalizeMultiAnswer normalizes comma-separated responses.
func normalizeMultiAnswer(answer string) string {
	parts := strings.Split(answer, ",")
	normalized := make([]string, 0, len(parts))
	for _, part := range parts {
		trimmed := strings.TrimSpace(part)
		if trimmed == "" {
			continue
		}
		normalized = append(normalized, trimmed)
	}
	return strings.Join(normalized, ", ")
}
