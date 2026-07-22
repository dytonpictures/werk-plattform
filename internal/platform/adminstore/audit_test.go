package adminstore

import (
	"errors"
	"testing"
	"time"
)

func TestNormalizeSecurityAuditQuery(t *testing.T) {
	validCursor := &SecurityAuditCursor{
		OccurredAt: time.Date(2026, time.July, 21, 12, 0, 0, 0, time.UTC),
		ID:         "0196f000-0000-7000-8000-000000000801",
	}
	tests := []struct {
		name    string
		query   SecurityAuditQuery
		wantErr bool
	}{
		{name: "defaults", query: SecurityAuditQuery{}},
		{name: "all filters", query: SecurityAuditQuery{TenantID: "0196f000-0000-7000-8000-000000000201", EventType: "identity.login.succeeded.v1", Outcome: "succeeded", Limit: 100, Cursor: validCursor}},
		{name: "limit too high", query: SecurityAuditQuery{Limit: 101}, wantErr: true},
		{name: "invalid tenant", query: SecurityAuditQuery{TenantID: "not-a-tenant"}, wantErr: true},
		{name: "invalid event", query: SecurityAuditQuery{EventType: "login"}, wantErr: true},
		{name: "event too long", query: SecurityAuditQuery{EventType: "identity." + string(make([]byte, 257)) + ".v1"}, wantErr: true},
		{name: "invalid outcome", query: SecurityAuditQuery{Outcome: "unknown"}, wantErr: true},
		{name: "invalid cursor", query: SecurityAuditQuery{Cursor: &SecurityAuditCursor{OccurredAt: validCursor.OccurredAt, ID: "invalid"}}, wantErr: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			normalized, _, err := normalizeSecurityAuditQuery(test.query)
			if test.wantErr {
				if !errors.Is(err, ErrInvalidAuditQuery) {
					t.Fatalf("error = %v, want invalid audit query", err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if test.query.Limit == 0 && normalized.Limit != defaultSecurityAuditLimit {
				t.Fatalf("default limit = %d, want %d", normalized.Limit, defaultSecurityAuditLimit)
			}
		})
	}
}
