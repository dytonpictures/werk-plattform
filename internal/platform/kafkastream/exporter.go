package kafkastream

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/dytonpictures/werk/internal/core/events"
	"github.com/dytonpictures/werk/internal/platform/config"
	"github.com/dytonpictures/werk/internal/platform/database"
	"github.com/dytonpictures/werk/internal/platform/outbox"
)

const (
	envelopeVersion   = "platform.event-envelope.v1"
	domainConsumerKey = "platform.kafka.domain-events.v1"
)

type Exporter struct {
	writer            Writer
	domainEventsTopic string
	auditTopic        string
	logsTopic         string
}

func NewExporter(writer Writer, configuration config.KafkaConfig) (*Exporter, error) {
	if writer == nil || configuration.DomainEventsTopic == "" || configuration.SecurityAuditTopic == "" || configuration.RuntimeLogsTopic == "" {
		return nil, errors.New("invalid Kafka exporter configuration")
	}
	return &Exporter{
		writer:            writer,
		domainEventsTopic: configuration.DomainEventsTopic,
		auditTopic:        configuration.SecurityAuditTopic,
		logsTopic:         configuration.RuntimeLogsTopic,
	}, nil
}

type domainEnvelope struct {
	SpecVersion   string            `json:"spec_version"`
	Category      string            `json:"category"`
	ID            string            `json:"id"`
	Type          string            `json:"type"`
	OccurredAt    time.Time         `json:"occurred_at"`
	TenantID      string            `json:"tenant_id"`
	Producer      string            `json:"producer"`
	Subject       subjectRef        `json:"subject"`
	PartitionKey  string            `json:"partition_key"`
	CorrelationID string            `json:"correlation_id"`
	CausationID   string            `json:"causation_id,omitempty"`
	Tags          map[string]string `json:"tags"`
	Data          json.RawMessage   `json:"data"`
}

type subjectRef struct {
	Kind string `json:"kind"`
	ID   string `json:"id"`
}

type AuditRecord struct {
	ID            [16]byte
	OccurredAt    time.Time
	EventType     string
	Outcome       string
	AccountID     *[16]byte
	TenantID      *[16]byte
	RequestID     [16]byte
	CorrelationID [16]byte
}

type auditEnvelope struct {
	SpecVersion   string            `json:"spec_version"`
	Category      string            `json:"category"`
	ID            string            `json:"id"`
	Type          string            `json:"type"`
	OccurredAt    time.Time         `json:"occurred_at"`
	Outcome       string            `json:"outcome"`
	TenantID      string            `json:"tenant_id,omitempty"`
	Actor         *auditActor       `json:"actor,omitempty"`
	RequestID     string            `json:"request_id"`
	CorrelationID string            `json:"correlation_id"`
	Tags          map[string]string `json:"tags"`
}

type auditActor struct {
	AccountID string `json:"account_id"`
}

func (exporter *Exporter) PublishDomain(ctx context.Context, event events.Event) error {
	if event.Validate() != nil {
		return events.ErrInvalidEvent
	}
	tags := events.NormalizeTags(event.Tags)
	envelope := domainEnvelope{
		SpecVersion: envelopeVersion,
		Category:    "domain-event",
		ID:          uuidString(event.ID), Type: event.Type, OccurredAt: event.OccurredAt,
		TenantID: event.TenantID.String(), Producer: event.Producer,
		Subject:      subjectRef{Kind: event.SubjectKind, ID: uuidString(event.SubjectID)},
		PartitionKey: event.PartitionKey, CorrelationID: uuidString(event.CorrelationID),
		Tags: tags, Data: event.Payload,
	}
	if event.CausationID != nil {
		envelope.CausationID = uuidString(*event.CausationID)
	}
	encoded, err := json.Marshal(envelope)
	if err != nil {
		return fmt.Errorf("encode domain event envelope: %w", err)
	}
	return exporter.writer.Publish(ctx, Message{
		Topic:     exporter.domainEventsTopic,
		Key:       event.TenantID.String() + ":" + event.PartitionKey,
		Value:     encoded,
		Timestamp: event.OccurredAt,
		Headers: map[string]string{
			"content-type":        "application/json",
			"spec-version":        envelopeVersion,
			"event-id":            uuidString(event.ID),
			"event-type":          event.Type,
			"tenant-id":           event.TenantID.String(),
			"data-classification": tags[events.TagDataClassification],
		},
	})
}

func (exporter *Exporter) PublishAudit(ctx context.Context, record AuditRecord) error {
	if record.ID == [16]byte{} || record.OccurredAt.IsZero() || record.EventType == "" || record.RequestID == [16]byte{} || record.CorrelationID == [16]byte{} {
		return errors.New("invalid audit stream record")
	}
	tags := map[string]string{
		events.TagDataClassification: "restricted",
		events.TagProcessingPurpose:  "security-and-accountability",
		events.TagRetentionClass:     "security-audit",
	}
	envelope := auditEnvelope{
		SpecVersion: envelopeVersion, Category: "security-audit",
		ID: uuidString(record.ID), Type: record.EventType, OccurredAt: record.OccurredAt,
		Outcome: record.Outcome, RequestID: uuidString(record.RequestID),
		CorrelationID: uuidString(record.CorrelationID), Tags: tags,
	}
	key := "installation"
	headers := map[string]string{
		"content-type": "application/json", "spec-version": envelopeVersion,
		"event-id": uuidString(record.ID), "event-type": record.EventType,
		"data-classification": tags[events.TagDataClassification],
	}
	if record.TenantID != nil {
		envelope.TenantID = uuidString(*record.TenantID)
		key = envelope.TenantID
		headers["tenant-id"] = envelope.TenantID
	}
	if record.AccountID != nil {
		envelope.Actor = &auditActor{AccountID: uuidString(*record.AccountID)}
	}
	encoded, err := json.Marshal(envelope)
	if err != nil {
		return fmt.Errorf("encode audit envelope: %w", err)
	}
	return exporter.writer.Publish(ctx, Message{
		Topic: exporter.auditTopic, Key: key, Value: encoded,
		Timestamp: record.OccurredAt, Headers: headers,
	})
}

type DomainConsumer struct{ exporter *Exporter }

func NewDomainConsumer(exporter *Exporter) (*DomainConsumer, error) {
	if exporter == nil {
		return nil, errors.New("Kafka exporter is required")
	}
	return &DomainConsumer{exporter: exporter}, nil
}

func (consumer *DomainConsumer) Key() string       { return domainConsumerKey }
func (consumer *DomainConsumer) EventType() string { return outbox.AllEventTypes }
func (consumer *DomainConsumer) Handle(ctx context.Context, _ database.TenantTx, event events.Event) error {
	return consumer.exporter.PublishDomain(ctx, event)
}

func uuidString(value [16]byte) string {
	return fmt.Sprintf("%x-%x-%x-%x-%x", value[0:4], value[4:6], value[6:8], value[8:10], value[10:16])
}
