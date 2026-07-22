package kafkastream

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"testing"
	"time"

	"github.com/dytonpictures/werk/internal/core/events"
	"github.com/dytonpictures/werk/internal/core/tenancy"
	"github.com/dytonpictures/werk/internal/platform/config"
)

type recordingWriter struct{ messages chan Message }

func (writer *recordingWriter) Publish(_ context.Context, message Message) error {
	writer.messages <- message
	return nil
}

func testExporter(t *testing.T) (*Exporter, *recordingWriter) {
	t.Helper()
	writer := &recordingWriter{messages: make(chan Message, 10)}
	exporter, err := NewExporter(writer, config.KafkaConfig{
		DomainEventsTopic:  "platform.domain-events.v1",
		SecurityAuditTopic: "platform.security-audit.v1",
		RuntimeLogsTopic:   "platform.runtime-logs.v1",
	})
	if err != nil {
		t.Fatal(err)
	}
	return exporter, writer
}

func TestDomainEnvelopeIncludesStableContextAndTags(t *testing.T) {
	exporter, writer := testExporter(t)
	event := events.Event{
		ID: [16]byte{1}, TenantID: tenancy.TenantID{2}, Type: "core.example.created.v1",
		Producer: "core.example", SubjectKind: "example.item", SubjectID: [16]byte{3},
		PartitionKey: "item:3", OccurredAt: time.Date(2026, 7, 22, 12, 0, 0, 0, time.UTC),
		CorrelationID: [16]byte{4}, Tags: map[string]string{"organization.unit": "finance"},
		Payload: json.RawMessage(`{"name":"example"}`),
	}
	if err := exporter.PublishDomain(context.Background(), event); err != nil {
		t.Fatal(err)
	}
	message := <-writer.messages
	if message.Topic != "platform.domain-events.v1" || message.Key != event.TenantID.String()+":item:3" {
		t.Fatalf("unexpected Kafka routing: %#v", message)
	}
	var envelope domainEnvelope
	if err := json.Unmarshal(message.Value, &envelope); err != nil {
		t.Fatal(err)
	}
	if envelope.SpecVersion != envelopeVersion || envelope.Tags[events.TagDataClassification] != "restricted" || envelope.Tags["organization.unit"] != "finance" {
		t.Fatalf("unexpected envelope: %#v", envelope)
	}
}

func TestAuditEnvelopeIsMinimizedAndClassified(t *testing.T) {
	exporter, writer := testExporter(t)
	tenantID := [16]byte{2}
	accountID := [16]byte{3}
	record := AuditRecord{
		ID: [16]byte{1}, OccurredAt: time.Now().UTC(), EventType: "identity.login.succeeded.v1",
		Outcome: "succeeded", TenantID: &tenantID, AccountID: &accountID,
		RequestID: [16]byte{4}, CorrelationID: [16]byte{5},
	}
	if err := exporter.PublishAudit(context.Background(), record); err != nil {
		t.Fatal(err)
	}
	message := <-writer.messages
	if message.Topic != "platform.security-audit.v1" || bytes.Contains(message.Value, []byte("details")) || bytes.Contains(message.Value, []byte("session")) {
		t.Fatalf("audit stream contains unexpected data: %s", message.Value)
	}
	var envelope auditEnvelope
	if err := json.Unmarshal(message.Value, &envelope); err != nil {
		t.Fatal(err)
	}
	if envelope.Tags[events.TagProcessingPurpose] != "security-and-accountability" || envelope.Actor == nil {
		t.Fatalf("unexpected audit envelope: %#v", envelope)
	}
}

func TestKafkaLogHandlerRedactsSecrets(t *testing.T) {
	exporter, writer := testExporter(t)
	base := slog.New(slog.NewJSONHandler(&bytes.Buffer{}, nil))
	logger, sink := NewKafkaLogger(base, exporter, LogMetadata{
		Service: "api", Environment: "test", BuildVersion: "1.0.0", InstanceID: "api-1",
	})
	logger.Info("request handled", "correlation_id", "correlation-1", "access_token", "never-export")
	select {
	case message := <-writer.messages:
		if message.Topic != "platform.runtime-logs.v1" || bytes.Contains(message.Value, []byte("never-export")) || !bytes.Contains(message.Value, []byte("[REDACTED]")) {
			t.Fatalf("unexpected log envelope: %s", message.Value)
		}
	case <-time.After(time.Second):
		t.Fatal("log was not exported")
	}
	closeContext, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	sink.Close(closeContext)
}
