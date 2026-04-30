package sso

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/base64"
	"encoding/pem"
	"encoding/xml"
	"log/slog"
	"math/big"
	"net/url"
	"strings"
	"sync"
	"time"

	cssaml "github.com/crewjam/saml"
	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"

	"github.com/netscope/hub-api/clickhouse"
	"github.com/netscope/hub-api/enterprise/license"
	"github.com/netscope/hub-api/sessions"
)

// SAMLHandler manages the SAML 2.0 SP-initiated login flow.
// An ephemeral RSA key is generated at startup; log the SP certificate so
// operators can register it with the IdP.
type SAMLHandler struct {
	CH           *clickhouse.Client
	Sessions     *sessions.Store
	License      *license.License
	AppURL       string
	FrontendURL  string
	SecureCookie bool // add Secure flag to session cookies in production

	spKey  *rsa.PrivateKey
	spCert *x509.Certificate

	mu     sync.Mutex
	states map[string]pendingState // relay state → post-login redirect + expiry
}

// NewSAMLHandler creates a SAMLHandler, generates an ephemeral SP key pair,
// and logs the SP certificate for IdP registration.
func NewSAMLHandler(ch *clickhouse.Client, sess *sessions.Store, lic *license.License,
	appURL, frontendURL string, secureCookie bool) *SAMLHandler {

	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		slog.Error("saml: key generation failed", "err", err)
		return nil
	}

	template := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: "NetScope SAML SP"},
		NotBefore:    time.Now().Add(-time.Minute),
		NotAfter:     time.Now().Add(10 * 365 * 24 * time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature,
	}
	certDER, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	if err != nil {
		slog.Error("saml: certificate generation failed", "err", err)
		return nil
	}
	cert, err := x509.ParseCertificate(certDER)
	if err != nil {
		slog.Error("saml: certificate parse failed", "err", err)
		return nil
	}

	// Log the PEM certificate so the operator can register it in the IdP.
	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certDER})
	slog.Info("SAML SP certificate generated — register with your IdP:\n" + string(certPEM))

	h := &SAMLHandler{
		CH:           ch,
		Sessions:     sess,
		License:      lic,
		AppURL:       appURL,
		FrontendURL:  frontendURL,
		SecureCookie: secureCookie,
		spKey:        key,
		spCert:       cert,
		states:       make(map[string]pendingState),
	}
	go h.cleanupStates()
	return h
}

func (h *SAMLHandler) cleanupStates() {
	t := time.NewTicker(5 * time.Minute)
	defer t.Stop()
	for range t.C {
		h.mu.Lock()
		for k, v := range h.states {
			if time.Now().After(v.expiry) {
				delete(h.states, k)
			}
		}
		h.mu.Unlock()
	}
}

func (h *SAMLHandler) storeRelayState(redirectURI string) (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	nonce := base64.RawURLEncoding.EncodeToString(b)
	h.mu.Lock()
	h.states[nonce] = pendingState{redirectURI: redirectURI, expiry: time.Now().Add(stateExpiry)}
	h.mu.Unlock()
	return nonce, nil
}

func (h *SAMLHandler) consumeRelayState(state string) (pendingState, bool) {
	h.mu.Lock()
	defer h.mu.Unlock()
	ps, ok := h.states[state]
	if !ok || time.Now().After(ps.expiry) {
		delete(h.states, state)
		return pendingState{}, false
	}
	delete(h.states, state)
	return ps, true
}

// loadSAMLConfig fetches SAML IdP settings from ClickHouse.
func (h *SAMLHandler) loadSAMLConfig(ctx context.Context) (entityID, ssoURL, cert string, enabled bool, err error) {
	rows, err := h.CH.Query(ctx,
		`SELECT entity_id, sso_url, certificate, enabled
		 FROM sso_configs
		 WHERE org_id = 'default' AND provider = 'saml'
		 ORDER BY updated_at DESC
		 LIMIT 1`)
	if err != nil {
		return "", "", "", false, err
	}
	defer rows.Close()
	if !rows.Next() {
		return "", "", "", false, nil
	}
	var e uint8
	_ = rows.Scan(&entityID, &ssoURL, &cert, &e)
	return entityID, ssoURL, cert, e == 1, nil
}

