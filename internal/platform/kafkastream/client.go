package kafkastream

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"os"
	"time"

	"github.com/twmb/franz-go/pkg/kgo"
	"github.com/twmb/franz-go/pkg/sasl/plain"
	"github.com/twmb/franz-go/pkg/sasl/scram"

	"github.com/dytonpictures/werk/internal/platform/config"
)

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
	client  *kgo.Client
	timeout time.Duration
}

func NewClient(configuration config.KafkaConfig) (*Client, error) {
	if !configuration.Enabled || len(configuration.Brokers) == 0 {
		return nil, errors.New("Kafka is not enabled")
	}
	options := []kgo.Opt{
		kgo.SeedBrokers(configuration.Brokers...),
		kgo.ClientID(configuration.ClientID),
		kgo.RequiredAcks(kgo.AllISRAcks()),
		kgo.ProducerBatchCompression(kgo.ZstdCompression()),
		kgo.ProducerBatchMaxBytes(2 << 20),
		kgo.MaxBufferedRecords(10_000),
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
	return &Client{client: client, timeout: configuration.PublishTimeout}, nil
}

func (client *Client) Publish(ctx context.Context, message Message) error {
	if client == nil || client.client == nil || message.Topic == "" || message.Key == "" || len(message.Value) == 0 {
		return errors.New("invalid Kafka message")
	}
	publishContext, cancel := context.WithTimeout(ctx, client.timeout)
	defer cancel()
	record := &kgo.Record{
		Topic:     message.Topic,
		Key:       []byte(message.Key),
		Value:     append([]byte(nil), message.Value...),
		Timestamp: message.Timestamp,
	}
	for key, value := range message.Headers {
		record.Headers = append(record.Headers, kgo.RecordHeader{Key: key, Value: []byte(value)})
	}
	if err := client.client.ProduceSync(publishContext, record).FirstErr(); err != nil {
		return fmt.Errorf("publish Kafka record: %w", err)
	}
	return nil
}

func (client *Client) Ping(ctx context.Context) error {
	if client == nil || client.client == nil {
		return errors.New("Kafka client is not initialized")
	}
	checkContext, cancel := context.WithTimeout(ctx, client.timeout)
	defer cancel()
	if err := client.client.Ping(checkContext); err != nil {
		return fmt.Errorf("ping Kafka: %w", err)
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
