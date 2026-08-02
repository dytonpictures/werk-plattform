package kafkastream

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"os"
	"sync/atomic"
	"time"

	"github.com/twmb/franz-go/pkg/kerr"
	"github.com/twmb/franz-go/pkg/kgo"
	"github.com/twmb/franz-go/pkg/kmsg"
	"github.com/twmb/franz-go/pkg/sasl/plain"
	"github.com/twmb/franz-go/pkg/sasl/scram"

	"github.com/dytonpictures/werk/internal/platform/config"
)

const kafkaMaximumBufferedRecords = 512

type Message struct {
	Topic     string
	Key       string
	Value     []byte
	Headers   map[string]string
	Timestamp time.Time
}

type Writer interface {
	Publish(context.Context, Message) error
}

type Client struct {
	client   *kgo.Client
	timeout  time.Duration
	topics   [3]string
	failures atomic.Uint64
}

func NewClient(configuration config.KafkaConfig) (*Client, error) {
	if !configuration.Enabled || len(configuration.Brokers) == 0 {
		return nil, errors.New("Kafka is not enabled")
	}
	topics := [3]string{
		configuration.DomainEventsTopic,
		configuration.SecurityAuditTopic,
		configuration.RuntimeLogsTopic,
	}
	if topics[0] == "" || topics[1] == "" || topics[2] == "" ||
		topics[0] == topics[1] || topics[0] == topics[2] || topics[1] == topics[2] {
		return nil, errors.New("Kafka topics must be non-empty and distinct")
	}
	options := []kgo.Opt{
		kgo.SeedBrokers(configuration.Brokers...),
		kgo.ClientID(configuration.ClientID),
		kgo.RequiredAcks(kgo.AllISRAcks()),
		kgo.ProducerBatchCompression(kgo.ZstdCompression()),
		kgo.ProducerBatchMaxBytes(2 << 20),
		// WERK publishes synchronously. This only needs to cover concurrent
		// worker slots and leaves ample headroom without reserving a five-digit
		// record backlog inside every process.
		kgo.MaxBufferedRecords(kafkaMaximumBufferedRecords),
	}
	if configuration.TLS {
		tlsConfiguration, err := clientTLSConfig(configuration)
		if err != nil {
			return nil, err
		}
		options = append(options, kgo.DialTLSConfig(tlsConfiguration))
	}
	switch configuration.SASLMechanism {
	case "none":
	case "plain":
		options = append(options, kgo.SASL(plain.Auth{
			User: configuration.SASLUsername,
			Pass: configuration.SASLPassword,
		}.AsMechanism()))
	case "scram-sha-256":
		options = append(options, kgo.SASL(scram.Auth{
			User: configuration.SASLUsername,
			Pass: configuration.SASLPassword,
		}.AsSha256Mechanism()))
	case "scram-sha-512":
		options = append(options, kgo.SASL(scram.Auth{
			User: configuration.SASLUsername,
			Pass: configuration.SASLPassword,
		}.AsSha512Mechanism()))
	default:
		return nil, errors.New("unsupported Kafka SASL mechanism")
	}
	client, err := kgo.NewClient(options...)
	if err != nil {
		return nil, fmt.Errorf("create Kafka client: %w", err)
	}
	return &Client{client: client, timeout: configuration.PublishTimeout, topics: topics}, nil
}

func (client *Client) Publish(ctx context.Context, message Message) error {
	if client == nil || client.client == nil || message.Topic == "" || message.Key == "" || len(message.Value) == 0 {
		return errors.New("invalid Kafka message")
	}
	publishContext, cancel := context.WithTimeout(ctx, client.timeout)
	defer cancel()
	record := &kgo.Record{
		Topic: message.Topic,
		Key:   []byte(message.Key),
		// ProduceSync completes before Publish returns, so the caller-owned
		// immutable payload remains valid for the complete franz-go lifetime.
		Value:     message.Value,
		Timestamp: message.Timestamp,
		Headers:   make([]kgo.RecordHeader, 0, len(message.Headers)),
	}
	for key, value := range message.Headers {
		record.Headers = append(record.Headers, kgo.RecordHeader{Key: key, Value: []byte(value)})
	}
	if err := client.client.ProduceSync(publishContext, record).FirstErr(); err != nil {
		client.failures.Add(1)
		return fmt.Errorf("publish Kafka record: %w", err)
	}
	return nil
}

