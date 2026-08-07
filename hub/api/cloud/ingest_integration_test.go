package cloud

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"testing"

	"github.com/gofiber/fiber/v2"

	"github.com/klyzar/hub-api/handlers"
	"github.com/klyzar/hub-api/testutil"
)

// TestCloudPull_FixtureVPCLog_VisibleInFlows is REMAINING_WORK.md's V1:
// "AWS/GCP/Azure cloud pull actually ingests a fixture VPC log → visible in
// /flows". It exercises the real parse → write → query pipeline against a
// throwaway ClickHouse container rather than mocks:
//
//  1. Parse one fixture VPC Flow Log record per provider (AWS space-delimited,
//     GCP JSON, Azure CSV tuple) using the same unexported parse functions the
//     ingester calls.
//  2. Write the parsed flows via the ingester's real writeFlows path.
//  3. Query them back through the same handlers.FlowHandler.Query the /flows
//     API route uses, filtered by each fixture's unique src_ip.
func TestCloudPull_FixtureVPCLog_VisibleInFlows(t *testing.T) {
	ch := testutil.StartClickHouse(t)
	ctx := context.Background()
	ing := &Ingester{ch: ch}

	const (
		awsSrcIP   = "172.31.16.139"
		gcpSrcIP   = "10.128.0.5"
		azureSrcIP = "10.0.0.4"
	)

	// AWS VPC Flow Log v2 default format (space-delimited, 14 fields).
	awsLine := "2 123456789010 eni-1235b8ca123456789 " + awsSrcIP +
		" 172.31.16.21 20641 22 6 20 4249 1418530010 1418530070 ACCEPT OK"
	awsFlow, err := parseAWSVPCRecord(awsLine, "src-aws-1", "aws-fixture")
	if err != nil {
		t.Fatalf("parseAWSVPCRecord: %v", err)
	}
	if awsFlow == nil {
		t.Fatal("parseAWSVPCRecord returned nil flow for a valid record")
	}
	if awsFlow.Protocol != "TCP" || awsFlow.DstPort != 22 {
		t.Errorf("aws flow fields wrong: protocol=%s dstPort=%d", awsFlow.Protocol, awsFlow.DstPort)
	}

	// GCP VPC Flow Log JSON record.
	gcpRecord := map[string]any{
		"connection": map[string]any{
			"src_ip":    gcpSrcIP,
			"dest_ip":   "10.128.0.10",
			"src_port":  float64(54321),
			"dest_port": float64(443),
			"protocol":  "TCP",
		},
		"bytes_sent": float64(2048),
		"start_time": "2025-01-15T12:00:00Z",
		"end_time":   "2025-01-15T12:00:01Z",
	}
	gcpFlow, err := parseGCPVPCRecord(gcpRecord, "src-gcp-1", "gcp-fixture")
	if err != nil {
		t.Fatalf("parseGCPVPCRecord: %v", err)
	}
	if gcpFlow.DstPort != 443 {
		t.Errorf("gcp flow dstPort wrong: %d", gcpFlow.DstPort)
	}

	// Azure NSG flow log tuple (v2 format).
	azureTuple := "1421927882,10.0.0.4,10.0.0.5,53109,443,T,I,A,C,10,1000,10,1000"
	azureFlow, err := parseAzureNSGRecord(azureTuple, "src-azure-1", "azure-fixture")
	if err != nil {
		t.Fatalf("parseAzureNSGRecord: %v", err)
	}
	if azureFlow.Protocol != "TCP" || azureFlow.DstPort != 443 {
		t.Errorf("azure flow fields wrong: protocol=%s dstPort=%d", azureFlow.Protocol, azureFlow.DstPort)
	}

	if err := ing.writeFlows(ctx, []*ParsedFlow{awsFlow, gcpFlow, azureFlow}); err != nil {
		t.Fatalf("writeFlows: %v", err)
	}

	// Verify each fixture is queryable through the real /flows handler path.
	flowH := &handlers.FlowHandler{CH: ch}
	app := fiber.New()
	app.Get("/api/v1/flows", flowH.Query)

	cases := []struct {
		name     string
		srcIP    string
		wantPort float64
	}{
		{"aws", awsSrcIP, 22},
		{"gcp", gcpSrcIP, 443},
		{"azure", azureSrcIP, 443},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest("GET", "/api/v1/flows?src_ip="+tc.srcIP, nil)
			resp, err := app.Test(req, -1)
			if err != nil {
				t.Fatalf("app.Test: %v", err)
			}
			defer resp.Body.Close()
			if resp.StatusCode != 200 {
				t.Fatalf("want 200, got %d", resp.StatusCode)
			}
			var body struct {
				Total uint64           `json:"total"`
				Flows []map[string]any `json:"flows"`
			}
			if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
				t.Fatalf("decode response: %v", err)
			}
			if len(body.Flows) == 0 {
				t.Fatalf("expected at least one flow for src_ip=%s, got none (total=%d)", tc.srcIP, body.Total)
			}
			got := body.Flows[0]
			if got["src_ip"] != tc.srcIP {
				t.Errorf("src_ip mismatch: want %s, got %v", tc.srcIP, got["src_ip"])
			}
			if got["dst_port"] != tc.wantPort {
				t.Errorf("dst_port mismatch: want %v, got %v", tc.wantPort, got["dst_port"])
			}
		})
	}
}
