package tools

import (
	"bufio"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/openclaude/openclaude/internal/session"
)

// TestTaskToolPersistsRecord verifies task creation and output records are persisted.
func TestTaskToolPersistsRecord(testingHandle *testing.T) {
	store := &session.Store{BaseDir: testingHandle.TempDir()}
	toolCtx := ToolContext{
		Store:        store,
		SessionID:    "session-1",
		TaskMaxDepth: 2,
		TaskExecutor: TaskExecutorFunc(func(ctx context.Context, request TaskRequest) (TaskResult, error) {
			_ = ctx
			_ = request
			return TaskResult{Output: "done"}, nil
		}),
	}

	tool := &TaskTool{}
	payload, err := json.Marshal(map[string]any{
		"title": "demo",
	})
	if err != nil {
		testingHandle.Fatalf("marshal payload: %v", err)
	}

	result, runErr := tool.Run(context.Background(), payload, toolCtx)
	if runErr != nil {
		testingHandle.Fatalf("run tool: %v", runErr)
	}
	if result.IsError {
		testingHandle.Fatalf("unexpected error: %s", result.Content)
	}

	var response map[string]any
	if err := json.Unmarshal([]byte(result.Content), &response); err != nil {
		testingHandle.Fatalf("parse response: %v", err)
	}
	if response["status"] != "completed" {
		testingHandle.Fatalf("unexpected response: %v", response)
	}
	taskID, _ := response["id"].(string)
	if taskID == "" {
		testingHandle.Fatalf("expected task id in response")
	}

	records := loadTaskRecords(testingHandle, store, toolCtx.SessionID)
	if !hasTaskRecord(records, "task", taskID, "created") {
		testingHandle.Fatalf("expected task created record for %s", taskID)
	}
	if !hasTaskRecordWithOutput(records, "output", taskID, "completed", "done") {
		testingHandle.Fatalf("expected task output record for %s", taskID)
	}
}

// TestTaskToolAsyncCompletes verifies async tasks report running then complete.
func TestTaskToolAsyncCompletes(testingHandle *testing.T) {
	store := &session.Store{BaseDir: testingHandle.TempDir()}
	toolCtx := ToolContext{
		Store:        store,
		SessionID:    "session-async",
		TaskMaxDepth: 2,
		TaskManager:  NewTaskManager(),
		TaskExecutor: TaskExecutorFunc(func(ctx context.Context, request TaskRequest) (TaskResult, error) {
			_ = ctx
			_ = request
			return TaskResult{Output: "async-done"}, nil
		}),
	}

	tool := &TaskTool{}
	payload, err := json.Marshal(map[string]any{
		"title":  "async-demo",
		"async":  true,
		"input":  "go",
		"model":  "model-x",
		"prompt": "ignored",
	})
	if err != nil {
		testingHandle.Fatalf("marshal payload: %v", err)
	}

	result, runErr := tool.Run(context.Background(), payload, toolCtx)
	if runErr != nil {
		testingHandle.Fatalf("run tool: %v", runErr)
	}
	if result.IsError {
		testingHandle.Fatalf("unexpected error: %s", result.Content)
	}

	var response map[string]any
	if err := json.Unmarshal([]byte(result.Content), &response); err != nil {
		testingHandle.Fatalf("parse response: %v", err)
	}
	if response["status"] != "running" {
		testingHandle.Fatalf("expected running status, got %v", response["status"])
	}
	taskID, _ := response["id"].(string)
	if taskID == "" {
		testingHandle.Fatalf("expected task id in response")
	}

	waitForTaskRecord(testingHandle, store, toolCtx.SessionID, func(record taskRecord) bool {
		return record.Type == "output" && record.ID == taskID && record.Status == "completed"
	})
}

// TestTaskOutputReturnsLatest verifies TaskOutput can read the latest task output.
func TestTaskOutputReturnsLatest(testingHandle *testing.T) {
	store := &session.Store{BaseDir: testingHandle.TempDir()}
	toolCtx := ToolContext{
		Store:        store,
		SessionID:    "session-output",
		TaskMaxDepth: 2,
		TaskExecutor: TaskExecutorFunc(func(ctx context.Context, request TaskRequest) (TaskResult, error) {
			_ = ctx
			_ = request
			return TaskResult{Output: "final-output"}, nil
		}),
	}

	taskTool := &TaskTool{}
	payload, err := json.Marshal(map[string]any{
		"title": "demo",
	})
	if err != nil {
		testingHandle.Fatalf("marshal payload: %v", err)
	}
	result, runErr := taskTool.Run(context.Background(), payload, toolCtx)
	if runErr != nil {
		testingHandle.Fatalf("run task tool: %v", runErr)
	}
	if result.IsError {
		testingHandle.Fatalf("unexpected error: %s", result.Content)
	}

	var response map[string]any
	if err := json.Unmarshal([]byte(result.Content), &response); err != nil {
		testingHandle.Fatalf("parse response: %v", err)
	}
	taskID, _ := response["id"].(string)
	if taskID == "" {
		testingHandle.Fatalf("expected task id in response")
	}

	outputTool := &TaskOutputTool{}
	outputPayload, err := json.Marshal(map[string]any{
		"task_id": taskID,
	})
	if err != nil {
		testingHandle.Fatalf("marshal payload: %v", err)
	}
	outputResult, runErr := outputTool.Run(context.Background(), outputPayload, toolCtx)
	if runErr != nil {
		testingHandle.Fatalf("run output tool: %v", runErr)
	}
	if outputResult.IsError {
		testingHandle.Fatalf("unexpected output error: %s", outputResult.Content)
	}
	if outputResult.Content != "final-output" {
		testingHandle.Fatalf("expected latest output, got %s", outputResult.Content)
	}
}

