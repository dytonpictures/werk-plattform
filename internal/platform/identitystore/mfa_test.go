package identitystore

import (
	"strings"
	"testing"
)

func TestMFASecretEncryptionIsBoundToAccountAndFactor(t *testing.T) {
	service := &Service{
		mfaCurrentKeyID: "primary",
		mfaKeys:         map[string][]byte{"primary": []byte("0123456789abcdef0123456789abcdef")},
	}
	reference, err := service.encryptMFASecret("account-a", "factor-a", "TOPSECRET")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(reference, "TOPSECRET") {
		t.Fatal("encrypted reference exposes plaintext")
	}
	plaintext, err := service.decryptMFASecret("account-a", "factor-a", reference)
	if err != nil || plaintext != "TOPSECRET" {
		t.Fatalf("decrypt = %q, %v", plaintext, err)
	}
	if _, err := service.decryptMFASecret("account-b", "factor-a", reference); err == nil {
		t.Fatal("ciphertext was accepted for another account")
	}
}

func TestMFASecretKeyringRetainsPreviousKeysForRotation(t *testing.T) {
	oldKey := []byte("0123456789abcdef0123456789abcdef")
	newKey := []byte("abcdef0123456789abcdef0123456789")
	oldService := &Service{mfaCurrentKeyID: "old", mfaKeys: map[string][]byte{"old": oldKey}}
	reference, err := oldService.encryptMFASecret("account-a", "factor-a", "TOPSECRET")
	if err != nil {
		t.Fatal(err)
	}
	rotatedService := &Service{
		mfaCurrentKeyID: "current",
		mfaKeys:         map[string][]byte{"current": newKey, "old": oldKey},
	}
	plaintext, err := rotatedService.decryptMFASecret("account-a", "factor-a", reference)
	if err != nil || plaintext != "TOPSECRET" {
		t.Fatalf("decrypt after rotation = %q, %v", plaintext, err)
	}
	newReference, err := rotatedService.encryptMFASecret("account-a", "factor-a", "NEWSECRET")
	if err != nil || !strings.HasPrefix(newReference, "enc:v2:current:") {
		t.Fatalf("new key reference = %q, %v", newReference, err)
	}
}

func TestRecoveryCodesAreRandomAndNormalizedForHashing(t *testing.T) {
	codes, err := newRecoveryCodes(10)
	if err != nil {
		t.Fatal(err)
	}
	if len(codes) != 10 {
		t.Fatalf("code count = %d", len(codes))
	}
	seen := map[string]bool{}
	for _, code := range codes {
		if seen[code] || len(code) != 19 {
			t.Fatalf("invalid or duplicate recovery code %q", code)
		}
		seen[code] = true
		if recoveryCodeHash(code) != recoveryCodeHash(strings.ToLower(strings.ReplaceAll(code, "-", ""))) {
			t.Fatal("recovery code normalization changed its hash")
		}
	}
}
