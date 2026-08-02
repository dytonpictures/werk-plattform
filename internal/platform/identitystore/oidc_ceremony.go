package identitystore

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/dytonpictures/werk/internal/platform/database"
	"github.com/dytonpictures/werk/internal/platform/identityprotocol/oidc"
)

const maximumOpenOIDCCeremonies = 4096

var _ oidc.CeremonyPort = (*Service)(nil)

// Save implements oidc.CeremonyPort. The payload contains nonce and PKCE
// verifier and is therefore encrypted with purpose-bound associated data.
func (service *Service) Save(ctx context.Context, ceremony oidc.Ceremony) error {
	if service == nil || ceremony.StateDigest == ([32]byte{}) || ceremony.ProviderID.IsZero() ||
		ceremony.RedirectURI == "" || ceremony.StartedAt.IsZero() || ceremony.ExpiresAt.IsZero() ||
		!ceremony.ExpiresAt.After(ceremony.StartedAt) || ceremony.ExpiresAt.Sub(ceremony.StartedAt) > 5*time.Minute {
		return oidc.ErrLoginUnavailable
	}
	payload, err := json.Marshal(ceremony)
	if err != nil || len(payload) > 48<<10 {
		return oidc.ErrLoginUnavailable
	}
	coordinate := hex.EncodeToString(ceremony.StateDigest[:])
	encrypted, err := service.encryptMFASecret("oidc-login", coordinate, string(payload))
	if err != nil {
		return oidc.ErrLoginUnavailable
	}
	err = service.database.WithinWrite(ctx, func(ctx context.Context, tx database.TenantTx) error {
		if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(6288511189404317026)`); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `WITH expired AS (SELECT state_digest FROM werk_core.identity_oidc_login_ceremonies WHERE expires_at<=$1 ORDER BY expires_at LIMIT 128 FOR UPDATE SKIP LOCKED) DELETE FROM werk_core.identity_oidc_login_ceremonies AS ceremony USING expired WHERE ceremony.state_digest=expired.state_digest`, service.now()); err != nil {
			return err
		}
		var count int
		if err := tx.QueryRow(ctx, `SELECT count(*) FROM werk_core.identity_oidc_login_ceremonies`).Scan(&count); err != nil || count >= maximumOpenOIDCCeremonies {
			return oidc.ErrLoginUnavailable
		}
		_, err := tx.Exec(ctx, `INSERT INTO werk_core.identity_oidc_login_ceremonies (state_digest,registry_provider_id,redirect_uri,encrypted_payload,created_at,expires_at) VALUES ($1,$2::uuid,$3,$4,$5,$6)`, ceremony.StateDigest[:], formatUUID([16]byte(ceremony.ProviderID)), ceremony.RedirectURI, encrypted, ceremony.StartedAt, ceremony.ExpiresAt)
		return err
	})
	if err != nil {
		return oidc.ErrLoginUnavailable
	}
	return nil
}

// Consume atomically deletes before decrypting. A malformed payload or failed
// callback therefore still burns state and cannot be replayed.
func (service *Service) Consume(ctx context.Context, request oidc.ConsumeRequest) (oidc.Ceremony, error) {
	if service == nil || request.StateDigest == ([32]byte{}) || request.ProviderID.IsZero() || request.RedirectURI == "" {
		return oidc.Ceremony{}, oidc.ErrAuthenticationFailed
	}
	var encrypted string
	err := service.database.WithinWrite(ctx, func(ctx context.Context, tx database.TenantTx) error {
		return tx.QueryRow(ctx, `DELETE FROM werk_core.identity_oidc_login_ceremonies WHERE state_digest=$1 AND registry_provider_id=$2::uuid AND redirect_uri=$3 AND expires_at>$4 RETURNING encrypted_payload`, request.StateDigest[:], formatUUID([16]byte(request.ProviderID)), request.RedirectURI, service.now()).Scan(&encrypted)
	})
	if errors.Is(err, pgx.ErrNoRows) || err != nil {
		return oidc.Ceremony{}, oidc.ErrAuthenticationFailed
	}
	coordinate := hex.EncodeToString(request.StateDigest[:])
	plaintext, err := service.decryptMFASecret("oidc-login", coordinate, encrypted)
	if err != nil {
		return oidc.Ceremony{}, oidc.ErrAuthenticationFailed
	}
	var ceremony oidc.Ceremony
	if json.Unmarshal([]byte(plaintext), &ceremony) != nil || ceremony.StateDigest != request.StateDigest || ceremony.ProviderID != request.ProviderID || ceremony.RedirectURI != request.RedirectURI {
		return oidc.Ceremony{}, oidc.ErrAuthenticationFailed
	}
	return ceremony, nil
}
