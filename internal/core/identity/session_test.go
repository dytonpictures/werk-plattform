package identity

import (
	"testing"
	"time"
)

func TestResolveSessionRejectsWrongAudienceAndExpiredRecords(t *testing.T) {
	now := time.Date(2026, 7, 19, 12, 0, 0, 0, time.UTC)
	record := SessionRecord{
		ID:        SessionID{1},
		Account:   AuthenticatedActor{AccountID: AccountID{2}, AccountClass: AccountClassAdmin, Audience: AudienceAdmin, Kind: AuthenticationInteractive, Assurance: AssuranceMultiFactor},
		Audience:  AudienceWork,
		ExpiresAt: now.Add(time.Hour),
	}
	if _, err := ResolveSession(record, AccessPlaneAdmin, now); err != ErrSessionInvalid {
		t.Fatalf("wrong audience error = %v, want %v", err, ErrSessionInvalid)
	}
	record.Audience = AudienceAdmin
	record.ExpiresAt = now
	if _, err := ResolveSession(record, AccessPlaneAdmin, now); err != ErrSessionExpired {
		t.Fatalf("expired error = %v, want %v", err, ErrSessionExpired)
	}
}

func TestValidateSessionRecordAllowsAdminSetupSessionWithoutAuthorizingAdminPlane(t *testing.T) {
	now := time.Date(2026, 7, 21, 12, 0, 0, 0, time.UTC)
	record := SessionRecord{
		ID: SessionID{1},
		Account: AuthenticatedActor{
			AccountID: AccountID{2}, AccountClass: AccountClassAdmin, Audience: AudienceAdmin,
			Kind: AuthenticationInteractive, Assurance: AssuranceSingleFactor,
		},
		Audience: AudienceAdmin, ExpiresAt: now.Add(time.Hour),
	}
	if _, err := ValidateSessionRecord(record, now); err != nil {
		t.Fatalf("ValidateSessionRecord() error = %v", err)
	}
	if _, err := ResolveSession(record, AccessPlaneAdmin, now); err != ErrAccessDenied {
		t.Fatalf("ResolveSession() error = %v, want ErrAccessDenied", err)
	}
}
