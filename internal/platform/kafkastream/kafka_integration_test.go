package kafkastream

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/twmb/franz-go/pkg/kgo"

	"github.com/dytonpictures/werk/internal/core/events"
	"github.com/dytonpictures/werk/internal/core/tenancy"
	"github.com/dytonpictures/werk/internal/platform/config"
)

func TestKafkaIntegrationPublishesConsumableEnvelope(t *testing.T) {
	brokersValue := strings.TrimSpace(os.Getenv("WERK_TEST_KAFKA_BROKERS"))
	if brokersValue == "" {
		t.Skip("WERK_TEST_KAFKA_BROKERS is not configured")
	}
	brokers := strings.Split(brokersValue, ",")
	topic := "platform.domain-events.v1"
	configuration := config.KafkaConfig{
		Enabled: true, Brokers: brokers, ClientID: "platform-integration-producer",
		DomainEventsTopic: topic, SecurityAuditTopic: "platform.security-audit.v1",
		RuntimeLogsTopic: "platform.runtime-logs.v1", SASLMechanism: "none",
		PublishTimeout: 10 * time.Second,
	}
	client, err := NewClient(configuration)
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	checkContext, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := client.Ping(checkContext); err != nil {
		t.Fatal(err)
	}
	exporter, err := NewExporter(client, configuration)
	if err != nil {
		t.Fatal(err)
	}
	eventID := randomBytes16(t)
	event := events.Event{
		ID: eventID, TenantID: tenancy.TenantID(randomBytes16(t)), Type: "core.integration.streamed.v1",
		Producer: "core.integration", SubjectKind: "integration.item", SubjectID: randomBytes16(t),
		PartitionKey: "integration:" + uuidString(eventID), OccurredAt: time.Now().UTC(),
		CorrelationID: randomBytes16(t), Payload: json.RawMessage(`{"verified":true}`),
	}
	if err := exporter.PublishDomain(checkContext, event); err != nil {
		t.Fatal(err)
	}

	consumer, err := kgo.NewClient(
		kgo.SeedBrokers(brokers...),
		kgo.ClientID("platform-integration-consumer"),
		kgo.ConsumerGroup("platform-integration-"+uuidString(eventID)),
		kgo.ConsumeTopics(topic),
		kgo.ConsumeResetOffset(kgo.NewOffset().AtStart()),
	)
	if err != nil {
		t.Fatal(err)
	}
	defer consumer.Close()
	consumeContext, consumeCancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer consumeCancel()
	for consumeContext.Err() == nil {
		fetches := consumer.PollFetches(consumeContext)
		if err := fetches.Err0(); err != nil {
			if consumeContext.Err() != nil {
				break
			}
			t.Fatal(err)
		}
		for _, record := range fetches.Records() {
			for _, header := range record.Headers {
				if header.Key == "event-id" && string(header.Value) == uuidString(eventID) {
					return
				}
			}
		}
	}
	t.Fatal("published event was not observed by Kafka consumer")
}

func randomBytes16(t *testing.T) [16]byte {
	t.Helper()
	var value [16]byte
	if _, err := rand.Read(value[:]); err != nil {
		t.Fatal(err)
	}
	return value
}
