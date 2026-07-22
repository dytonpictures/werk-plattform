package identity

import (
	"testing"
	"time"
)

func TestSecurityAuditEventRequiresVersionedStableType(t *testing.T) {
	event := SecurityAuditEvent{
		ID: [16]byte{1}, OccurredAt: time.Now().UTC(), EventType: "identity.login.succeeded.v1",
		Outcome: SecurityAuditSucceeded, RequestID: [16]byte{2}, CorrelationID: [16]byte{3},
	}
	if err := event.Validate(); err != nil {
		t.Fatalf("validate event: %v", err)
	}
	event.EventType = "login succeeded"
	if err := event.Validate(); err != ErrSecurityAuditInvalid {
		t.Fatalf("invalid event error = %v", err)
	}
}
