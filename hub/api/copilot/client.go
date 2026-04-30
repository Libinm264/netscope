// Package copilot wraps the Anthropic Messages API for the AI Security Copilot.
//
// Design
// ──────
// A single Chat() call runs the full tool-use loop internally:
//   1. Call Claude with the conversation + tool schema.
//   2. Stream text deltas → forward as StreamEvent{Type:"text"} immediately.
//   3. On tool_use block → validate SELECT-only, execute against ClickHouse,
//      emit StreamEvent{Type:"query"} + StreamEvent{Type:"result"}.
//   4. Append assistant + tool_result messages; loop until stop_reason="end_turn"
//      or max iterations (5) to prevent runaway costs.
//
// The SSE events written to the channel are JSON-encoded and forwarded by the
// Fiber handler as "data: ...\n\n" SSE lines — the proxy already handles the
// text/event-stream passthrough.
package copilot

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/netscope/hub-api/clickhouse"
)

const (
	anthropicAPIURL = "https://api.anthropic.com/v1/messages"
	anthropicVersion = "2023-06-01"
	defaultModel    = "claude-opus-4-5"
	maxTokens       = 4096
	maxToolLoops    = 5
	queryRowLimit   = 200
)

// StreamEvent is one SSE data payload sent to the browser.
type StreamEvent struct {
	Type    string   `json:"type"`
	Text    string   `json:"text,omitempty"`
	SQL     string   `json:"sql,omitempty"`
	Desc    string   `json:"description,omitempty"`
	Columns []string `json:"columns,omitempty"`
	// Rows is [][]any — each cell is a scalar (string/number/bool).
	Rows    [][]any  `json:"rows,omitempty"`
	Total   int      `json:"total,omitempty"`
	Error   string   `json:"error,omitempty"`
}

// UserMessage is what the browser POSTs.
type UserMessage struct {
	Role    string `json:"role"`    // "user" | "assistant"
	Content string `json:"content"`
}

// ── Anthropic wire types ──────────────────────────────────────────────────────

type aMessage struct {
	Role    string     `json:"role"`
	Content []aContent `json:"content"`
}

type aContent struct {
	Type      string          `json:"type"`
	Text      string          `json:"text,omitempty"`
	ID        string          `json:"id,omitempty"`
	Name      string          `json:"name,omitempty"`
	Input     json.RawMessage `json:"input,omitempty"`
	ToolUseID string          `json:"tool_use_id,omitempty"`
	Content   any             `json:"content,omitempty"` // tool_result payload
}

type aRequest struct {
	Model     string     `json:"model"`
	MaxTokens int        `json:"max_tokens"`
	Stream    bool       `json:"stream"`
	System    string     `json:"system"`
	Tools     []aTool    `json:"tools"`
	Messages  []aMessage `json:"messages"`
}

type aTool struct {
	Name        string      `json:"name"`
	Description string      `json:"description"`
	InputSchema aToolSchema `json:"input_schema"`
}

type aToolSchema struct {
	Type       string              `json:"type"`
	Properties map[string]aProp    `json:"properties"`
	Required   []string            `json:"required"`
}

type aProp struct {
	Type        string `json:"type"`
	Description string `json:"description"`
}

// ── Streaming event types from Anthropic ─────────────────────────────────────

type aDelta struct {
	Type        string `json:"type"`         // "text_delta" | "input_json_delta"
	Text        string `json:"text"`         // text_delta
	PartialJSON string `json:"partial_json"` // input_json_delta
	StopReason  string `json:"stop_reason"`  // message_delta
}

type aStreamEvent struct {
	Type         string   `json:"type"`
	Index        int      `json:"index"`
	Delta        aDelta   `json:"delta"`
	ContentBlock aContent `json:"content_block"` // content_block_start
}

type toolRunInput struct {
	SQL         string `json:"sql"`
	Description string `json:"description"`
}

// ── System prompt ─────────────────────────────────────────────────────────────

