package identity

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha1"
	"encoding/base32"
	"encoding/binary"
	"errors"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"time"
)

var (
	ErrMFAInvalid       = errors.New("invalid multi-factor authentication state")
	ErrMFARequired      = errors.New("multi-factor authentication required")
	ErrMFAEnrollment    = errors.New("multi-factor enrollment required")
	ErrMFAChallengeUsed = errors.New("multi-factor challenge already used")
)

const (
	TOTPPeriod = 30 * time.Second
	TOTPDigits = 6
)

type LoginResult struct {
	SessionToken   string
	ChallengeToken string
	Redirect       string
	MFARequired    bool
}

type TOTPEnrollment struct {
	FactorID   string `json:"factor_id"`
	Secret     string `json:"secret"`
	OTPAuthURI string `json:"otpauth_uri"`
}

type TOTPActivation struct {
	RecoveryCodes []string `json:"recovery_codes"`
	// Rotation is transported as protected cookies by the HTTP adapter. It is
	// deliberately excluded from JSON so no session credential reaches
	// browser-visible response bodies or API logs.
	Rotation SessionRotation `json:"-"`
}

type MFAFactorKind string

const (
	MFAFactorWebAuthn MFAFactorKind = "webauthn"
	MFAFactorTOTP     MFAFactorKind = "totp"
)

type MFAFactorStatus string

const (
	MFAFactorPending MFAFactorStatus = "pending"
	MFAFactorActive  MFAFactorStatus = "active"
	MFAFactorRevoked MFAFactorStatus = "revoked"
)

type MFAChallengePurpose string

const (
	MFAChallengeEnrollment     MFAChallengePurpose = "enrollment"
	MFAChallengeAuthentication MFAChallengePurpose = "authentication"
	MFAChallengeReauth         MFAChallengePurpose = "reauthentication"
)

type MFAFactor struct {
	ID          [16]byte
	AccountID   AccountID
	Kind        MFAFactorKind
	Status      MFAFactorStatus
	DisplayName string
	CreatedAt   time.Time
	ActivatedAt *time.Time
	RevokedAt   *time.Time
}

func (factor MFAFactor) Validate() error {
	if factor.ID == [16]byte{} || factor.AccountID.IsZero() || strings.TrimSpace(factor.DisplayName) == "" {
		return ErrMFAInvalid
	}
	if factor.Kind != MFAFactorWebAuthn && factor.Kind != MFAFactorTOTP {
		return ErrMFAInvalid
	}
	if factor.Status != MFAFactorPending && factor.Status != MFAFactorActive && factor.Status != MFAFactorRevoked {
		return ErrMFAInvalid
	}
	if factor.CreatedAt.IsZero() || (factor.Status == MFAFactorActive && factor.ActivatedAt == nil) ||
		(factor.Status == MFAFactorRevoked && factor.RevokedAt == nil) {
		return ErrMFAInvalid
	}
	return nil
}

type MFAChallenge struct {
	ID        [16]byte
	AccountID AccountID
	FactorID  *[16]byte
	Purpose   MFAChallengePurpose
	ExpiresAt time.Time
	UsedAt    *time.Time
}

func (challenge MFAChallenge) Validate(now time.Time) error {
	if challenge.ID == [16]byte{} || challenge.AccountID.IsZero() || !now.Before(challenge.ExpiresAt) {
		return ErrMFAInvalid
	}
	if challenge.Purpose != MFAChallengeEnrollment && challenge.Purpose != MFAChallengeAuthentication && challenge.Purpose != MFAChallengeReauth {
		return ErrMFAInvalid
	}
	return nil
}

// NewTOTPSecret creates 160 bits of entropy as recommended for an HMAC-SHA1
// TOTP secret. The encoded value is suitable for authenticator applications.
func NewTOTPSecret() (string, error) {
	value := make([]byte, 20)
	if _, err := rand.Read(value); err != nil {
		return "", err
	}
	return base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(value), nil
}

// TOTPCode implements RFC 6238 with the interoperable SHA-1/6-digit profile.
func TOTPCode(secret string, at time.Time) (string, error) {
	decoded, err := base32.StdEncoding.WithPadding(base32.NoPadding).DecodeString(strings.ToUpper(strings.TrimSpace(secret)))
	if err != nil || len(decoded) < 16 {
		return "", ErrMFAInvalid
	}
	counter := uint64(at.UTC().Unix() / int64(TOTPPeriod/time.Second))
	var message [8]byte
	binary.BigEndian.PutUint64(message[:], counter)
	mac := hmac.New(sha1.New, decoded)
	_, _ = mac.Write(message[:])
	sum := mac.Sum(nil)
	offset := sum[len(sum)-1] & 0x0f
	value := (uint32(sum[offset])&0x7f)<<24 |
		uint32(sum[offset+1])<<16 |
		uint32(sum[offset+2])<<8 |
		uint32(sum[offset+3])
	return fmt.Sprintf("%0*d", TOTPDigits, value%1_000_000), nil
}

// VerifyTOTP accepts one time-step on either side to tolerate ordinary clock
// skew. Callers still need replay protection at the challenge layer.
func VerifyTOTP(secret, code string, at time.Time) bool {
	code = strings.TrimSpace(code)
	if len(code) != TOTPDigits {
		return false
	}
	if _, err := strconv.Atoi(code); err != nil {
		return false
	}
	for offset := -1; offset <= 1; offset++ {
		expected, err := TOTPCode(secret, at.Add(time.Duration(offset)*TOTPPeriod))
		if err == nil && hmac.Equal([]byte(expected), []byte(code)) {
			return true
		}
	}
	return false
}

func TOTPUri(issuer, account, secret string) (string, error) {
	issuer = strings.TrimSpace(issuer)
	account = strings.TrimSpace(account)
	if issuer == "" || account == "" {
		return "", ErrMFAInvalid
	}
	if _, err := TOTPCode(secret, time.Unix(0, 0)); err != nil {
		return "", err
	}
	query := url.Values{}
	query.Set("secret", secret)
	query.Set("issuer", issuer)
	query.Set("algorithm", "SHA1")
	query.Set("digits", strconv.Itoa(TOTPDigits))
	query.Set("period", strconv.Itoa(int(TOTPPeriod/time.Second)))
	return "otpauth://totp/" + url.PathEscape(issuer+":"+account) + "?" + query.Encode(), nil
}
