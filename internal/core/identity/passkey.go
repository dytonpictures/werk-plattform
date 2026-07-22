package identity

import (
	"bytes"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"unicode/utf8"
)

const (
	PasskeyPublicKeyCredentialType = "public-key"

	// PasskeyChallengeSize is deliberately larger than WebAuthn's 16-byte
	// minimum and gives every ceremony a fresh 256-bit challenge.
	PasskeyChallengeSize = 32

	// Account IDs are used as opaque, non-PII WebAuthn user handles.
	PasskeyUserHandleSize = 16

	PasskeyCredentialIDMinSize = 1
	PasskeyCredentialIDMaxSize = 1023

	maxPasskeyClientDataJSONSize    = 64 << 10
	maxPasskeyAttestationSize       = 512 << 10
	minPasskeyAuthenticatorDataSize = 37
	maxPasskeyAuthenticatorDataSize = 64 << 10
	maxPasskeySignatureSize         = 16 << 10
	maxPasskeyTransports            = 16
	maxPasskeyTransportLength       = 64
)

var ErrPasskeyInvalid = errors.New("invalid passkey data")

// PasskeyChallenge contains the browser-safe challenge and the digest that may
// be stored server-side. The plaintext challenge must only be returned to the
// browser and must not be persisted in place of Hash.
type PasskeyChallenge struct {
	Value string            `json:"challenge"`
	Hash  [sha256.Size]byte `json:"-"`
}

// PasskeyCredentialDescriptor is shared by registration exclusion lists and
// authentication allow lists. Unknown transports are intentionally representable
// so that values introduced by newer WebAuthn clients can be retained.
type PasskeyCredentialDescriptor struct {
	Type       string   `json:"type"`
	ID         string   `json:"id"`
	Transports []string `json:"transports,omitempty"`
}

type PasskeyRelyingPartyEntity struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type PasskeyUserEntity struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	DisplayName string `json:"displayName"`
}

type PasskeyCredentialParameter struct {
	Type      string `json:"type"`
	Algorithm int    `json:"alg"`
}

type PasskeyAuthenticatorSelection struct {
	AuthenticatorAttachment string `json:"authenticatorAttachment,omitempty"`
	ResidentKey             string `json:"residentKey,omitempty"`
	RequireResidentKey      bool   `json:"requireResidentKey,omitempty"`
	UserVerification        string `json:"userVerification,omitempty"`
}

// PasskeyRegistrationOptions is the publicKey member supplied to
// navigator.credentials.create(). Algorithms and authenticator policy are
// selected by the WebAuthn adapter, not by this domain package.
type PasskeyRegistrationOptions struct {
	Challenge              string                        `json:"challenge"`
	RelyingParty           PasskeyRelyingPartyEntity     `json:"rp"`
	User                   PasskeyUserEntity             `json:"user"`
	Parameters             []PasskeyCredentialParameter  `json:"pubKeyCredParams"`
	Timeout                uint32                        `json:"timeout,omitempty"`
	ExcludeCredentials     []PasskeyCredentialDescriptor `json:"excludeCredentials,omitempty"`
	AuthenticatorSelection PasskeyAuthenticatorSelection `json:"authenticatorSelection,omitempty"`
	Attestation            string                        `json:"attestation,omitempty"`
}

// PasskeyAuthenticationOptions is the publicKey member supplied to
// navigator.credentials.get().
type PasskeyAuthenticationOptions struct {
	Challenge        string                        `json:"challenge"`
	Timeout          uint32                        `json:"timeout,omitempty"`
	RelyingPartyID   string                        `json:"rpId"`
	AllowCredentials []PasskeyCredentialDescriptor `json:"allowCredentials,omitempty"`
	UserVerification string                        `json:"userVerification,omitempty"`
}

// PasskeyRegistrationCredential is the JSON-safe registration result returned
// by a browser after navigator.credentials.create().
type PasskeyRegistrationCredential struct {
	ID                      string                                `json:"id"`
	RawID                   string                                `json:"rawId"`
	Type                    string                                `json:"type"`
	AuthenticatorAttachment string                                `json:"authenticatorAttachment,omitempty"`
	Response                PasskeyRegistrationCredentialResponse `json:"response"`
}

type PasskeyRegistrationCredentialResponse struct {
	ClientDataJSON    string   `json:"clientDataJSON"`
	AttestationObject string   `json:"attestationObject"`
	Transports        []string `json:"transports,omitempty"`
}

// PasskeyAuthenticationCredential is the JSON-safe assertion returned by a
// browser after navigator.credentials.get().
type PasskeyAuthenticationCredential struct {
	ID                      string                                  `json:"id"`
	RawID                   string                                  `json:"rawId"`
	Type                    string                                  `json:"type"`
	AuthenticatorAttachment string                                  `json:"authenticatorAttachment,omitempty"`
	Response                PasskeyAuthenticationCredentialResponse `json:"response"`
}

