package tools

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/openclaude/openclaude/internal/llm/openai"
)

// taskRecord captures task metadata persisted in the session store.
type taskRecord struct {
	// Type distinguishes task creation/output/stop entries.
	Type string `json:"type"`
	// ID identifies the task this entry belongs to.
	ID string `json:"id"`
	// Status summarizes task lifecycle changes, when relevant.
	Status string `json:"status,omitempty"`
	// Timestamp records when the entry was created.
	Timestamp string `json:"timestamp"`
	// Payload stores raw task inputs for later inspection.
	Payload map[string]any `json:"payload,omitempty"`
	// Output captures any task output content.
	Output string `json:"output,omitempty"`
}

// TaskTool records a task request in the session store.
type TaskTool struct{}

// Name returns the tool identifier used in tool calls.
func (t *TaskTool) Name() string {
	return "Task"
}

// Description summarizes the task recording behavior.
func (t *TaskTool) Description() string {
	return "Record a sub-task request for later inspection."
}

// Schema accepts arbitrary JSON so upstream payloads remain compatible.
func (t *TaskTool) Schema() map[string]any {
	return map[string]any{
		"type":                 "object",
		"additionalProperties": true,
	}
}

// Run stores a task entry and returns a task id.
func (t *TaskTool) Run(ctx context.Context, input json.RawMessage, toolCtx ToolContext) (ToolResult, error) {
	// The tool is synchronous, so the context is unused by design.
	_ = ctx

	payload := map[string]any{}
	if len(input) > 0 {
		if err := json.Unmarshal(input, &payload); err != nil {
			return ToolResult{IsError: true, Content: fmt.Sprintf("invalid input: %v", err)}, nil
		}
	}

	if toolCtx.TaskExecutor == nil {
		return ToolResult{IsError: true, Content: "task executor is not configured"}, nil
	}
	if toolCtx.TaskMaxDepth > 0 && toolCtx.TaskDepth >= toolCtx.TaskMaxDepth {
		return ToolResult{IsError: true, Content: "task nesting limit reached"}, nil
	}

	taskID := extractTaskID(payload)
	if taskID == "" {
		taskID = uuid.NewString()
	}
	payload["task_id"] = taskID

	record := taskRecord{
		Type:      "task",
		ID:        taskID,
		Status:    "created",
		Timestamp: time.Now().UTC().Format(time.RFC3339),
		Payload:   payload,
	}
	if err := appendTaskRecord(toolCtx, record); err != nil {
		return ToolResult{IsError: true, Content: fmt.Sprintf("persist task: %v", err)}, nil
	}

	request, err := buildTaskRequest(payload)
	if err != nil {
		return ToolResult{IsError: true, Content: err.Error()}, nil
	}

	if isAsyncTask(payload) {
		if toolCtx.TaskManager == nil {
			return ToolResult{IsError: true, Content: "task manager is not configured"}, nil
		}
		taskCtx, cancel := context.WithCancel(context.Background())
		toolCtx.TaskManager.Register(taskID, cancel)

		_ = appendTaskRecord(toolCtx, taskRecord{
			Type:      "output",
			ID:        taskID,
			Status:    "running",
			Timestamp: time.Now().UTC().Format(time.RFC3339),
			Payload:   payload,
		})

		go func() {
			defer toolCtx.TaskManager.Unregister(taskID)
			taskResult, err := toolCtx.TaskExecutor.ExecuteTask(taskCtx, request)
			status := "completed"
			output := taskResult.Output
			if err != nil {
				if errors.Is(err, context.Canceled) {
					status = "cancelled"
				} else {
					status = "failed"
				}
				output = err.Error()
			}
			_ = appendTaskRecord(toolCtx, taskRecord{
				Type:      "output",
				ID:        taskID,
				Status:    status,
				Timestamp: time.Now().UTC().Format(time.RFC3339),
				Payload:   payload,
				Output:    output,
			})
		}()

		response := map[string]any{
			"id":     taskID,
			"status": "running",
		}
		encoded, _ := json.Marshal(response)
		return ToolResult{Content: string(encoded)}, nil
	}

	taskResult, err := toolCtx.TaskExecutor.ExecuteTask(ctx, request)
	if err != nil {
		_ = appendTaskRecord(toolCtx, taskRecord{
			Type:      "output",
			ID:        taskID,
			Status:    "failed",
			Timestamp: time.Now().UTC().Format(time.RFC3339),
			Payload:   payload,
			Output:    err.Error(),
		})
		return ToolResult{IsError: true, Content: err.Error()}, nil
	}

	_ = appendTaskRecord(toolCtx, taskRecord{
		Type:      "output",
		ID:        taskID,
		Status:    "completed",
		Timestamp: time.Now().UTC().Format(time.RFC3339),
		Payload:   payload,
		Output:    taskResult.Output,
	})

	response := map[string]any{
		"id":     taskID,
		"status": "completed",
		"result": taskResult.Output,
	}
	for key, value := range taskResult.Metadata {
		if _, exists := response[key]; exists {
			continue
		}
		response[key] = value
	}
	encoded, _ := json.Marshal(response)
	return ToolResult{Content: string(encoded)}, nil
}

