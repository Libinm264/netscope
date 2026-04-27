// Package cloud implements VPC Flow Log ingestion for AWS, GCP, and Azure.
// Parsed records are normalised into models.Flow and written to ClickHouse via
// the same ingest path used by the agent → hub pipeline.
//
// # Tier gating
//
//   - AWS: Community + Enterprise
//   - GCP: Enterprise only (license.FeatureCloudIngestGCP)
//   - Azure: Enterprise only (license.FeatureCloudIngestAzure)
package cloud

import (
	"fmt"
	"net"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/netscope/hub-api/models"
)

// ParsedFlow is an intermediate representation used internally before writing
// to ClickHouse. It maps to the flows table columns.
type ParsedFlow struct {
	ID          string
	AgentID     string // set to source ID for cloud flows
	Hostname    string // set to source name
	TS          time.Time
	Protocol    string
	SrcIP       string
	SrcPort     uint16
	DstIP       string
	DstPort     uint16
	BytesIn     uint64
	BytesOut    uint64
	DurationMs  uint32
	CountryCode string
	CountryName string
	AsOrg       string
	ThreatScore float32
	ThreatLevel string
	Source      string // "aws-vpc" | "gcp-vpc" | "azure-nsg"
}

// parseAWSVPCRecord converts one AWS VPC Flow Log record (space-delimited v2
// default format) into a ParsedFlow.
//
// AWS default field order (v2):
//
//	version account-id interface-id srcaddr dstaddr srcport dstport protocol
//	packets bytes start end action log-status
func parseAWSVPCRecord(line, sourceID, sourceName string) (*ParsedFlow, error) {
	fields := strings.Fields(line)
	// Accept lines with at least 14 fields (v2 default format).
	if len(fields) < 14 {
		return nil, fmt.Errorf("too few fields: %d", len(fields))
	}
	// Skip header line or NODATA/SKIPDATA records.
	if fields[0] == "version" || fields[13] == "NODATA" || fields[13] == "SKIPDATA" {
		return nil, nil //nolint:nilnil
	}

	srcIP := fields[3]
	dstIP := fields[4]

	// Validate IPs — skip if not valid.
	if net.ParseIP(srcIP) == nil || net.ParseIP(dstIP) == nil {
		return nil, fmt.Errorf("invalid IP pair: %s → %s", srcIP, dstIP)
	}

	srcPort, _ := strconv.ParseUint(fields[5], 10, 16)
	dstPort, _ := strconv.ParseUint(fields[6], 10, 16)
	protoNum, _ := strconv.ParseUint(fields[7], 10, 8)
	bytesVal, _ := strconv.ParseUint(fields[9], 10, 64)
	startTs, _ := strconv.ParseInt(fields[10], 10, 64)
	endTs, _ := strconv.ParseInt(fields[11], 10, 64)

	protocol := protocolNumToName(uint8(protoNum))

	var durationMs uint32
	if endTs > startTs {
		durationMs = uint32((endTs - startTs) * 1000)
	}

	ts := time.Unix(startTs, 0).UTC()
	if ts.Year() < 2000 {
		ts = time.Now().UTC()
	}

	// AWS reports bytes for the whole flow; split evenly for in/out.
	bytesIn := bytesVal / 2
	bytesOut := bytesVal - bytesIn

	return &ParsedFlow{
		ID:         uuid.New().String(),
		AgentID:    sourceID,
		Hostname:   sourceName,
		TS:         ts,
		Protocol:   protocol,
		SrcIP:      srcIP,
		SrcPort:    uint16(srcPort),
		DstIP:      dstIP,
		DstPort:    uint16(dstPort),
		BytesIn:    bytesIn,
		BytesOut:   bytesOut,
		DurationMs: durationMs,
		Source:     "aws-vpc",
	}, nil
}

// parseGCPVPCRecord converts a GCP VPC Flow Log JSON object (Pub/Sub message
// data already decoded) into a ParsedFlow.
//
// Expected keys from GCP: connection.src_ip, connection.dest_ip,
// connection.src_port, connection.dest_port, connection.protocol,
// bytes_sent, packets_sent, start_time, end_time.
func parseGCPVPCRecord(data map[string]any, sourceID, sourceName string) (*ParsedFlow, error) {
	conn, _ := data["connection"].(map[string]any)
	if conn == nil {
		return nil, fmt.Errorf("missing connection block")
	}

	srcIP, _ := conn["src_ip"].(string)
	dstIP, _ := conn["dest_ip"].(string)
	if net.ParseIP(srcIP) == nil || net.ParseIP(dstIP) == nil {
		return nil, fmt.Errorf("invalid IPs: %s → %s", srcIP, dstIP)
	}

	srcPort := toUint16(conn["src_port"])
	dstPort := toUint16(conn["dest_port"])
	protoStr, _ := conn["protocol"].(string)
	protocol := strings.ToUpper(protoStr)
	if protocol == "" {
		protocol = "TCP"
	}

	bytesSent, _ := toUint64Any(data["bytes_sent"])

	var ts time.Time
	if st, ok := data["start_time"].(string); ok {
		ts, _ = time.Parse(time.RFC3339, st)
	}
	if ts.IsZero() {
		ts = time.Now().UTC()
	}

	var durationMs uint32
	if et, ok := data["end_time"].(string); ok {
		if end, err := time.Parse(time.RFC3339, et); err == nil && end.After(ts) {
			durationMs = uint32(end.Sub(ts).Milliseconds())
		}
	}

	bytesIn := bytesSent / 2
	bytesOut := bytesSent - bytesIn

	return &ParsedFlow{
		ID:         uuid.New().String(),
		AgentID:    sourceID,
		Hostname:   sourceName,
		TS:         ts,
		Protocol:   protocol,
		SrcIP:      srcIP,
		SrcPort:    srcPort,
		DstIP:      dstIP,
		DstPort:    dstPort,
		BytesIn:    bytesIn,
		BytesOut:   bytesOut,
		DurationMs: durationMs,
		Source:     "gcp-vpc",
	}, nil
}

