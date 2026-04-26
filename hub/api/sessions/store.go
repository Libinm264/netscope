// Package sessions provides a lightweight in-memory session store for the
// NetScope Hub enterprise authentication layer.
//
// Sessions are keyed by a random UUID token issued after a successful
// OIDC or SAML callback. The token is stored as an httpOnly cookie on the
// browser and forwarded by the Next.js proxy to the Go backend for validation.
//
// The store is intentionally simple — no external dependency, no persistence.
// On hub restart users must re-authenticate, which is acceptable for enterprise
// deployments where SSO makes re-login frictionless.
package sessions

import (
	"sync"
	"time"

	"github.com/google/uuid"
)

// DefaultTTL is how long a session remains valid after creation.
const DefaultTTL = 24 * time.Hour

// Session holds the identity resolved after SSO authentication.
type Session struct {
	Token       string
	UserID      string
	OrgID       string
	Email       string
	DisplayName string
	Role        string    // "owner" | "admin" | "analyst" | "viewer"
	SSOProvider string    // "oidc" | "saml"
	CreatedAt   time.Time
	ExpiresAt   time.Time
}

// Store is a thread-safe in-memory map of token → Session.
type Store struct {
	mu   sync.RWMutex
	data map[string]*Session
}

// NewStore creates a Store and starts the background expiry goroutine.
func NewStore() *Store {
	s := &Store{data: make(map[string]*Session)}
	go s.runCleanup()
	return s
}

// Create stores the session and returns the new token.
// The token is a random UUID; the caller must set it as a cookie.
func (s *Store) Create(sess Session) string {
	sess.Token = uuid.NewString()
	if sess.CreatedAt.IsZero() {
		sess.CreatedAt = time.Now().UTC()
	}
	if sess.ExpiresAt.IsZero() {
		sess.ExpiresAt = sess.CreatedAt.Add(DefaultTTL)
	}
	s.mu.Lock()
	s.data[sess.Token] = &sess
	s.mu.Unlock()
	return sess.Token
}

// Get returns the session for the given token, or (nil, false) if it is
// absent or expired.
func (s *Store) Get(token string) (*Session, bool) {
	s.mu.RLock()
	sess, ok := s.data[token]
	s.mu.RUnlock()
	if !ok {
		return nil, false
	}
	if time.Now().After(sess.ExpiresAt) {
		s.Delete(token)
		return nil, false
	}
	return sess, true
}

// Delete removes a session (used on logout or forced expiry).
func (s *Store) Delete(token string) {
	s.mu.Lock()
	delete(s.data, token)
	s.mu.Unlock()
}

// runCleanup evicts expired sessions every 15 minutes to prevent
// unbounded memory growth in long-running deployments.
func (s *Store) runCleanup() {
	ticker := time.NewTicker(15 * time.Minute)
	defer ticker.Stop()
	for range ticker.C {
		now := time.Now()
		s.mu.Lock()
		for k, v := range s.data {
			if now.After(v.ExpiresAt) {
				delete(s.data, k)
			}
		}
		s.mu.Unlock()
	}
}