// TaskOutputTool appends output metadata for a task.
type TaskOutputTool struct{}

// Name returns the tool identifier used in tool calls.
func (t *TaskOutputTool) Name() string {
	return "TaskOutput"
}

// Description summarizes the task output recording behavior.
func (t *TaskOutputTool) Description() string {
	return "Record output for a previously created task."
}

// Schema accepts arbitrary JSON so upstream payloads remain compatible.
func (t *TaskOutputTool) Schema() map[string]any {
	return map[string]any{
		"type":                 "object",
		"additionalProperties": true,
	}
}

// Run stores a task output entry in the session store.
func (t *TaskOutputTool) Run(ctx context.Context, input json.RawMessage, toolCtx ToolContext) (ToolResult, error) {
	// The tool is synchronous, so the context is unused by design.
	_ = ctx

	payload := map[string]any{}
	if len(input) > 0 {
		if err := json.Unmarshal(input, &payload); err != nil {
			return ToolResult{IsError: true, Content: fmt.Sprintf("invalid input: %v", err)}, nil
		}
	}

	taskID := extractTaskID(payload)
	if taskID == "" {
		return ToolResult{IsError: true, Content: "task_id is required"}, nil
	}
	output, hasOutput := payload["output"].(string)
	if !hasOutput {
		latest, err := loadLatestTaskOutput(toolCtx, taskID)
		if err != nil {
			return ToolResult{IsError: true, Content: err.Error()}, nil
		}
		return ToolResult{Content: latest}, nil
	}

	record := taskRecord{
		Type:      "output",
		ID:        taskID,
		Timestamp: time.Now().UTC().Format(time.RFC3339),
		Payload:   payload,
		Output:    output,
	}
	if err := appendTaskRecord(toolCtx, record); err != nil {
		return ToolResult{IsError: true, Content: fmt.Sprintf("persist task output: %v", err)}, nil
	}
	return ToolResult{Content: "ok"}, nil
}

// TaskStopTool records a task stop request.
type TaskStopTool struct{}

// Name returns the tool identifier used in tool calls.
func (t *TaskStopTool) Name() string {
	return "TaskStop"
}

// Description summarizes the task stop recording behavior.
func (t *TaskStopTool) Description() string {
	return "Record a request to stop a task."
}

// Schema accepts arbitrary JSON so upstream payloads remain compatible.
func (t *TaskStopTool) Schema() map[string]any {
	return map[string]any{
		"type":                 "object",
		"additionalProperties": true,
	}
}

// Run stores a task stop entry in the session store.
func (t *TaskStopTool) Run(ctx context.Context, input json.RawMessage, toolCtx ToolContext) (ToolResult, error) {
	// The tool is synchronous, so the context is unused by design.
	_ = ctx

	payload := map[string]any{}
	if len(input) > 0 {
		if err := json.Unmarshal(input, &payload); err != nil {
			return ToolResult{IsError: true, Content: fmt.Sprintf("invalid input: %v", err)}, nil
		}
	}

	taskID := extractTaskID(payload)
	if taskID == "" {
		return ToolResult{IsError: true, Content: "task_id is required"}, nil
	}

	cancelled := false
	if toolCtx.TaskManager != nil {
		cancelled = toolCtx.TaskManager.Cancel(taskID)
	}

	record := taskRecord{
		Type:      "stop",
		ID:        taskID,
		Status:    "stopped",
		Timestamp: time.Now().UTC().Format(time.RFC3339),
		Payload:   payload,
	}
	if err := appendTaskRecord(toolCtx, record); err != nil {
		return ToolResult{IsError: true, Content: fmt.Sprintf("persist task stop: %v", err)}, nil
	}
	if cancelled {
		return ToolResult{Content: "cancelled"}, nil
	}
	return ToolResult{Content: "ok"}, nil
}

