package handlers

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gofiber/fiber/v2"

	"github.com/klyzar/hub-api/testutil"
)

// TestFleetClusters_ThreeAgentsTwoClusters_ReturnsNonZeroGrid is
// REMAINING_WORK.md's V2: "Fleet cluster grid returns non-zero data on a hub
// with three agents in two clusters".
//
// NOTE: requires a real Docker daemon capable of pulling
// clickhouse/clickhouse-server:24.3-alpine. On JNJ/Zscaler-restricted
// machines this container pull is blocked — run this on an unrestricted
// machine (e.g. a MacBook) with:
//
//	go test ./handlers/... -run TestFleetClusters_ThreeAgentsTwoClusters -v
func TestFleetClusters_ThreeAgentsTwoClusters_ReturnsNonZeroGrid(t *testing.T) {
	ch := testutil.StartClickHouse(t)
	ctx := context.Background()
	now := time.Now().UTC()

	// Three agents: two in "prod-us", one in "prod-eu". All recently seen.
	agents := []struct {
		id, hostname, version, cluster string
	}{
		{"agent-1", "web01", "v0.7.0", "prod-us"},
		{"agent-2", "web02", "v0.7.0", "prod-us"},
		{"agent-3", "web03", "v0.6.0", "prod-eu"},
	}
	for _, a := range agents {
		if err := ch.Exec(ctx,
			`INSERT INTO agents (agent_id, hostname, version, interface, last_seen, registered_at, cluster)
			 VALUES (?, ?, ?, 'eth0', ?, ?, ?)`,
			a.id, a.hostname, a.version, now, now, a.cluster,
		); err != nil {
			t.Fatalf("insert agent %s: %v", a.id, err)
		}
	}

	// A handful of recent flows per agent so flows_1h is non-zero.
	flowAgents := []string{"agent-1", "agent-1", "agent-2", "agent-3"}
	for i, agentID := range flowAgents {
		if err := ch.Exec(ctx,
			`INSERT INTO flows (agent_id, hostname, ts, protocol, src_ip, src_port, dst_ip, dst_port)
			 VALUES (?, ?, ?, 'TCP', '10.0.0.1', 12345, '8.8.8.8', 443)`,
			agentID, agentID, now.Add(time.Duration(-i)*time.Minute),
		); err != nil {
			t.Fatalf("insert flow %d: %v", i, err)
		}
	}

	fleetH := &FleetHandler{CH: ch}
	app := fiber.New()
	app.Get("/api/v1/fleet/clusters", fleetH.Clusters)

	req := httptest.NewRequest("GET", "/api/v1/fleet/clusters", nil)
	resp, err := app.Test(req, -1)
	if err != nil {
		t.Fatalf("app.Test: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("want 200, got %d", resp.StatusCode)
	}

	var body struct {
		Clusters []struct {
			Cluster     string   `json:"cluster"`
			AgentCount  uint64   `json:"agent_count"`
			OnlineCount uint64   `json:"online_count"`
			Versions    []string `json:"versions"`
			Flows1h     uint64   `json:"flows_1h"`
		} `json:"clusters"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode response: %v", err)
	}

	if len(body.Clusters) != 2 {
		t.Fatalf("want 2 clusters (prod-us, prod-eu), got %d: %+v", len(body.Clusters), body.Clusters)
	}

	byName := make(map[string]struct {
		AgentCount  uint64
		OnlineCount uint64
		Flows1h     uint64
	})
	for _, c := range body.Clusters {
		byName[c.Cluster] = struct {
			AgentCount  uint64
			OnlineCount uint64
			Flows1h     uint64
		}{c.AgentCount, c.OnlineCount, c.Flows1h}
	}

	us, ok := byName["prod-us"]
	if !ok {
		t.Fatalf("expected a prod-us cluster row, got %+v", body.Clusters)
	}
	if us.AgentCount != 2 {
		t.Errorf("prod-us agent_count: want 2, got %d", us.AgentCount)
	}
	if us.OnlineCount != 2 {
		t.Errorf("prod-us online_count: want 2 (both seen recently), got %d", us.OnlineCount)
	}
	if us.Flows1h == 0 {
		t.Errorf("prod-us flows_1h: want non-zero, got 0")
	}

	eu, ok := byName["prod-eu"]
	if !ok {
		t.Fatalf("expected a prod-eu cluster row, got %+v", body.Clusters)
	}
	if eu.AgentCount != 1 {
		t.Errorf("prod-eu agent_count: want 1, got %d", eu.AgentCount)
	}
	if eu.Flows1h == 0 {
		t.Errorf("prod-eu flows_1h: want non-zero, got 0")
	}
}
