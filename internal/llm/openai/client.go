package openai

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// APIError represents an HTTP error from the OpenAI-compatible gateway.
type APIError struct {
	StatusCode int
	Body       string
}

func (e *APIError) Error() string {
	return fmt.Sprintf("openai api error: status %d: %s", e.StatusCode, e.Body)
}

// Client talks to an OpenAI-compatible chat/completions endpoint.
type Client struct {
	// baseURL points to the OpenAI-compatible gateway.
	baseURL string
	// apiKey is sent as a bearer token, if provided.
	apiKey string
	// httpClient executes requests with timeouts.
	httpClient *http.Client
}

// NewClient constructs a new client with timeout settings.
func NewClient(baseURL string, apiKey string, timeout time.Duration) *Client {
	return &Client{
		baseURL: strings.TrimRight(baseURL, "/"),
		apiKey:  apiKey,
		httpClient: &http.Client{
			Timeout: timeout,
		},
	}
}

// ChatCompletions executes a non-streaming chat/completions request.
func (c *Client) ChatCompletions(ctx context.Context, req *ChatRequest) (*ChatResponse, error) {
	// Marshal request payload once for consistent retries.
	payload, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("marshal chat request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(
		ctx,
		http.MethodPost,
		c.completionsURL(),
		bytes.NewReader(payload),
	)
	if err != nil {
		return nil, fmt.Errorf("create chat request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	if c.apiKey != "" {
		httpReq.Header.Set("Authorization", "Bearer "+c.apiKey)
	}

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("send chat request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read chat response: %w", err)
	}

	// Non-2xx responses return a structured API error for fallback logic.
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, &APIError{StatusCode: resp.StatusCode, Body: strings.TrimSpace(string(body))}
	}

	var parsed ChatResponse
	if err := json.Unmarshal(body, &parsed); err != nil {
		return nil, fmt.Errorf("parse chat response: %w", err)
	}
	if len(parsed.Choices) == 0 {
		return nil, errors.New("empty response choices")
	}
	return &parsed, nil
}

// completionsURL normalizes the base URL to a chat/completions endpoint.
func (c *Client) completionsURL() string {
	if strings.HasSuffix(c.baseURL, "/chat/completions") {
		return c.baseURL
	}
	return c.baseURL + "/chat/completions"
}
