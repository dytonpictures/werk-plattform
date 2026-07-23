package identity

import (
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"testing"
)

func TestPasskeyChallenge(t *testing.T) {
	first, err := NewPasskeyChallenge()
	if err != nil {
		t.Fatalf("NewPasskeyChallenge() error = %v", err)
	}
	second, err := NewPasskeyChallenge()
	if err != nil {
		t.Fatalf("NewPasskeyChallenge() second error = %v", err)
	}
	if first.Value == second.Value {
		t.Fatal("NewPasskeyChallenge() returned the same challenge twice")
	}
	raw, err := base64.RawURLEncoding.DecodeString(first.Value)
	if err != nil {
		t.Fatalf("decode challenge: %v", err)
	}
	if len(raw) != PasskeyChallengeSize {
		t.Fatalf("challenge length = %d, want %d", len(raw), PasskeyChallengeSize)
	}
	hash, err := HashPasskeyChallenge(first.Value)
	if err != nil {
		t.Fatalf("HashPasskeyChallenge() error = %v", err)
	}
	if hash != first.Hash || hash != sha256.Sum256(raw) {
		t.Fatal("HashPasskeyChallenge() returned an unexpected digest")
	}

	for _, value := range []string{
		first.Value + "=",
		base64.RawURLEncoding.EncodeToString(make([]byte, PasskeyChallengeSize-1)),
		"not base64url!",
	} {
		if _, err := HashPasskeyChallenge(value); !errors.Is(err, ErrPasskeyInvalid) {
			t.Fatalf("HashPasskeyChallenge(%q) error = %v, want ErrPasskeyInvalid", value, err)
		}
	}
}

func TestPasskeyUserHandleRoundTrip(t *testing.T) {
	accountID := AccountID{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16}
	encoded, err := EncodePasskeyUserHandle(accountID)
	if err != nil {
		t.Fatalf("EncodePasskeyUserHandle() error = %v", err)
	}
	decoded, err := ParsePasskeyUserHandle(encoded)
	if err != nil {
		t.Fatalf("ParsePasskeyUserHandle() error = %v", err)
	}
	if decoded != accountID {
		t.Fatalf("ParsePasskeyUserHandle() = %v, want %v", decoded, accountID)
	}

	if _, err := EncodePasskeyUserHandle(AccountID{}); !errors.Is(err, ErrPasskeyInvalid) {
		t.Fatalf("EncodePasskeyUserHandle(zero) error = %v, want ErrPasskeyInvalid", err)
	}
	for _, value := range []string{
		base64.RawURLEncoding.EncodeToString(make([]byte, PasskeyUserHandleSize-1)),
		base64.RawURLEncoding.EncodeToString(make([]byte, PasskeyUserHandleSize)),
		encoded + "=",
	} {
		if _, err := ParsePasskeyUserHandle(value); !errors.Is(err, ErrPasskeyInvalid) {
			t.Fatalf("ParsePasskeyUserHandle(%q) error = %v, want ErrPasskeyInvalid", value, err)
		}
	}
}

func TestHashPasskeyCredentialIDBoundsAndEncoding(t *testing.T) {
	for _, size := range []int{PasskeyCredentialIDMinSize, PasskeyCredentialIDMaxSize} {
		raw := bytes.Repeat([]byte{0x42}, size)
		value := encodePasskeyTestValue(raw)
		got, err := HashPasskeyCredentialID(value)
		if err != nil {
			t.Fatalf("HashPasskeyCredentialID(%d bytes) error = %v", size, err)
		}
		if want := sha256.Sum256(raw); got != want {
			t.Fatalf("HashPasskeyCredentialID(%d bytes) returned an unexpected digest", size)
		}
	}

	for _, value := range []string{
		"",
		encodePasskeyTestValue(make([]byte, PasskeyCredentialIDMaxSize+1)),
		encodePasskeyTestValue([]byte{1}) + "=",
	} {
		if _, err := HashPasskeyCredentialID(value); !errors.Is(err, ErrPasskeyInvalid) {
			t.Fatalf("HashPasskeyCredentialID(%q) error = %v, want ErrPasskeyInvalid", value, err)
		}
	}
}

