package clickhouse

import (
	"context"
	"fmt"

	"github.com/ClickHouse/clickhouse-go/v2"
	"github.com/ClickHouse/clickhouse-go/v2/lib/driver"
)

// Client wraps a ClickHouse driver connection.
type Client struct {
	conn driver.Conn
}

// New opens a ClickHouse connection using the provided DSN.
// The DSN format is: clickhouse://user:password@host:port/database
func New(dsn string) (*Client, error) {
	opts, err := clickhouse.ParseDSN(dsn)
	if err != nil {
		return nil, fmt.Errorf("clickhouse: parse dsn: %w", err)
	}

	conn, err := clickhouse.Open(opts)
	if err != nil {
		return nil, fmt.Errorf("clickhouse: open: %w", err)
	}

	return &Client{conn: conn}, nil
}

// Ping checks liveness of the ClickHouse connection.
func (c *Client) Ping(ctx context.Context) error {
	return c.conn.Ping(ctx)
}

// Exec executes a DDL or DML statement that returns no rows.
func (c *Client) Exec(ctx context.Context, query string, args ...interface{}) error {
	return c.conn.Exec(ctx, query, args...)
}

// Query executes a SELECT statement and returns the row iterator.
func (c *Client) Query(ctx context.Context, query string, args ...interface{}) (driver.Rows, error) {
	return c.conn.Query(ctx, query, args...)
}

// PrepareBatch prepares a batch insert statement.
func (c *Client) PrepareBatch(ctx context.Context, query string) (driver.Batch, error) {
	return c.conn.PrepareBatch(ctx, query)
}
