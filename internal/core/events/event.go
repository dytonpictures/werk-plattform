// Package events defines provider-independent contracts for durable asynchronous
// work. It contains no PostgreSQL, Valkey, HTTP, or module-specific behavior.
package events

import (
	"encoding/json"
	"errors"
	"regexp"
	"strings"
	"time"

	"github.com/dytonpictures/werk/internal/core/tenancy"
)

var (
	ErrInvalidEvent    = errors.New("invalid domain event")
	ErrInvalidConsumer = errors.New("invalid event consumer")
	eventTypePattern   = regexp.MustCompile(`^[a-z][a-z0-9-]*(?:\.[a-z][a-z0-9-]*)+\.v[1-9][0-9]*$`)
	stableKeyPattern   = regexp.MustCompile(`^[a-z][a-z0-9._-]{0,127}$`)
	tagKeyPattern      = regexp.MustCompile(`^[a-z][a-z0-9._-]{0,63}$`)
	tagValuePattern    = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9._:/-]{0,127}$`)
)

const (
	MaximumPayloadBytes = 1 << 20
	MaximumTagBytes     = 8 << 10
	MaximumTagCount     = 32

	TagDataClassification = "data.classification"
	TagProcessingPurpose  = "processing.purpose"
	TagRetentionClass     = "retention.class"
)

var defaultTags = map[string]string{
	TagDataClassification: "restricted",
	TagProcessingPurpose:  "platform-event-delivery",
	TagRetentionClass:     "domain-event",
}

type Event struct {
	ID            [16]byte
	TenantID      tenancy.TenantID
	Type          string
	Producer      string
	SubjectKind   string
	SubjectID     [16]byte
	PartitionKey  string
	OccurredAt    time.Time
	CorrelationID [16]byte
	CausationID   *[16]byte
	Tags          map[string]string
	Payload       json.RawMessage
}

func (event Event) Validate() error {
	if event.ID == [16]byte{} || event.TenantID.IsZero() || event.SubjectID == [16]byte{} || event.CorrelationID == [16]byte{} || event.OccurredAt.IsZero() {
		return ErrInvalidEvent
	}
	if !eventTypePattern.MatchString(event.Type) || !stableKeyPattern.MatchString(event.Producer) || !stableKeyPattern.MatchString(event.SubjectKind) {
		return ErrInvalidEvent
	}
	if strings.TrimSpace(event.PartitionKey) == "" || len(event.PartitionKey) > 200 || len(event.Payload) == 0 || len(event.Payload) > MaximumPayloadBytes || !json.Valid(event.Payload) {
		return ErrInvalidEvent
	}
	if err := ValidateTags(NormalizeTags(event.Tags)); err != nil {
		return ErrInvalidEvent
	}
	return nil
}

func ValidConsumerKey(value string) bool { return stableKeyPattern.MatchString(value) }
func ValidEventType(value string) bool   { return eventTypePattern.MatchString(value) }

// NormalizeTags returns an owned tag map with conservative platform defaults.
// Producers may add context, but omitting classification, purpose, or retention
// never creates an unclassified event.
func NormalizeTags(tags map[string]string) map[string]string {
	normalized := make(map[string]string, len(tags)+len(defaultTags))
	for key, value := range defaultTags {
		normalized[key] = value
	}
	for key, value := range tags {
		normalized[key] = value
	}
	return normalized
}

func ValidateTags(tags map[string]string) error {
	if len(tags) < len(defaultTags) || len(tags) > MaximumTagCount {
		return ErrInvalidEvent
	}
	for key, value := range tags {
		if !tagKeyPattern.MatchString(key) || !tagValuePattern.MatchString(value) {
			return ErrInvalidEvent
		}
	}
	for key := range defaultTags {
		if tags[key] == "" {
			return ErrInvalidEvent
		}
	}
	switch tags[TagDataClassification] {
	case "public", "internal", "confidential", "restricted":
	default:
		return ErrInvalidEvent
	}
	encoded, err := json.Marshal(tags)
	if err != nil || len(encoded) > MaximumTagBytes {
		return ErrInvalidEvent
	}
	return nil
}
