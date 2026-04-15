package kafka

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"

	"github.com/twmb/franz-go/pkg/kgo"

	"github.com/netscope/hub-api/models"
)

// Producer publishes flow records to a Kafka/Redpanda topic.
type Producer struct {
	client *kgo.Client
	topic  string
}

// NewProducer creates and connects a Kafka producer.
func NewProducer(brokers []string, topic string) (*Producer, error) {
	client, err := kgo.NewClient(
		kgo.SeedBrokers(brokers...),
		kgo.DefaultProduceTopic(topic),
	)
	if err != nil {
		return nil, fmt.Errorf("kafka: new producer: %w", err)
	}
	return &Producer{client: client, topic: topic}, nil
}

// Publish serialises a flow to JSON and produces it to the configured topic.
func (p *Producer) Publish(ctx context.Context, flow models.Flow) error {
	data, err := json.Marshal(flow)
	if err != nil {
		return fmt.Errorf("kafka: marshal flow: %w", err)
	}
	rec := &kgo.Record{
		Topic: p.topic,
		Key:   []byte(flow.AgentID),
		Value: data,
	}
	if err := p.client.ProduceSync(ctx, rec).FirstErr(); err != nil {
		return fmt.Errorf("kafka: produce: %w", err)
	}
	return nil
}

// Close flushes pending messages and closes the Kafka client.
func (p *Producer) Close() {
	p.client.Close()
}

// ─────────────────────────────────────────────────────────────────────────────

// Consumer reads flow records from a Kafka topic and calls a handler for each.
type Consumer struct {
	client *kgo.Client
}

// NewConsumer creates a Kafka consumer that is part of the given consumer group.
func NewConsumer(brokers []string, topic, groupID string) (*Consumer, error) {
	client, err := kgo.NewClient(
		kgo.SeedBrokers(brokers...),
		kgo.ConsumeTopics(topic),
		kgo.ConsumerGroup(groupID),
	)
	if err != nil {
		return nil, fmt.Errorf("kafka: new consumer: %w", err)
	}
	return &Consumer{client: client}, nil
}

// Consume polls Kafka in a blocking loop and passes decoded flows to handler.
// Returns when ctx is cancelled or the client is closed.
func (c *Consumer) Consume(ctx context.Context, handler func(models.Flow)) error {
	for {
		fetches := c.client.PollFetches(ctx)
		if fetches.IsClientClosed() {
			return nil
		}
		if ctx.Err() != nil {
			return ctx.Err()
		}
		for _, err := range fetches.Errors() {
			slog.Error("kafka fetch error", "topic", err.Topic, "err", err.Err)
		}
		fetches.EachRecord(func(rec *kgo.Record) {
			var flow models.Flow
			if err := json.Unmarshal(rec.Value, &flow); err != nil {
				slog.Warn("kafka: unmarshal flow failed", "err", err)
				return
			}
			handler(flow)
		})
	}
}

// Close shuts down the consumer client.
func (c *Consumer) Close() {
	c.client.Close()
}
