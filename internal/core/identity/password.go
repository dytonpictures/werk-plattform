package identity

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"

	"golang.org/x/crypto/argon2"
)

const (
	passwordMemory      = 64 * 1024
	passwordIterations  = 3
	passwordParallelism = 2
	passwordSaltLength  = 16
	passwordKeyLength   = 32

	minimumPasswordMemory      = 32 * 1024
	minimumPasswordIterations  = 2
	minimumPasswordParallelism = 1
	minimumPasswordSaltLength  = 16
	minimumPasswordKeyLength   = 32
	maximumPasswordMemory      = 256 * 1024
	maximumPasswordIterations  = 10
	maximumPasswordParallelism = 8
	maximumPasswordSaltLength  = 64
	maximumPasswordKeyLength   = 64
)

var ErrPasswordInvalid = errors.New("invalid password")

// HashPassword returns an Argon2id PHC string. Passwords are never truncated.
func HashPassword(password string) ([]byte, error) {
	if len(password) < 12 || len(password) > 1024 {
		return nil, ErrPasswordInvalid
	}
	salt := make([]byte, passwordSaltLength)
	if _, err := rand.Read(salt); err != nil {
		return nil, fmt.Errorf("generate password salt: %w", err)
	}
	hash := argon2.IDKey([]byte(password), salt, passwordIterations, passwordMemory, passwordParallelism, passwordKeyLength)
	encoded := fmt.Sprintf("$argon2id$v=%d$m=%d,t=%d,p=%d$%s$%s",
		argon2.Version, passwordMemory, passwordIterations, passwordParallelism,
		base64.RawStdEncoding.EncodeToString(salt), base64.RawStdEncoding.EncodeToString(hash))
	return []byte(encoded), nil
}

// VerifyPassword rejects malformed or unexpectedly expensive hashes before
// deriving a candidate key.
func VerifyPassword(encoded []byte, password string) bool {
	if len(password) == 0 || len(password) > 1024 {
		return false
	}
	parameters, salt, want, ok := parsePasswordHash(encoded)
	if !ok {
		return false
	}
	got := argon2.IDKey([]byte(password), salt, parameters.iterations, parameters.memory, parameters.parallelism, uint32(len(want)))
	return subtle.ConstantTimeCompare(got, want) == 1
}

// PasswordNeedsRehash permits bounded historic Argon2id parameters while
// allowing successful logins to upgrade them atomically to the current policy.
func PasswordNeedsRehash(encoded []byte) bool {
	parameters, salt, hash, ok := parsePasswordHash(encoded)
	if !ok {
		return false
	}
	return parameters.memory < passwordMemory || parameters.iterations < passwordIterations ||
		parameters.parallelism < passwordParallelism || len(salt) < passwordSaltLength || len(hash) < passwordKeyLength
}

type passwordHashParameters struct {
	memory      uint32
	iterations  uint32
	parallelism uint8
}

func parsePasswordHash(encoded []byte) (passwordHashParameters, []byte, []byte, bool) {
	if len(encoded) == 0 || len(encoded) > 512 {
		return passwordHashParameters{}, nil, nil, false
	}
	parts := strings.Split(string(encoded), "$")
	if len(parts) != 6 || parts[1] != "argon2id" {
		return passwordHashParameters{}, nil, nil, false
	}
	var version int
	if _, err := fmt.Sscanf(parts[2], "v=%d", &version); err != nil || version != argon2.Version || parts[2] != fmt.Sprintf("v=%d", version) {
		return passwordHashParameters{}, nil, nil, false
	}
	var parameters passwordHashParameters
	if _, err := fmt.Sscanf(parts[3], "m=%d,t=%d,p=%d", &parameters.memory, &parameters.iterations, &parameters.parallelism); err != nil ||
		parts[3] != fmt.Sprintf("m=%d,t=%d,p=%d", parameters.memory, parameters.iterations, parameters.parallelism) ||
		parameters.memory < minimumPasswordMemory || parameters.memory > maximumPasswordMemory ||
		parameters.iterations < minimumPasswordIterations || parameters.iterations > maximumPasswordIterations ||
		parameters.parallelism < minimumPasswordParallelism || parameters.parallelism > maximumPasswordParallelism {
		return passwordHashParameters{}, nil, nil, false
	}
	salt, err := base64.RawStdEncoding.DecodeString(parts[4])
	if err != nil || len(salt) < minimumPasswordSaltLength || len(salt) > maximumPasswordSaltLength ||
		base64.RawStdEncoding.EncodeToString(salt) != parts[4] {
		return passwordHashParameters{}, nil, nil, false
	}
	want, err := base64.RawStdEncoding.DecodeString(parts[5])
	if err != nil || len(want) < minimumPasswordKeyLength || len(want) > maximumPasswordKeyLength ||
		base64.RawStdEncoding.EncodeToString(want) != parts[5] {
		return passwordHashParameters{}, nil, nil, false
	}
	return parameters, salt, want, true
}
