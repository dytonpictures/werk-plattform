package identity

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"testing"

	"golang.org/x/crypto/argon2"
)

func TestPasswordHashRoundTrip(t *testing.T) {
	encoded, err := HashPassword("correct horse battery staple")
	if err != nil {
		t.Fatalf("hash password: %v", err)
	}
	if !VerifyPassword(encoded, "correct horse battery staple") {
		t.Fatal("valid password was rejected")
	}
	if VerifyPassword(encoded, "incorrect password") {
		t.Fatal("invalid password was accepted")
	}
}

func TestPasswordHashUsesRandomSalt(t *testing.T) {
	first, err := HashPassword("correct horse battery staple")
	if err != nil {
		t.Fatal(err)
	}
	second, err := HashPassword("correct horse battery staple")
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Equal(first, second) {
		t.Fatal("password hashes unexpectedly match")
	}
}

func TestPasswordHashRejectsInvalidInputAndEncoding(t *testing.T) {
	if _, err := HashPassword("short"); err != ErrPasswordInvalid {
		t.Fatalf("short password error = %v", err)
	}
	if VerifyPassword([]byte("not-a-phc-string"), "a password value") {
		t.Fatal("malformed hash was accepted")
	}
}

func TestPasswordHashAcceptsBoundedHistoricPolicyAndRequestsRehash(t *testing.T) {
	password := "correct horse battery staple"
	salt := bytes.Repeat([]byte{0x42}, passwordSaltLength)
	iterations := uint32(minimumPasswordIterations)
	hash := argon2.IDKey([]byte(password), salt, iterations, passwordMemory, passwordParallelism, passwordKeyLength)
	encoded := []byte(fmt.Sprintf("$argon2id$v=%d$m=%d,t=%d,p=%d$%s$%s",
		argon2.Version, passwordMemory, iterations, passwordParallelism,
		base64.RawStdEncoding.EncodeToString(salt), base64.RawStdEncoding.EncodeToString(hash)))
	if !VerifyPassword(encoded, password) {
		t.Fatal("bounded historic password hash was rejected")
	}
	if !PasswordNeedsRehash(encoded) {
		t.Fatal("historic password parameters did not request rehash")
	}
	current, err := HashPassword(password)
	if err != nil {
		t.Fatal(err)
	}
	if PasswordNeedsRehash(current) {
		t.Fatal("current password parameters unexpectedly request rehash")
	}
}
