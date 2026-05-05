package copilot

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// SearchFilters holds the structured flow-filter parameters extracted from a
// natural-language query.  All fields are optional; unset fields are omitted.
type SearchFilters struct {
	Protocol string `json:"protocol,omitempty"`
	SrcIP    string `json:"src_ip,omitempty"`
	DstIP    string `json:"dst_ip,omitempty"`
	Hostname string `json:"hostname,omitempty"`
	From     string `json:"from,omitempty"` // RFC3339
	To       string `json:"to,omitempty"`   // RFC3339
}

// SearchResult is the JSON response from POST /api/v1/copilot/search.
type SearchResult struct {
	Filters     SearchFilters `json:"filters"`
	Explanation string        `json:"explanation"`
}

const searchSystemPromptTpl = `You are a network-observability assistant for NetScope.

The user describes flows they want to find in plain English.
Extract structured filter parameters and return them as JSON.

Available filter fields (all optional — omit any that don't apply):
  protocol  one of: HTTP, HTTPS, HTTP2, GRPC, DNS, TLS, TCP, UDP, ICMP, ARP
  src_ip    exact source IP address
  dst_ip    exact destination IP address
  hostname  agent hostname (exact match)
  from      window start — ISO 8601 UTC (e.g. "2026-05-05T10:00:00Z")
  to        window end   — ISO 8601 UTC

Current UTC time: %s
Use it to resolve relative phrases like "last hour", "last 30 minutes", "today".

Return ONLY a JSON object with exactly two top-level keys:
  "filters"     — object with zero or more of the fields above
  "explanation" — one short sentence describing the search

Example input : "DNS failures in the last 30 minutes"
Example output: {"filters":{"protocol":"DNS","from":"...","to":"..."},"explanation":"DNS flows from the last 30 minutes"}`

// Search translates a natural-language query into structured flow filters by
// calling the Anthropic Messages API synchronously (non-streaming).
func (c *Client) Search(ctx context.Context, query string) (SearchResult, error) {
	now := time.Now().UTC()
	sysPrompt := fmt.Sprintf(searchSystemPromptTpl, now.Format(time.RFC3339))

	reqBody := map[string]any{
		"model":      defaultModel,
		"max_tokens": 512,
		"system":     sysPrompt,
		"messages": []map[string]string{
			{"role": "user", "content": query},
		},
	}

	data, err := json.Marshal(reqBody)
	if err != nil {
		return SearchResult{}, err
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, anthropicAPIURL, bytes.NewReader(data))
	if err != nil {
		return SearchResult{}, err
	}
	httpReq.Header.Set("x-api-key", c.apiKey)
	httpReq.Header.Set("anthropic-version", anthropicVersion)
	httpReq.Header.Set("content-type", "application/json")

	resp, err := c.http.Do(httpReq)
	if err != nil {
		return SearchResult{}, err
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return SearchResult{}, fmt.Errorf("anthropic API %d: %s", resp.StatusCode, body)
	}

	// Parse the Anthropic response envelope.
	var msg struct {
		Content []struct {
			Type string `json:"type"`
			Text string `json:"text"`
		} `json:"content"`
	}
	if err := json.Unmarshal(body, &msg); err != nil {
		return SearchResult{}, fmt.Errorf("parse anthropic response: %w", err)
	}

	text := ""
	for _, b := range msg.Content {
		if b.Type == "text" {
			text = strings.TrimSpace(b.Text)
			break
		}
	}

	// Strip optional markdown fences (```json … ```) that the model may add.
	if idx := strings.Index(text, "{"); idx > 0 {
		text = text[idx:]
	}
	if idx := strings.LastIndex(text, "}"); idx >= 0 && idx < len(text)-1 {
		text = text[:idx+1]
	}

	var result SearchResult
	if err := json.Unmarshal([]byte(text), &result); err != nil {
		// Graceful degradation: return the raw text as the explanation.
		return SearchResult{Explanation: text}, nil
	}
	return result, nil
}
