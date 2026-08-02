package identity

import "time"

type PasskeyView struct {
	ID          string     `json:"id"`
	DisplayName string     `json:"display_name"`
	CreatedAt   time.Time  `json:"created_at"`
	ActivatedAt time.Time  `json:"activated_at"`
	LastUsedAt  *time.Time `json:"last_used_at,omitempty"`
}

type SessionLifecycleView struct {
	ID                      string                  `json:"id"`
	CreatedAt               time.Time               `json:"created_at"`
	ExpiresAt               time.Time               `json:"expires_at"`
	AuthenticationKind      AuthenticationKind      `json:"authentication_kind"`
	AuthenticationAssurance AuthenticationAssurance `json:"authentication_assurance"`
	Current                 bool                    `json:"current"`
}
