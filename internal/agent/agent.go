package agent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/openclaude/openclaude/internal/config"
	"github.com/openclaude/openclaude/internal/llm/openai"
	"github.com/openclaude/openclaude/internal/tools"
)

var (
	// ErrMaxTurns signals that the agent exceeded the allowed turn count.
	ErrMaxTurns = errors.New("max turns exceeded")
	// ErrMaxBudget signals that the cost limit was exceeded.
	ErrMaxBudget = errors.New("max budget exceeded")
	// ErrToolDenied signals a user denied a tool call.
	ErrToolDenied = errors.New("tool denied")
	// ErrPlanMode signals that tools are disabled in plan mode.
	ErrPlanMode = errors.New("tools are disabled in plan mode")
)

// ToolEvent captures tool call/result events for streaming output.
type ToolEvent struct {
	// Type is either "tool_call" or "tool_result".
	Type string `json:"type"`
	// ToolName is the function name, if available.
	ToolName string `json:"tool_name,omitempty"`
	// ToolID associates tool results with calls.
	ToolID string `json:"tool_id,omitempty"`
	// Arguments stores serialized tool arguments.
	Arguments json.RawMessage `json:"arguments,omitempty"`
	// Result stores tool output content.
	Result string `json:"result,omitempty"`
	// IsError indicates whether the tool result represents a failure.
	IsError bool `json:"is_error,omitempty"`
}

// RunResult captures the outcome of a single user turn.
type RunResult struct {
	// Messages is the full conversation history.
	Messages []openai.Message
	// Final is the last assistant message in the turn.
	Final openai.Message
	// Usage reports token counts for the last call.
	Usage openai.Usage
	// TotalUsage accumulates usage across all calls.
	TotalUsage openai.Usage
	// ModelUsage aggregates usage by model identifier.
	ModelUsage map[string]openai.Usage
	// Events contains tool call and result events.
	Events []ToolEvent
	// CostUSD is the accumulated cost for the run.
	CostUSD float64
	// NumTurns counts the number of assistant turns executed.
	NumTurns int
	// Duration is the total runtime for the run.
	Duration time.Duration
	// APIDuration is the cumulative time spent in API calls.
	APIDuration time.Duration
}

// ToolAuthorizer controls interactive permission prompts.
type ToolAuthorizer func(toolName string, args json.RawMessage) (bool, error)

// Runner executes the agent loop.
type Runner struct {
	// Client executes OpenAI-compatible requests.
	Client *openai.Client
	// ToolRunner dispatches tool calls.
	ToolRunner *tools.Runner
	// ToolContext provides filesystem/session context to tools.
	ToolContext tools.ToolContext
	// Permissions defines how tool approval works.
	Permissions tools.Permissions
	// AuthorizeTool prompts user approval when required.
	AuthorizeTool ToolAuthorizer
	// MaxTurns limits the number of tool-assisted turns.
	MaxTurns int
	// Pricing provides per-model costs for budget tracking.
	Pricing map[string]config.ModelPricing
	// MaxBudgetUSD enforces a ceiling on estimated cost.
	MaxBudgetUSD float64
}

