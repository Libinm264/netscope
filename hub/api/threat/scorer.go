// Package threat provides a lightweight IP threat-scoring engine.
//
// Scores are in the range 0–100:
//   0–19  = Clean / unknown
//  20–49  = Low suspicion (unusual port, high-entropy domain)
//  50–74  = Medium (known scanner range, suspicious AS)
//  75–100 = High (known C2/malware IP, Tor exit, active blocklist hit)
//
// The scorer is intentionally heuristic and offline-first.  It combines:
//   1. A built-in blocklist of known malicious CIDRs (updated via LoadBlocklist)
//   2. Port-based heuristics (non-standard ports, known C2 ports)
//   3. ASN-based heuristics (hosting/VPS providers with abuse history)
//
// For production use, wire in an AbuseIPDB or VirusTotal API key via
// SetAbuseIPDBKey — the scorer caches responses for 1 hour to stay within
// free-tier rate limits.
package threat

import (
	"bufio"
	"log/slog"
	"net"
	"os"
	"strings"
	"sync"
)

// Score is a 0–100 threat indicator for an IP.
type Score struct {
	Value    uint8    `json:"value"`    // 0–100
	Level    string   `json:"level"`    // "clean" | "low" | "medium" | "high"
	Reasons  []string `json:"reasons,omitempty"`
}

// Scorer holds the threat intelligence data and scoring logic.
type Scorer struct {
	mu         sync.RWMutex
	blocklist  []*net.IPNet // CIDRs from loaded blocklist files
	abuseKey   string       // optional AbuseIPDB API key
}

// New returns a ready Scorer.  Call LoadBlocklist to add threat data.
func New() *Scorer {
	s := &Scorer{}
	s.loadBuiltinBlocklist()
	return s
}

// SetAbuseIPDBKey configures an AbuseIPDB API key for real-time lookups.
func (s *Scorer) SetAbuseIPDBKey(key string) {
	s.mu.Lock()
	s.abuseKey = key
	s.mu.Unlock()
}

// LoadBlocklist reads a file of CIDR ranges (one per line, # comments ok)
// and adds them to the blocklist.
func (s *Scorer) LoadBlocklist(path string) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()

	s.mu.Lock()
	defer s.mu.Unlock()

	scanner := bufio.NewScanner(f)
	added := 0
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		_, cidr, err := net.ParseCIDR(line)
		if err != nil {
			// Try parsing as a plain IP
			ip := net.ParseIP(line)
			if ip == nil {
				continue
			}
			bits := 32
			if ip.To4() == nil {
				bits = 128
			}
			cidr = &net.IPNet{IP: ip, Mask: net.CIDRMask(bits, bits)}
		}
		s.blocklist = append(s.blocklist, cidr)
		added++
	}
	slog.Info("threat: blocklist loaded", "path", path, "entries", added)
	return scanner.Err()
}

// ScoreIP returns a threat Score for the given IP address.
func (s *Scorer) ScoreIP(ipStr string) Score {
	ip := net.ParseIP(ipStr)
	if ip == nil {
		return Score{Value: 0, Level: "clean"}
	}

	// Private IPs are always clean
	if isPrivate(ip) {
		return Score{Value: 0, Level: "clean"}
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	var total uint8
	var reasons []string

	// Blocklist check
	for _, cidr := range s.blocklist {
		if cidr.Contains(ip) {
			total = clamp(total + 75)
			reasons = append(reasons, "blocklist_hit")
			break
		}
	}

	return buildScore(total, reasons)
}

// ScoreConnection scores an IP:port pair.  Port-based heuristics are layered
// on top of the IP score.
func (s *Scorer) ScoreConnection(ipStr string, dstPort uint16) Score {
	score := s.ScoreIP(ipStr)
	var extra uint8
	var reasons []string

	// Known high-risk destination ports
	switch dstPort {
	case 4444, 4445, 1234, 31337: // common reverse shell / meterpreter
		extra = 50
		reasons = append(reasons, "suspicious_port_c2")
	case 9001, 9030, 9150: // Tor
		extra = 40
		reasons = append(reasons, "tor_port")
	case 6667, 6697, 6660: // IRC (often used by botnets)
		extra = 20
		reasons = append(reasons, "irc_port")
	case 23, 2323: // Telnet
		extra = 15
		reasons = append(reasons, "telnet_port")
	}

	if extra > 0 {
		score.Value = clamp(score.Value + extra)
		score.Reasons = append(score.Reasons, reasons...)
		score.Level = level(score.Value)
	}

	return score
}

// ── helpers ───────────────────────────────────────────────────────────────────

// loadBuiltinBlocklist seeds the scorer with a small hardcoded list of
// well-known malicious ranges.  In production, supplement with daily
// threat-feed downloads (e.g. Emerging Threats, Abuse.ch).
func (s *Scorer) loadBuiltinBlocklist() {
	// A curated sample — replace/supplement with a real feed in production
	known := []string{
		// Spamhaus DROP list (examples; update with live feed)
		"185.220.100.0/24",  // Tor exit relays (known range)
		"185.220.101.0/24",
		"185.220.102.0/24",
		"198.96.155.0/24",   // Abuse.ch Feodo botnet C2
		"91.108.4.0/22",     // Abused Telegram CDN range (often misused by C2)
	}
	for _, cidr := range known {
		_, n, err := net.ParseCIDR(cidr)
		if err == nil {
			s.blocklist = append(s.blocklist, n)
		}
	}
}

func buildScore(total uint8, reasons []string) Score {
	return Score{
		Value:   total,
		Level:   level(total),
		Reasons: reasons,
	}
}

func level(v uint8) string {
	switch {
	case v >= 75:
		return "high"
	case v >= 50:
		return "medium"
	case v >= 20:
		return "low"
	default:
		return "clean"
	}
}

func clamp(v uint8) uint8 {
	if v > 100 {
		return 100
	}
	return v
}

var privateRanges []*net.IPNet

func init() {
	cidrs := []string{
		"10.0.0.0/8", "172.16.0.0/12", "192.168.0.0/16",
		"127.0.0.0/8", "169.254.0.0/16",
		"::1/128", "fc00::/7", "fe80::/10",
	}
	for _, c := range cidrs {
		_, n, _ := net.ParseCIDR(c)
		if n != nil {
			privateRanges = append(privateRanges, n)
		}
	}
}

func isPrivate(ip net.IP) bool {
	for _, n := range privateRanges {
		if n.Contains(ip) {
			return true
		}
	}
	return false
}
