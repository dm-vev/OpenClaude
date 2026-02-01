package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/openclaude/openclaude/internal/llm/openai"
	"github.com/openclaude/openclaude/internal/tools"
)

// StreamCallbacks wires streaming lifecycle hooks.
type StreamCallbacks struct {
	// OnStreamStart fires before each streaming request.
	OnStreamStart func(model string) error
	// OnStreamEvent receives raw OpenAI stream events.
	OnStreamEvent func(event openai.StreamResponse) error
	// OnStreamComplete fires after the assistant message is assembled.
	OnStreamComplete func(summary StreamSummary) error
	// OnToolResult fires after a tool result is appended to messages.
	OnToolResult func(event ToolEvent, message openai.Message) error
}

// StreamSummary captures metadata for a completed streaming response.
type StreamSummary struct {
	// Message is the completed assistant message.
	Message openai.Message
	// Usage reports token usage when available.
	Usage openai.Usage
	// HasUsage reports whether Usage was populated.
	HasUsage bool
	// FinishReason is the OpenAI finish reason.
	FinishReason string
	// Model is the model identifier for the call.
	Model string
}

// RunStream executes a single user turn using streaming responses.
func (r *Runner) RunStream(
	ctx context.Context,
	messages []openai.Message,
	systemPrompt string,
	model string,
	toolsEnabled bool,
	callbacks *StreamCallbacks,
) (*RunResult, error) {
	// Ensure a client is available for upstream calls.
	if r.Client == nil {
		return nil, fmt.Errorf("client is required")
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
			StreamOptions: &openai.StreamOptions{
				IncludeUsage: true,
			},
		}
		if toolsEnabled && r.ToolRunner != nil {
			req.Tools = r.ToolRunner.ToolSpecs()
			req.ToolChoice = "auto"
		}

		if callbacks != nil && callbacks.OnStreamStart != nil {
			if err := callbacks.OnStreamStart(model); err != nil {
				return nil, fmt.Errorf("stream start callback: %w", err)
			}
		}

		accumulator := openai.NewStreamAccumulator()
		callStart := time.Now()
		_, err := r.Client.ChatCompletionsStream(ctx, req, func(event openai.StreamResponse) error {
			if err := accumulator.Apply(event); err != nil {
				return fmt.Errorf("apply stream delta: %w", err)
			}
			if callbacks != nil && callbacks.OnStreamEvent != nil {
				if err := callbacks.OnStreamEvent(event); err != nil {
					return fmt.Errorf("stream event callback: %w", err)
				}
			}
			return nil
		})
		result.APIDuration += time.Since(callStart)
		if err != nil {
			return nil, fmt.Errorf("stream request: %w", err)
		}

		message := accumulator.Message()
		usage, hasUsage := accumulator.Usage()

		result.Usage = usage
		if hasUsage {
			accumulateUsage(&result.TotalUsage, usage)
			accumulateUsageMap(result.ModelUsage, model, usage)
		}
		result.Messages = append(result.Messages, message)
		result.Final = message
		result.CostUSD += estimateCost(model, usage, r.Pricing)
		result.NumTurns++
		if r.MaxBudgetUSD > 0 && result.CostUSD > r.MaxBudgetUSD {
			result.Duration = time.Since(startTime)
			return nil, fmt.Errorf("%w: %.4f > %.4f", ErrMaxBudget, result.CostUSD, r.MaxBudgetUSD)
		}

		if callbacks != nil && callbacks.OnStreamComplete != nil {
			if err := callbacks.OnStreamComplete(StreamSummary{
				Message:      message,
				Usage:        usage,
				HasUsage:     hasUsage,
				FinishReason: accumulator.FinishReason(),
				Model:        model,
			}); err != nil {
				return nil, fmt.Errorf("stream complete callback: %w", err)
			}
		}

		// If no tool calls are requested, return the assistant response.
		if len(message.ToolCalls) == 0 || !toolsEnabled || r.ToolRunner == nil {
			result.Duration = time.Since(startTime)
			return result, nil
		}

		for _, call := range message.ToolCalls {
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
					return nil, fmt.Errorf("authorize tool %s: %w", call.Function.Name, err)
				}
				if !allowed {
					return nil, fmt.Errorf("%w: %s", ErrToolDenied, call.Function.Name)
				}
			}

			toolResult, err := r.ToolRunner.Run(ctx, call.Function.Name, args, r.ToolContext)
			if err != nil {
				toolResult = tools.ToolResult{IsError: true, Content: err.Error()}
			}

			resultEvent := ToolEvent{
				Type:     "tool_result",
				ToolName: call.Function.Name,
				ToolID:   call.ID,
				Result:   toolResult.Content,
				IsError:  toolResult.IsError,
			}
			result.Events = append(result.Events, resultEvent)

			toolMessage := openai.Message{
				Role:       "tool",
				ToolCallID: call.ID,
				Content:    toolResult.Content,
			}
			result.Messages = append(result.Messages, toolMessage)
			if callbacks != nil && callbacks.OnToolResult != nil {
				if err := callbacks.OnToolResult(resultEvent, toolMessage); err != nil {
					return nil, fmt.Errorf("tool result callback: %w", err)
				}
			}
		}
	}

	result.Duration = time.Since(startTime)
	return result, ErrMaxTurns
}