// Run executes a single user turn with tool handling.
func (r *Runner) Run(
	ctx context.Context,
	messages []openai.Message,
	systemPrompt string,
	model string,
	toolsEnabled bool,
) (*RunResult, error) {
	// Ensure a client is available for upstream calls.
	if r.Client == nil {
		return nil, errors.New("client is required")
	}
	if r.MaxTurns <= 0 {
		r.MaxTurns = 8
	}

	// Prepend a system prompt if provided.
	if systemPrompt != "" {
		messages = prependSystem(messages, systemPrompt)
	}

	result := &RunResult{
		Messages:   messages,
		ModelUsage: map[string]openai.Usage{},
	}

	startTime := time.Now()

	for turn := 0; turn < r.MaxTurns; turn++ {
		req := &openai.ChatRequest{
			Model:    model,
			Messages: result.Messages,
		}
		if toolsEnabled && r.ToolRunner != nil {
			req.Tools = r.ToolRunner.ToolSpecs()
			req.ToolChoice = "auto"
		}

		callStart := time.Now()
		resp, err := r.Client.ChatCompletions(ctx, req)
		result.APIDuration += time.Since(callStart)
		if err != nil {
			return nil, err
		}

		choice := resp.Choices[0]
		result.Usage = resp.Usage
		accumulateUsage(&result.TotalUsage, resp.Usage)
		accumulateUsageMap(result.ModelUsage, model, resp.Usage)
		result.Messages = append(result.Messages, choice.Message)
		result.Final = choice.Message
		result.CostUSD += estimateCost(model, resp.Usage, r.Pricing)
		result.NumTurns++
		if r.MaxBudgetUSD > 0 && result.CostUSD > r.MaxBudgetUSD {
			result.Duration = time.Since(startTime)
			return nil, fmt.Errorf("%w: %.4f > %.4f", ErrMaxBudget, result.CostUSD, r.MaxBudgetUSD)
		}

		// If no tool calls are requested, return the assistant response.
		if len(choice.Message.ToolCalls) == 0 || !toolsEnabled || r.ToolRunner == nil {
			result.Duration = time.Since(startTime)
			return result, nil
		}

		for _, call := range choice.Message.ToolCalls {
			args := json.RawMessage(call.Function.Arguments)
			event := ToolEvent{
				Type:      "tool_call",
				ToolName:  call.Function.Name,
				ToolID:    call.ID,
				Arguments: args,
			}
			result.Events = append(result.Events, event)

			// Plan mode must not execute any tools.
			if r.Permissions.Mode == tools.PermissionPlan {
				return nil, ErrPlanMode
			}

			// If configured, ask for user permission before invoking tools.
			if r.AuthorizeTool != nil && r.Permissions.ShouldPrompt(call.Function.Name) {
				allowed, err := r.AuthorizeTool(call.Function.Name, args)
				if err != nil {
					return nil, err
				}
				if !allowed {
					return nil, fmt.Errorf("%w: %s", ErrToolDenied, call.Function.Name)
				}
			}

			toolResult, err := r.ToolRunner.Run(ctx, call.Function.Name, args, r.ToolContext)
			if err != nil {
				toolResult = tools.ToolResult{IsError: true, Content: err.Error()}
			}

			result.Events = append(result.Events, ToolEvent{
				Type:     "tool_result",
				ToolName: call.Function.Name,
				ToolID:   call.ID,
				Result:   toolResult.Content,
				IsError:  toolResult.IsError,
			})

			toolMessage := openai.Message{
				Role:       "tool",
				ToolCallID: call.ID,
				Content:    toolResult.Content,
			}
			result.Messages = append(result.Messages, toolMessage)
		}
	}

	result.Duration = time.Since(startTime)
	return result, ErrMaxTurns
}

// prependSystem injects a system message at the start of the conversation.
func prependSystem(messages []openai.Message, prompt string) []openai.Message {
	if len(messages) > 0 && messages[0].Role == "system" {
		messages[0].Content = fmt.Sprintf("%v\n\n%v", messages[0].Content, prompt)
		return messages
	}
	system := openai.Message{Role: "system", Content: prompt}
	return append([]openai.Message{system}, messages...)
}

// estimateCost computes cost using pricing per million tokens.
func estimateCost(model string, usage openai.Usage, pricing map[string]config.ModelPricing) float64 {
	if pricing == nil {
		return 0
	}
	price, ok := pricing[model]
	if !ok {
		return 0
	}
	input := float64(usage.PromptTokens) / 1_000_000
	output := float64(usage.CompletionTokens) / 1_000_000
	return input*price.InputPer1M + output*price.OutputPer1M
}

// accumulateUsage adds usage counts into the accumulator.
func accumulateUsage(acc *openai.Usage, usage openai.Usage) {
	acc.PromptTokens += usage.PromptTokens
	acc.CompletionTokens += usage.CompletionTokens
	acc.TotalTokens += usage.TotalTokens
}

// accumulateUsageMap adds usage counts into a per-model map.
func accumulateUsageMap(target map[string]openai.Usage, model string, usage openai.Usage) {
	current := target[model]
	current.PromptTokens += usage.PromptTokens
	current.CompletionTokens += usage.CompletionTokens
	current.TotalTokens += usage.TotalTokens
	target[model] = current
}
