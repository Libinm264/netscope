package handlers

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"

	"github.com/netscope/hub-api/clickhouse"
	"github.com/netscope/hub-api/enterprise/compliance"
	"github.com/netscope/hub-api/enterprise/license"
	"github.com/netscope/hub-api/models"
	"github.com/netscope/hub-api/util"
)

// ComplianceReportHandler manages compliance report schedules and on-demand runs.
// All endpoints require Enterprise plan (FeatureComplianceReports).
type ComplianceReportHandler struct {
	CH      *clickhouse.Client
	License *license.License
}

func (h *ComplianceReportHandler) isEnterprise() bool {
	return h.License != nil && h.License.HasFeature(license.FeatureComplianceReports)
}

// List returns all compliance report schedules.
//
// GET /api/v1/enterprise/compliance/reports
func (h *ComplianceReportHandler) List(c *fiber.Ctx) error {
	if !h.isEnterprise() {
		return enterpriseGate(c, "compliance reports")
	}
	if h.CH == nil {
		return c.JSON(fiber.Map{"schedules": []any{}})
	}
	ctx, cancel := context.WithTimeout(c.Context(), 5*time.Second)
	defer cancel()

	rows, err := h.CH.Query(ctx,
		`SELECT id, name, framework, format, schedule, recipients, enabled, last_sent, created_at
		 FROM compliance_report_schedules
		 ORDER BY created_at DESC`)
	if err != nil {
		return util.InternalError(c, err)
	}
	defer rows.Close()

	var schedules []models.ReportSchedule
	for rows.Next() {
		var s models.ReportSchedule
		var enabledU uint8
		var recipientsJSON string
		if err := rows.Scan(
			&s.ID, &s.Name, &s.Framework, &s.Format, &s.Schedule,
			&recipientsJSON, &enabledU, &s.LastSent, &s.CreatedAt,
		); err != nil {
			continue
		}
		s.Enabled = enabledU == 1
		_ = json.Unmarshal([]byte(recipientsJSON), &s.Recipients)
		if s.Recipients == nil {
			s.Recipients = []string{}
		}
		schedules = append(schedules, s)
	}
	if schedules == nil {
		schedules = []models.ReportSchedule{}
	}
	return c.JSON(fiber.Map{"schedules": schedules})
}

// Create adds a new compliance report schedule.
//
// POST /api/v1/enterprise/compliance/reports
func (h *ComplianceReportHandler) Create(c *fiber.Ctx) error {
	if !h.isEnterprise() {
		return enterpriseGate(c, "compliance reports")
	}
	if h.CH == nil {
		return c.Status(fiber.StatusServiceUnavailable).JSON(fiber.Map{"error": "storage unavailable"})
	}

	var body struct {
		Name       string   `json:"name"`
		Framework  string   `json:"framework"`
		Format     string   `json:"format"`
		Schedule   string   `json:"schedule"`
		Recipients []string `json:"recipients"`
		Enabled    bool     `json:"enabled"`
	}
	if err := c.BodyParser(&body); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid body"})
	}
	if body.Framework == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "framework is required"})
	}
	if body.Format == "" {
		body.Format = "pdf"
	}
	if body.Schedule == "" {
		body.Schedule = "weekly"
	}
	if body.Recipients == nil {
		body.Recipients = []string{}
	}

	recipJSON, _ := json.Marshal(body.Recipients)
	id := uuid.New().String()
	now := time.Now().UTC()
	enabledU := uint8(0)
	if body.Enabled {
		enabledU = 1
	}

	ctx, cancel := context.WithTimeout(c.Context(), 5*time.Second)
	defer cancel()

	if err := h.CH.Exec(ctx,
		`INSERT INTO compliance_report_schedules
		 (id, name, framework, format, schedule, recipients, enabled, last_sent, created_at, version)
		 VALUES (?, ?, ?, ?, ?, ?, ?, toDateTime64(0,3), ?, ?)`,
		id, body.Name, body.Framework, body.Format, body.Schedule,
		string(recipJSON), enabledU, now, now.UnixMilli(),
	); err != nil {
		return util.InternalError(c, err)
	}
	return c.Status(fiber.StatusCreated).JSON(fiber.Map{"id": id, "ok": true})
}