type PasskeyAuthenticationCredentialResponse struct {
	ClientDataJSON    string `json:"clientDataJSON"`
	AuthenticatorData string `json:"authenticatorData"`
	Signature         string `json:"signature"`
	UserHandle        string `json:"userHandle,omitempty"`
}

type DecodedPasskeyRegistration struct {
	CredentialID      []byte
	CredentialIDHash  [sha256.Size]byte
	ClientDataJSON    []byte
	AttestationObject []byte
	Transports        []string
}

type DecodedPasskeyAuthentication struct {
	CredentialID      []byte
	CredentialIDHash  [sha256.Size]byte
	ClientDataJSON    []byte
	AuthenticatorData []byte
	Signature         []byte
	UserHandle        *AccountID
}

// NewPasskeyChallenge creates a cryptographically random, unpadded base64url
// challenge. Persist only the returned hash and consume it atomically once.
func NewPasskeyChallenge() (PasskeyChallenge, error) {
	raw := make([]byte, PasskeyChallengeSize)
	if _, err := rand.Read(raw); err != nil {
		return PasskeyChallenge{}, fmt.Errorf("create passkey challenge: %w", err)
	}
	return PasskeyChallenge{
		Value: base64.RawURLEncoding.EncodeToString(raw),
		Hash:  sha256.Sum256(raw),
	}, nil
}

// HashPasskeyChallenge validates a challenge created by NewPasskeyChallenge
// and returns its storage/comparison digest.
func HashPasskeyChallenge(value string) ([sha256.Size]byte, error) {
	raw, err := decodePasskeyValue(value, PasskeyChallengeSize, PasskeyChallengeSize)
	if err != nil {
		return [sha256.Size]byte{}, invalidPasskey("challenge")
	}
	return sha256.Sum256(raw), nil
}

// EncodePasskeyUserHandle maps an account to an opaque WebAuthn user handle.
// Display names and login identifiers must never be used for authorization.
func EncodePasskeyUserHandle(accountID AccountID) (string, error) {
	if accountID == (AccountID{}) {
		return "", invalidPasskey("user handle")
	}
	return base64.RawURLEncoding.EncodeToString(accountID[:]), nil
}

// ParsePasskeyUserHandle reverses EncodePasskeyUserHandle. This package uses a
// fixed-width account ID even though WebAuthn permits larger user handles.
func ParsePasskeyUserHandle(value string) (AccountID, error) {
	raw, err := decodePasskeyValue(value, PasskeyUserHandleSize, PasskeyUserHandleSize)
	if err != nil {
		return AccountID{}, invalidPasskey("user handle")
	}
	var accountID AccountID
	copy(accountID[:], raw)
	if accountID == (AccountID{}) {
		return AccountID{}, invalidPasskey("user handle")
	}
	return accountID, nil
}

// HashPasskeyCredentialID validates a browser credential ID and returns the
// digest used for lookup. Keeping the raw identifier out of ordinary read paths
// limits unnecessary exposure; an adapter may retain it separately when it
// explicitly supports non-discoverable credentials.
func HashPasskeyCredentialID(value string) ([sha256.Size]byte, error) {
	raw, err := decodePasskeyValue(value, PasskeyCredentialIDMinSize, PasskeyCredentialIDMaxSize)
	if err != nil {
		return [sha256.Size]byte{}, invalidPasskey("credential id")
	}
	return sha256.Sum256(raw), nil
}

// DecodeForVerification performs bounded, structural decoding only. The caller
// must still verify the client-data type, challenge, origin, RP ID hash,
// authenticator flags, attestation chain and public key before storing a factor.
func (credential PasskeyRegistrationCredential) DecodeForVerification() (DecodedPasskeyRegistration, error) {
	credentialID, credentialIDHash, err := decodeCredentialIdentity(credential.ID, credential.RawID, credential.Type)
	if err != nil {
		return DecodedPasskeyRegistration{}, err
	}
	clientDataJSON, err := decodeClientDataJSON(credential.Response.ClientDataJSON)
	if err != nil {
		return DecodedPasskeyRegistration{}, err
	}
	attestationObject, err := decodePasskeyValue(credential.Response.AttestationObject, 1, maxPasskeyAttestationSize)
	if err != nil {
		return DecodedPasskeyRegistration{}, invalidPasskey("attestation object")
	}
	transports, err := normalizePasskeyTransports(credential.Response.Transports)
	if err != nil {
		return DecodedPasskeyRegistration{}, err
	}
	return DecodedPasskeyRegistration{
		CredentialID:      credentialID,
		CredentialIDHash:  credentialIDHash,
		ClientDataJSON:    clientDataJSON,
		AttestationObject: attestationObject,
		Transports:        transports,
	}, nil
}