// extractTaskID pulls a task identifier from common payload keys.
func extractTaskID(payload map[string]any) string {
	if payload == nil {
		return ""
	}
	if value, ok := payload["task_id"].(string); ok {
		return value
	}
	if value, ok := payload["id"].(string); ok {
		return value
	}
	return ""
}

// buildTaskRequest converts a raw task payload into a TaskRequest.
func buildTaskRequest(payload map[string]any) (TaskRequest, error) {
	request := TaskRequest{
		Metadata: payload,
	}
	if payload == nil {
		return request, fmt.Errorf("task payload is required")
	}

	if value, ok := payload["prompt"].(string); ok {
		request.Prompt = strings.TrimSpace(value)
	}
	if request.Prompt == "" {
		if value, ok := payload["task"].(string); ok {
			request.Prompt = strings.TrimSpace(value)
		}
	}
	if request.Prompt == "" {
		if value, ok := payload["title"].(string); ok {
			request.Prompt = strings.TrimSpace(value)
		}
	}
	if request.Prompt == "" {
		if value, ok := payload["description"].(string); ok {
			request.Prompt = strings.TrimSpace(value)
		}
	}
	if request.Prompt == "" {
		if value, ok := payload["instructions"].(string); ok {
			request.Prompt = strings.TrimSpace(value)
		}
	}
	if request.Prompt == "" {
		if value, ok := payload["input"].(string); ok {
			request.Prompt = strings.TrimSpace(value)
		}
	}
	if request.Prompt == "" {
		if value, ok := payload["message"].(string); ok {
			request.Prompt = strings.TrimSpace(value)
		}
	}

	if value, ok := payload["system_prompt"].(string); ok {
		request.SystemPrompt = strings.TrimSpace(value)
	}
	if request.SystemPrompt == "" {
		if value, ok := payload["systemPrompt"].(string); ok {
			request.SystemPrompt = strings.TrimSpace(value)
		}
	}
	if value, ok := payload["model"].(string); ok {
		request.Model = strings.TrimSpace(value)
	}
	if value, ok := payload["max_turns"]; ok {
		if num, ok := value.(float64); ok {
			request.MaxTurns = int(num)
		}
	}

	if rawMessages, ok := payload["messages"]; ok {
		encoded, err := json.Marshal(rawMessages)
		if err == nil {
			var messages []openai.Message
			if err := json.Unmarshal(encoded, &messages); err == nil {
				request.Messages = messages
			}
		}
	}

	if len(request.Messages) == 0 && request.Prompt == "" {
		return request, fmt.Errorf("task prompt is required")
	}
	return request, nil
}

// isAsyncTask reports whether the payload requests background execution.
func isAsyncTask(payload map[string]any) bool {
	for _, key := range []string{"async", "background", "detached", "run_in_background"} {
		if value, ok := payload[key]; ok {
			if enabled, ok := value.(bool); ok {
				return enabled
			}
		}
	}
	return false
}

// loadLatestTaskOutput returns the last recorded output for a task.
func loadLatestTaskOutput(toolCtx ToolContext, taskID string) (string, error) {
	if taskID == "" {
		return "", fmt.Errorf("task_id is required")
	}
	path := taskLogPath(toolCtx)
	if path == "" {
		return "", fmt.Errorf("task store unavailable")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("read task log: %v", err)
	}

	var latest string
	lines := strings.Split(string(data), "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		var record taskRecord
		if err := json.Unmarshal([]byte(line), &record); err != nil {
			continue
		}
		if record.ID != taskID || record.Output == "" {
			continue
		}
		latest = record.Output
	}
	if latest == "" {
		return "", fmt.Errorf("task output not found")
	}
	return latest, nil
}

// taskLogPath returns the tasks.jsonl path.
func taskLogPath(toolCtx ToolContext) string {
	if toolCtx.Store == nil || toolCtx.SessionID == "" {
		return ""
	}
	return filepath.Join(toolCtx.Store.BaseDir, "session-env", toolCtx.SessionID, "tasks.jsonl")
}

// appendTaskRecord appends a task record to the session store when available.
func appendTaskRecord(toolCtx ToolContext, record taskRecord) error {
	if toolCtx.Store == nil || toolCtx.SessionID == "" {
		return nil
	}

	path := taskLogPath(toolCtx)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	file, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	defer file.Close()

	data, err := json.Marshal(record)
	if err != nil {
		return err
	}
	if _, err := file.Write(append(data, '\n')); err != nil {
		return err
	}
	return nil
}