const systemPrompt = `You are NetScope's AI Security Copilot — an expert network security analyst assistant embedded in the NetScope network observability platform.

You help security engineers and network operators understand their network traffic, investigate anomalies, identify threats, and answer questions about the data captured by NetScope agents.

## ClickHouse Schema

**flows** — every network connection observed by agents
  ts (DateTime64), agent_id, hostname, protocol (TCP/UDP/HTTP/DNS/TLS/ICMP/ARP),
  src_ip, src_port, dst_ip, dst_port, bytes_in, bytes_out, duration_ms,
  process_name, pid, country_code, country_name, as_org,
  threat_score (0-100), threat_level (high/medium/low/empty),
  http_method, http_path, http_status, dns_query, dns_type, trace_id,
  pod_name, k8s_namespace, source (agent/cloud)

**agents** — registered agents
  agent_id, hostname, os, capture_mode (pcap/ebpf), ebpf_enabled, cluster, last_seen

**anomaly_events** — behavioral anomaly detections (Z-score outliers)
  id, agent_id, hostname, protocol, anomaly_type (spike/drop),
  z_score, observed, expected, description, severity (low/medium/high), detected_at

**traffic_baselines** — per-(agent, protocol, hour_of_week) 7-day rolling baseline
  agent_id, protocol, hour_of_week (0=Mon00 … 167=Sun23),
  flow_count_mean, flow_count_std, bytes_in_mean, bytes_out_mean, sample_count

**alert_rules** — configured alert conditions
  id, name, metric, condition, threshold, window_minutes, enabled

**alert_events** — fired alert instances
  id, rule_id, rule_name, metric, value, threshold, fired_at

**sigma_matches** — Sigma detection rule matches
  id, rule_id, rule_title, severity, match_data (JSON), fired_at

**tls_certs** — observed TLS certificates
  fingerprint, cn, issuer, expiry, expired, agent_id, hostname, dst_ip

**process_policies** — process network behaviour policies
  id, name, process_name, action (alert/deny), dst_ip_cidr, dst_port, enabled

## Rules
- Always use the run_query tool to fetch real data before answering quantitative questions.
- Only SELECT queries are allowed. Never INSERT, UPDATE, DELETE, DROP, CREATE, ALTER.
- Current time: use now(). All timestamps are UTC.
- Keep queries efficient — add LIMIT, narrow time windows, filter early.
- Format numbers readably (e.g. "1.2 M flows", "450 MB").
- Be concise and precise. Use bullet points for lists.
- When you find anomalies or threats, explain the security implications clearly.
- If the query returns no results, say so and suggest why (e.g. no data in the time window).`

// ── tools definition ──────────────────────────────────────────────────────────

var copilotTools = []aTool{
	{
		Name:        "run_query",
		Description: "Execute a SELECT query against the NetScope ClickHouse database and return the results. Use this to answer any question that requires real data.",
		InputSchema: aToolSchema{
			Type: "object",
			Properties: map[string]aProp{
				"sql": {
					Type:        "string",
					Description: "A SELECT SQL query compatible with ClickHouse syntax. Must not contain INSERT, UPDATE, DELETE, DROP, CREATE, ALTER, or TRUNCATE.",
				},
				"description": {
					Type:        "string",
					Description: "Short human-readable description of what this query does (shown to the user).",
				},
			},
			Required: []string{"sql"},
		},
	},
}

// ── Client ────────────────────────────────────────────────────────────────────

// Client wraps the Anthropic API + ClickHouse tool executor.
type Client struct {
	apiKey string
	ch     *clickhouse.Client
	http   *http.Client
}

// New creates a Copilot client.
func New(apiKey string, ch *clickhouse.Client) *Client {
	return &Client{
		apiKey: apiKey,
		ch:     ch,
		http:   &http.Client{Timeout: 120 * time.Second},
	}
}

