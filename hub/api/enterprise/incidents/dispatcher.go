// Package incidents manages in-hub incident lifecycle and routes Sigma match
// events to external ticketing / alerting systems.
//
// # Supported integrations
//
//   - PagerDuty   — Events v2 API
//   - OpsGenie    — Alert API v2
//   - Jira        — REST API v3 (issue create)
//   - Linear      — GraphQL API (issueCreate mutation)
//
// # Enterprise gate
//
// All incident workflow functionality requires license.FeatureIncidentWorkflow.
// The dispatcher is a no-op when the feature is not licensed.
package incidents

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/netscope/hub-api/clickhouse"
	"github.com/netscope/hub-api/enterprise/license"
)

// SigmaMatchEvent is passed from the Sigma engine when a rule fires.
// Must match the sigma.DispatchEvent shape so the Dispatcher satisfies the
// sigma.Dispatcher interface without importing the sigma package (avoids cycle).
type SigmaMatchEvent struct {
	RuleID    string
	RuleTitle string
	Severity  string // "low" | "medium" | "high" | "critical"
	SrcIP     string
	DstIP     string
	FiredAt   time.Time
}

// Dispatcher creates incidents and routes them to external systems.
type Dispatcher struct {
	ch  *clickhouse.Client
	lic *license.License
}

// New returns a new Dispatcher.
func New(ch *clickhouse.Client, lic *license.License) *Dispatcher {
	return &Dispatcher{ch: ch, lic: lic}
}

// Dispatch is called by the Sigma engine after recording a match.
// The ev parameter is compatible with sigma.DispatchEvent — same field names
// and types — allowing *Dispatcher to satisfy sigma.Dispatcher without a
// circular import.
func (d *Dispatcher) Dispatch(ctx context.Context, ev SigmaMatchEvent) {
	if !d.lic.HasFeature(license.FeatureIncidentWorkflow) {
		return
	}
	incidentID := uuid.New().String()
	title := fmt.Sprintf("[%s] %s — %s → %s",
		strings.ToUpper(ev.Severity), ev.RuleTitle, ev.SrcIP, ev.DstIP)

	now := time.Now().UTC()
	if err := d.ch.Exec(ctx,
		`INSERT INTO incidents
		 (id, title, severity, status, source, source_id, notes, external_ref, created_at, updated_at, version)
		 VALUES (?, ?, ?, 'open', 'sigma', ?, '', '', ?, ?, ?)`,
		incidentID, title, ev.Severity, ev.RuleID, now, now, now.UnixMilli(),
	); err != nil {
		slog.Warn("incidents: create incident", "err", err)
		return
	}

	// Load enabled workflow configs.
	cfgs, err := d.loadConfigs(ctx)
	if err != nil {
		slog.Warn("incidents: load configs", "err", err)
		return
	}

	description := fmt.Sprintf(
		"NetScope Sigma rule fired at %s.\n\nRule: %s\nSeverity: %s\nSource IP: %s\nDest IP: %s",
		ev.FiredAt.Format(time.RFC3339), ev.RuleTitle, ev.Severity, ev.SrcIP, ev.DstIP,
	)

	for integration, rawCfg := range cfgs {
		var extRef string
		var dispErr error

		switch integration {
		case "pagerduty":
			extRef, dispErr = d.firePagerDuty(rawCfg, title, description, ev.Severity)
		case "opsgenie":
			extRef, dispErr = d.fireOpsGenie(rawCfg, title, description, ev.Severity)
		case "jira":
			var cfg JiraConfig
			if jsonErr := json.Unmarshal([]byte(rawCfg), &cfg); jsonErr == nil {
				extRef, dispErr = CreateJiraIssue(cfg, title, description, ev.Severity)
			} else {
				dispErr = jsonErr
			}
		case "linear":
			var cfg LinearConfig
			if jsonErr := json.Unmarshal([]byte(rawCfg), &cfg); jsonErr == nil {
				extRef, dispErr = CreateLinearIssue(cfg, title, description, ev.Severity)
			} else {
				dispErr = jsonErr
			}
		}

		if dispErr != nil {
			slog.Warn("incidents: dispatch error", "integration", integration, "err", dispErr)
			continue
		}
		if extRef != "" {
			// Update external_ref on the incident (append if multiple integrations).
			_ = d.ch.Exec(ctx,
				`INSERT INTO incidents
				 (id, title, severity, status, source, source_id, notes, external_ref, created_at, updated_at, version)
				 SELECT id, title, severity, status, source, source_id, notes,
				        if(external_ref = '', ?, concat(external_ref, ', ', ?)),
				        created_at, now64(), toUInt64(toUnixTimestamp64Milli(now64()))
				 FROM incidents FINAL WHERE id = ?`,
				extRef, extRef, incidentID,
			)
		}
		slog.Info("incidents: dispatched", "integration", integration, "ref", extRef)
	}
}

