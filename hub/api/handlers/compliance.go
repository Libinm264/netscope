package handlers

import (
	"fmt"
	"log/slog"
	"net"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"

	"github.com/netscope/hub-api/util"

	"github.com/netscope/hub-api/clickhouse"
	"github.com/netscope/hub-api/models"
)

// ComplianceHandler provides security and compliance audit endpoints.
type ComplianceHandler struct {
	CH *clickhouse.Client
}

// Summary handles GET /api/v1/compliance/summary.
// Returns a quick overview of compliance posture for the requested window.
func (h *ComplianceHandler) Summary(c *fiber.Ctx) error {
	if h.CH == nil {
		return c.Status(503).JSON(fiber.Map{"error": "ClickHouse unavailable"})
	}

	window := c.Query("window", "24h")
	interval := windowToInterval(window)

	rows, err := h.CH.Query(c.Context(), fmt.Sprintf(`
		SELECT
			count()                                                       AS total,
			countIf(NOT (
				src_ip LIKE '10.%%'  OR src_ip LIKE '172.16.%%' OR
				src_ip LIKE '172.17.%%' OR src_ip LIKE '172.18.%%' OR
				src_ip LIKE '172.19.%%' OR src_ip LIKE '172.2%%'  OR
				src_ip LIKE '172.30.%%' OR src_ip LIKE '172.31.%%' OR
				src_ip LIKE '192.168.%%' OR src_ip LIKE '127.%%'
			))                                                            AS external_conns
		FROM flows
		WHERE ts >= now() - INTERVAL %s
	`, interval))
	if err != nil {
		return util.InternalError(c, err)
	}

	var totalConns, externalConns uint64
	if rows.Next() {
		if err := rows.Scan(&totalConns, &externalConns); err != nil {
			return util.InternalError(c, err)
		}
	}
	rows.Close()

	// TLS issues: only count certs that are actually expired
	certRows, err := h.CH.Query(c.Context(), `
		SELECT count() FROM tls_certs
		WHERE expired = 1
	`)
	tlsIssues := 0
	if err == nil {
		if certRows.Next() {
			if err := certRows.Scan(&tlsIssues); err != nil {
				slog.Warn("compliance: tls_issues scan", "err", err)
			}
		}
		certRows.Close()
	}

	return c.JSON(models.ComplianceSummary{
		TotalConnections:    totalConns,
		ExternalConnections: externalConns,
		TLSIssues:           tlsIssues,
		Window:              window,
	})
}

// Connections handles GET /api/v1/compliance/connections — full audit log.
func (h *ComplianceHandler) Connections(c *fiber.Ctx) error {
	if h.CH == nil {
		return c.Status(503).JSON(fiber.Map{"error": "ClickHouse unavailable"})
	}

	window   := c.Query("window", "24h")
	srcIP    := c.Query("src_ip")
	dstIP    := c.Query("dst_ip")
	protocol := c.Query("protocol")
	external := c.QueryBool("external_only")
	limit    := c.QueryInt("limit", 200)

	interval := windowToInterval(window)
	where := fmt.Sprintf("ts >= now() - INTERVAL %s", interval)
	args := make([]interface{}, 0)

	if srcIP != "" {
		where += " AND src_ip = ?"
		args = append(args, srcIP)
	}
	if dstIP != "" {
		where += " AND dst_ip = ?"
		args = append(args, dstIP)
	}
	if protocol != "" {
		where += " AND protocol = ?"
		args = append(args, protocol)
	}

	args = append(args, uint64(limit))

	rows, err := h.CH.Query(c.Context(), fmt.Sprintf(`
		SELECT id, agent_id, hostname, ts, protocol,
		       src_ip, src_port, dst_ip, dst_port,
		       bytes_in, bytes_out, duration_ms, info,
		       process_name, pid
		FROM flows
		WHERE %s
		ORDER BY ts DESC
		LIMIT ?
	`, where), args...)
	if err != nil {
		return util.InternalError(c, err)
	}
	defer rows.Close()

	records := make([]models.ConnectionRecord, 0, limit)
	for rows.Next() {
		var r models.ConnectionRecord
		if err := rows.Scan(
			&r.ID, &r.AgentID, &r.Hostname, &r.Timestamp, &r.Protocol,
			&r.SrcIP, &r.SrcPort, &r.DstIP, &r.DstPort,
			&r.BytesIn, &r.BytesOut, &r.DurationMs, &r.Info,
			&r.ProcessName, &r.PID,
		); err != nil {
			continue
		}
		r.IsExternal = isExternalIP(r.DstIP)
		if external && !r.IsExternal {
			continue
		}
		records = append(records, r)
	}

	return c.JSON(fiber.Map{"connections": records, "window": window, "total": len(records)})
}