// DecodeForVerification performs bounded, structural decoding only. The caller
// must still verify the client-data type, challenge, origin, RP ID hash,
// authenticator flags, signature, credential ownership and signature counter.
func (credential PasskeyAuthenticationCredential) DecodeForVerification() (DecodedPasskeyAuthentication, error) {
	credentialID, credentialIDHash, err := decodeCredentialIdentity(credential.ID, credential.RawID, credential.Type)
	if err != nil {
		return DecodedPasskeyAuthentication{}, err
	}
	clientDataJSON, err := decodeClientDataJSON(credential.Response.ClientDataJSON)
	if err != nil {
		return DecodedPasskeyAuthentication{}, err
	}
	authenticatorData, err := decodePasskeyValue(
		credential.Response.AuthenticatorData,
		minPasskeyAuthenticatorDataSize,
		maxPasskeyAuthenticatorDataSize,
	)
	if err != nil {
		return DecodedPasskeyAuthentication{}, invalidPasskey("authenticator data")
	}
	signature, err := decodePasskeyValue(credential.Response.Signature, 1, maxPasskeySignatureSize)
	if err != nil {
		return DecodedPasskeyAuthentication{}, invalidPasskey("signature")
	}
	var userHandle *AccountID
	if credential.Response.UserHandle != "" {
		accountID, err := ParsePasskeyUserHandle(credential.Response.UserHandle)
		if err != nil {
			return DecodedPasskeyAuthentication{}, err
		}
		userHandle = &accountID
	}
	return DecodedPasskeyAuthentication{
		CredentialID:      credentialID,
		CredentialIDHash:  credentialIDHash,
		ClientDataJSON:    clientDataJSON,
		AuthenticatorData: authenticatorData,
		Signature:         signature,
		UserHandle:        userHandle,
	}, nil
}

func decodeCredentialIdentity(id, rawID, credentialType string) ([]byte, [sha256.Size]byte, error) {
	if credentialType != PasskeyPublicKeyCredentialType {
		return nil, [sha256.Size]byte{}, invalidPasskey("credential type")
	}
	decodedID, err := decodePasskeyValue(id, PasskeyCredentialIDMinSize, PasskeyCredentialIDMaxSize)
	if err != nil {
		return nil, [sha256.Size]byte{}, invalidPasskey("credential id")
	}
	decodedRawID, err := decodePasskeyValue(rawID, PasskeyCredentialIDMinSize, PasskeyCredentialIDMaxSize)
	if err != nil || !bytes.Equal(decodedID, decodedRawID) {
		return nil, [sha256.Size]byte{}, invalidPasskey("credential id mismatch")
	}
	return decodedID, sha256.Sum256(decodedID), nil
}

func decodeClientDataJSON(value string) ([]byte, error) {
	decoded, err := decodePasskeyValue(value, 1, maxPasskeyClientDataJSONSize)
	if err != nil {
		return nil, invalidPasskey("client data")
	}
	var object map[string]json.RawMessage
	if err := json.Unmarshal(decoded, &object); err != nil || object == nil {
		return nil, invalidPasskey("client data")
	}
	return decoded, nil
}

func decodePasskeyValue(value string, minimumSize, maximumSize int) ([]byte, error) {
	if value == "" || len(value) > base64.RawURLEncoding.EncodedLen(maximumSize) {
		return nil, ErrPasskeyInvalid
	}
	decoded, err := base64.RawURLEncoding.DecodeString(value)
	if err != nil || len(decoded) < minimumSize || len(decoded) > maximumSize {
		return nil, ErrPasskeyInvalid
	}
	if base64.RawURLEncoding.EncodeToString(decoded) != value {
		return nil, ErrPasskeyInvalid
	}
	return decoded, nil
}

func normalizePasskeyTransports(transports []string) ([]string, error) {
	if len(transports) > maxPasskeyTransports {
		return nil, invalidPasskey("transports")
	}
	normalized := make([]string, 0, len(transports))
	seen := make(map[string]struct{}, len(transports))
	for _, transport := range transports {
		if transport == "" || len(transport) > maxPasskeyTransportLength ||
			!utf8.ValidString(transport) || strings.TrimSpace(transport) != transport {
			return nil, invalidPasskey("transport")
		}
		if _, exists := seen[transport]; exists {
			return nil, invalidPasskey("transport")
		}
		seen[transport] = struct{}{}
		normalized = append(normalized, transport)
	}
	return normalized, nil
}

func invalidPasskey(field string) error {
	return fmt.Errorf("%w: %s", ErrPasskeyInvalid, field)
}
