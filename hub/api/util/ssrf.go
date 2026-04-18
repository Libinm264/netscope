// Package util provides shared helper utilities for hub API handlers.
package util

import (
	"fmt"
	"net"
	"net/url"
)

// blockedCIDRs contains all IP ranges that must never receive outbound webhook
// requests (RFC 1918 private networks, loopback, link-local, cloud metadata,
// carrier-grade NAT, and IPv6 equivalents).
var blockedCIDRs = func() []*net.IPNet {
	ranges := []string{
		"10.0.0.0/8",      // RFC 1918 private
		"172.16.0.0/12",   // RFC 1918 private
		"192.168.0.0/16",  // RFC 1918 private
		"127.0.0.0/8",     // Loopback
		"169.254.0.0/16",  // Link-local / AWS EC2 metadata
		"100.64.0.0/10",   // Carrier-grade NAT (RFC 6598)
		"192.0.0.0/24",    // IETF protocol assignments
		"198.18.0.0/15",   // Benchmark testing
		"198.51.100.0/24", // TEST-NET-2 (documentation)
		"203.0.113.0/24",  // TEST-NET-3 (documentation)
		"240.0.0.0/4",     // Reserved
		"0.0.0.0/8",       // This network
		"::1/128",          // IPv6 loopback
		"fc00::/7",         // IPv6 unique-local (ULA)
		"fe80::/10",        // IPv6 link-local
		"::ffff:0:0/96",    // IPv4-mapped IPv6 (extra safety)
	}
	nets := make([]*net.IPNet, 0, len(ranges))
	for _, r := range ranges {
		_, n, err := net.ParseCIDR(r)
		if err == nil {
			nets = append(nets, n)
		}
	}
	return nets
}()

// ValidateWebhookURL returns an error if rawURL is not a safe, publicly-routable
// HTTP/HTTPS destination.
//
// It checks:
//   - Scheme is http or https
//   - Host is non-empty
//   - Literal IP addresses are not in reserved/private ranges
//   - Resolved DNS IPs are not in reserved/private ranges
//
// Note: DNS-rebinding attacks are mitigated by also validating at delivery time
// in delivery.go (defence-in-depth).  For full protection, deploy with a DNS
// resolver that filters RFC 1918 responses.
func ValidateWebhookURL(rawURL string) error {
	if rawURL == "" {
		return nil // empty = disabled, not an error
	}

	u, err := url.Parse(rawURL)
	if err != nil {
		return fmt.Errorf("invalid webhook URL: %w", err)
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return fmt.Errorf("webhook URL must use http or https scheme (got %q)", u.Scheme)
	}
	host := u.Hostname()
	if host == "" {
		return fmt.Errorf("webhook URL has no host")
	}

	// Bare IP literal — check immediately without DNS lookup
	if ip := net.ParseIP(host); ip != nil {
		if isBlockedIP(ip) {
			return fmt.Errorf("webhook URL points to a reserved/private IP address")
		}
		return nil
	}

	// Resolve hostname — guard against SSRF via internal DNS names
	addrs, err := net.LookupHost(host)
	if err != nil {
		return fmt.Errorf("webhook host %q could not be resolved: %w", host, err)
	}
	for _, raw := range addrs {
		ip := net.ParseIP(raw)
		if ip == nil {
			continue
		}
		if isBlockedIP(ip) {
			return fmt.Errorf("webhook host %q resolves to a reserved/private address (%s)", host, raw)
		}
	}
	return nil
}

// isBlockedIP reports whether ip falls within any of the reserved/private
// IP ranges defined in blockedCIDRs.
func isBlockedIP(ip net.IP) bool {
	for _, cidr := range blockedCIDRs {
		if cidr.Contains(ip) {
			return true
		}
	}
	return false
}