// TLSAudit handles GET /api/v1/compliance/tls — cert issues across the fleet.
func (h *ComplianceHandler) TLSAudit(c *fiber.Ctx) error {
	if h.CH == nil {
		return c.Status(503).JSON(fiber.Map{"error": "ClickHouse unavailable"})
	}

	rows, err := h.CH.Query(c.Context(), `
		SELECT fingerprint, cn, issuer, expiry, expired,
		       hostname, dst_ip, last_seen
		FROM tls_certs
		ORDER BY expiry ASC
		LIMIT 500
	`)
	if err != nil {
		return util.InternalError(c, err)
	}
	defer rows.Close()

	now := time.Now().UTC()
	records := make([]models.TLSAuditRecord, 0)

	for rows.Next() {
		var r models.TLSAuditRecord
		var expiredInt uint8
		if err := rows.Scan(
			&r.Fingerprint, &r.CN, &r.Issuer, &r.Expiry, &expiredInt,
			&r.Hostname, &r.DstIP, &r.LastSeen,
		); err != nil {
			continue
		}
		r.Expired = expiredInt == 1

		if r.Expiry != "" {
			if exp, err := time.Parse("2006-01-02", r.Expiry); err == nil {
				r.DaysLeft = int(exp.Sub(now).Hours() / 24)
			}
		}

		// Classify the issue
		switch {
		case r.Expired || r.DaysLeft < 0:
			r.Issue = "expired"
		case r.DaysLeft <= 7:
			r.Issue = "expiring_critical"
		case r.DaysLeft <= 30:
			r.Issue = "expiring_soon"
		case strings.Contains(strings.ToLower(r.Issuer), r.CN) ||
			r.Issuer == "" || r.Issuer == r.CN:
			r.Issue = "self_signed"
		default:
			r.Issue = "ok"
		}

		records = append(records, r)
	}

	return c.JSON(fiber.Map{"certs": records, "total": len(records)})
}

// TopTalkers handles GET /api/v1/compliance/top-talkers.
// Returns hosts with the highest outbound traffic — potential data exfiltration signal.
func (h *ComplianceHandler) TopTalkers(c *fiber.Ctx) error {
	if h.CH == nil {
		return c.Status(503).JSON(fiber.Map{"error": "ClickHouse unavailable"})
	}

	window := c.Query("window", "24h")
	interval := windowToInterval(window)

	rows, err := h.CH.Query(c.Context(), fmt.Sprintf(`
		SELECT
			src_ip,
			any(hostname)               AS hostname,
			sum(bytes_out)              AS bytes_out,
			sum(bytes_in)               AS bytes_in,
			count()                     AS flow_count,
			uniqExact(dst_ip)           AS unique_destinations
		FROM flows
		WHERE ts >= now() - INTERVAL %s
		GROUP BY src_ip
		ORDER BY bytes_out DESC
		LIMIT 20
	`, interval))
	if err != nil {
		return util.InternalError(c, err)
	}
	defer rows.Close()

	talkers := make([]models.TopTalker, 0, 20)
	for rows.Next() {
		var t models.TopTalker
		if err := rows.Scan(
			&t.IP, &t.Hostname, &t.BytesOut, &t.BytesIn,
			&t.FlowCount, &t.UniqueDestinations,
		); err != nil {
			continue
		}
		talkers = append(talkers, t)
	}

	return c.JSON(fiber.Map{"talkers": talkers, "window": window})
}