// Chat runs the full conversation turn, emitting StreamEvents to out.
// It handles multi-turn tool-use loops internally.
// The caller is responsible for closing out after Chat returns.
func (c *Client) Chat(ctx context.Context, history []UserMessage, out chan<- StreamEvent) {
	// Convert browser history (user/assistant text only) to Anthropic messages.
	msgs := make([]aMessage, 0, len(history))
	for _, m := range history {
		msgs = append(msgs, aMessage{
			Role:    m.Role,
			Content: []aContent{{Type: "text", Text: m.Content}},
		})
	}

	for loop := 0; loop < maxToolLoops; loop++ {
		assistantContents, stopReason, err := c.streamTurn(ctx, msgs, out)
		if err != nil {
			out <- StreamEvent{Type: "error", Error: err.Error()}
			return
		}

		if stopReason != "tool_use" {
			// end_turn or stop sequence — we're done.
			break
		}

		// Build tool_result messages and execute each tool call.
		toolResults := make([]aContent, 0, len(assistantContents))
		for _, blk := range assistantContents {
			if blk.Type != "tool_use" {
				continue
			}

			var inp toolRunInput
			if err := json.Unmarshal(blk.Input, &inp); err != nil {
				toolResults = append(toolResults, aContent{
					Type:      "tool_result",
					ToolUseID: blk.ID,
					Content:   "error: malformed tool input",
				})
				continue
			}

			result := c.executeQuery(ctx, inp.SQL, inp.Description, out)
			toolResults = append(toolResults, aContent{
				Type:      "tool_result",
				ToolUseID: blk.ID,
				Content:   result,
			})
		}

		// Append assistant turn + tool_results user turn.
		msgs = append(msgs,
			aMessage{Role: "assistant", Content: assistantContents},
			aMessage{Role: "user", Content: toolResults},
		)
	}

	out <- StreamEvent{Type: "done"}
}

