package handlers

import (
	"fmt"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"

	"github.com/netscope/hub-api/clickhouse"
	"github.com/netscope/hub-api/models"
)

// OtelHandler exports flows in OpenTelemetry JSON format.
type OtelHandler struct {
	CH *clickhouse.Client
}

// ExportTraces handles GET /api/v1/otel/traces.
// Converts recent HTTP flows into OTLP-compatible JSON spans, enabling
// import into Jaeger, Tempo, or any OTEL-compatible backend.
func (h *OtelHandler) ExportTraces(c *fiber.Ctx) error {
	if h.CH == nil {
		return c.Status(fiber.StatusServiceUnavailable).JSON(fiber.Map{
			"error": "ClickHouse is not available",
		})
	}

	window := c.Query("window", "1h")
	interval := windowToInterval(window)

	rows, err := h.CH.Query(c.Context(), fmt.Sprintf(`
		SELECT
			id, agent_id, hostname,
			ts, src_ip, src_port, dst_ip, dst_port,
			duration_ms, http_method, http_path, http_status
		FROM flows
		WHERE protocol IN ('HTTP', 'HTTPS')
		  AND ts >= now() - INTERVAL %s
		  AND http_method != ''
		ORDER BY ts DESC
		LIMIT 1000
	`, interval))
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}
	defer rows.Close()

	spans := make([]models.OtelSpan, 0)

	for rows.Next() {
		var (
			id, agentID, hostname, srcIP, dstIP, method, path string
			ts                                                  time.Time
			srcPort, dstPort, httpStatus                        uint16
			durationMs                                          uint32
		)
		if err := rows.Scan(
			&id, &agentID, &hostname,
			&ts, &srcIP, &srcPort, &dstIP, &dstPort,
			&durationMs, &method, &path, &httpStatus,
		); err != nil {
			continue
		}

		startNs := fmt.Sprintf("%d", ts.UnixNano())
		endNs := fmt.Sprintf("%d", ts.Add(time.Duration(durationMs)*time.Millisecond).UnixNano())

		// STATUS_CODE_OK=1, STATUS_CODE_ERROR=2
		statusCode := 1
		if httpStatus >= 400 {
			statusCode = 2
		}

		// Derive 32-char hex traceId and 16-char hex spanId from the UUID.
		traceID := strings.ReplaceAll(id, "-", "")
		if len(traceID) > 32 {
			traceID = traceID[:32]
		}
		spanID := traceID
		if len(spanID) > 16 {
			spanID = spanID[:16]
		}

		spans = append(spans, models.OtelSpan{
			TraceID:           traceID,
			SpanID:            spanID,
			Name:              method + " " + path,
			Kind:              3, // SPAN_KIND_CLIENT
			StartTimeUnixNano: startNs,
			EndTimeUnixNano:   endNs,
			Attributes: []models.OtelAttribute{
				{Key: "http.method", Value: models.OtelValue{StringValue: method}},
				{Key: "http.target", Value: models.OtelValue{StringValue: path}},
				{Key: "http.status_code", Value: models.OtelValue{IntValue: fmt.Sprintf("%d", httpStatus)}},
				{Key: "net.peer.ip", Value: models.OtelValue{StringValue: dstIP}},
				{Key: "net.peer.port", Value: models.OtelValue{IntValue: fmt.Sprintf("%d", dstPort)}},
				{Key: "net.host.ip", Value: models.OtelValue{StringValue: srcIP}},
				{Key: "netscope.agent_id", Value: models.OtelValue{StringValue: agentID}},
				{Key: "netscope.hostname", Value: models.OtelValue{StringValue: hostname}},
			},
			Status: models.OtelStatus{Code: statusCode},
		})
	}

	// Return as OTLP JSON (ResourceSpans format)
	return c.JSON(fiber.Map{
		"resourceSpans": []fiber.Map{
			{
				"resource": fiber.Map{
					"attributes": []fiber.Map{
						{"key": "service.name", "value": fiber.Map{"stringValue": "netscope"}},
						{"key": "telemetry.sdk.name", "value": fiber.Map{"stringValue": "netscope-hub"}},
					},
				},
				"scopeSpans": []fiber.Map{
					{
						"scope": fiber.Map{
							"name":    "netscope-hub",
							"version": "0.1.0",
						},
						"spans": spans,
					},
				},
			},
		},
	})
}
