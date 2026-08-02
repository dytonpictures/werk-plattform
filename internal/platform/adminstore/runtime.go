package adminstore

import (
	"errors"
	"strings"
	"time"
	"unicode/utf8"
)

// RuntimeConfiguration contains only non-secret coordinates that the admin
// read models may disclose. Database URLs, provider secrets, origins and key
// material intentionally have no place in this contract.
type RuntimeConfiguration struct {
	Environment         string
	BuildVersion        string
	APIVersion          string
	APIStartedAt        time.Time
	TransportSecurity   string
	KafkaEnabled        bool
	IdentityMFAEnabled  bool
	WebAuthnRPID        string
	WebAuthnRPName      string
	WebAuthnOriginCount int
}

type Option func(*Service) error

func WithRuntimeConfiguration(configuration RuntimeConfiguration) Option {
	return func(service *Service) error {
		if service == nil || !validRuntimeConfiguration(configuration) {
			return errors.New("invalid admin runtime configuration")
		}
		configuration.Environment = strings.TrimSpace(configuration.Environment)
		configuration.BuildVersion = strings.TrimSpace(configuration.BuildVersion)
		configuration.APIVersion = strings.TrimSpace(configuration.APIVersion)
		configuration.TransportSecurity = strings.TrimSpace(configuration.TransportSecurity)
		configuration.WebAuthnRPID = strings.TrimSpace(configuration.WebAuthnRPID)
		configuration.WebAuthnRPName = strings.TrimSpace(configuration.WebAuthnRPName)
		configuration.APIStartedAt = configuration.APIStartedAt.UTC()
		service.runtime = configuration
		service.runtimeConfigured = true
		return nil
	}
}

func validRuntimeConfiguration(configuration RuntimeConfiguration) bool {
	if configuration.Environment != "development" && configuration.Environment != "test" && configuration.Environment != "production" {
		return false
	}
	if !boundedRuntimeText(configuration.BuildVersion, 128) || configuration.APIVersion != "v1" ||
		configuration.APIStartedAt.IsZero() || configuration.WebAuthnOriginCount < 0 || configuration.WebAuthnOriginCount > 64 {
		return false
	}
	switch configuration.TransportSecurity {
	case "disabled", "tls", "mtls":
	default:
		return false
	}
	if configuration.WebAuthnRPID == "" || configuration.WebAuthnRPName == "" ||
		!boundedRuntimeText(configuration.WebAuthnRPID, 253) || !boundedRuntimeText(configuration.WebAuthnRPName, 120) {
		return false
	}
	return true
}

func boundedRuntimeText(value string, maximumRunes int) bool {
	return value == strings.TrimSpace(value) && value != "" && utf8.ValidString(value) &&
		strings.IndexByte(value, 0) < 0 && utf8.RuneCountInString(value) <= maximumRunes
}
