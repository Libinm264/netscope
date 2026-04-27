package cloud

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"cloud.google.com/go/pubsub"
	"google.golang.org/api/option"
)

// gcpPuller pulls VPC Flow Logs delivered to a GCP Pub/Sub subscription.
// Requires Enterprise license (FeatureCloudIngestGCP).
type gcpPuller struct {
	cfg        gcpSourceCfg
	sourceName string
}

type gcpSourceCfg struct {
	ProjectID       string `json:"project_id"`
	SubscriptionID  string `json:"subscription_id"`
	CredentialsJSON string `json:"credentials_json,omitempty"`
}

func newGCPPuller(configJSON, sourceName string) (*gcpPuller, error) {
	var cfg gcpSourceCfg
	if err := json.Unmarshal([]byte(configJSON), &cfg); err != nil {
		return nil, fmt.Errorf("gcp config parse: %w", err)
	}
	if cfg.ProjectID == "" || cfg.SubscriptionID == "" {
		return nil, fmt.Errorf("gcp: project_id and subscription_id are required")
	}
	return &gcpPuller{cfg: cfg, sourceName: sourceName}, nil
}

// pull reads up to maxMessages from the Pub/Sub subscription, parses them as
// GCP VPC Flow Log JSON messages, and acknowledges each one.
func (p *gcpPuller) pull(ctx context.Context, sourceID string, _ time.Time) ([]*ParsedFlow, error) {
	const maxMessages = 1000

	opts := []option.ClientOption{}
	if p.cfg.CredentialsJSON != "" {
		opts = append(opts, option.WithCredentialsJSON([]byte(p.cfg.CredentialsJSON)))
	}

	client, err := pubsub.NewClient(ctx, p.cfg.ProjectID, opts...)
	if err != nil {
		return nil, fmt.Errorf("gcp pubsub client: %w", err)
	}
	defer client.Close()

	sub := client.Subscription(p.cfg.SubscriptionID)
	sub.ReceiveSettings.MaxOutstandingMessages = maxMessages
	sub.ReceiveSettings.Synchronous = true
	sub.ReceiveSettings.MaxExtension = 30 * time.Second

	var flows []*ParsedFlow
	pullCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	err = sub.Receive(pullCtx, func(_ context.Context, msg *pubsub.Message) {
		defer msg.Ack()

		// Each message is a GCP log entry — the VPC flow data lives in
		// jsonPayload or protoPayload depending on the export sink config.
		var envelope map[string]any
		if jsonErr := json.Unmarshal(msg.Data, &envelope); jsonErr != nil {
			slog.Warn("cloud/gcp: unmarshal", "err", jsonErr)
			return
		}

		// Unwrap jsonPayload if present (Cloud Logging export format).
		payload := envelope
		if jp, ok := envelope["jsonPayload"].(map[string]any); ok {
			payload = jp
		}

		f, parseErr := parseGCPVPCRecord(payload, sourceID, p.sourceName)
		if parseErr != nil || f == nil {
			return
		}
		flows = append(flows, f)
	})

	// Timeout is expected — it just means no more messages.
	if err != nil && err != context.DeadlineExceeded {
		return flows, fmt.Errorf("gcp receive: %w", err)
	}
	return flows, nil
}