// streamTurn sends one streaming API call to Anthropic, forwards text deltas
// to out, and returns the complete list of content blocks + stop_reason.
func (c *Client) streamTurn(ctx context.Context, msgs []aMessage, out chan<- StreamEvent) ([]aContent, string, error) {
	req := aRequest{
		Model:     defaultModel,
		MaxTokens: maxTokens,
		Stream:    true,
		System:    systemPrompt,
		Tools:     copilotTools,
		Messages:  msgs,
	}

	body, err := json.Marshal(req)
	if err != nil {
		return nil, "", fmt.Errorf("marshal request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, anthropicAPIURL, bytes.NewReader(body))
	if err != nil {
		return nil, "", fmt.Errorf("build request: %w", err)
	}
	httpReq.Header.Set("x-api-key", c.apiKey)
	httpReq.Header.Set("anthropic-version", anthropicVersion)
	httpReq.Header.Set("content-type", "application/json")

	resp, err := c.http.Do(httpReq)
	if err != nil {
		return nil, "", fmt.Errorf("anthropic request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		errBody, _ := io.ReadAll(resp.Body)
		return nil, "", fmt.Errorf("anthropic API error %d: %s", resp.StatusCode, string(errBody))
	}

	// ── Parse SSE stream ──────────────────────────────────────────────────────
	//
	// We track per-block accumulators indexed by block index:
	//   textBuf[i]  — accumulated text for text blocks
	//   toolBuf[i]  — accumulated partial_json for tool_use blocks
	//   blockTypes[i] — "text" | "tool_use"
	//   blockMeta[i]  — aContent with id/name populated from content_block_start
	//
	// On content_block_stop we finalise the block.

	textBuf   := map[int]*strings.Builder{}
	toolBuf   := map[int]*strings.Builder{}
	blockType := map[int]string{}
	blockMeta := map[int]aContent{}
	var stopReason string

	scanner := bufio.NewScanner(resp.Body)
	// Anthropic SSE lines can be long (large tool inputs) — give scanner room.
	scanner.Buffer(make([]byte, 256*1024), 256*1024)

	var eventType string // current SSE event: type line
	for scanner.Scan() {
		line := scanner.Text()

		if strings.HasPrefix(line, "event: ") {
			eventType = strings.TrimPrefix(line, "event: ")
			continue
		}

		if !strings.HasPrefix(line, "data: ") {
			continue
		}

		data := strings.TrimPrefix(line, "data: ")
		if data == "[DONE]" {
			break
		}

		_ = eventType // already encoded in the "type" field inside data JSON

		var ev aStreamEvent
		if err := json.Unmarshal([]byte(data), &ev); err != nil {
			slog.Warn("copilot: failed to parse SSE event", "data", data, "err", err)
			continue
		}

		switch ev.Type {
		case "content_block_start":
			idx := ev.Index
			blockType[idx] = ev.ContentBlock.Type
			blockMeta[idx] = ev.ContentBlock
			if ev.ContentBlock.Type == "text" {
				textBuf[idx] = &strings.Builder{}
			} else if ev.ContentBlock.Type == "tool_use" {
				toolBuf[idx] = &strings.Builder{}
			}

		case "content_block_delta":
			idx := ev.Index
			switch ev.Delta.Type {
			case "text_delta":
				if ev.Delta.Text != "" {
					if sb, ok := textBuf[idx]; ok {
						sb.WriteString(ev.Delta.Text)
					}
					// Forward immediately so the browser renders tokens as they arrive.
					out <- StreamEvent{Type: "text", Text: ev.Delta.Text}
				}
			case "input_json_delta":
				if ev.Delta.PartialJSON != "" {
					if sb, ok := toolBuf[idx]; ok {
						sb.WriteString(ev.Delta.PartialJSON)
					}
				}
			}

		case "message_delta":
			stopReason = ev.Delta.StopReason
		}
	}

	if err := scanner.Err(); err != nil && ctx.Err() == nil {
		return nil, "", fmt.Errorf("reading stream: %w", err)
	}

	// Assemble the full list of content blocks for the assistant message.
	// Use sequential indices.
	maxIdx := 0
	for i := range blockType {
		if i > maxIdx {
			maxIdx = i
		}
	}

	contents := make([]aContent, 0, maxIdx+1)
	for i := 0; i <= maxIdx; i++ {
		bt, ok := blockType[i]
		if !ok {
			continue
		}
		meta := blockMeta[i]
		switch bt {
		case "text":
			text := ""
			if sb, ok := textBuf[i]; ok {
				text = sb.String()
			}
			contents = append(contents, aContent{Type: "text", Text: text})
		case "tool_use":
			rawJSON := ""
			if sb, ok := toolBuf[i]; ok {
				rawJSON = sb.String()
			}
			contents = append(contents, aContent{
				Type:  "tool_use",
				ID:    meta.ID,
				Name:  meta.Name,
				Input: json.RawMessage(rawJSON),
			})
		}
	}

	return contents, stopReason, nil
}

// executeQuery validates and runs a SELECT query, emitting query/result events.
// Returns a string summary of the result for the tool_result message.
func (c *Client) executeQuery(ctx context.Context, sql, desc string, out chan<- StreamEvent) string {
	// Emit a "query" event so the UI shows the SQL card.
	out <- StreamEvent{Type: "query", SQL: sql, Desc: desc}

	// SELECT-only enforcement.
	if err := validateSQL(sql); err != nil {
		out <- StreamEvent{Type: "error", Error: "Query blocked: " + err.Error()}
		return "error: " + err.Error()
	}

	if c.ch == nil {
		out <- StreamEvent{Type: "error", Error: "ClickHouse not available"}
		return "error: ClickHouse not available"
	}

	qctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()

	// Inject LIMIT if not present to prevent enormous result sets.
	limitedSQL := injectLimit(sql, queryRowLimit)

	rows, err := c.ch.Query(qctx, limitedSQL)
	if err != nil {
		msg := fmt.Sprintf("query error: %s", err.Error())
		out <- StreamEvent{Type: "error", Error: msg}
		return msg
	}
	defer rows.Close()

	cols := rows.Columns()

	// Scan rows into [][]any.
	var result [][]any
	for rows.Next() {
		ptrs := make([]any, len(cols))
		for i := range ptrs {
			var v any
			ptrs[i] = &v
		}
		if err := rows.Scan(ptrs...); err != nil {
			continue
		}
		row := make([]any, len(cols))
		for i, p := range ptrs {
			row[i] = *(p.(*any))
		}
		result = append(result, row)
	}

	total := len(result)
	out <- StreamEvent{
		Type:    "result",
		Columns: cols,
		Rows:    result,
		Total:   total,
	}

	// Build a compact text summary for Claude's tool_result.
	return buildResultSummary(cols, result)
}

// ── Helpers ───────────────────────────────────────────────────────────────────

// validateSQL returns an error if the query is not a safe SELECT.
func validateSQL(sql string) error {
	upper := strings.ToUpper(strings.TrimSpace(sql))
	// Strip leading comments
	for strings.HasPrefix(upper, "--") || strings.HasPrefix(upper, "/*") {
		nl := strings.IndexByte(upper, '\n')
		if nl < 0 {
			return fmt.Errorf("query must be a SELECT statement")
		}
		upper = strings.TrimSpace(upper[nl+1:])
	}
	if !strings.HasPrefix(upper, "SELECT") {
		return fmt.Errorf("only SELECT queries are allowed")
	}
	for _, kw := range []string{"INSERT", "UPDATE", "DELETE", "DROP", "CREATE", "ALTER", "TRUNCATE", "RENAME", "ATTACH", "DETACH"} {
		// Use word-boundary check: keyword followed by space, (, or end.
		idx := strings.Index(upper, kw)
		if idx >= 0 {
			after := upper[idx+len(kw):]
			if len(after) == 0 || after[0] == ' ' || after[0] == '(' || after[0] == '\t' || after[0] == '\n' {
				return fmt.Errorf("forbidden keyword: %s", kw)
			}
		}
	}
	return nil
}

// injectLimit appends LIMIT n to the query if the outer SELECT has no LIMIT.
// It scans at parenthesis depth 0 only, so a LIMIT inside a subquery does not
// prevent the outer query from being capped — closing the previous blind-spot.
func injectLimit(sql string, n int) string {
	upper := strings.ToUpper(sql)
	depth := 0
	for i := 0; i < len(upper); i++ {
		switch upper[i] {
		case '(':
			depth++
		case ')':
			depth--
		}
		// Match "LIMIT" at the outermost level only (depth == 0).
		if depth == 0 && strings.HasPrefix(upper[i:], "LIMIT") {
			// Confirm it is a word boundary (preceded by whitespace or start).
			if i == 0 || upper[i-1] == ' ' || upper[i-1] == '\t' || upper[i-1] == '\n' {
				return sql // outer LIMIT already present
			}
		}
	}
	return strings.TrimRight(sql, " \t\n\r;") + fmt.Sprintf(" LIMIT %d", n)
}

// buildResultSummary creates a compact Markdown table for Claude's context.
func buildResultSummary(cols []string, rows [][]any) string {
	if len(rows) == 0 {
		return "Query returned 0 rows."
	}
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Query returned %d row(s).\n\n", len(rows)))

	// Header
	sb.WriteString("| " + strings.Join(cols, " | ") + " |\n")
	sb.WriteString("|" + strings.Repeat("---|", len(cols)) + "\n")

	// Limit summary rows to avoid huge context
	maxRows := 50
	if len(rows) < maxRows {
		maxRows = len(rows)
	}
	for _, row := range rows[:maxRows] {
		cells := make([]string, len(row))
		for i, v := range row {
			cells[i] = fmt.Sprintf("%v", v)
		}
		sb.WriteString("| " + strings.Join(cells, " | ") + " |\n")
	}
	if len(rows) > maxRows {
		sb.WriteString(fmt.Sprintf("... (%d more rows)\n", len(rows)-maxRows))
	}
	return sb.String()
}
