package models

import "time"

// TlsCert is a deduplicated TLS certificate record assembled from Certificate
// handshake flows seen by any agent in the fleet.
type TlsCert struct {
	Fingerprint string    `json:"fingerprint"` // CN|expiry dedup key
	CN          string    `json:"cn"`
	Issuer      string    `json:"issuer"`
	Expiry      string    `json:"expiry"`      // YYYY-MM-DD
	Expired     bool      `json:"expired"`
	DaysLeft    int       `json:"days_left"`   // negative = already expired
	SANs        []string  `json:"sans"`
	AgentID     string    `json:"agent_id"`
	Hostname    string    `json:"hostname"`
	SrcIP       string    `json:"src_ip"`
	DstIP       string    `json:"dst_ip"`
	FirstSeen   time.Time `json:"first_seen"`
	LastSeen    time.Time `json:"last_seen"`
}

// CertStatus categorises a certificate by urgency.
type CertStatus string

const (
	CertStatusExpired  CertStatus = "expired"
	CertStatusCritical CertStatus = "critical" // ≤ 7 days
	CertStatusWarning  CertStatus = "warning"  // ≤ 30 days
	CertStatusOK       CertStatus = "ok"
)

// Status returns the urgency category for this certificate.
func (c *TlsCert) Status() CertStatus {
	if c.Expired || c.DaysLeft < 0 {
		return CertStatusExpired
	}
	if c.DaysLeft <= 7 {
		return CertStatusCritical
	}
	if c.DaysLeft <= 30 {
		return CertStatusWarning
	}
	return CertStatusOK
}