// ExternalConnections handles GET /api/v1/compliance/external.
// Shows all connections to non-RFC1918 destinations.
func (h *ComplianceHandler) ExternalConnections(c *fiber.Ctx) error {
	if h.CH == nil {
		return c.Status(503).JSON(fiber.Map{"error": "ClickHouse unavailable"})
	}

	window := c.Query("window", "24h")
	interval := windowToInterval(window)

	rows, err := h.CH.Query(c.Context(), fmt.Sprintf(`
		SELECT
			dst_ip,
			any(dst_port)               AS dst_port,
			any(protocol)               AS protocol,
			count()                     AS flow_count,
			sum(bytes_out)              AS bytes_out,
			max(ts)                     AS last_seen,
			groupUniqArray(5)(src_ip)   AS src_ips
		FROM flows
		WHERE ts >= now() - INTERVAL %s
		  AND NOT (
			dst_ip LIKE '10.%%'      OR
			dst_ip LIKE '172.16.%%'  OR dst_ip LIKE '172.17.%%' OR
			dst_ip LIKE '172.18.%%'  OR dst_ip LIKE '172.19.%%' OR
			dst_ip LIKE '172.2%%'    OR dst_ip LIKE '172.30.%%' OR
			dst_ip LIKE '172.31.%%'  OR
			dst_ip LIKE '192.168.%%' OR
			dst_ip LIKE '127.%%'     OR dst_ip = '::1'
		  )
		GROUP BY dst_ip
		ORDER BY bytes_out DESC
		LIMIT 100
	`, interval))
	if err != nil {
		return util.InternalError(c, err)
	}
	defer rows.Close()

	type ExternalDest struct {
		DstIP     string    `json:"dst_ip"`
		DstPort   uint16    `json:"dst_port"`
		Protocol  string    `json:"protocol"`
		FlowCount uint64    `json:"flow_count"`
		BytesOut  uint64    `json:"bytes_out"`
		LastSeen  time.Time `json:"last_seen"`
		SrcIPs    []string  `json:"src_ips"`
	}

	destinations := make([]ExternalDest, 0, 100)
	for rows.Next() {
		var d ExternalDest
		var srcIPsStr string
		if err := rows.Scan(
			&d.DstIP, &d.DstPort, &d.Protocol,
			&d.FlowCount, &d.BytesOut, &d.LastSeen, &srcIPsStr,
		); err != nil {
			continue
		}
		if srcIPsStr != "" && srcIPsStr != "[]" {
			d.SrcIPs = strings.Split(strings.Trim(srcIPsStr, "[]'"), "','")
		}
		destinations = append(destinations, d)
	}

	return c.JSON(fiber.Map{"destinations": destinations, "window": window})
}

// GeoSummary handles GET /api/v1/compliance/geo.
// Returns outbound connections broken down by destination country.
func (h *ComplianceHandler) GeoSummary(c *fiber.Ctx) error {
	if h.CH == nil {
		return c.Status(503).JSON(fiber.Map{"error": "ClickHouse unavailable"})
	}

	window := c.Query("window", "24h")
	interval := windowToInterval(window)

	rows, err := h.CH.Query(c.Context(), fmt.Sprintf(`
		SELECT
			country_code,
			any(country_name)        AS country_name,
			count()                  AS connections,
			sum(bytes_out)           AS bytes_out,
			uniqExact(src_ip)        AS unique_sources,
			max(threat_score)        AS max_threat_score
		FROM flows
		WHERE ts >= now() - INTERVAL %s
		  AND country_code != ''
		GROUP BY country_code
		ORDER BY connections DESC
		LIMIT 50
	`, interval))
	if err != nil {
		return util.InternalError(c, err)
	}
	defer rows.Close()

	type CountryRow struct {
		Code          string `json:"code"`
		Name          string `json:"name"`
		Connections   uint64 `json:"connections"`
		BytesOut      uint64 `json:"bytes_out"`
		UniqueSources uint64 `json:"unique_sources"`
		MaxThreat     uint8  `json:"max_threat_score"`
	}

	countries := make([]CountryRow, 0)
	for rows.Next() {
		var r CountryRow
		if err := rows.Scan(
			&r.Code, &r.Name, &r.Connections,
			&r.BytesOut, &r.UniqueSources, &r.MaxThreat,
		); err != nil {
			continue
		}
		countries = append(countries, r)
	}

	return c.JSON(fiber.Map{"countries": countries, "window": window, "total": len(countries)})
}

// ── helpers ───────────────────────────────────────────────────────────────────

func isExternalIP(ipStr string) bool {
	ip := net.ParseIP(ipStr)
	if ip == nil {
		return false
	}
	private := []string{
		"10.0.0.0/8", "172.16.0.0/12", "192.168.0.0/16",
		"127.0.0.0/8", "::1/128", "fc00::/7",
	}
	for _, cidr := range private {
		_, network, _ := net.ParseCIDR(cidr)
		if network != nil && network.Contains(ip) {
			return false
		}
	}
	return true
}
