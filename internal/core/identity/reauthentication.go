package identity

import (
	"errors"
	"strings"
	"time"
)

var ErrReauthenticationRequired = errors.New("reauthentication required")

type ReauthenticationBinding struct {
	PermissionKey string `json:"permission_key"`
	ResourceKind  string `json:"resource_kind"`
	ResourceID    string `json:"resource_id"`
}

type ReauthenticationTicket struct {
	Token     string    `json:"token"`
	ExpiresAt time.Time `json:"expires_at"`
}

func ValidateReauthenticationBinding(binding ReauthenticationBinding) error {
	if strings.TrimSpace(binding.PermissionKey) != binding.PermissionKey || binding.PermissionKey == "" || len(binding.PermissionKey) > 160 ||
		strings.TrimSpace(binding.ResourceKind) != binding.ResourceKind || binding.ResourceKind == "" || len(binding.ResourceKind) > 160 ||
		strings.TrimSpace(binding.ResourceID) != binding.ResourceID || binding.ResourceID == "" || len(binding.ResourceID) > 255 {
		return ErrReauthenticationRequired
	}
	return nil
}
