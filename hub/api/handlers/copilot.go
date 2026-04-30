package handlers

import (
	"bufio"
	"context"
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

	// Validate each message: role must be "user" or "assistant", content must be
	// non-empty and ≤ 8 KB to limit prompt-injection surface area.
	const maxMsgBytes = 8 * 1024
	for i, m := range req.Messages {
		if m.Role != "user" && m.Role != "assistant" {
			return c.Status(fiber.StatusBadRequest).JSON(
				fiber.Map{"error": fmt.Sprintf("message[%d]: role must be 'user' or 'assistant'", i)})
		}
		if m.Content == "" {
			return c.Status(fiber.StatusBadRequest).JSON(
				fiber.Map{"error": fmt.Sprintf("message[%d]: content must not be empty", i)})
		}
		if len(m.Content) > maxMsgBytes {
			return c.Status(fiber.StatusBadRequest).JSON(
				fiber.Map{"error": fmt.Sprintf("message[%d]: content exceeds 8 KB limit", i)})
		}
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

	// Create a cancellable context so the Chat goroutine stops when the client
	// disconnects — prevents a goroutine leak when the buffered channel is full.
	ctx, cancel := context.WithCancel(context.Background())

	// Run the chat loop in a goroutine; close eventCh when done.
	go func() {
		defer close(eventCh)
		client.Chat(ctx, msgs, eventCh)
	}()

	// Stream events to the client.
	c.Context().SetBodyStreamWriter(func(w *bufio.Writer) {
		// Always cancel the context when the stream writer exits so the Chat
		// goroutine receives the signal and terminates its Anthropic HTTP call.
		defer cancel()

		for ev := range eventCh {
			data, err := json.Marshal(ev)
			if err != nil {
				slog.Warn("copilot: failed to marshal event", "err", err)
				continue
			}
			_, writeErr := fmt.Fprintf(w, "data: %s\n\n", data)
			if writeErr != nil {
				// Client disconnected: cancel() fires via defer.
				// Drain remaining buffered events in the background so the Chat
				// goroutine can unblock from any pending channel send and exit.
				go func() {
					for range eventCh { //nolint:revive
					}
				}()
				return
			}
			w.Flush()
		}
	})

	return nil
}