// parseAzureNSGRecord converts an Azure NSG flow log tuple (v2 format) into a
// ParsedFlow. Azure stores records as CSV tuples inside a JSON blob.
//
// Tuple format: timestamp,src_ip,dst_ip,src_port,dst_port,protocol,
//
//	direction,decision,flow_state,packets_sent,bytes_sent,packets_recv,bytes_recv
func parseAzureNSGRecord(tuple, sourceID, sourceName string) (*ParsedFlow, error) {
	parts := strings.Split(tuple, ",")
	if len(parts) < 9 {
		return nil, fmt.Errorf("too few tuple fields: %d", len(parts))
	}

	tsUnix, _ := strconv.ParseInt(parts[0], 10, 64)
	srcIP := parts[1]
	dstIP := parts[2]

	if net.ParseIP(srcIP) == nil || net.ParseIP(dstIP) == nil {
		return nil, fmt.Errorf("invalid IPs: %s → %s", srcIP, dstIP)
	}

	srcPort, _ := strconv.ParseUint(parts[3], 10, 16)
	dstPort, _ := strconv.ParseUint(parts[4], 10, 16)
	protoAzure := strings.ToUpper(parts[5]) // "T" or "U"
	protocol := "TCP"
	if protoAzure == "U" {
		protocol = "UDP"
	}

	var bytesSent, bytesRecv uint64
	if len(parts) >= 13 {
		bytesSent, _ = strconv.ParseUint(parts[10], 10, 64)
		bytesRecv, _ = strconv.ParseUint(parts[12], 10, 64)
	}

	ts := time.Unix(tsUnix, 0).UTC()
	if ts.Year() < 2000 {
		ts = time.Now().UTC()
	}

	return &ParsedFlow{
		ID:         uuid.New().String(),
		AgentID:    sourceID,
		Hostname:   sourceName,
		TS:         ts,
		Protocol:   protocol,
		SrcIP:      srcIP,
		SrcPort:    uint16(srcPort),
		DstIP:      dstIP,
		DstPort:    uint16(dstPort),
		BytesIn:    bytesRecv,
		BytesOut:   bytesSent,
		DurationMs: 0,
		Source:     "azure-nsg",
	}, nil
}

// toParsedFlows is a convenience helper that converts a slice of ParsedFlows
// to models.Flow, applying geo+threat enrichment placeholders (enrichment runs
// at the ingester layer).
func toParsedFlow(p *ParsedFlow) models.Flow {
	return models.Flow{
		ID:          p.ID,
		AgentID:     p.AgentID,
		Hostname:    p.Hostname,
		TS:          p.TS,
		Protocol:    p.Protocol,
		SrcIP:       p.SrcIP,
		SrcPort:     p.SrcPort,
		DstIP:       p.DstIP,
		DstPort:     p.DstPort,
		BytesIn:     p.BytesIn,
		BytesOut:    p.BytesOut,
		DurationMs:  p.DurationMs,
		CountryCode: p.CountryCode,
		CountryName: p.CountryName,
		AsOrg:       p.AsOrg,
		ThreatScore: p.ThreatScore,
		ThreatLevel: p.ThreatLevel,
	}
}

// ── helpers ──────────────────────────────────────────────────────────────────

func protocolNumToName(n uint8) string {
	switch n {
	case 6:
		return "TCP"
	case 17:
		return "UDP"
	case 1:
		return "ICMP"
	case 58:
		return "ICMPv6"
	case 47:
		return "GRE"
	case 50:
		return "ESP"
	default:
		return fmt.Sprintf("PROTO_%d", n)
	}
}

func toUint16(v any) uint16 {
	switch val := v.(type) {
	case float64:
		return uint16(val)
	case string:
		n, _ := strconv.ParseUint(val, 10, 16)
		return uint16(n)
	}
	return 0
}

func toUint64Any(v any) (uint64, bool) {
	switch val := v.(type) {
	case float64:
		return uint64(val), true
	case string:
		n, err := strconv.ParseUint(val, 10, 64)
		return n, err == nil
	}
	return 0, false
}