// TestTaskOutputRequiresID verifies task output requires a task id.
func TestTaskOutputRequiresID(testingHandle *testing.T) {
	tool := &TaskOutputTool{}
	payload, err := json.Marshal(map[string]any{
		"output": "hello",
	})
	if err != nil {
		testingHandle.Fatalf("marshal payload: %v", err)
	}

	result, runErr := tool.Run(context.Background(), payload, ToolContext{})
	if runErr != nil {
		testingHandle.Fatalf("run tool: %v", runErr)
	}
	if !result.IsError {
		testingHandle.Fatalf("expected error for missing task_id")
	}
}

// TestTaskStopCancels verifies TaskStop cancels running async tasks.
func TestTaskStopCancels(testingHandle *testing.T) {
	store := &session.Store{BaseDir: testingHandle.TempDir()}
	manager := NewTaskManager()
	toolCtx := ToolContext{
		Store:        store,
		SessionID:    "session-stop",
		TaskMaxDepth: 2,
		TaskManager:  manager,
		TaskExecutor: TaskExecutorFunc(func(ctx context.Context, request TaskRequest) (TaskResult, error) {
			_ = request
			<-ctx.Done()
			return TaskResult{}, ctx.Err()
		}),
	}

	taskTool := &TaskTool{}
	payload, err := json.Marshal(map[string]any{
		"title": "demo",
		"async": true,
	})
	if err != nil {
		testingHandle.Fatalf("marshal payload: %v", err)
	}
	result, runErr := taskTool.Run(context.Background(), payload, toolCtx)
	if runErr != nil {
		testingHandle.Fatalf("run task tool: %v", runErr)
	}
	if result.IsError {
		testingHandle.Fatalf("unexpected error: %s", result.Content)
	}

	var response map[string]any
	if err := json.Unmarshal([]byte(result.Content), &response); err != nil {
		testingHandle.Fatalf("parse response: %v", err)
	}
	taskID, _ := response["id"].(string)
	if taskID == "" {
		testingHandle.Fatalf("expected task id in response")
	}

	stopTool := &TaskStopTool{}
	stopPayload, err := json.Marshal(map[string]any{
		"task_id": taskID,
	})
	if err != nil {
		testingHandle.Fatalf("marshal stop payload: %v", err)
	}
	stopResult, runErr := stopTool.Run(context.Background(), stopPayload, toolCtx)
	if runErr != nil {
		testingHandle.Fatalf("run stop tool: %v", runErr)
	}
	if stopResult.IsError {
		testingHandle.Fatalf("unexpected stop error: %s", stopResult.Content)
	}
	if stopResult.Content != "cancelled" {
		testingHandle.Fatalf("expected cancelled stop, got %s", stopResult.Content)
	}

	waitForTaskRecord(testingHandle, store, toolCtx.SessionID, func(record taskRecord) bool {
		return record.Type == "output" && record.ID == taskID && record.Status == "cancelled"
	})
}

// TestTaskStopRequiresID verifies task stop requires a task id.
func TestTaskStopRequiresID(testingHandle *testing.T) {
	tool := &TaskStopTool{}
	payload, err := json.Marshal(map[string]any{})
	if err != nil {
		testingHandle.Fatalf("marshal payload: %v", err)
	}

	result, runErr := tool.Run(context.Background(), payload, ToolContext{})
	if runErr != nil {
		testingHandle.Fatalf("run tool: %v", runErr)
	}
	if !result.IsError {
		testingHandle.Fatalf("expected error for missing task_id")
	}
}

// loadTaskRecords reads task records from the session store for assertions.
func loadTaskRecords(testingHandle *testing.T, store *session.Store, sessionID string) []taskRecord {
	testingHandle.Helper()

	path := filepath.Join(store.BaseDir, "session-env", sessionID, "tasks.jsonl")
	file, err := os.Open(path)
	if err != nil {
		testingHandle.Fatalf("open tasks file: %v", err)
	}
	defer file.Close()

	var records []taskRecord
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		var record taskRecord
		if err := json.Unmarshal([]byte(line), &record); err != nil {
			testingHandle.Fatalf("parse record: %v", err)
		}
		records = append(records, record)
	}
	if err := scanner.Err(); err != nil {
		testingHandle.Fatalf("scan records: %v", err)
	}
	return records
}

// hasTaskRecord reports whether a task record exists for the given fields.
func hasTaskRecord(records []taskRecord, typ string, taskID string, status string) bool {
	for _, record := range records {
		if record.Type == typ && record.ID == taskID && record.Status == status {
			return true
		}
	}
	return false
}

// hasTaskRecordWithOutput reports whether a task record matches output text.
func hasTaskRecordWithOutput(records []taskRecord, typ string, taskID string, status string, output string) bool {
	for _, record := range records {
		if record.Type == typ && record.ID == taskID && record.Status == status && record.Output == output {
			return true
		}
	}
	return false
}

// waitForTaskRecord polls the task log until a matching record is present.
func waitForTaskRecord(testingHandle *testing.T, store *session.Store, sessionID string, match func(taskRecord) bool) {
	testingHandle.Helper()

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		records := loadTaskRecords(testingHandle, store, sessionID)
		for _, record := range records {
			if match(record) {
				return
			}
		}
		time.Sleep(10 * time.Millisecond)
	}
	testingHandle.Fatalf("timed out waiting for task record")
}
