package identity

import (
	"errors"
	"strings"
	"time"

	"github.com/dytonpictures/werk/internal/core/tenancy"
)

var ErrSecurityAuditInvalid = errors.New("invalid security audit event")

type SecurityAuditOutcome string

const (
	SecurityAuditSucceeded SecurityAuditOutcome = "succeeded"
	SecurityAuditDenied    SecurityAuditOutcome = "denied"
	SecurityAuditFailed    SecurityAuditOutcome = "failed"
)

type SecurityAuditEvent struct {
	ID            [16]byte
	OccurredAt    time.Time
	EventType     string
	Outcome       SecurityAuditOutcome
	AccountID     *AccountID
	TenantID      *tenancy.TenantID
	RequestID     [16]byte
	CorrelationID [16]byte
	//LastUpdate	   StringTime      // Timestamp of the last update to the event optionally used for event versioning and concurrency control
}

func (event SecurityAuditEvent) Validate() error {
	if event.ID == [16]byte{} || event.OccurredAt.IsZero() || event.RequestID == [16]byte{} || event.CorrelationID == [16]byte{} {
		return ErrSecurityAuditInvalid
	}
	if event.Outcome != SecurityAuditSucceeded && event.Outcome != SecurityAuditDenied && event.Outcome != SecurityAuditFailed {
		return ErrSecurityAuditInvalid
	}
	parts := strings.Split(event.EventType, ".")
	if len(parts) < 4 || parts[0] != "identity" || parts[len(parts)-1] != "v1" {
		return ErrSecurityAuditInvalid
	}
	for _, part := range parts {
		if !stableAuditKeyPart(part) {
			return ErrSecurityAuditInvalid
		}
	}
	return nil
}

func stableAuditKeyPart(value string) bool {
	if value == "" {
		return false
	}
	for _, character := range value {
		if (character >= 'a' && character <= 'z') || (character >= '0' && character <= '9') || character == '-' {
			continue
		}
		return false
	}
	return true
}
