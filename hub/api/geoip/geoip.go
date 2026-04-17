// Package geoip wraps the MaxMind GeoLite2 database to enrich IP addresses
// with country, city, and ASN information.
//
// # Setup
//
// Download the free GeoLite2 databases from MaxMind:
//
//	curl -o /etc/netscope/GeoLite2-City.mmdb    https://… (requires free account)
//	curl -o /etc/netscope/GeoLite2-ASN.mmdb     https://…
//
// Set the paths via environment variables:
//
//	GEOIP_CITY_DB=/etc/netscope/GeoLite2-City.mmdb
//	GEOIP_ASN_DB=/etc/netscope/GeoLite2-ASN.mmdb
//
// If the files are absent the Reader degrades gracefully — all lookups return
// empty GeoInfo structs, so ingest continues without enrichment.
package geoip

import (
	"log/slog"
	"net"
	"sync"

	"github.com/oschwald/geoip2-golang"
)

// GeoInfo holds the enrichment data for a single IP address.
type GeoInfo struct {
	// CountryCode is the ISO 3166-1 alpha-2 country code (e.g. "US", "DE").
	CountryCode string `json:"country_code"`
	// CountryName is the English country name.
	CountryName string `json:"country_name"`
	// City name (may be empty for small or rural IPs).
	City string `json:"city,omitempty"`
	// Latitude and Longitude of the best-guess location.
	Latitude  float64 `json:"latitude,omitempty"`
	Longitude float64 `json:"longitude,omitempty"`
	// ASNumber is the Autonomous System number (e.g. 15169 for Google).
	ASNumber uint `json:"as_number,omitempty"`
	// ASOrg is the registered organisation name for the AS.
	ASOrg string `json:"as_org,omitempty"`
}

// Reader is a thread-safe GeoIP enrichment client.
type Reader struct {
	cityDB *geoip2.Reader
	asnDB  *geoip2.Reader
	mu     sync.RWMutex
}

// New opens the GeoLite2 City and ASN databases at the given paths.
// Pass empty strings to skip loading the respective database.
// The returned Reader is always valid — missing databases are simply skipped.
func New(cityDBPath, asnDBPath string) *Reader {
	r := &Reader{}

	if cityDBPath != "" {
		db, err := geoip2.Open(cityDBPath)
		if err != nil {
			slog.Warn("geoip: city database unavailable", "path", cityDBPath, "err", err)
		} else {
			r.cityDB = db
			slog.Info("geoip: city database loaded", "path", cityDBPath)
		}
	}

	if asnDBPath != "" {
		db, err := geoip2.Open(asnDBPath)
		if err != nil {
			slog.Warn("geoip: ASN database unavailable", "path", asnDBPath, "err", err)
		} else {
			r.asnDB = db
			slog.Info("geoip: ASN database loaded", "path", asnDBPath)
		}
	}

	return r
}

// Lookup returns geo enrichment for the given IP address string.
// Returns an empty GeoInfo (not an error) when the databases are unavailable
// or the IP is private/loopback.
func (r *Reader) Lookup(ipStr string) GeoInfo {
	ip := net.ParseIP(ipStr)
	if ip == nil || isPrivate(ip) {
		return GeoInfo{}
	}

	r.mu.RLock()
	defer r.mu.RUnlock()

	info := GeoInfo{}

	if r.cityDB != nil {
		if record, err := r.cityDB.City(ip); err == nil {
			info.CountryCode = record.Country.IsoCode
			if name, ok := record.Country.Names["en"]; ok {
				info.CountryName = name
			}
			if len(record.City.Names) > 0 {
				info.City = record.City.Names["en"]
			}
			info.Latitude = record.Location.Latitude
			info.Longitude = record.Location.Longitude
		}
	}

	if r.asnDB != nil {
		if record, err := r.asnDB.ASN(ip); err == nil {
			info.ASNumber = record.AutonomousSystemNumber
			info.ASOrg = record.AutonomousSystemOrganization
		}
	}

	return info
}

// Close releases the database file handles.
func (r *Reader) Close() {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.cityDB != nil {
		r.cityDB.Close()
	}
	if r.asnDB != nil {
		r.asnDB.Close()
	}
}

// ── helpers ───────────────────────────────────────────────────────────────────

var privateRanges []*net.IPNet

func init() {
	cidrs := []string{
		"10.0.0.0/8", "172.16.0.0/12", "192.168.0.0/16",
		"127.0.0.0/8", "169.254.0.0/16",
		"::1/128", "fc00::/7", "fe80::/10",
	}
	for _, cidr := range cidrs {
		_, n, _ := net.ParseCIDR(cidr)
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
