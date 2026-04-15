// Package metrics provides lightweight in-process counters exposed in
// Prometheus text format at GET /metrics.
package metrics

import (
	"fmt"
	"sync/atomic"
	"time"
)

var (
	// IngestFlowsTotal counts every flow received via POST /api/v1/ingest.
	IngestFlowsTotal atomic.Int64
	// APIRequestsTotal counts all authenticated API requests.
	APIRequestsTotal atomic.Int64
	// ActiveSSEClients tracks the current number of live SSE connections.
	ActiveSSEClients atomic.Int32
	// AlertsFiredTotal counts alert webhook deliveries.
	AlertsFiredTotal atomic.Int64

	startTime = time.Now()
)

// Text returns the current metric snapshot in Prometheus exposition format.
func Text() string {
	return fmt.Sprintf(
		`# HELP netscope_ingest_flows_total Total network flows received from agents.
# TYPE netscope_ingest_flows_total counter
netscope_ingest_flows_total %d

# HELP netscope_api_requests_total Total authenticated API requests handled.
# TYPE netscope_api_requests_total counter
netscope_api_requests_total %d

# HELP netscope_active_sse_clients Current number of live SSE stream connections.
# TYPE netscope_active_sse_clients gauge
netscope_active_sse_clients %d

# HELP netscope_alerts_fired_total Total alert webhooks dispatched.
# TYPE netscope_alerts_fired_total counter
netscope_alerts_fired_total %d

# HELP netscope_uptime_seconds Seconds elapsed since the API process started.
# TYPE netscope_uptime_seconds gauge
netscope_uptime_seconds %.0f
`,
		IngestFlowsTotal.Load(),
		APIRequestsTotal.Load(),
		ActiveSSEClients.Load(),
		AlertsFiredTotal.Load(),
		time.Since(startTime).Seconds(),
	)
}
