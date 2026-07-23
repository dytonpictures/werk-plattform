package identity

import (
	"strings"
	"testing"
	"time"
)

func TestMFAFactorValidation(t *testing.T) {
	now := time.Now().UTC()
	factor := MFAFactor{
		ID: [16]byte{1}, AccountID: AccountID{2}, Kind: MFAFactorWebAuthn,
		Status: MFAFactorActive, DisplayName: "Security key", CreatedAt: now, ActivatedAt: &now,
	}
	if err := factor.Validate(); err != nil {
		t.Fatalf("validate factor: %v", err)
	}
	factor.Kind = "password"
	if err := factor.Validate(); err != ErrMFAInvalid {
		t.Fatalf("invalid factor error = %v", err)
	}
}

func TestTOTPMatchesRFC6238SHA1Vector(t *testing.T) {
	secret := "GEZDGNBVGY3TQOJQGEZDGNBVGY3TQOJQ" // gitleaks:allow -- RFC 6238 test vector
	code, err := TOTPCode(secret, time.Unix(59, 0))
	if err != nil {
		t.Fatal(err)
	}
	// RFC 6238 uses eight digits and yields 94287082. The same dynamic
	// truncation in WERK's six-digit profile therefore yields 287082.
	if code != "287082" {
		t.Fatalf("code = %q, want 287082", code)
	}
	if !VerifyTOTP(secret, code, time.Unix(59, 0)) {
		t.Fatal("valid code was rejected")
	}
	if VerifyTOTP(secret, "287083", time.Unix(59, 0)) {
		t.Fatal("invalid code was accepted")
	}
}

func TestTOTPUriDoesNotLoseAccountContext(t *testing.T) {
	uri, err := TOTPUri("WERK", "admin@werk.local", "GEZDGNBVGY3TQOJQGEZDGNBVGY3TQOJQ")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(uri, "otpauth://totp/WERK:admin@werk.local?") {
		t.Fatalf("unexpected uri %q", uri)
	}
}

func TestMFAChallengeValidation(t *testing.T) {
	now := time.Now().UTC()
	challenge := MFAChallenge{
		ID: [16]byte{1}, AccountID: AccountID{2}, Purpose: MFAChallengeReauth,
		ExpiresAt: now.Add(5 * time.Minute),
	}
	if err := challenge.Validate(now); err != nil {
		t.Fatalf("validate challenge: %v", err)
	}
	if err := challenge.Validate(now.Add(6 * time.Minute)); err != ErrMFAInvalid {
		t.Fatalf("expired challenge error = %v", err)
	}
}
