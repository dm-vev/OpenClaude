package openai

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
)

// ChatCompletionsStream executes a streaming chat/completions request.
func (c *Client) ChatCompletionsStream(ctx context.Context, req *ChatRequest, handler StreamHandler) (*StreamSummary, error) {
	if handler == nil {
		return nil, errors.New("stream handler is required")
	}
	if req == nil {
		return nil, errors.New("chat request is required")
	}

	req.Stream = true
	if req.StreamOptions == nil {
		req.StreamOptions = &StreamOptions{IncludeUsage: true}
	}

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

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, readErr := io.ReadAll(resp.Body)
		if readErr != nil {
			return nil, fmt.Errorf("read stream error body: %w", readErr)
		}
		return nil, &APIError{StatusCode: resp.StatusCode, Body: strings.TrimSpace(string(body))}
	}

	reader := bufio.NewReader(resp.Body)
	summary := &StreamSummary{}

	for {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		data, err := readSSEEvent(reader)
		if err != nil {
			if errors.Is(err, io.EOF) {
				return summary, nil
			}
			return nil, fmt.Errorf("read stream event: %w", err)
		}
		if data == "" {
			continue
		}
		if data == "[DONE]" {
			return summary, nil
		}
		var event StreamResponse
		if err := json.Unmarshal([]byte(data), &event); err != nil {
			return nil, fmt.Errorf("parse stream response: %w", err)
		}
		if summary.ID == "" && event.ID != "" {
			summary.ID = event.ID
		}
		if summary.Model == "" && event.Model != "" {
			summary.Model = event.Model
		}
		if event.Usage != nil {
			summary.Usage = *event.Usage
			summary.HasUsage = true
		}
		if err := handler(event); err != nil {
			return nil, err
		}
	}
}

// readSSEEvent reads a single SSE event payload.
func readSSEEvent(reader *bufio.Reader) (string, error) {
	var builder strings.Builder
	for {
		line, err := reader.ReadString('\n')
		if err != nil && !errors.Is(err, io.EOF) {
			return "", err
		}
		line = strings.TrimRight(line, "\r\n")
		if line == "" {
			if builder.Len() == 0 {
				if errors.Is(err, io.EOF) {
					return "", io.EOF
				}
				continue
			}
			return strings.TrimSuffix(builder.String(), "\n"), nil
		}
		if strings.HasPrefix(line, "data:") {
			payload := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
			builder.WriteString(payload)
			builder.WriteByte('\n')
		}
		if errors.Is(err, io.EOF) {
			if builder.Len() == 0 {
				return "", io.EOF
			}
			return strings.TrimSuffix(builder.String(), "\n"), nil
		}
	}
}
