package identity

import (
	"crypto/sha256"
	"encoding/base64"
	"errors"
)

const InitialWorkAccountInvitationTokenSize = 32

var (
	ErrInitialWorkAccountInvitationInvalid = errors.New("invalid initial work account invitation")
	ErrInitialWorkAccountInvitationBusy    = errors.New("initial work account invitation activation is busy")
)

// HashInitialWorkAccountInvitationToken validates the public token's canonical
// transport representation before deriving its database lookup hash. The hash
// deliberately covers the exact canonical string supplied by the client, not
// the decoded random bytes.
func HashInitialWorkAccountInvitationToken(token string) ([sha256.Size]byte, error) {
	if len(token) != base64.RawURLEncoding.EncodedLen(InitialWorkAccountInvitationTokenSize) {
		return [sha256.Size]byte{}, ErrInitialWorkAccountInvitationInvalid
	}
	decoded, err := base64.RawURLEncoding.DecodeString(token)
	if err != nil || len(decoded) != InitialWorkAccountInvitationTokenSize ||
		base64.RawURLEncoding.EncodeToString(decoded) != token {
		return [sha256.Size]byte{}, ErrInitialWorkAccountInvitationInvalid
	}
	return sha256.Sum256([]byte(token)), nil
}
