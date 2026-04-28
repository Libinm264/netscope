package handlers

import (
	"context"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"

	"github.com/netscope/hub-api/clickhouse"
	"github.com/netscope/hub-api/models"
	"github.com/netscope/hub-api/util"
)

// CertHandler provides the TLS certificate fleet endpoints.
type CertHandler struct {
	CH *clickhouse.Client
}

// List handles GET /api/v1/certs.
// Returns all unique TLS certificates seen across the fleet, sorted by expiry (soonest first).
func (h *CertHandler) List(c *fiber.Ctx) error {
	if h.CH == nil {
		return c.Status(503).JSON(fiber.Map{"error": "ClickHouse unavailable"})
	}

	rows, err := h.CH.Query(c.Context(), `
		SELECT
			fingerprint, cn, issuer, expiry, expired,
			sans, agent_id, hostname, src_ip, dst_ip,
			first_seen, last_seen
		FROM tls_certs
		ORDER BY expiry ASC
		LIMIT 500
	`)
	if err != nil {
		return util.InternalError(c, err)
	}
	defer rows.Close()

	certs := make([]models.TlsCert, 0)
	now := time.Now().UTC()

	for rows.Next() {
		var cert models.TlsCert
		var expiredInt uint8
		var sansStr string
		if err := rows.Scan(
			&cert.Fingerprint, &cert.CN, &cert.Issuer, &cert.Expiry, &expiredInt,
			&sansStr, &cert.AgentID, &cert.Hostname, &cert.SrcIP, &cert.DstIP,
			&cert.FirstSeen, &cert.LastSeen,
		); err != nil {
			continue
		}
		cert.Expired = expiredInt == 1
		if sansStr != "" {
			cert.SANs = strings.Split(sansStr, ",")
		}

		// Compute days left
		if cert.Expiry != "" {
			if expDate, err := time.Parse("2006-01-02", cert.Expiry); err == nil {
				cert.DaysLeft = int(expDate.Sub(now).Hours() / 24)
			}
		}
		certs = append(certs, cert)
	}

	// Summary counts
	expired, critical, warning, ok := 0, 0, 0, 0
	for i := range certs {
		switch certs[i].Status() {
		case models.CertStatusExpired:
			expired++
		case models.CertStatusCritical:
			critical++
		case models.CertStatusWarning:
			warning++
		case models.CertStatusOK:
			ok++
		}
	}

	return c.JSON(fiber.Map{
		"certs":   certs,
		"total":   len(certs),
		"summary": fiber.Map{"expired": expired, "critical": critical, "warning": warning, "ok": ok},
	})
}

// ExtractAndStoreCert is called from the ingest path when a TLS Certificate flow arrives.
// It upserts the cert record into tls_certs via the ClickHouse Writer.
func ExtractAndStoreCert(ch *clickhouse.Client, flow models.Flow) {
	if ch == nil || flow.TLS == nil {
		return
	}
	tls := flow.TLS
	if tls.RecordType != "Certificate" || tls.CertCN == "" {
		return
	}

	fp := tls.CertCN + "|" + tls.CertExpiry // simple dedup fingerprint
	sansStr := strings.Join(tls.CertSANs, ",")
	var expiredInt uint8
	if tls.CertExpired {
		expiredInt = 1
	}
	now := time.Now().UTC()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_ = ch.Exec(
		ctx,
		`INSERT INTO tls_certs
		 (fingerprint, cn, issuer, expiry, expired, sans,
		  agent_id, hostname, src_ip, dst_ip, first_seen, last_seen)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		fp, tls.CertCN, tls.CertIssuer, tls.CertExpiry, expiredInt, sansStr,
		flow.AgentID, flow.Hostname, flow.SrcIP, flow.DstIP, now, now,
	)
}
