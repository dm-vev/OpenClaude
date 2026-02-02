package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"html"
	"io"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"strings"
	"time"
)

// defaultWebSearchTimeout bounds outbound search requests.
const defaultWebSearchTimeout = 10 * time.Second

// defaultWebSearchURL is the fallback search endpoint when not overridden.
const defaultWebSearchURL = "https://duckduckgo.com/html/"

// WebSearchTool performs a simple web search and returns summarized results.
type WebSearchTool struct{}

// Name returns the tool identifier used in tool calls.
func (t *WebSearchTool) Name() string {
	return "WebSearch"
}

// Description summarizes the search behavior for the model.
func (t *WebSearchTool) Description() string {
	return "Search the web for relevant documents."
}

// Schema describes the supported WebSearch payload fields.
func (t *WebSearchTool) Schema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"query": map[string]any{
				"type":        "string",
				"description": "Search query to execute.",
			},
			"num_results": map[string]any{
				"type":        "integer",
				"description": "Maximum number of results to return.",
			},
		},
		"required": []string{"query"},
	}
}

// Run executes the search and returns a JSON summary of results.
func (t *WebSearchTool) Run(ctx context.Context, input json.RawMessage, toolCtx ToolContext) (ToolResult, error) {
	// The tool is synchronous, so the context is only used for cancellation.
	_ = toolCtx

	var payload struct {
		Query      string `json:"query"`
		NumResults int    `json:"num_results"`
	}
	if err := json.Unmarshal(input, &payload); err != nil {
		return ToolResult{IsError: true, Content: fmt.Sprintf("invalid input: %v", err)}, nil
	}
	query := strings.TrimSpace(payload.Query)
	if query == "" {
		return ToolResult{IsError: true, Content: "query is required"}, nil
	}
	limit := payload.NumResults
	if limit <= 0 {
		limit = 5
	}

	baseURL := strings.TrimSpace(os.Getenv("OPENCLOUDE_WEBSEARCH_URL"))
	if baseURL == "" {
		baseURL = defaultWebSearchURL
	}
	searchURL, err := buildSearchURL(baseURL, query)
	if err != nil {
		return ToolResult{IsError: true, Content: err.Error()}, nil
	}

	client := &http.Client{Timeout: defaultWebSearchTimeout}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, searchURL, nil)
	if err != nil {
		return ToolResult{IsError: true, Content: fmt.Sprintf("build request: %v", err)}, nil
	}
	resp, err := client.Do(req)
	if err != nil {
		return ToolResult{IsError: true, Content: fmt.Sprintf("request failed: %v", err)}, nil
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return ToolResult{IsError: true, Content: fmt.Sprintf("read response: %v", err)}, nil
	}
	if resp.StatusCode >= http.StatusBadRequest {
		return ToolResult{IsError: true, Content: fmt.Sprintf("search failed: %s", resp.Status)}, nil
	}

	results, err := parseSearchResults(body, resp.Header.Get("Content-Type"))
	if err != nil {
		return ToolResult{IsError: true, Content: err.Error()}, nil
	}
	if len(results) > limit {
		results = results[:limit]
	}

	payloadOut := webSearchResponse{
		Query:   query,
		Results: results,
	}
	out, err := json.Marshal(payloadOut)
	if err != nil {
		return ToolResult{IsError: true, Content: fmt.Sprintf("encode results: %v", err)}, nil
	}
	return ToolResult{Content: string(out)}, nil
}

// webSearchResponse captures the serialized search response.
type webSearchResponse struct {
	Query   string            `json:"query"`
	Results []webSearchResult `json:"results"`
}

// webSearchResult captures a single search result entry.
type webSearchResult struct {
	Title   string `json:"title"`
	URL     string `json:"url"`
	Snippet string `json:"snippet,omitempty"`
}

// buildSearchURL injects the query parameter into a base URL.
func buildSearchURL(baseURL string, query string) (string, error) {
	parsed, err := url.Parse(baseURL)
	if err != nil {
		return "", fmt.Errorf("invalid search base URL: %v", err)
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return "", fmt.Errorf("search base URL must be http or https")
	}
	values := parsed.Query()
	values.Set("q", query)
	parsed.RawQuery = values.Encode()
	return parsed.String(), nil
}

// parseSearchResults extracts results from JSON or HTML payloads.
func parseSearchResults(body []byte, contentType string) ([]webSearchResult, error) {
	if strings.Contains(contentType, "application/json") {
		return parseSearchJSON(body)
	}
	if results := parseSearchHTML(string(body)); len(results) > 0 {
		return results, nil
	}
	if results, err := parseSearchJSON(body); err == nil && len(results) > 0 {
		return results, nil
	}
	return nil, fmt.Errorf("no search results found")
}

// parseSearchJSON reads a JSON response with a results array.
func parseSearchJSON(body []byte) ([]webSearchResult, error) {
	var payload struct {
		Results []webSearchResult `json:"results"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, err
	}
	return payload.Results, nil
}

// parseSearchHTML extracts results from a simple HTML page.
func parseSearchHTML(body string) []webSearchResult {
	linkPattern := regexp.MustCompile(`<a[^>]*class=\"result__a\"[^>]*href=\"([^\"]+)\"[^>]*>(.*?)</a>`)
	snippetPattern := regexp.MustCompile(`<a[^>]*class=\"result__snippet\"[^>]*>(.*?)</a>`)

	matches := linkPattern.FindAllStringSubmatch(body, -1)
	snippets := snippetPattern.FindAllStringSubmatch(body, -1)

	results := make([]webSearchResult, 0, len(matches))
	for index, match := range matches {
		title := stripHTML(match[2])
		entry := webSearchResult{
			Title: title,
			URL:   html.UnescapeString(match[1]),
		}
		if index < len(snippets) {
			entry.Snippet = stripHTML(snippets[index][1])
		}
		results = append(results, entry)
	}
	return results
}

// stripHTML removes HTML tags and unescapes entities.
func stripHTML(fragment string) string {
	tagPattern := regexp.MustCompile(`<[^>]+>`)
	plain := tagPattern.ReplaceAllString(fragment, "")
	return strings.TrimSpace(html.UnescapeString(plain))
}
