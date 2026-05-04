package util

import (
	"net"
	"strings"
	"testing"
)

func TestValidateWebhookURL(t *testing.T) {
	tests := []struct {
		name    string
		url     string
		wantErr bool
		errFrag string // substring that must appear in the error, if wantErr
	}{
		// ── empty URL is valid (means "disabled") ──────────────────────────────
		{name: "empty URL is ok", url: "", wantErr: false},

		// ── RFC 1918 private IP ranges ─────────────────────────────────────────
		{name: "blocks 10.x", url: "https://10.0.0.1/hook", wantErr: true, errFrag: "reserved"},
		{name: "blocks 10.255.255.255", url: "https://10.255.255.255/hook", wantErr: true, errFrag: "reserved"},
		{name: "blocks 172.16.x", url: "https://172.16.0.1/hook", wantErr: true, errFrag: "reserved"},
		{name: "blocks 172.31.x", url: "https://172.31.255.255/hook", wantErr: true, errFrag: "reserved"},
		{name: "blocks 192.168.x", url: "https://192.168.1.1/hook", wantErr: true, errFrag: "reserved"},
		{name: "blocks 192.168.0.0", url: "https://192.168.0.0/hook", wantErr: true, errFrag: "reserved"},

		// ── loopback ──────────────────────────────────────────────────────────
		{name: "blocks 127.0.0.1", url: "https://127.0.0.1/hook", wantErr: true, errFrag: "reserved"},
		{name: "blocks 127.255.255.255", url: "https://127.255.255.255/hook", wantErr: true, errFrag: "reserved"},
		{name: "blocks IPv6 loopback ::1", url: "https://[::1]/hook", wantErr: true, errFrag: "reserved"},

		// ── link-local / metadata ─────────────────────────────────────────────
		{name: "blocks link-local 169.254.x", url: "https://169.254.1.1/hook", wantErr: true, errFrag: "reserved"},
		{name: "blocks AWS metadata 169.254.169.254", url: "https://169.254.169.254/latest/meta-data/", wantErr: true, errFrag: "reserved"},

		// ── carrier-grade NAT ─────────────────────────────────────────────────
		{name: "blocks CGNAT 100.64.x", url: "https://100.64.0.1/hook", wantErr: true, errFrag: "reserved"},

		// ── IPv6 ULA / link-local ─────────────────────────────────────────────
		{name: "blocks IPv6 ULA fc00::", url: "https://[fc00::1]/hook", wantErr: true, errFrag: "reserved"},
		{name: "blocks IPv6 link-local fe80::", url: "https://[fe80::1]/hook", wantErr: true, errFrag: "reserved"},

		// ── bad scheme ────────────────────────────────────────────────────────
		{name: "rejects ftp scheme", url: "ftp://hooks.example.com/foo", wantErr: true, errFrag: "scheme"},
		{name: "rejects file scheme", url: "file:///etc/passwd", wantErr: true, errFrag: "scheme"},

		// ── no host ───────────────────────────────────────────────────────────
		{name: "rejects empty host", url: "https:///path", wantErr: true, errFrag: "host"},

		// ── legitimate public IPs (no DNS required, deterministic) ───────────
		{name: "allows public IP 1.1.1.1 https", url: "https://1.1.1.1/hook", wantErr: false},
		{name: "allows public IP 8.8.8.8 https", url: "https://8.8.8.8/hook", wantErr: false},
		{name: "allows public IP 1.1.1.1 http", url: "http://1.1.1.1/hook", wantErr: false},

		// ── edge: addresses adjacent to (but outside) 172.16/12 ──────────────
		{name: "allows 172.15.x (not RFC1918)", url: "https://172.15.0.1/hook", wantErr: false},
		{name: "allows 172.32.x (not RFC1918)", url: "https://172.32.0.1/hook", wantErr: false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := ValidateWebhookURL(tc.url)
			if tc.wantErr && err == nil {
				t.Fatalf("expected error but got nil for URL %q", tc.url)
			}
			if !tc.wantErr && err != nil {
				t.Fatalf("expected no error but got %v for URL %q", err, tc.url)
			}
			if tc.wantErr && tc.errFrag != "" && !strings.Contains(err.Error(), tc.errFrag) {
				t.Fatalf("error %q does not contain expected fragment %q for URL %q", err.Error(), tc.errFrag, tc.url)
			}
		})
	}
}

// TestIsBlockedIP exercises the internal helper directly for completeness.
func TestIsBlockedIP(t *testing.T) {
	blocked := []string{
		"10.0.0.1",
		"172.16.0.1",
		"172.31.255.255",
		"192.168.1.1",
		"127.0.0.1",
		"169.254.169.254",
		"100.64.0.1",
		"::1",
		"fc00::1",
		"fe80::1",
	}
	allowed := []string{
		"1.1.1.1",
		"8.8.8.8",
		"172.15.0.1",
		"172.32.0.1",
		"203.0.114.1",  // outside 203.0.113/24 documentation block
	}

	for _, raw := range blocked {
		ip := net.ParseIP(raw)
		if ip == nil {
			t.Fatalf("failed to parse test IP %q", raw)
		}
		if !isBlockedIP(ip) {
			t.Errorf("expected %q to be blocked", raw)
		}
	}

	for _, raw := range allowed {
		ip := net.ParseIP(raw)
		if ip == nil {
			t.Fatalf("failed to parse test IP %q", raw)
		}
		if isBlockedIP(ip) {
			t.Errorf("expected %q to be allowed", raw)
		}
	}
}
