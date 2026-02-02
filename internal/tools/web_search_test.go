package tools

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
)

// TestWebSearchToolJSON verifies JSON result parsing via an override URL.
func TestWebSearchToolJSON(testingHandle *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		_, _ = writer.Write([]byte(`{"results":[{"title":"Doc","url":"https://example.com","snippet":"Example"}]}`))
	}))
	defer server.Close()

	previous := os.Getenv("OPENCLOUDE_WEBSEARCH_URL")
	if err := os.Setenv("OPENCLOUDE_WEBSEARCH_URL", server.URL); err != nil {
		testingHandle.Fatalf("set env: %v", err)
	}
	defer os.Setenv("OPENCLOUDE_WEBSEARCH_URL", previous)

	tool := &WebSearchTool{}
	payload, err := json.Marshal(map[string]any{
		"query": "test",
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

	var parsed struct {
		Query   string            `json:"query"`
		Results []webSearchResult `json:"results"`
	}
	if err := json.Unmarshal([]byte(result.Content), &parsed); err != nil {
		testingHandle.Fatalf("parse result: %v", err)
	}
	if parsed.Query != "test" || len(parsed.Results) != 1 {
		testingHandle.Fatalf("unexpected search output: %+v", parsed)
	}
	if parsed.Results[0].URL != "https://example.com" {
		testingHandle.Fatalf("unexpected result URL: %s", parsed.Results[0].URL)
	}
}
