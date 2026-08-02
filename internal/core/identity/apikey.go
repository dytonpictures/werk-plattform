package identity

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"strings"
)

const (
	apiKeyVersion       = "ak1"
	apiKeyPublicIDSize  = 16
	apiKeySecretSize    = 32
	maximumAPIKeyLength = 96
)

var ErrAPIKeyInvalid = errors.New("invalid API key")

// APIKeyMaterial contains the one-time plaintext token and the only values
// that may be persisted. Token must never be logged or stored.
type APIKeyMaterial struct {
	Token        string            `json:"token"`
	PublicIDHash [sha256.Size]byte `json:"-"`
	SecretHash   [sha256.Size]byte `json:"-"`
}

type APIKeyDigest struct {
	PublicIDHash [sha256.Size]byte
	SecretHash   [sha256.Size]byte
}

func NewAPIKey() (APIKeyMaterial, error) {
	publicID := make([]byte, apiKeyPublicIDSize)
	secret := make([]byte, apiKeySecretSize)
	if _, err := rand.Read(publicID); err != nil {
		return APIKeyMaterial{}, err
	}
	if _, err := rand.Read(secret); err != nil {
		return APIKeyMaterial{}, err
	}
	return APIKeyMaterial{
		Token:        apiKeyVersion + "." + base64.RawURLEncoding.EncodeToString(publicID) + "." + base64.RawURLEncoding.EncodeToString(secret),
		PublicIDHash: sha256.Sum256(publicID),
		SecretHash:   sha256.Sum256(secret),
	}, nil
}

func DigestAPIKey(token string) (APIKeyDigest, error) {
	if token == "" || len(token) > maximumAPIKeyLength || strings.TrimSpace(token) != token {
		return APIKeyDigest{}, ErrAPIKeyInvalid
	}
	parts := strings.Split(token, ".")
	if len(parts) != 3 || parts[0] != apiKeyVersion {
		return APIKeyDigest{}, ErrAPIKeyInvalid
	}
	publicID, err := decodeCanonicalAPIKeyPart(parts[1], apiKeyPublicIDSize)
	if err != nil {
		return APIKeyDigest{}, err
	}
	secret, err := decodeCanonicalAPIKeyPart(parts[2], apiKeySecretSize)
	if err != nil {
		return APIKeyDigest{}, err
	}
	return APIKeyDigest{PublicIDHash: sha256.Sum256(publicID), SecretHash: sha256.Sum256(secret)}, nil
}

func VerifyAPIKeySecret(expected, provided [sha256.Size]byte) bool {
	return subtle.ConstantTimeCompare(expected[:], provided[:]) == 1
}

func decodeCanonicalAPIKeyPart(value string, size int) ([]byte, error) {
	decoded, err := base64.RawURLEncoding.DecodeString(value)
	if err != nil || len(decoded) != size || base64.RawURLEncoding.EncodeToString(decoded) != value {
		return nil, ErrAPIKeyInvalid
	}
	return decoded, nil
}
