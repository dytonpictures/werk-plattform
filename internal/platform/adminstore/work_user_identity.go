package adminstore

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/dytonpictures/werk/internal/core/identity"
	"github.com/dytonpictures/werk/internal/core/tenancy"
	"github.com/dytonpictures/werk/internal/platform/database"
)

type WorkUserIdentityView struct {
	AccountID      string                      `json:"account_id"`
	TenantID       string                      `json:"tenant_id"`
	Password       *WorkUserCredentialView     `json:"password,omitempty"`
	Factors        []WorkUserFactorView        `json:"factors"`
	ActiveSessions []WorkUserSessionView       `json:"active_sessions"`
	RecentActivity []WorkUserIdentityEventView `json:"recent_activity"`
}

type WorkUserCredentialView struct {
	Status     string     `json:"status"`
	ChangedAt  time.Time  `json:"changed_at"`
	LastUsedAt *time.Time `json:"last_used_at,omitempty"`
}

type WorkUserFactorView struct {
	ID          string     `json:"id"`
	Kind        string     `json:"kind"`
	Status      string     `json:"status"`
	DisplayName string     `json:"display_name"`
	CreatedAt   time.Time  `json:"created_at"`
	ActivatedAt *time.Time `json:"activated_at,omitempty"`
	LastUsedAt  *time.Time `json:"last_used_at,omitempty"`
}

type WorkUserSessionView struct {
	ID        string    `json:"id"`
	CreatedAt time.Time `json:"created_at"`
	ExpiresAt time.Time `json:"expires_at"`
	Assurance string    `json:"assurance"`
}

type WorkUserIdentityEventView struct {
	ID         string    `json:"id"`
	OccurredAt time.Time `json:"occurred_at"`
	EventType  string    `json:"event_type"`
	Outcome    string    `json:"outcome"`
}

func (service *Service) GetWorkUserIdentity(ctx context.Context, tenantIDValue, accountIDValue string, _ identity.AuthenticatedActor, _, _ string) (WorkUserIdentityView, error) {
	tenantID, err := tenancy.ParseTenantID(strings.TrimSpace(tenantIDValue))
	accountID := strings.ToLower(strings.TrimSpace(accountIDValue))
	if err != nil || !validCanonicalUUID(accountID) {
		return WorkUserIdentityView{}, errors.New("invalid work user identity query")
	}
	view := WorkUserIdentityView{AccountID: accountID, TenantID: tenantID.String(), Factors: []WorkUserFactorView{}, ActiveSessions: []WorkUserSessionView{}, RecentActivity: []WorkUserIdentityEventView{}}
	err = service.database.WithinTenantRead(ctx, tenantID, func(ctx context.Context, tx database.TenantTx) error {
		var credentialStatus *string
		var changedAt, lastUsedAt *time.Time
		if err := tx.QueryRow(ctx, `
			SELECT credential.status, credential.changed_at, credential.last_used_at
			FROM werk_core.accounts AS account
			LEFT JOIN werk_core.account_credentials AS credential
			  ON credential.account_id=account.id AND credential.credential_kind='password' AND credential.status='active'
			WHERE account.id=$1::uuid AND account.tenant_id=$2::uuid AND account.account_class='work'
		`, accountID, tenantID.String()).Scan(&credentialStatus, &changedAt, &lastUsedAt); err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return ErrNotFound
			}
			return err
		}
		if credentialStatus != nil && changedAt != nil {
			view.Password = &WorkUserCredentialView{Status: *credentialStatus, ChangedAt: *changedAt, LastUsedAt: lastUsedAt}
		}
		factorRows, err := tx.Query(ctx, `
			SELECT id::text, factor_kind, status, display_name, created_at, activated_at, last_used_at
			FROM werk_core.identity_mfa_factors
			WHERE account_id=$1::uuid AND status <> 'revoked'
			ORDER BY created_at DESC, id DESC
		`, accountID)
		if err != nil {
			return err
		}
		for factorRows.Next() {
			var item WorkUserFactorView
			if err := factorRows.Scan(&item.ID, &item.Kind, &item.Status, &item.DisplayName, &item.CreatedAt, &item.ActivatedAt, &item.LastUsedAt); err != nil {
				factorRows.Close()
				return err
			}
			view.Factors = append(view.Factors, item)
		}
		if err := factorRows.Err(); err != nil {
			factorRows.Close()
			return err
		}
		factorRows.Close()
		sessionRows, err := tx.Query(ctx, `
			SELECT session.id::text, session.created_at, session.expires_at, session.authentication_assurance
			FROM werk_core.sessions AS session
			JOIN werk_core.accounts AS account
			  ON account.id=session.account_id
			 AND account.session_generation=session.session_generation
			WHERE session.account_id=$1::uuid AND session.tenant_id=$2::uuid AND session.audience='work'
			  AND session.revoked_at IS NULL AND session.expires_at > $3
			ORDER BY session.created_at DESC, session.id DESC LIMIT 20
		`, accountID, tenantID.String(), service.now())
		if err != nil {
			return err
		}
		for sessionRows.Next() {
			var item WorkUserSessionView
			if err := sessionRows.Scan(&item.ID, &item.CreatedAt, &item.ExpiresAt, &item.Assurance); err != nil {
				sessionRows.Close()
				return err
			}
			view.ActiveSessions = append(view.ActiveSessions, item)
		}
		if err := sessionRows.Err(); err != nil {
			sessionRows.Close()
			return err
		}
		sessionRows.Close()
		activityRows, err := tx.Query(ctx, `
			SELECT id::text, occurred_at, event_type, outcome
			FROM werk_core.security_audit_events
			WHERE tenant_id=$2::uuid AND (
			  account_id=$1::uuid OR subject_id=$1::text OR
			  details->>'created_account_id'=$1::text OR details->>'target_account_id'=$1::text
			)
			ORDER BY occurred_at DESC, id DESC LIMIT 20
		`, accountID, tenantID.String())
		if err != nil {
			return err
		}
		defer activityRows.Close()
		for activityRows.Next() {
			var item WorkUserIdentityEventView
			if err := activityRows.Scan(&item.ID, &item.OccurredAt, &item.EventType, &item.Outcome); err != nil {
				return err
			}
			view.RecentActivity = append(view.RecentActivity, item)
		}
		return activityRows.Err()
	})
	return view, err
}