func TestPasskeyRegistrationCredentialDecodeForVerification(t *testing.T) {
	credentialID := bytes.Repeat([]byte{0x21}, 32)
	clientData := []byte(`{"type":"webauthn.create","challenge":"challenge","origin":"https://example.test"}`)
	attestation := []byte{0xa3, 0x01, 0x02, 0x03}
	valid := PasskeyRegistrationCredential{
		ID:    encodePasskeyTestValue(credentialID),
		RawID: encodePasskeyTestValue(credentialID),
		Type:  PasskeyPublicKeyCredentialType,
		Response: PasskeyRegistrationCredentialResponse{
			ClientDataJSON:    encodePasskeyTestValue(clientData),
			AttestationObject: encodePasskeyTestValue(attestation),
			Transports:        []string{"internal", "future-transport"},
		},
	}

	decoded, err := valid.DecodeForVerification()
	if err != nil {
		t.Fatalf("DecodeForVerification() error = %v", err)
	}
	if !bytes.Equal(decoded.CredentialID, credentialID) {
		t.Fatalf("credential ID = %x, want %x", decoded.CredentialID, credentialID)
	}
	if decoded.CredentialIDHash != sha256.Sum256(credentialID) {
		t.Fatal("credential ID hash is incorrect")
	}
	if !bytes.Equal(decoded.ClientDataJSON, clientData) || !bytes.Equal(decoded.AttestationObject, attestation) {
		t.Fatal("registration payload was not decoded losslessly")
	}
	if len(decoded.Transports) != 2 || decoded.Transports[1] != "future-transport" {
		t.Fatalf("transports = %v, want unknown transport retained", decoded.Transports)
	}

	tests := map[string]func(*PasskeyRegistrationCredential){
		"wrong type": func(value *PasskeyRegistrationCredential) {
			value.Type = "password"
		},
		"mismatched id": func(value *PasskeyRegistrationCredential) {
			value.RawID = encodePasskeyTestValue([]byte{0x99})
		},
		"non-object client data": func(value *PasskeyRegistrationCredential) {
			value.Response.ClientDataJSON = encodePasskeyTestValue([]byte(`[]`))
		},
		"missing attestation": func(value *PasskeyRegistrationCredential) {
			value.Response.AttestationObject = ""
		},
		"duplicate transport": func(value *PasskeyRegistrationCredential) {
			value.Response.Transports = []string{"internal", "internal"}
		},
		"transport whitespace": func(value *PasskeyRegistrationCredential) {
			value.Response.Transports = []string{" internal"}
		},
	}
	for name, mutate := range tests {
		t.Run(name, func(t *testing.T) {
			candidate := valid
			mutate(&candidate)
			if _, err := candidate.DecodeForVerification(); !errors.Is(err, ErrPasskeyInvalid) {
				t.Fatalf("DecodeForVerification() error = %v, want ErrPasskeyInvalid", err)
			}
		})
	}
}

func TestPasskeyAuthenticationCredentialDecodeForVerification(t *testing.T) {
	credentialID := bytes.Repeat([]byte{0x31}, 32)
	clientData := []byte(`{"type":"webauthn.get","challenge":"challenge","origin":"https://example.test"}`)
	authenticatorData := bytes.Repeat([]byte{0x41}, minPasskeyAuthenticatorDataSize)
	signature := []byte{0x30, 0x44, 0x02, 0x20}
	accountID := AccountID{16, 15, 14, 13, 12, 11, 10, 9, 8, 7, 6, 5, 4, 3, 2, 1}
	userHandle, err := EncodePasskeyUserHandle(accountID)
	if err != nil {
		t.Fatalf("EncodePasskeyUserHandle() error = %v", err)
	}
	valid := PasskeyAuthenticationCredential{
		ID:    encodePasskeyTestValue(credentialID),
		RawID: encodePasskeyTestValue(credentialID),
		Type:  PasskeyPublicKeyCredentialType,
		Response: PasskeyAuthenticationCredentialResponse{
			ClientDataJSON:    encodePasskeyTestValue(clientData),
			AuthenticatorData: encodePasskeyTestValue(authenticatorData),
			Signature:         encodePasskeyTestValue(signature),
			UserHandle:        userHandle,
		},
	}

	decoded, err := valid.DecodeForVerification()
	if err != nil {
		t.Fatalf("DecodeForVerification() error = %v", err)
	}
	if !bytes.Equal(decoded.AuthenticatorData, authenticatorData) || !bytes.Equal(decoded.Signature, signature) {
		t.Fatal("authentication payload was not decoded losslessly")
	}
	if decoded.UserHandle == nil || *decoded.UserHandle != accountID {
		t.Fatalf("user handle = %v, want %v", decoded.UserHandle, accountID)
	}

	withoutUserHandle := valid
	withoutUserHandle.Response.UserHandle = ""
	decoded, err = withoutUserHandle.DecodeForVerification()
	if err != nil {
		t.Fatalf("DecodeForVerification() without user handle error = %v", err)
	}
	if decoded.UserHandle != nil {
		t.Fatalf("user handle = %v, want nil", decoded.UserHandle)
	}

	tests := map[string]func(*PasskeyAuthenticationCredential){
		"short authenticator data": func(value *PasskeyAuthenticationCredential) {
			value.Response.AuthenticatorData = encodePasskeyTestValue(make([]byte, minPasskeyAuthenticatorDataSize-1))
		},
		"missing signature": func(value *PasskeyAuthenticationCredential) {
			value.Response.Signature = ""
		},
		"invalid user handle": func(value *PasskeyAuthenticationCredential) {
			value.Response.UserHandle = encodePasskeyTestValue([]byte{1})
		},
		"invalid client data": func(value *PasskeyAuthenticationCredential) {
			value.Response.ClientDataJSON = encodePasskeyTestValue([]byte(`not-json`))
		},
	}
	for name, mutate := range tests {
		t.Run(name, func(t *testing.T) {
			candidate := valid
			mutate(&candidate)
			if _, err := candidate.DecodeForVerification(); !errors.Is(err, ErrPasskeyInvalid) {
				t.Fatalf("DecodeForVerification() error = %v, want ErrPasskeyInvalid", err)
			}
		})
	}
}

func encodePasskeyTestValue(value []byte) string {
	return base64.RawURLEncoding.EncodeToString(value)
}
