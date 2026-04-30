package handlers

import (
	"bufio"
	"encoding/json"
	"fmt"
	"log/slog"

	"github.com/gofiber/fiber/v2"

	"github.com/netscope/hub-api/clickhouse"
	"github.com/netscope/hub-api/copilot"
)

// CopilotHandler serves the AI Security Copilot chat endpoint.
type CopilotHandler struct {
	CH           *clickhouse.Client
	AnthropicKey string
}

type copilotChatRequest struct {
	Messages []copilot.UserMessage `json:"messages"`
}

// Chat handles POST /api/v1/copilot/chat
//
// Request body: {"messages":[{"role":"user","content":"..."},...]}
//
// Response: Server-Sent Events stream.
//
//	data: {"type":"text","text":"..."}
//	data: {"type":"query","sql":"...","description":"..."}
//	data: {"type":"result","columns":[...],"rows":[...],"total":n}
//	data: {"type":"error","error":"..."}
//	data: {"type":"done"}
func (h *CopilotHandler) Chat(c *fiber.Ctx) error {
	if h.AnthropicKey == "" {
		return c.Status(fiber.StatusServiceUnavailable).JSON(
			fiber.Map{"error": "AI Copilot is not configured — set ANTHROPIC_API_KEY on the hub"})
	}

	var req copilotChatRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(
			fiber.Map{"error": "invalid request body: " + err.Error()})
	}

	if len(req.Messages) == 0 {
		return c.Status(fiber.StatusBadRequest).JSON(
			fiber.Map{"error": "messages must not be empty"})
	}

	// Validate last message is from user.
	if req.Messages[len(req.Messages)-1].Role != "user" {
		return c.Status(fiber.StatusBadRequest).JSON(
			fiber.Map{"error": "last message must have role 'user'"})
	}

	// Cap conversation history to last 20 messages to bound token usage.
	msgs := req.Messages
	if len(msgs) > 20 {
		msgs = msgs[len(msgs)-20:]
	}

	// ── Set up SSE ────────────────────────────────────────────────────────────
	c.Set("Content-Type", "text/event-stream")
	c.Set("Cache-Control", "no-cache, no-transform")
	c.Set("Connection", "keep-alive")
	c.Set("X-Accel-Buffering", "no")

	client := copilot.New(h.AnthropicKey, h.CH)
	eventCh := make(chan copilot.StreamEvent, 64)

	// Run the chat loop in a goroutine; close eventCh when done.
	go func() {
		defer close(eventCh)
		client.Chat(c.Context(), msgs, eventCh)
	}()

	// Stream events to the client.
	c.Context().SetBodyStreamWriter(func(w *bufio.Writer) {
		for ev := range eventCh {
			data, err := json.Marshal(ev)
			if err != nil {
				slog.Warn("copilot: failed to marshal event", "err", err)
				continue
			}
			_, writeErr := fmt.Fprintf(w, "data: %s\n\n", data)
			if writeErr != nil {
				// Client disconnected.
				return
			}
			w.Flush()
		}
	})

	return nil
}
