package cloud

import (
	"bufio"
	"compress/gzip"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/cloudwatchlogs"
	"github.com/aws/aws-sdk-go-v2/service/s3"
)

// awsPuller implements VPC Flow Log ingestion from AWS S3 or CloudWatch Logs.
type awsPuller struct {
	cfg        awsSourceCfg
	sourceName string
}

type awsSourceCfg struct {
	Region          string `json:"region"`
	AccessKeyID     string `json:"access_key_id"`
	SecretAccessKey string `json:"secret_access_key"`
	S3Bucket        string `json:"s3_bucket,omitempty"`
	S3Prefix        string `json:"s3_prefix,omitempty"`
	LogGroupName    string `json:"log_group_name,omitempty"`
}

func newAWSPuller(configJSON, sourceName string) (*awsPuller, error) {
	var cfg awsSourceCfg
	if err := json.Unmarshal([]byte(configJSON), &cfg); err != nil {
		return nil, fmt.Errorf("aws config parse: %w", err)
	}
	if cfg.Region == "" {
		cfg.Region = "us-east-1"
	}
	return &awsPuller{cfg: cfg, sourceName: sourceName}, nil
}

// pull fetches VPC flow records since lastPulled and returns parsed flows.
func (p *awsPuller) pull(ctx context.Context, sourceID string, lastPulled time.Time) ([]*ParsedFlow, error) {
	awsCfg, err := awsconfig.LoadDefaultConfig(ctx,
		awsconfig.WithRegion(p.cfg.Region),
		awsconfig.WithCredentialsProvider(
			credentials.NewStaticCredentialsProvider(
				p.cfg.AccessKeyID, p.cfg.SecretAccessKey, "",
			),
		),
	)
	if err != nil {
		return nil, fmt.Errorf("aws config: %w", err)
	}

	if p.cfg.LogGroupName != "" {
		return p.pullCloudWatch(ctx, awsCfg, sourceID, lastPulled)
	}
	return p.pullS3(ctx, awsCfg, sourceID, lastPulled)
}

// pullS3 lists and downloads new VPC flow log files from S3.
func (p *awsPuller) pullS3(ctx context.Context, awsCfg aws.Config, sourceID string, lastPulled time.Time) ([]*ParsedFlow, error) {
	s3Client := s3.NewFromConfig(awsCfg)

	prefix := p.cfg.S3Prefix
	if prefix == "" {
		prefix = "AWSLogs/"
	}

	var flows []*ParsedFlow
	var continuationToken *string

	for {
		input := &s3.ListObjectsV2Input{
			Bucket:            aws.String(p.cfg.S3Bucket),
			Prefix:            aws.String(prefix),
			ContinuationToken: continuationToken,
		}
		out, err := s3Client.ListObjectsV2(ctx, input)
		if err != nil {
			return nil, fmt.Errorf("s3 list: %w", err)
		}

		for _, obj := range out.Contents {
			if obj.LastModified == nil || obj.LastModified.Before(lastPulled) {
				continue
			}
			objFlows, err := p.downloadAndParse(ctx, s3Client, *obj.Key, sourceID)
			if err != nil {
				slog.Warn("cloud/aws: s3 parse error", "key", *obj.Key, "err", err)
				continue
			}
			flows = append(flows, objFlows...)
		}

		if !out.IsTruncated || out.NextContinuationToken == nil {
			break
		}
		continuationToken = out.NextContinuationToken
	}

	return flows, nil
}

// downloadAndParse fetches one S3 object and parses it as VPC flow log records.
func (p *awsPuller) downloadAndParse(ctx context.Context, client *s3.Client, key, sourceID string) ([]*ParsedFlow, error) {
	out, err := client.GetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(p.cfg.S3Bucket),
		Key:    aws.String(key),
	})
	if err != nil {
		return nil, err
	}
	defer out.Body.Close()

	var reader io.Reader = out.Body
	if strings.HasSuffix(key, ".gz") {
		gz, err := gzip.NewReader(out.Body)
		if err != nil {
			return nil, fmt.Errorf("gzip: %w", err)
		}
		defer gz.Close()
		reader = gz
	}

	var flows []*ParsedFlow
	scanner := bufio.NewScanner(reader)
	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			continue
		}
		f, err := parseAWSVPCRecord(line, sourceID, p.sourceName)
		if err != nil || f == nil {
			continue
		}
		flows = append(flows, f)
	}
	return flows, scanner.Err()
}

// pullCloudWatch reads VPC flow log events from a CloudWatch Logs group.
func (p *awsPuller) pullCloudWatch(ctx context.Context, awsCfg aws.Config, sourceID string, lastPulled time.Time) ([]*ParsedFlow, error) {
	cwClient := cloudwatchlogs.NewFromConfig(awsCfg)

	startTime := aws.Int64(lastPulled.UnixMilli())
	endTime := aws.Int64(time.Now().UnixMilli())

	input := &cloudwatchlogs.FilterLogEventsInput{
		LogGroupName: aws.String(p.cfg.LogGroupName),
		StartTime:    startTime,
		EndTime:      endTime,
	}

	var flows []*ParsedFlow
	paginator := cloudwatchlogs.NewFilterLogEventsPaginator(cwClient, input)
	for paginator.HasMorePages() {
		page, err := paginator.NextPage(ctx)
		if err != nil {
			return nil, fmt.Errorf("cloudwatch: %w", err)
		}
		for _, event := range page.Events {
			if event.Message == nil {
				continue
			}
			f, err := parseAWSVPCRecord(*event.Message, sourceID, p.sourceName)
			if err != nil || f == nil {
				continue
			}
			flows = append(flows, f)
		}
	}
	return flows, nil
}
