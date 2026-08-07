// Package testutil provides shared integration-test infrastructure for
// hub/api — currently a throwaway ClickHouse container used to verify the
// v0.5 fleet-observability features (cloud ingestion, fleet grid, compliance
// scheduling, Sigma → incident dispatch) end-to-end against a real database
// instead of mocks.
package testutil

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"

	"github.com/klyzar/hub-api/clickhouse"
)

// chImage matches the version pinned in docker-compose.yml so integration
// tests exercise the same ClickHouse release used in production.
const chImage = "clickhouse/clickhouse-server:24.3-alpine"

// StartClickHouse launches a throwaway ClickHouse container, applies the
// full Nexor schema via clickhouse.Migrate, and returns a connected client.
// The container is torn down automatically via t.Cleanup.
//
// If Docker is not available in the current environment the test is skipped
// (not failed) so this suite degrades gracefully in sandboxes without a
// Docker daemon — see REMAINING_WORK.md H5 for the corresponding CI note.
func StartClickHouse(t *testing.T) *clickhouse.Client {
	t.Helper()
	ctx := context.Background()

	req := testcontainers.ContainerRequest{
		Image:        chImage,
		ExposedPorts: []string{"9000/tcp"},
		Env: map[string]string{
			"CLICKHOUSE_DB":       "nexor",
			"CLICKHOUSE_USER":     "nexor",
			"CLICKHOUSE_PASSWORD": "nexor-test",
		},
		WaitingFor: wait.ForLog("Ready for connections").WithStartupTimeout(90 * time.Second),
	}
	container, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: req,
		Started:          true,
	})
	if err != nil {
		t.Skipf("skipping integration test: could not start ClickHouse container (is Docker available?): %v", err)
	}
	t.Cleanup(func() {
		_ = container.Terminate(context.Background())
	})

	host, err := container.Host(ctx)
	if err != nil {
		t.Fatalf("clickhouse container host: %v", err)
	}
	port, err := container.MappedPort(ctx, "9000/tcp")
	if err != nil {
		t.Fatalf("clickhouse container port: %v", err)
	}
	dsn := fmt.Sprintf("clickhouse://nexor:nexor-test@%s:%s/nexor", host, port.Port())

	var client *clickhouse.Client
	deadline := time.Now().Add(30 * time.Second)
	for {
		client, err = clickhouse.New(dsn)
		if err == nil {
			pingCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
			pingErr := client.Ping(pingCtx)
			cancel()
			if pingErr == nil {
				break
			}
			err = pingErr
		}
		if time.Now().After(deadline) {
			t.Fatalf("clickhouse never became ready: %v", err)
		}
		time.Sleep(500 * time.Millisecond)
	}

	migCtx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()
	if err := clickhouse.Migrate(migCtx, client); err != nil {
		t.Fatalf("apply schema: %v", err)
	}

	return client
}