// Update modifies an existing compliance report schedule.
//
// PATCH /api/v1/enterprise/compliance/reports/:id
func (h *ComplianceReportHandler) Update(c *fiber.Ctx) error {
	if !h.isEnterprise() {
		return enterpriseGate(c, "compliance reports")
	}
	id := c.Params("id")
	if h.CH == nil {
		return c.Status(fiber.StatusServiceUnavailable).JSON(fiber.Map{"error": "storage unavailable"})
	}

	var body struct {
		Name       *string  `json:"name"`
		Schedule   *string  `json:"schedule"`
		Format     *string  `json:"format"`
		Recipients []string `json:"recipients"`
		Enabled    *bool    `json:"enabled"`
	}
	if err := c.BodyParser(&body); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid body"})
	}

	ctx, cancel := context.WithTimeout(c.Context(), 5*time.Second)
	defer cancel()

	// Load current.
	rows, err := h.CH.Query(ctx,
		`SELECT id, name, framework, format, schedule, recipients, enabled, last_sent, created_at
		 FROM compliance_report_schedules WHERE id = ? ORDER BY created_at DESC LIMIT 1`, id)
	if err != nil {
		return util.InternalError(c, err)
	}
	defer rows.Close()
	if !rows.Next() {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "schedule not found"})
	}
	var cur models.ReportSchedule
	var enabledU uint8
	var recipientsJSON string
	_ = rows.Scan(&cur.ID, &cur.Name, &cur.Framework, &cur.Format, &cur.Schedule,
		&recipientsJSON, &enabledU, &cur.LastSent, &cur.CreatedAt)
	cur.Enabled = enabledU == 1
	_ = json.Unmarshal([]byte(recipientsJSON), &cur.Recipients)
	rows.Close()

	if body.Name != nil {
		cur.Name = *body.Name
	}
	if body.Schedule != nil {
		cur.Schedule = *body.Schedule
	}
	if body.Format != nil {
		cur.Format = *body.Format
	}
	if body.Enabled != nil {
		cur.Enabled = *body.Enabled
	}
	if body.Recipients != nil {
		cur.Recipients = body.Recipients
	}

	enabledUNew := uint8(0)
	if cur.Enabled {
		enabledUNew = 1
	}
	recipJSONNew, _ := json.Marshal(cur.Recipients)
	now := time.Now().UTC()

	if err := h.CH.Exec(ctx,
		`INSERT INTO compliance_report_schedules
		 (id, name, framework, format, schedule, recipients, enabled, last_sent, created_at, version)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		cur.ID, cur.Name, cur.Framework, cur.Format, cur.Schedule,
		string(recipJSONNew), enabledUNew, cur.LastSent, cur.CreatedAt, now.UnixMilli(),
	); err != nil {
		return util.InternalError(c, err)
	}
	return c.JSON(fiber.Map{"ok": true})
}

// Delete removes a compliance report schedule.
//
// DELETE /api/v1/enterprise/compliance/reports/:id
func (h *ComplianceReportHandler) Delete(c *fiber.Ctx) error {
	if !h.isEnterprise() {
		return enterpriseGate(c, "compliance reports")
	}
	id := c.Params("id")
	if h.CH == nil {
		return c.Status(fiber.StatusServiceUnavailable).JSON(fiber.Map{"error": "storage unavailable"})
	}
	ctx, cancel := context.WithTimeout(c.Context(), 5*time.Second)
	defer cancel()

	now := time.Now().UTC()
	_ = h.CH.Exec(ctx,
		`INSERT INTO compliance_report_schedules
		 (id, name, framework, format, schedule, recipients, enabled, last_sent, created_at, version)
		 SELECT id, name, framework, format, schedule, recipients, 0, last_sent, created_at, ?
		 FROM compliance_report_schedules WHERE id = ?
		 ORDER BY created_at DESC LIMIT 1`,
		now.UnixMilli()+1, id,
	)
	return c.JSON(fiber.Map{"ok": true})
}

// Run executes a report schedule immediately and returns the result.
//
// POST /api/v1/enterprise/compliance/reports/:id/run
func (h *ComplianceReportHandler) Run(c *fiber.Ctx) error {
	if !h.isEnterprise() {
		return enterpriseGate(c, "compliance reports")
	}
	id := c.Params("id")
	if h.CH == nil {
		return c.Status(fiber.StatusServiceUnavailable).JSON(fiber.Map{"error": "storage unavailable"})
	}

	ctx, cancel := context.WithTimeout(c.Context(), 5*time.Minute)
	defer cancel()

	// Load schedule.
	rows, err := h.CH.Query(ctx,
		`SELECT framework, format, schedule FROM compliance_report_schedules WHERE id = ? ORDER BY created_at DESC LIMIT 1`, id)
	if err != nil {
		return util.InternalError(c, err)
	}
	defer rows.Close()
	if !rows.Next() {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "schedule not found"})
	}
	var framework, format, schedule string
	_ = rows.Scan(&framework, &format, &schedule)
	rows.Close()

	since := sinceForScheduleStr(schedule)
	data, err := compliance.RunFramework(ctx, h.CH, framework, since)
	if err != nil {
		return util.InternalError(c, err)
	}

	rowCount := 0
	for _, ch := range data.Checks {
		rowCount += ch.RowCount
	}

	// Record run.
	_ = h.CH.Exec(ctx,
		`INSERT INTO compliance_report_runs
		 (id, schedule_id, framework, format, recipients, rows, sent_at, error)
		 VALUES (?, ?, ?, ?, '[]', ?, ?, '')`,
		uuid.New().String(), id, framework, format, rowCount, time.Now().UTC(),
	)

	return c.JSON(fiber.Map{
		"ok":        true,
		"framework": framework,
		"checks":    data.Checks,
		"generated": data.GeneratedAt,
	})
}

// History returns the run history for a schedule.
//
// GET /api/v1/enterprise/compliance/reports/:id/history
func (h *ComplianceReportHandler) History(c *fiber.Ctx) error {
	if !h.isEnterprise() {
		return enterpriseGate(c, "compliance reports")
	}
	id := c.Params("id")
	if h.CH == nil {
		return c.JSON(fiber.Map{"runs": []any{}})
	}
	ctx, cancel := context.WithTimeout(c.Context(), 5*time.Second)
	defer cancel()

	rows, err := h.CH.Query(ctx,
		`SELECT id, schedule_id, framework, format, recipients, rows, sent_at, error
		 FROM compliance_report_runs
		 WHERE schedule_id = ?
		 ORDER BY sent_at DESC LIMIT 50`, id)
	if err != nil {
		return util.InternalError(c, err)
	}
	defer rows.Close()

	var runs []models.ReportRun
	for rows.Next() {
		var r models.ReportRun
		var recipientsJSON string
		if err := rows.Scan(
			&r.ID, &r.ScheduleID, &r.Framework, &r.Format,
			&recipientsJSON, &r.Rows, &r.SentAt, &r.Error,
		); err != nil {
			continue
		}
		_ = json.Unmarshal([]byte(recipientsJSON), &r.Recipients)
		runs = append(runs, r)
	}
	if runs == nil {
		runs = []models.ReportRun{}
	}
	return c.JSON(fiber.Map{"runs": runs})
}

// Preview generates and streams the latest report as a file download.
//
// GET /api/v1/enterprise/compliance/reports/:id/preview
func (h *ComplianceReportHandler) Preview(c *fiber.Ctx) error {
	if !h.isEnterprise() {
		return enterpriseGate(c, "compliance reports")
	}
	id := c.Params("id")
	if h.CH == nil {
		return c.Status(fiber.StatusServiceUnavailable).JSON(fiber.Map{"error": "storage unavailable"})
	}

	ctx, cancel := context.WithTimeout(c.Context(), 5*time.Minute)
	defer cancel()

	rows, err := h.CH.Query(ctx,
		`SELECT framework, format, schedule FROM compliance_report_schedules WHERE id = ? ORDER BY created_at DESC LIMIT 1`, id)
	if err != nil {
		return util.InternalError(c, err)
	}
	defer rows.Close()
	if !rows.Next() {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "schedule not found"})
	}
	var framework, format, schedule string
	_ = rows.Scan(&framework, &format, &schedule)
	rows.Close()

	since := sinceForScheduleStr(schedule)
	data, err := compliance.RunFramework(ctx, h.CH, framework, since)
	if err != nil {
		return util.InternalError(c, err)
	}

	var payload []byte
	var mimeType, ext string
	switch strings.ToLower(format) {
	case "csv":
		payload, err = compliance.RenderCSV(data)
		mimeType = "text/csv"
		ext = "csv"
	default:
		payload, err = compliance.RenderPDF(data)
		mimeType = "application/pdf"
		ext = "pdf"
	}
	if err != nil {
		return util.InternalError(c, err)
	}

	filename := framework + "-report-" + time.Now().Format("2006-01-02") + "." + ext
	c.Set("Content-Disposition", "attachment; filename=\""+filename+"\"")
	c.Set("Content-Type", mimeType)
	return c.Send(payload)
}

// ── helpers ───────────────────────────────────────────────────────────────────

func enterpriseGate(c *fiber.Ctx, feature string) error {
	return c.Status(fiber.StatusPaymentRequired).JSON(fiber.Map{
		"error":        feature + " requires Enterprise plan",
		"upgrade_hint": "Upgrade to Enterprise to unlock " + feature + ".",
	})
}

func sinceForScheduleStr(schedule string) time.Time {
	now := time.Now().UTC()
	switch schedule {
	case "daily":
		return now.AddDate(0, 0, -1)
	case "monthly":
		return now.AddDate(0, -1, 0)
	default:
		return now.AddDate(0, 0, -7)
	}
}
