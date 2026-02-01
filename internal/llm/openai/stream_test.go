package openai

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/openclaude/openclaude/internal/testutil"
)

// TestChatCompletionsStreamParsesEvents verifies SSE parsing and summary data.
func TestChatCompletionsStreamParsesEvents(testingHandle *testing.T) {
	// Arrange a deterministic SSE server response.
	server := httptest.NewServer(http.HandlerFunc(func(responseWriter http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/chat/completions" {
			http.NotFound(responseWriter, request)
			return
		}
		responseWriter.Header().Set("Content-Type", "text/event-stream")

		flusher, ok := responseWriter.(http.Flusher)
		if !ok {
			http.Error(responseWriter, "streaming unsupported", http.StatusInternalServerError)
			return
		}

		events := []string{
			`{"id":"req-1","model":"model-x","choices":[{"index":0,"delta":{"role":"assistant"}}]}`,
			`{"choices":[{"index":0,"delta":{"content":"Hello "}}]}`,
			`{"choices":[{"index":0,"delta":{"content":"world"}}]}`,
			`{"choices":[{"index":0,"delta":{},"finish_reason":"stop"}],"usage":{"prompt_tokens":2,"completion_tokens":2,"total_tokens":4}}`,
		}

		for _, payload := range events {
			_, _ = fmt.Fprintf(responseWriter, "data: %s\n\n", payload)
			flusher.Flush()
		}
		_, _ = fmt.Fprint(responseWriter, "data: [DONE]\n\n")
		flusher.Flush()
	}))
	defer server.Close()

	client := NewClient(server.URL, "", 5*time.Second)
	request := &ChatRequest{
		Model: "model-x",
		Messages: []Message{
			{Role: "user", Content: "hello"},
		},
	}

	var collected []StreamResponse
	handler := func(event StreamResponse) error {
		collected = append(collected, event)
		return nil
	}

	summary, err := client.ChatCompletionsStream(context.Background(), request, handler)
	testutil.RequireNoError(testingHandle, err, "stream request")
	testutil.RequireTrue(testingHandle, summary != nil, "expected stream summary")
	testutil.RequireEqual(testingHandle, summary.ID, "req-1", "stream id mismatch")
	testutil.RequireEqual(testingHandle, summary.Model, "model-x", "stream model mismatch")
	testutil.RequireTrue(testingHandle, summary.HasUsage, "expected usage in summary")
	testutil.RequireEqual(testingHandle, summary.Usage.TotalTokens, 4, "usage mismatch")

	collectedPayloads := make([]string, 0, len(collected))
	for _, event := range collected {
		if event.ID != "" {
			collectedPayloads = append(collectedPayloads, event.ID)
		}
	}
	joined := strings.Join(collectedPayloads, ",")
	testutil.RequireStringContains(testingHandle, joined, "req-1", "expected event id in stream")
}
