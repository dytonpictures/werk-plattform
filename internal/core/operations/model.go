// Package operations defines the minimized, installation-scoped observation
// model used by WERK's operator surfaces. It contains bounded technical state,
// never tenant, event, credential, host or process identifiers.
package operations

import (
	"errors"
	"strings"
	"time"
	"unicode/utf8"
)

var ErrInvalidSummary = errors.New("invalid platform operations summary")

const (
	maximumBuildVersionLength = 128
	maximumComponents         = 8
	maximumQueues             = 4
)

// State is deliberately not boolean. Disabled optional infrastructure and an
// unobserved component must not be presented as either healthy or failed.
type State string

const (
	StateReady     State = "ready"
	StateDegraded  State = "degraded"
	StateAttention State = "attention"
	StateDisabled  State = "disabled"
	StateUnknown   State = "unknown"
)

func (state State) Valid() bool {
	switch state {
	case StateReady, StateDegraded, StateAttention, StateDisabled, StateUnknown:
		return true
	default:
		return false
	}
}

type ComponentKey string

const (
	ComponentAPI        ComponentKey = "api"
	ComponentPostgreSQL ComponentKey = "postgresql"
	ComponentWorker     ComponentKey = "worker"
	ComponentKafka      ComponentKey = "kafka"
)

func (key ComponentKey) Valid() bool {
	switch key {
	case ComponentAPI, ComponentPostgreSQL, ComponentWorker, ComponentKafka:
		return true
	default:
		return false
	}
}

type QueueKey string

const (
	QueueDomainEvents        QueueKey = "domain-events"
	QueueSecurityAuditExport QueueKey = "security-audit-export"
)

func (key QueueKey) Valid() bool {
	return key == QueueDomainEvents || key == QueueSecurityAuditExport
}

// Installation identifies only the running software contract. It deliberately
// excludes network addresses, database coordinates and host information.
type Installation struct {
	Environment  string
	BuildVersion string
	APIVersion   string
	Uptime       time.Duration
}

func (installation Installation) Validate() error {
	if installation.Environment != "development" && installation.Environment != "test" && installation.Environment != "production" {
		return ErrInvalidSummary
	}
	if !ValidBuildVersion(installation.BuildVersion) || !validAPIVersion(installation.APIVersion) || installation.Uptime < 0 {
		return ErrInvalidSummary
	}
	return nil
}

// ValidBuildVersion accepts development and release labels while excluding
// blank, non-canonical or control-bearing values from operational records.
func ValidBuildVersion(value string) bool {
	return validBoundedText(value, maximumBuildVersionLength)
}

type Component struct {
	Key            ComponentKey
	State          State
	Required       bool
	LastObservedAt time.Time
}

func (component Component) validate(summaryObservedAt time.Time) error {
	if !component.Key.Valid() || !component.State.Valid() || (component.Required && component.State == StateDisabled) {
		return ErrInvalidSummary
	}
	if component.State != StateDisabled && component.State != StateUnknown && component.LastObservedAt.IsZero() {
		return ErrInvalidSummary
	}
	if !component.LastObservedAt.IsZero() &&
		(component.LastObservedAt.Location() != time.UTC || component.LastObservedAt.After(summaryObservedAt)) {
		return ErrInvalidSummary
	}
	return nil
}

// Queue contains only installation-wide delivery aggregates. No event type,
// tenant, subject, lease owner, error or payload belongs in this model.
type Queue struct {
	Key                 QueueKey
	State               State
	Pending             uint64
	Processing          uint64
	Retry               uint64
	Dead                uint64
	OldestOutstandingAt time.Time
}

func (queue Queue) validate(summaryObservedAt time.Time) error {
	if !queue.Key.Valid() || !queue.State.Valid() {
		return ErrInvalidSummary
	}
	hasOutstanding := queue.Pending > 0 || queue.Processing > 0 || queue.Retry > 0 || queue.Dead > 0
	if queue.State == StateUnknown && (hasOutstanding || !queue.OldestOutstandingAt.IsZero()) {
		return ErrInvalidSummary
	}
	if queue.State == StateReady && (queue.Retry > 0 || queue.Dead > 0) {
		return ErrInvalidSummary
	}
	if !queue.OldestOutstandingAt.IsZero() &&
		(queue.OldestOutstandingAt.Location() != time.UTC || queue.OldestOutstandingAt.After(summaryObservedAt)) {
		return ErrInvalidSummary
	}
	return nil
}

type Summary struct {
	ObservedAt   time.Time
	Overall      State
	Installation Installation
	Components   []Component
	Queues       []Queue
}

// Validate fails closed when mandatory synchronous components or the two
// authoritative delivery queues are absent. A ready overall state is only
// valid when every required component is ready and every queue is either ready
// or intentionally disabled.
func (summary Summary) Validate() error {
	if summary.ObservedAt.IsZero() || summary.ObservedAt.Location() != time.UTC ||
		!summary.Overall.Valid() || summary.Overall == StateDisabled ||
		summary.Installation.Validate() != nil || len(summary.Components) == 0 ||
		len(summary.Components) > maximumComponents || len(summary.Queues) == 0 || len(summary.Queues) > maximumQueues {
		return ErrInvalidSummary
	}

	components := make(map[ComponentKey]struct{}, len(summary.Components))
	for _, component := range summary.Components {
		if component.validate(summary.ObservedAt) != nil {
			return ErrInvalidSummary
		}
		if _, duplicate := components[component.Key]; duplicate {
			return ErrInvalidSummary
		}
		components[component.Key] = struct{}{}
		if summary.Overall == StateReady && component.Required && component.State != StateReady {
			return ErrInvalidSummary
		}
	}
	for _, required := range []ComponentKey{ComponentAPI, ComponentPostgreSQL} {
		if _, present := components[required]; !present {
			return ErrInvalidSummary
		}
	}

	queues := make(map[QueueKey]struct{}, len(summary.Queues))
	for _, queue := range summary.Queues {
		if queue.validate(summary.ObservedAt) != nil {
			return ErrInvalidSummary
		}
		if _, duplicate := queues[queue.Key]; duplicate {
			return ErrInvalidSummary
		}
		queues[queue.Key] = struct{}{}
		if summary.Overall == StateReady && queue.State != StateReady && queue.State != StateDisabled {
			return ErrInvalidSummary
		}
	}
	for _, required := range []QueueKey{QueueDomainEvents, QueueSecurityAuditExport} {
		if _, present := queues[required]; !present {
			return ErrInvalidSummary
		}
	}
	return nil
}

func validAPIVersion(value string) bool {
	if len(value) < 2 || value[0] != 'v' {
		return false
	}
	for _, character := range value[1:] {
		if character < '0' || character > '9' {
			return false
		}
	}
	return true
}

func validBoundedText(value string, maximumRunes int) bool {
	return value != "" && value == strings.TrimSpace(value) && utf8.ValidString(value) &&
		strings.IndexByte(value, 0) < 0 && utf8.RuneCountInString(value) <= maximumRunes
}
