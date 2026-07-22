package identity

import (
	"errors"
	"strings"
	"testing"
)

func TestAPIKeyRoundTripUsesOnlyPersistableDigests(t *testing.T) {
	material, err := NewAPIKey()
	if err != nil {
		t.Fatalf("NewAPIKey() error = %v", err)
	}
	if material.Token == "" || strings.Contains(material.Token, "=") {
		t.Fatalf("NewAPIKey() token is not canonical: %q", material.Token)
	}
	digest, err := DigestAPIKey(material.Token)
	if err != nil {
		t.Fatalf("DigestAPIKey() error = %v", err)
	}
	if digest.PublicIDHash != material.PublicIDHash || !VerifyAPIKeySecret(material.SecretHash, digest.SecretHash) {
		t.Fatal("API key digests do not match generated material")
	}
	other, err := NewAPIKey()
	if err != nil {
		t.Fatal(err)
	}
	otherDigest, err := DigestAPIKey(other.Token)
	if err != nil {
		t.Fatal(err)
	}
	if VerifyAPIKeySecret(material.SecretHash, otherDigest.SecretHash) {
		t.Fatal("different API key secret was accepted")
	}
}

func TestDigestAPIKeyRejectsMalformedOrNonCanonicalValues(t *testing.T) {
	material, err := NewAPIKey()
	if err != nil {
		t.Fatal(err)
	}
	parts := strings.Split(material.Token, ".")
	for _, value := range []string{
		"",
		"ak2." + parts[1] + "." + parts[2],
		material.Token + "=",
		" " + material.Token,
		"ak1.short.short",
	} {
		if _, err := DigestAPIKey(value); !errors.Is(err, ErrAPIKeyInvalid) {
			t.Fatalf("DigestAPIKey(%q) error = %v, want ErrAPIKeyInvalid", value, err)
		}
	}
}
