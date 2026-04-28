package handlers

import (
	"context"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"

	"github.com/netscope/hub-api/clickhouse"
	"github.com/netscope/hub-api/enterprise/license"
	"github.com/netscope/hub-api/util"
)

// StorageHandler manages the S3/GCS long-term storage configuration.
// Requires Enterprise plan (FeatureAuditExport gate reused).
type StorageHandler struct {
	CH      *clickhouse.Client
	License *license.License
}

type storageConfig struct {
	Provider   string    `json:"provider"` // "s3"|"gcs"|"minio"|"r2"
	Enabled    bool      `json:"enabled"`
	Bucket     string    `json:"bucket"`
	Region     string    `json:"region"`
	Endpoint   string    `json:"endpoint"`
	AccessKey  string    `json:"access_key"`
	SecretKey  string    `json:"secret_key"` // redacted on read
	Prefix     string    `json:"prefix"`
	Schedule   string    `json:"schedule"` // "hourly"|"daily"
	LastExport string    `json:"last_export,omitempty"`
	UpdatedAt  time.Time `json:"updated_at"`
}

// GetConfig returns the current storage configuration.
func (h *StorageHandler) GetConfig(c *fiber.Ctx) error {
	if !h.isEnterprise() {
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{
			"error":        "long-term storage requires Enterprise plan",
			"upgrade_hint": "Upgrade to Enterprise to configure S3/GCS export.",
		})
	}
	if h.CH == nil {
		return c.JSON(fiber.Map{"config": nil})
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	rows, err := h.CH.Query(ctx,
		`SELECT provider, enabled, bucket, region, endpoint,
		        access_key, secret_key, prefix, schedule, last_export, updated_at
		 FROM storage_config ORDER BY version DESC LIMIT 1`)
	if err != nil {
		return util.InternalError(c, err)
	}
	defer rows.Close()

	if !rows.Next() {
		return c.JSON(fiber.Map{"config": nil})
	}
	var cfg storageConfig
	var enabledU uint8
	var lastExport time.Time
	if err := rows.Scan(
		&cfg.Provider, &enabledU, &cfg.Bucket, &cfg.Region, &cfg.Endpoint,
		&cfg.AccessKey, &cfg.SecretKey, &cfg.Prefix, &cfg.Schedule,
		&lastExport, &cfg.UpdatedAt,
	); err != nil {
		return util.InternalError(c, err)
	}
	cfg.Enabled = enabledU == 1
	if !lastExport.IsZero() && lastExport.Year() > 2000 {
		cfg.LastExport = lastExport.UTC().Format(time.RFC3339)
	}
	// Redact secret key — show only last 4 chars
	if len(cfg.SecretKey) > 4 {
		cfg.SecretKey = "***" + cfg.SecretKey[len(cfg.SecretKey)-4:]
	}
	return c.JSON(fiber.Map{"config": cfg})
}

// UpsertConfig creates or replaces the storage configuration.
func (h *StorageHandler) UpsertConfig(c *fiber.Ctx) error {
	if !h.isEnterprise() {
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{
			"error": "long-term storage requires Enterprise plan",
		})
	}
	if h.CH == nil {
		return c.Status(fiber.StatusServiceUnavailable).JSON(fiber.Map{"error": "storage unavailable"})
	}

	var body storageConfig
	if err := c.BodyParser(&body); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid body"})
	}
	if body.Bucket == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "bucket is required"})
	}
	if body.Provider == "" {
		body.Provider = "s3"
	}
	if body.Prefix == "" {
		body.Prefix = "netscope/flows"
	}
	if body.Schedule == "" {
		body.Schedule = "hourly"
	}

	// If secret_key is a redacted placeholder, load the existing value.
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if strings.HasPrefix(body.SecretKey, "***") {
		rows, _ := h.CH.Query(ctx, `SELECT secret_key FROM storage_config ORDER BY version DESC LIMIT 1`)
		if rows != nil {
			if rows.Next() {
				_ = rows.Scan(&body.SecretKey)
			}
			rows.Close()
		}
	}

	enabledU := uint8(0)
	if body.Enabled {
		enabledU = 1
	}
	now := time.Now()
	if err := h.CH.Exec(ctx,
		`INSERT INTO storage_config
		 (provider, enabled, bucket, region, endpoint, access_key, secret_key,
		  prefix, schedule, last_export, updated_at, version)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, toDateTime64(0, 3), ?, ?)`,
		body.Provider, enabledU, body.Bucket, body.Region, body.Endpoint,
		body.AccessKey, body.SecretKey, body.Prefix, body.Schedule,
		now, now.UnixMilli(),
	); err != nil {
		return util.InternalError(c, err)
	}
	return c.JSON(fiber.Map{"ok": true})
}

// DeleteConfig disables storage export by writing a disabled row.
func (h *StorageHandler) DeleteConfig(c *fiber.Ctx) error {
	if !h.isEnterprise() {
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{"error": "Enterprise required"})
	}
	if h.CH == nil {
		return c.Status(fiber.StatusServiceUnavailable).JSON(fiber.Map{"error": "storage unavailable"})
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	now := time.Now()
	_ = h.CH.Exec(ctx,
		`INSERT INTO storage_config
		 (provider, enabled, bucket, region, endpoint, access_key, secret_key,
		  prefix, schedule, last_export, updated_at, version)
		 VALUES ('s3', 0, '', '', '', '', '', 'netscope/flows', 'hourly', toDateTime64(0,3), ?, ?)`,
		now, now.UnixMilli()+1,
	)
	return c.JSON(fiber.Map{"ok": true})
}

// ListExports returns the last 50 export run results.
func (h *StorageHandler) ListExports(c *fiber.Ctx) error {
	if !h.isEnterprise() {
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{"error": "Enterprise required"})
	}
	if h.CH == nil {
		return c.JSON(fiber.Map{"exports": []any{}})
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	rows, err := h.CH.Query(ctx,
		`SELECT window, object_key, row_count, exported_at, error
		 FROM storage_exports
		 ORDER BY exported_at DESC LIMIT 50`)
	if err != nil {
		return util.InternalError(c, err)
	}
	defer rows.Close()

	type exportRow struct {
		Window     string    `json:"window"`
		ObjectKey  string    `json:"object_key"`
		RowCount   uint64    `json:"row_count"`
		ExportedAt time.Time `json:"exported_at"`
		Error      string    `json:"error,omitempty"`
	}
	exports := make([]exportRow, 0, 50)
	for rows.Next() {
		var r exportRow
		if err := rows.Scan(&r.Window, &r.ObjectKey, &r.RowCount, &r.ExportedAt, &r.Error); err == nil {
			exports = append(exports, r)
		}
	}
	return c.JSON(fiber.Map{"exports": exports})
}

func (h *StorageHandler) isEnterprise() bool {
	return h.License != nil && h.License.Plan == "enterprise"
}