// buildSP constructs a crewjam ServiceProvider from the stored IdP config.
func (h *SAMLHandler) buildSP(idpEntityID, idpSSOURL, idpCertPEM string) (*cssaml.ServiceProvider, error) {
	acsURL, err := url.Parse(h.AppURL + "/api/v1/enterprise/auth/saml/callback")
	if err != nil {
		return nil, err
	}
	metaURL, err := url.Parse(h.AppURL + "/saml/metadata")
	if err != nil {
		return nil, err
	}

	// Strip PEM header/footer from the IdP certificate; crewjam wants raw base64.
	rawCert := strings.TrimSpace(idpCertPEM)
	rawCert = strings.TrimPrefix(rawCert, "-----BEGIN CERTIFICATE-----")
	rawCert = strings.TrimSuffix(rawCert, "-----END CERTIFICATE-----")
	rawCert = strings.ReplaceAll(rawCert, "\n", "")
	rawCert = strings.TrimSpace(rawCert)

	idpMeta := &cssaml.EntityDescriptor{
		EntityID: idpEntityID,
		IDPSSODescriptors: []cssaml.IDPSSODescriptor{
			{
				SSODescriptor: cssaml.SSODescriptor{
					RoleDescriptor: cssaml.RoleDescriptor{
						ProtocolSupportEnumeration: "urn:oasis:names:tc:SAML:2.0:protocol",
						KeyDescriptors: []cssaml.KeyDescriptor{
							{
								Use: "signing",
								KeyInfo: cssaml.KeyInfo{
									X509Data: cssaml.X509Data{
										X509Certificates: []cssaml.X509Certificate{
											{Data: rawCert},
										},
									},
								},
							},
						},
					},
				},
				SingleSignOnServices: []cssaml.Endpoint{
					{
						Binding:  cssaml.HTTPRedirectBinding,
						Location: idpSSOURL,
					},
				},
			},
		},
	}

	sp := &cssaml.ServiceProvider{
		EntityID:              h.AppURL + "/saml/metadata",
		Key:                   h.spKey,
		Certificate:           h.spCert,
		MetadataURL:           *metaURL,
		AcsURL:                *acsURL,
		IDPMetadata:           idpMeta,
		AuthnNameIDFormat:     cssaml.EmailAddressNameIDFormat,
		AllowIDPInitiated:     false,
		MetadataValidDuration: 48 * time.Hour,
	}
	return sp, nil
}

// ── Initiate ──────────────────────────────────────────────────────────────────

// Initiate handles GET /api/v1/enterprise/auth/saml/initiate?redirect_uri=<url>
func (h *SAMLHandler) Initiate(c *fiber.Ctx) error {
	if !h.License.HasFeature(license.FeatureSSO) {
		return c.Status(fiber.StatusPaymentRequired).JSON(fiber.Map{
			"error": "SSO requires Enterprise plan", "upgrade": true,
		})
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	entityID, ssoURL, cert, enabled, err := h.loadSAMLConfig(ctx)
	if err != nil {
		slog.Error("saml: loadSAMLConfig failed", "err", err)
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "SSO config unavailable"})
	}
	if !enabled || ssoURL == "" {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "SAML SSO is not configured or disabled"})
	}

	sp, err := h.buildSP(entityID, ssoURL, cert)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "SP configuration error"})
	}

	frontendDest := c.Query("redirect_uri", h.FrontendURL+"/")
	relayState, err := h.storeRelayState(frontendDest)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "state generation failed"})
	}

	redirectURL, err := sp.MakeRedirectAuthenticationRequest(relayState)
	if err != nil {
		slog.Error("saml: MakeRedirectAuthenticationRequest failed", "err", err)
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "could not build SAML request"})
	}

	return c.Redirect(redirectURL.String(), fiber.StatusFound)
}

// ── ACS (Callback) ────────────────────────────────────────────────────────────

// Callback handles POST /api/v1/enterprise/auth/saml/callback
// The IdP POSTs a SAMLResponse form field to this endpoint.
func (h *SAMLHandler) Callback(c *fiber.Ctx) error {
	relayState := c.FormValue("RelayState")
	samlResponseB64 := c.FormValue("SAMLResponse")
	if samlResponseB64 == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "SAMLResponse missing"})
	}

	ps, ok := h.consumeRelayState(relayState)
	if !ok {
		slog.Warn("saml: invalid or expired relay state")
		// Allow the login to proceed without stored state for IdP-initiated flows,
		// but redirect to the default frontend URL.
		ps = pendingState{redirectURI: h.FrontendURL + "/"}
	}

	decoded, err := base64.StdEncoding.DecodeString(samlResponseB64)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "SAMLResponse base64 decode failed"})
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	entityID, ssoURL, cert, enabled, err := h.loadSAMLConfig(ctx)
	if err != nil || !enabled || ssoURL == "" {
		return c.Status(fiber.StatusServiceUnavailable).JSON(fiber.Map{"error": "SSO not available"})
	}

	sp, err := h.buildSP(entityID, ssoURL, cert)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "SP configuration error"})
	}

	acsURL, _ := url.Parse(h.AppURL + "/api/v1/enterprise/auth/saml/callback")
	assertion, err := sp.ParseXMLResponse(decoded, []string{}, *acsURL)
	if err != nil {
		slog.Warn("saml: ParseXMLResponse failed", "err", err)
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "SAML assertion validation failed"})
	}

	email, displayName := extractSAMLClaims(assertion)
	if email == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "email claim missing from SAML assertion"})
	}

	userID, role, err := h.upsertSAMLUser(ctx, assertion.Subject.NameID.Value, email, displayName)
	if err != nil {
		slog.Error("saml: upsertUser failed", "err", err)
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "user record could not be created"})
	}

	expiresAt := time.Now().Add(sessions.DefaultTTL)
	sessionToken := h.Sessions.Create(sessions.Session{
		UserID:      userID,
		OrgID:       "default",
		Email:       email,
		DisplayName: displayName,
		Role:        role,
		SSOProvider: "saml",
		CreatedAt:   time.Now(),
		ExpiresAt:   expiresAt,
	})

	c.Cookie(&fiber.Cookie{
		Name:     sessionCookieName,
		Value:    sessionToken,
		Path:     "/",
		HTTPOnly: true,
		SameSite: "Lax",
		Secure:   h.SecureCookie,
		Expires:  expiresAt,
	})

	slog.Info("SAML login successful", "email", email, "role", role)

	dest := ps.redirectURI
	if dest == "" {
		dest = h.FrontendURL + "/"
	}
	return c.Redirect(dest, fiber.StatusFound)
}