func (client *Client) PublishFailures() uint64 {
	if client == nil {
		return 0
	}
	return client.failures.Load()
}

// VerifyTopics proves broker connectivity and checks the complete configured
// transport contract without allowing Kafka to auto-create a topic with broker
// defaults. Topic creation, ACLs, retention and replication remain explicit
// operator responsibilities.
func (client *Client) VerifyTopics(ctx context.Context) error {
	if client == nil || client.client == nil {
		return errors.New("Kafka client is not configured")
	}
	request := kmsg.NewPtrMetadataRequest()
	request.AllowAutoTopicCreation = false
	for _, topic := range client.topics {
		topicName := topic
		request.Topics = append(request.Topics, kmsg.MetadataRequestTopic{Topic: &topicName})
	}
	checkContext, cancel := context.WithTimeout(ctx, client.timeout)
	defer cancel()
	response, err := request.RequestWith(checkContext, client.client)
	if err != nil {
		return fmt.Errorf("request Kafka topic metadata: %w", err)
	}
	var seen [3]bool
	for _, topic := range response.Topics {
		if topic.Topic == nil {
			continue
		}
		name := *topic.Topic
		requested := -1
		for index, expected := range client.topics {
			if name == expected {
				requested = index
				break
			}
		}
		if requested < 0 {
			continue
		}
		if topicError := kerr.ErrorForCode(topic.ErrorCode); topicError != nil {
			return fmt.Errorf("Kafka topic %q is unavailable: %w", name, topicError)
		}
		if len(topic.Partitions) == 0 {
			return fmt.Errorf("Kafka topic %q has no partitions", name)
		}
		for _, partition := range topic.Partitions {
			if partitionError := kerr.ErrorForCode(partition.ErrorCode); partitionError != nil {
				return fmt.Errorf("Kafka topic %q partition %d is unavailable: %w", name, partition.Partition, partitionError)
			}
		}
		seen[requested] = true
	}
	for index, present := range seen {
		if !present {
			return fmt.Errorf("Kafka did not return metadata for configured topic %q", client.topics[index])
		}
	}
	return nil
}

func (client *Client) Close() {
	if client != nil && client.client != nil {
		client.client.Close()
	}
}

func clientTLSConfig(configuration config.KafkaConfig) (*tls.Config, error) {
	roots, err := x509.SystemCertPool()
	if err != nil || roots == nil {
		roots = x509.NewCertPool()
	}
	if configuration.TLSCAFile != "" {
		certificateAuthority, readErr := os.ReadFile(configuration.TLSCAFile)
		if readErr != nil {
			return nil, fmt.Errorf("read Kafka CA certificate: %w", readErr)
		}
		if !roots.AppendCertsFromPEM(certificateAuthority) {
			return nil, errors.New("Kafka CA file contains no valid certificate")
		}
	}
	tlsConfiguration := &tls.Config{
		MinVersion: tls.VersionTLS12,
		RootCAs:    roots,
		ServerName: configuration.TLSServerName,
	}
	if configuration.TLSCertFile != "" {
		certificate, err := tls.LoadX509KeyPair(configuration.TLSCertFile, configuration.TLSKeyFile)
		if err != nil {
			return nil, fmt.Errorf("load Kafka client certificate: %w", err)
		}
		tlsConfiguration.Certificates = []tls.Certificate{certificate}
	}
	return tlsConfiguration, nil
}
