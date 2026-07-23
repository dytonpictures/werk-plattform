package identitystore

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/dytonpictures/werk/internal/core/identity"
	"github.com/dytonpictures/werk/internal/platform/database"
)

func TestAgentAPIKeyAndProviderBindingIntegration(t *testing.T) {
	migratorURL := os.Getenv("WERK_TEST_MIGRATOR_DATABASE_URL")
	identityURL := os.Getenv("WERK_TEST_IDENTITY_DATABASE_URL")
	if migratorURL == "" || identityURL == "" {
		t.Skip("identity integration database URLs are not configured")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	ownerPool, err := pgxpool.New(ctx, migratorURL)
	if err != nil {
		t.Fatal(err)
	}
	defer ownerPool.Close()
	owner, err := ownerPool.Acquire(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer owner.Release()
	if _, err := owner.Exec(ctx, `SET ROLE werk_owner`); err != nil {
		t.Fatal(err)
	}

	const (
		tenantID  = "0196f000-0000-7000-8000-000000000901"
		agentID   = "0196f000-0000-7000-8000-000000000902"
		accountID = "0196f000-0000-7000-8000-000000000903"
	)
	material, err := identity.NewAPIKey()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := owner.Exec(ctx, `
		INSERT INTO werk_core.tenants (id, name, status, default_locale, default_timezone)
		VALUES ($1::uuid, 'Agent Integration', 'active', 'de-DE', 'Europe/Berlin')
	`, tenantID); err != nil {
		t.Fatal(err)
	}
	if _, err := owner.Exec(ctx, `
		INSERT INTO werk_core.identity_agents (id, tenant_id, agent_key, display_name)
		VALUES ($2::uuid, $1::uuid, 'integration-agent', 'Integration Agent')
	`, tenantID, agentID); err != nil {
		t.Fatal(err)
	}
	if _, err := owner.Exec(ctx, `
		INSERT INTO werk_core.accounts (
			id, account_class, tenant_id, agent_subject_id, login_name, status
		) VALUES ($3::uuid, 'agent', $1::uuid, $2::uuid, 'integration-agent', 'active')
	`, tenantID, agentID, accountID); err != nil {
		t.Fatal(err)
	}
	if _, err := owner.Exec(ctx, `
		INSERT INTO werk_core.account_identity_bindings (
			account_id, provider_key, provider_subject
		) VALUES ($1::uuid, 'local', 'integration-agent-subject')
	`, accountID); err != nil {
		t.Fatal(err)
	}
	if _, err := owner.Exec(ctx, `
		INSERT INTO werk_core.account_credentials (
			account_id, credential_kind, secret_hash, assurance, display_name,
			public_id_hash, use_limit
		) VALUES ($1::uuid, 'api-key', $2, 'single-factor', 'Integration key', $3, 2)
	`, accountID, material.SecretHash[:], material.PublicIDHash[:]); err != nil {
		t.Fatal(err)
	}
	if _, err := owner.Exec(ctx, `
		INSERT INTO werk_core.sessions (
			id, account_id, token_hash, audience, tenant_id, expires_at,
			authentication_assurance, authentication_kind, session_generation
		) VALUES (
			'0196f000-0000-7000-8000-000000000904', $1::uuid, $2,
			'work', $3::uuid, now() + interval '5 minutes', 'single-factor', 'workload',
			(SELECT session_generation FROM werk_core.accounts WHERE id = $1::uuid)
		)
	`, accountID, make([]byte, 32), tenantID); err == nil {
		t.Fatal("agent account received a work audience session")
	}
	if _, err := owner.Exec(ctx, `
		UPDATE werk_core.accounts SET account_class = 'service' WHERE id = $1::uuid
	`, accountID); err == nil {
		t.Fatal("agent account identity boundary was mutable")
	}
	defer func() {
		cleanupContext, cleanupCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cleanupCancel()
		_, _ = owner.Exec(cleanupContext, `DELETE FROM werk_core.security_audit_events WHERE account_id = $1::uuid`, accountID)
		_, _ = owner.Exec(cleanupContext, `DELETE FROM werk_core.accounts WHERE id = $1::uuid`, accountID)
		_, _ = owner.Exec(cleanupContext, `DELETE FROM werk_core.identity_agents WHERE id = $1::uuid`, agentID)
		_, _ = owner.Exec(cleanupContext, `DELETE FROM werk_core.tenants WHERE id = $1::uuid`, tenantID)
	}()

	identityDB, err := database.NewIdentity(ctx, identityURL, "identity-agent-integration")
	if err != nil {
		t.Fatal(err)
	}
	defer identityDB.Close()
	service, err := New(identityDB)
	if err != nil {
		t.Fatal(err)
	}
	service.now = func() time.Time { return time.Date(2026, 7, 21, 16, 0, 0, 0, time.UTC) }

	proof := identity.VerifiedIdentity{
		ProviderKey: "local", ProviderSubject: "integration-agent-subject",
		Method: identity.AuthenticationMethodAPIKey, Assurance: identity.AssuranceSingleFactor,
		AuthenticatedAt: service.now(),
	}
	resolved, err := service.ResolveVerifiedIdentity(ctx, proof)
	if err != nil || resolved.AccountClass != identity.AccountClassAgent || resolved.Audience != identity.AudienceService {
		t.Fatalf("ResolveVerifiedIdentity() = %#v, %v", resolved, err)
	}
	if err := identity.AuthorizeAccessPlane(resolved, identity.AccessPlaneService); err != nil {
		t.Fatalf("agent technical plane denied: %v", err)
	}

	requestID := "0196f000-0000-7000-8000-000000000911"
	correlationID := "0196f000-0000-7000-8000-000000000912"
	for attempt := 0; attempt < 2; attempt++ {
		actor, err := service.AuthenticateAPIKey(ctx, material.Token, requestID, correlationID)
		if err != nil || actor.AccountClass != identity.AccountClassAgent || actor.TenantID == nil {
			t.Fatalf("AuthenticateAPIKey() attempt %d = %#v, %v", attempt+1, actor, err)
		}
	}
	if _, err := service.AuthenticateAPIKey(ctx, material.Token, requestID, correlationID); err == nil {
		t.Fatal("API key exceeded its global use limit")
	}
	var useCount int64
	if err := owner.QueryRow(ctx, `
		SELECT use_count FROM werk_core.account_credentials
		WHERE account_id = $1::uuid AND credential_kind = 'api-key'
	`, accountID).Scan(&useCount); err != nil {
		t.Fatal(err)
	}
	if useCount != 2 {
		t.Fatalf("API key use count = %d, want 2", useCount)
	}
}
