package tools

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

// TestWebFetchToolSuccess verifies successful fetch output.
func TestWebFetchToolSuccess(testingHandle *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "text/plain")
		_, _ = writer.Write([]byte("hello"))
	}))
	defer server.Close()

	tool := &WebFetchTool{}
	payload, err := json.Marshal(map[string]any{
		"url": server.URL,
	})
	if err != nil {
		testingHandle.Fatalf("marshal payload: %v", err)
	}

	result, runErr := tool.Run(context.Background(), payload, ToolContext{})
	if runErr != nil {
		testingHandle.Fatalf("run tool: %v", runErr)
	}
	if result.IsError {
		testingHandle.Fatalf("unexpected error: %s", result.Content)
	}
	if result.Content != "hello" {
		testingHandle.Fatalf("unexpected content: %s", result.Content)
	}
}

// TestWebFetchToolRejectsScheme ensures non-http schemes are rejected.
func TestWebFetchToolRejectsScheme(testingHandle *testing.T) {
	tool := &WebFetchTool{}
	payload, err := json.Marshal(map[string]any{
		"url": "file:///etc/passwd",
	})
	if err != nil {
		testingHandle.Fatalf("marshal payload: %v", err)
	}

	result, runErr := tool.Run(context.Background(), payload, ToolContext{})
	if runErr != nil {
		testingHandle.Fatalf("run tool: %v", runErr)
	}
	if !result.IsError {
		testingHandle.Fatalf("expected error for non-http scheme")
	}
}

// TestWebFetchToolTruncates verifies max_bytes truncation behavior.
func TestWebFetchToolTruncates(testingHandle *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "text/plain")
		_, _ = writer.Write([]byte("hello world"))
	}))
	defer server.Close()

	tool := &WebFetchTool{}
	payload, err := json.Marshal(map[string]any{
		"url":       server.URL,
		"max_bytes": 5,
	})
	if err != nil {
		testingHandle.Fatalf("marshal payload: %v", err)
	}

	result, runErr := tool.Run(context.Background(), payload, ToolContext{})
	if runErr != nil {
		testingHandle.Fatalf("run tool: %v", runErr)
	}
	if result.IsError {
		testingHandle.Fatalf("unexpected error: %s", result.Content)
	}
	if result.Content != "hello\n...[truncated]" {
		testingHandle.Fatalf("unexpected truncated output: %s", result.Content)
	}
}
