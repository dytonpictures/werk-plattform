package adminstore

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"net/mail"
	"strings"
)

const (
	OnboardingInitialPassword = "initial-password"
	OnboardingInvitationLink  = "invitation-link"
	InvitationDeliveryManual  = "manual-email-draft"
	maximumInvitationHours    = 7 * 24
)

func validateWorkUserOnboarding(input CreateWorkUserInput) (string, bool, error) {
	switch strings.TrimSpace(input.OnboardingMethod) {
	case OnboardingInitialPassword:
		if input.RequirePasswordChange == nil || len(input.InitialPassword) < 12 ||
			strings.TrimSpace(input.InvitationEmail) != "" || input.InvitationExpiresInHours != 0 {
			return "", false, errors.New("invalid initial password onboarding")
		}
		return "", *input.RequirePasswordChange, nil
	case OnboardingInvitationLink:
		if input.RequirePasswordChange != nil || input.InitialPassword != "" ||
			input.InvitationExpiresInHours < 1 || input.InvitationExpiresInHours > maximumInvitationHours {
			return "", false, errors.New("invalid invitation onboarding")
		}
		recipient := strings.TrimSpace(input.InvitationEmail)
		parsed, err := mail.ParseAddress(recipient)
		if err != nil || parsed.Name != "" || parsed.Address != recipient || len(recipient) > 320 {
			return "", false, errors.New("invalid invitation recipient")
		}
		return recipient, false, nil
	default:
		return "", false, errors.New("invalid onboarding method")
	}
}

func newInvitationToken() (raw string, digest [32]byte, err error) {
	value := make([]byte, 32)
	if _, err = rand.Read(value); err != nil {
		return "", digest, err
	}
	raw = base64.RawURLEncoding.EncodeToString(value)
	digest = sha256.Sum256([]byte(raw))
	return raw, digest, nil
}
