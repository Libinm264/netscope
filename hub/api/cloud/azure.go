package cloud

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/Azure/azure-sdk-for-go/sdk/storage/azblob"
)

// azurePuller pulls Azure NSG Flow Logs from an Azure Blob Storage container.
// Requires Enterprise license (FeatureCloudIngestAzure).
type azurePuller struct {
	cfg        azureSourceCfg
	sourceName string
}

type azureSourceCfg struct {
	StorageAccount   string `json:"storage_account"`
	ContainerName    string `json:"container_name"`
	SASToken         string `json:"sas_token,omitempty"`
	ConnectionString string `json:"connection_string,omitempty"`
}

func newAzurePuller(configJSON, sourceName string) (*azurePuller, error) {
	var cfg azureSourceCfg
	if err := json.Unmarshal([]byte(configJSON), &cfg); err != nil {
		return nil, fmt.Errorf("azure config parse: %w", err)
	}
	if cfg.StorageAccount == "" {
		return nil, fmt.Errorf("azure: storage_account is required")
	}
	if cfg.ContainerName == "" {
		cfg.ContainerName = "insights-logs-networksecuritygroupflowevent"
	}
	return &azurePuller{cfg: cfg, sourceName: sourceName}, nil
}

// pull lists blobs modified since lastPulled and parses them as Azure NSG Flow
// Log v2 JSON files.
func (p *azurePuller) pull(ctx context.Context, sourceID string, lastPulled time.Time) ([]*ParsedFlow, error) {
	client, err := p.buildClient()
	if err != nil {
		return nil, err
	}

	containerClient := client.ServiceClient().NewContainerClient(p.cfg.ContainerName)
	pager := containerClient.NewListBlobsFlatPager(nil)

	var flows []*ParsedFlow
	for pager.More() {
		page, err := pager.NextPage(ctx)
		if err != nil {
			return nil, fmt.Errorf("azure list blobs: %w", err)
		}
		for _, item := range page.Segment.BlobItems {
			if item.Properties == nil || item.Properties.LastModified == nil {
				continue
			}
			if item.Properties.LastModified.Before(lastPulled) {
				continue
			}
			blobName := ""
			if item.Name != nil {
				blobName = *item.Name
			}
			blobFlows, err := p.downloadAndParse(ctx, containerClient, blobName, sourceID)
			if err != nil {
				slog.Warn("cloud/azure: blob parse error", "blob", blobName, "err", err)
				continue
			}
			flows = append(flows, blobFlows...)
		}
	}
	return flows, nil
}

// downloadAndParse fetches one blob and parses it as Azure NSG flow log v2 JSON.
func (p *azurePuller) downloadAndParse(ctx context.Context, container *azblob.ContainerClient, blobName, sourceID string) ([]*ParsedFlow, error) {
	blobClient := container.NewBlobClient(blobName)
	resp, err := blobClient.DownloadStream(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("download: %w", err)
	}
	defer resp.Body.Close()

	var root struct {
		Records []struct {
			Properties struct {
				Flows []struct {
					Flows []struct {
						FlowTuples []string `json:"flowTuples"`
					} `json:"flows"`
				} `json:"flows"`
			} `json:"properties"`
		} `json:"records"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&root); err != nil {
		return nil, fmt.Errorf("json decode: %w", err)
	}

	var flows []*ParsedFlow
	for _, rec := range root.Records {
		for _, outerFlow := range rec.Properties.Flows {
			for _, innerFlow := range outerFlow.Flows {
				for _, tuple := range innerFlow.FlowTuples {
					f, err := parseAzureNSGRecord(tuple, sourceID, p.sourceName)
					if err != nil || f == nil {
						continue
					}
					flows = append(flows, f)
				}
			}
		}
	}
	return flows, nil
}

// buildClient constructs an Azure Blob client from connection string or SAS URL.
func (p *azurePuller) buildClient() (*azblob.Client, error) {
	if p.cfg.ConnectionString != "" {
		return azblob.NewClientFromConnectionString(p.cfg.ConnectionString, nil)
	}
	if p.cfg.SASToken != "" {
		url := fmt.Sprintf("https://%s.blob.core.windows.net/?%s",
			p.cfg.StorageAccount,
			strings.TrimPrefix(p.cfg.SASToken, "?"),
		)
		return azblob.NewClientWithNoCredential(url, nil)
	}
	return nil, fmt.Errorf("azure: sas_token or connection_string is required")
}
