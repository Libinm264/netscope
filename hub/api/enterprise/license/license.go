// Enterprise Edition — see hub/enterprise/LICENSE (BSL-1.1)
package license

import (
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// DefaultDevKey is used when ENTERPRISE_LICENSE_SIGNING_KEY is not set.
// This allows local development without a real license key.
// MUST be overridden in production deployments.
const DefaultDevKey = "netscope-dev-license-signing-key-change-in-production"

// Plan constants.
const (
	PlanCommunity  = "community"
	PlanTeam       = "team"
	PlanEnterprise = "enterprise"
)

// Feature flag constants — used with License.HasFeature().
const (
	FeatureSSO             = "sso"
	FeatureMultiTenant     = "multi_tenant"
	FeatureSCIM            = "scim"
	FeatureAuditExport     = "audit_export"
	FeatureCustomRetention = "custom_retention"
	FeatureCustomRoles     = "custom_roles"
	FeaturePIIRedaction    = "pii_redaction"
	FeatureOTelCorrelation = "otel_correlation"

	// v0.5 features
	FeatureCloudIngestGCP   = "cloud_ingest_gcp"   // GCP Pub/Sub VPC flow ingestion
	FeatureCloudIngestAzure = "cloud_ingest_azure"  // Azure NSG flow ingestion
	FeatureComplianceReports = "compliance_reports" // SOC2/PCI/HIPAA scheduled PDF reports
	FeatureIncidentWorkflow  = "incident_workflow"  // Jira/Linear/PD/OpsGenie incident routing
	FeatureWindowsAgent      = "windows_agent"      // Npcap-based Windows capture agent
)

// LicenseClaims is the JWT payload embedded in a NetScope license key.
type LicenseClaims struct {
	jwt.RegisteredClaims
	OrgID      string   `json:"org_id"`
	OrgName    string   `json:"org_name"`
	Plan       string   `json:"plan"`
	AgentQuota int      `json:"agent_quota"` // -1 = unlimited
	Features   []string `json:"features"`
}

// License is the parsed, validated result of a license key.
type License struct {
	Valid      bool
	Expired    bool
	Plan       string
	OrgID      string
	OrgName    string
	AgentQuota int
	Features   map[string]bool
	ExpiresAt  time.Time
	Raw        string // the original key string, for display
}

// CommunityLicense is returned when no license key is configured.
var CommunityLicense = &License{
	Valid:      true,
	Plan:       PlanCommunity,
	OrgID:      "default",
	OrgName:    "Default Organisation",
	AgentQuota: 10,
	Features:   map[string]bool{},
}

// HasFeature reports whether the license allows the named feature.
// Enterprise plans implicitly include all features.
func (l *License) HasFeature(feature string) bool {
	if !l.Valid || l.Expired {
		return false
	}
	if l.Plan == PlanEnterprise {
		return true
	}
	return l.Features[feature]
}

// AgentAllowed reports whether adding another agent is within quota.
// agentCount is the current number of registered agents.
func (l *License) AgentAllowed(agentCount int) bool {
	if l.AgentQuota < 0 {
		return true // unlimited
	}
	return agentCount < l.AgentQuota
}

// PlanBadge returns a short display label for the plan.
func (l *License) PlanBadge() string {
	switch l.Plan {
	case PlanTeam:
		return "Team"
	case PlanEnterprise:
		return "Enterprise"
	default:
		return "Community"
	}
}

// Parse validates and decodes a license key JWT.
// signingKey is the HMAC-SHA256 secret used to verify the signature.
// If keyStr is empty, CommunityLicense is returned.
func Parse(keyStr, signingKey string) *License {
	if keyStr == "" {
		c := *CommunityLicense
		return &c
	}
	if signingKey == "" {
		signingKey = DefaultDevKey
	}

	token, err := jwt.ParseWithClaims(
		keyStr,
		&LicenseClaims{},
		func(t *jwt.Token) (interface{}, error) {
			if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
				return nil, fmt.Errorf("unexpected signing method: %v", t.Header["alg"])
			}
			return []byte(signingKey), nil
		},
	)
	if err != nil {
		return &License{Valid: false, Plan: "invalid", Raw: keyStr}
	}

	claims, ok := token.Claims.(*LicenseClaims)
	if !ok || !token.Valid {
		return &License{Valid: false, Plan: "invalid", Raw: keyStr}
	}

	features := make(map[string]bool, len(claims.Features))
	for _, f := range claims.Features {
		features[f] = true
	}

	expiry := time.Time{}
	if claims.ExpiresAt != nil {
		expiry = claims.ExpiresAt.Time
	}
	expired := !expiry.IsZero() && time.Now().After(expiry)

	quota := claims.AgentQuota
	if quota == 0 {
		quota = 10
	}

	return &License{
		Valid:      !expired,
		Expired:    expired,
		Plan:       claims.Plan,
		OrgID:      claims.OrgID,
		OrgName:    claims.OrgName,
		AgentQuota: quota,
		Features:   features,
		ExpiresAt:  expiry,
		Raw:        keyStr,
	}
}
