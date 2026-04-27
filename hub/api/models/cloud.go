package models

import "time"

// CloudSource represents a configured cloud VPC flow log pull source.
type CloudSource struct {
	ID         string    `json:"id"`
	Provider   string    `json:"provider"` // "aws" | "gcp" | "azure"
	Name       string    `json:"name"`
	Config     string    `json:"config"` // JSON — provider-specific fields
	Enabled    bool      `json:"enabled"`
	LastPulled time.Time `json:"last_pulled,omitempty"`
	ErrorMsg   string    `json:"error_msg,omitempty"`
	CreatedAt  time.Time `json:"created_at"`
}

// CloudPullResult summarises one pull run.
type CloudPullResult struct {
	ID           string    `json:"id"`
	SourceID     string    `json:"source_id"`
	Provider     string    `json:"provider"`
	RowsIngested uint64    `json:"rows_ingested"`
	PulledAt     time.Time `json:"pulled_at"`
	DurationMs   uint32    `json:"duration_ms"`
	Error        string    `json:"error,omitempty"`
}

// AWSSourceConfig holds the fields required to pull AWS VPC Flow Logs.
type AWSSourceConfig struct {
	Region          string `json:"region"`
	AccessKeyID     string `json:"access_key_id"`
	SecretAccessKey string `json:"secret_access_key"` // redacted on read
	// S3-mode fields
	S3Bucket string `json:"s3_bucket,omitempty"`
	S3Prefix string `json:"s3_prefix,omitempty"`
	// CloudWatch-mode fields (mutually exclusive with S3)
	LogGroupName string `json:"log_group_name,omitempty"`
}

// GCPSourceConfig holds the fields required to pull GCP VPC Flow Logs via Pub/Sub.
type GCPSourceConfig struct {
	ProjectID      string `json:"project_id"`
	SubscriptionID string `json:"subscription_id"`
	// CredentialsJSON is the service-account JSON key (redacted on read).
	CredentialsJSON string `json:"credentials_json,omitempty"`
}

// AzureSourceConfig holds the fields required to pull Azure NSG Flow Logs.
type AzureSourceConfig struct {
	SubscriptionID  string `json:"subscription_id"`
	ResourceGroup   string `json:"resource_group"`
	StorageAccount  string `json:"storage_account"`
	ContainerName   string `json:"container_name"`
	SASToken        string `json:"sas_token,omitempty"` // redacted on read
	ConnectionString string `json:"connection_string,omitempty"` // alternative to SAS
}
