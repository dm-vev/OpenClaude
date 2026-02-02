package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// defaultWebFetchTimeout bounds outbound HTTP requests for safety.
const defaultWebFetchTimeout = 10 * time.Second

// defaultWebFetchMaxBytes limits response bodies so tool output stays bounded.
const defaultWebFetchMaxBytes = 1024 * 1024

// WebFetchTool retrieves a URL over HTTP(S) and returns the response body.
type WebFetchTool struct{}

// Name returns the tool identifier used in tool calls.
func (t *WebFetchTool) Name() string {
	return "WebFetch"
}

// Description summarizes the fetch behavior for the model.
func (t *WebFetchTool) Description() string {
	return "Fetch the contents of a URL over HTTP(S)."
}

// Schema describes the supported WebFetch payload fields.
func (t *WebFetchTool) Schema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"url": map[string]any{
				"type":        "string",
				"description": "HTTP or HTTPS URL to fetch.",
			},
			"method": map[string]any{
				"type":        "string",
				"description": "HTTP method to use (only GET is supported).",
			},
			"headers": map[string]any{
				"type":        "object",
				"description": "Optional headers to include in the request.",
				"additionalProperties": map[string]any{
					"type": "string",
				},
			},
			"max_bytes": map[string]any{
				"type":        "integer",
				"description": "Maximum bytes to read from the response body.",
			},
			"timeout_ms": map[string]any{
				"type":        "integer",
				"description": "Request timeout in milliseconds.",
			},
		},
		"required": []string{"url"},
	}
}

// Run validates the payload, performs the request, and returns the response body.
func (t *WebFetchTool) Run(ctx context.Context, input json.RawMessage, toolCtx ToolContext) (ToolResult, error) {
	// The tool is synchronous, so the context is only used for cancellation.
	_ = toolCtx

	var payload struct {
		URL       string            `json:"url"`
		Method    string            `json:"method"`
		Headers   map[string]string `json:"headers"`
		MaxBytes  int64             `json:"max_bytes"`
		TimeoutMS int               `json:"timeout_ms"`
	}
	if err := json.Unmarshal(input, &payload); err != nil {
		return ToolResult{IsError: true, Content: fmt.Sprintf("invalid input: %v", err)}, nil
	}
	if strings.TrimSpace(payload.URL) == "" {
		return ToolResult{IsError: true, Content: "url is required"}, nil
	}

	parsed, err := url.Parse(payload.URL)
	if err != nil {
		return ToolResult{IsError: true, Content: fmt.Sprintf("invalid url: %v", err)}, nil
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return ToolResult{IsError: true, Content: "only http and https URLs are supported"}, nil
	}

	method := strings.ToUpper(strings.TrimSpace(payload.Method))
	if method == "" {
		method = http.MethodGet
	}
	if method != http.MethodGet {
		return ToolResult{IsError: true, Content: fmt.Sprintf("unsupported method: %s", method)}, nil
	}

	timeout := defaultWebFetchTimeout
	if payload.TimeoutMS > 0 {
		timeout = time.Duration(payload.TimeoutMS) * time.Millisecond
	}

	maxBytes := int64(defaultWebFetchMaxBytes)
	if payload.MaxBytes > 0 {
		maxBytes = payload.MaxBytes
	}

	req, err := http.NewRequestWithContext(ctx, method, parsed.String(), nil)
	if err != nil {
		return ToolResult{IsError: true, Content: fmt.Sprintf("build request: %v", err)}, nil
	}
	for key, value := range payload.Headers {
		if key == "" {
			continue
		}
		req.Header.Set(key, value)
	}

	client := &http.Client{Timeout: timeout}
	resp, err := client.Do(req)
	if err != nil {
		return ToolResult{IsError: true, Content: fmt.Sprintf("request failed: %v", err)}, nil
	}
	defer resp.Body.Close()

	body, truncated, readErr := readLimitedBody(resp.Body, maxBytes)
	if readErr != nil {
		return ToolResult{IsError: true, Content: fmt.Sprintf("read body: %v", readErr)}, nil
	}

	if resp.StatusCode >= http.StatusBadRequest {
		message := fmt.Sprintf("request failed: %s", resp.Status)
		if body != "" {
			message = fmt.Sprintf("%s\n%s", message, body)
		}
		return ToolResult{IsError: true, Content: message}, nil
	}

	if containsNullByte(body) {
		return ToolResult{IsError: true, Content: "binary response body is not supported"}, nil
	}
	if truncated {
		body += "\n...[truncated]"
	}

	return ToolResult{Content: body}, nil
}

// readLimitedBody reads up to maxBytes and reports whether truncation occurred.
func readLimitedBody(reader io.Reader, maxBytes int64) (string, bool, error) {
	if maxBytes <= 0 {
		return "", false, fmt.Errorf("max_bytes must be positive")
	}
	limited := io.LimitReader(reader, maxBytes+1)
	data, err := io.ReadAll(limited)
	if err != nil {
		return "", false, err
	}
	truncated := int64(len(data)) > maxBytes
	if truncated {
		data = data[:maxBytes]
	}
	return string(data), truncated, nil
}

// containsNullByte detects likely binary payloads by scanning for NULs.
func containsNullByte(payload string) bool {
	return strings.ContainsRune(payload, '\x00')
}