// loadConfigs returns the config JSON for each enabled integration.
func (d *Dispatcher) loadConfigs(ctx context.Context) (map[string]string, error) {
	rows, err := d.ch.Query(ctx,
		`SELECT integration, config
		 FROM incident_workflow_config FINAL
		 WHERE enabled = 1`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make(map[string]string)
	for rows.Next() {
		var integration, config string
		if err := rows.Scan(&integration, &config); err != nil {
			continue
		}
		out[integration] = config
	}
	return out, nil
}

// ── PagerDuty Events v2 ───────────────────────────────────────────────────────

type pdConfig struct {
	RoutingKey string `json:"routing_key"`
}

func (d *Dispatcher) firePagerDuty(rawCfg, title, description, severity string) (string, error) {
	var cfg pdConfig
	if err := json.Unmarshal([]byte(rawCfg), &cfg); err != nil {
		return "", err
	}

	pdSeverity := "warning"
	switch severity {
	case "critical":
		pdSeverity = "critical"
	case "high":
		pdSeverity = "error"
	case "low":
		pdSeverity = "info"
	}

	payload := map[string]any{
		"routing_key":  cfg.RoutingKey,
		"event_action": "trigger",
		"payload": map[string]any{
			"summary":   title,
			"source":    "netscope-hub",
			"severity":  pdSeverity,
			"custom_details": map[string]string{
				"description": description,
			},
		},
	}

	body, _ := json.Marshal(payload)
	resp, err := http.Post(
		"https://events.pagerduty.com/v2/enqueue",
		"application/json",
		bytes.NewReader(body),
	)
	if err != nil {
		return "", fmt.Errorf("pagerduty: %w", err)
	}
	defer resp.Body.Close()

	var result struct {
		DedupKey string `json:"dedup_key"`
	}
	respBody, _ := io.ReadAll(resp.Body)
	_ = json.Unmarshal(respBody, &result)
	if resp.StatusCode != http.StatusAccepted && resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("pagerduty %d: %s", resp.StatusCode, string(respBody))
	}
	return "pd:" + result.DedupKey, nil
}

// ── OpsGenie Alert API v2 ─────────────────────────────────────────────────────

type ogConfig struct {
	APIKey string `json:"api_key"`
	Region string `json:"region"` // "eu" | "" (US default)
}

func (d *Dispatcher) fireOpsGenie(rawCfg, title, description, severity string) (string, error) {
	var cfg ogConfig
	if err := json.Unmarshal([]byte(rawCfg), &cfg); err != nil {
		return "", err
	}

	endpoint := "https://api.opsgenie.com/v2/alerts"
	if strings.ToLower(cfg.Region) == "eu" {
		endpoint = "https://api.eu.opsgenie.com/v2/alerts"
	}

	ogPriority := "P3"
	switch severity {
	case "critical":
		ogPriority = "P1"
	case "high":
		ogPriority = "P2"
	case "low":
		ogPriority = "P5"
	}

	payload := map[string]any{
		"message":     title,
		"description": description,
		"priority":    ogPriority,
		"tags":        []string{"netscope", "security"},
		"source":      "netscope-hub",
	}

	body, _ := json.Marshal(payload)
	req, err := http.NewRequest("POST", endpoint, bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "GenieKey "+cfg.APIKey)

	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("opsgenie: %w", err)
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusAccepted && resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		return "", fmt.Errorf("opsgenie %d: %s", resp.StatusCode, string(respBody))
	}

	var result struct {
		RequestID string `json:"requestId"`
	}
	_ = json.Unmarshal(respBody, &result)
	return "og:" + result.RequestID, nil
}