// ── SP Metadata ───────────────────────────────────────────────────────────────

// Metadata serves GET /saml/metadata — the SP descriptor that IdPs import.
func (h *SAMLHandler) Metadata(c *fiber.Ctx) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	entityID, ssoURL, cert, _, err := h.loadSAMLConfig(ctx)
	if err != nil || ssoURL == "" {
		// Return minimal metadata even without IdP config.
		entityID = ""
		cert = ""
	}

	sp, err := h.buildSP(entityID, ssoURL, cert)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "SP configuration error"})
	}

	meta := sp.Metadata()
	xmlBytes, err := xml.MarshalIndent(meta, "", "  ")
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "metadata serialisation failed"})
	}
	c.Set("Content-Type", "application/samlmetadata+xml")
	return c.Send(xmlBytes)
}

// ── Helpers ───────────────────────────────────────────────────────────────────

// extractSAMLClaims returns email and display name from a SAML assertion.
// Checks the NameID (if email format) and common attribute names.
func extractSAMLClaims(a *cssaml.Assertion) (email, displayName string) {
	// NameID may carry the email directly.
	if a.Subject != nil && a.Subject.NameID != nil {
		if strings.Contains(a.Subject.NameID.Value, "@") {
			email = a.Subject.NameID.Value
		}
	}

	for _, stmt := range a.AttributeStatements {
		for _, attr := range stmt.Attributes {
			name := strings.ToLower(attr.Name)
			val := ""
			if len(attr.Values) > 0 {
				val = attr.Values[0].Value
			}
			switch {
			case email == "" && (name == "email" ||
				name == "emailaddress" ||
				strings.HasSuffix(name, "/emailaddress") ||
				strings.HasSuffix(name, "/upn")):
				email = val
			case displayName == "" && (name == "displayname" ||
				name == "name" ||
				name == "cn" ||
				strings.HasSuffix(name, "/name")):
				displayName = val
			}
		}
	}
	return email, displayName
}

// upsertSAMLUser finds or creates an org_members row for a SAML-authenticated user.
func (h *SAMLHandler) upsertSAMLUser(ctx context.Context, nameID, email, displayName string) (userID, role string, err error) {
	// Look up by SAML NameID first.
	rows, qErr := h.CH.Query(ctx,
		`SELECT user_id, role FROM org_members
		 WHERE org_id = 'default' AND sso_provider = 'saml' AND sso_subject = ?
		   AND is_active = 1
		 ORDER BY last_seen DESC LIMIT 1`, nameID)
	if qErr == nil {
		if rows.Next() {
			_ = rows.Scan(&userID, &role)
		}
		rows.Close()
	}

	// Fall back to email match.
	if userID == "" {
		rows2, qErr2 := h.CH.Query(ctx,
			`SELECT user_id, role FROM org_members
			 WHERE org_id = 'default' AND email = ? AND is_active = 1
			 ORDER BY last_seen DESC LIMIT 1`, email)
		if qErr2 == nil {
			if rows2.Next() {
				_ = rows2.Scan(&userID, &role)
			}
			rows2.Close()
		}
	}

	if userID == "" {
		userID = uuid.NewString()
		role = "viewer"
	}
	if displayName == "" {
		displayName = email
	}

	now := time.Now()
	err = h.CH.Exec(ctx,
		`INSERT INTO org_members
		 (user_id, org_id, email, display_name, role,
		  sso_provider, sso_subject, is_active, created_at, last_seen, version)
		 VALUES (?, 'default', ?, ?, ?, 'saml', ?, 1, ?, ?, ?)`,
		userID, email, displayName, role, nameID,
		now, now, now.UnixMilli(),
	)
	return userID, role, err
}
