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
)

const MaximumPayloadBytes = 1 << 20

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
	return nil
}

func ValidConsumerKey(value string) bool { return stableKeyPattern.MatchString(value) }
func ValidEventType(value string) bool   { return eventTypePattern.MatchString(value) }
