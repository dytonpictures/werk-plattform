package identitystore

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	coreauth "github.com/dytonpictures/werk/internal/core/authorization"
	"github.com/dytonpictures/werk/internal/core/identity"
	"github.com/dytonpictures/werk/internal/core/resource"
	"github.com/dytonpictures/werk/internal/platform/database"
)

func TestAdminTOTPFlowIntegration(t *testing.T) {
	databaseURL := os.Getenv("WERK_TEST_IDENTITY_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("WERK_TEST_IDENTITY_DATABASE_URL is not set")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	identityDB, err := database.NewIdentity(ctx, databaseURL, "werk-integration-mfa")
	if err != nil {
		t.Fatal(err)
	}
	defer identityDB.Close()
	service, err := New(identityDB, WithMFA(true, []byte("0123456789abcdef0123456789abcdef")))
	if err != nil {
		t.Fatal(err)
	}
	const initialPassword = "Werk-Integration-Initial-2026!"
	const currentPassword = "Werk-Integration-Changed-2026!"
	if err := service.BootstrapAdmin(ctx, "mfa-integration@werk.local", "MFA Integration", initialPassword); err != nil {
		t.Fatal(err)
	}
	requestID := "0196f000-0000-7000-8000-000000000091"
	correlationID := "0196f000-0000-7000-8000-000000000092"
	login, err := service.LoginWithMFA(ctx, "mfa-integration@werk.local", initialPassword, requestID, correlationID)
	if err != nil || login.Redirect != "/change-password" || login.SessionToken == "" {
		t.Fatalf("initial login = %#v, %v", login, err)
	}
	if err := service.ChangePasswordWithAudit(ctx, login.SessionToken, initialPassword, currentPassword, requestID, correlationID); err != nil {
		t.Fatal(err)
	}
	enrollment, err := service.StartTOTPEnrollment(ctx, login.SessionToken, currentPassword, "Integration Authenticator", requestID, correlationID)
	if err != nil || enrollment.Secret == "" || enrollment.FactorID == "" {
		t.Fatalf("enrollment = %#v, %v", enrollment, err)
	}
	code, err := identity.TOTPCode(enrollment.Secret, service.now())
	if err != nil {
		t.Fatal(err)
	}
	activation, err := service.ConfirmTOTPEnrollment(ctx, login.SessionToken, enrollment.FactorID, code, requestID, correlationID)
	if err != nil || len(activation.RecoveryCodes) != recoveryCodeCount {
		t.Fatalf("activation = %#v, %v", activation, err)
	}
	viewValue, err := service.Session(ctx, login.SessionToken)
	if err != nil {
		t.Fatal(err)
	}
	view := viewValue.(SessionView)
	if view.AuthenticationAssurance != identity.AssuranceMultiFactor || view.MFAEnrollmentRequired {
		t.Fatalf("session assurance = %q, enrollment required = %v", view.AuthenticationAssurance, view.MFAEnrollmentRequired)
	}
	actor, err := service.ResolveActor(ctx, login.SessionToken, identity.AccessPlaneAdmin)
	if err != nil {
		t.Fatal(err)
	}
	if err := service.Authorize(ctx, actor, "core.identity.work-account.create", coreauth.InstallationResource(resource.KindPlatformInstallation, resource.RootID)); err != nil {
		t.Fatalf("bootstrap admin permission denied: %v", err)
	}
	if err := service.Authorize(ctx, actor, "core.identity.work-account.read", coreauth.InstallationResource(resource.KindTenant, resource.RootID)); err != nil {
		t.Fatalf("bootstrap admin directory permission denied: %v", err)
	}
	for permission, kind := range map[string]resource.Kind{
		"core.authorization.work-role.read":   resource.KindTenant,
		"core.authorization.work-role.create": resource.KindTenant,
		"core.authorization.work-role.assign": resource.KindWorkAccount,
	} {
		if err := service.Authorize(ctx, actor, permission, coreauth.InstallationResource(kind, resource.RootID)); err != nil {
			t.Fatalf("bootstrap admin role permission %s denied: %v", permission, err)
		}
	}
	if err := service.LogoutWithAudit(ctx, login.SessionToken, requestID, correlationID); err != nil {
		t.Fatal(err)
	}
	challenge, err := service.LoginWithMFA(ctx, "mfa-integration@werk.local", currentPassword, requestID, correlationID)
	if err != nil || !challenge.MFARequired || challenge.ChallengeToken == "" || challenge.SessionToken != "" {
		t.Fatalf("MFA challenge = %#v, %v", challenge, err)
	}
	completed, err := service.CompleteMFAChallenge(ctx, challenge.ChallengeToken, activation.RecoveryCodes[0], requestID, correlationID)
	if err != nil || completed.SessionToken == "" || completed.Redirect != "/admin" {
		t.Fatalf("completed login = %#v, %v", completed, err)
	}
	if _, err := service.CompleteMFAChallenge(ctx, challenge.ChallengeToken, activation.RecoveryCodes[0], requestID, correlationID); err == nil {
		t.Fatal("used challenge and recovery code were accepted twice")
	}
	assertAuthenticationAuditEvents(t, ctx, correlationID)
}

func assertAuthenticationAuditEvents(t *testing.T, ctx context.Context, correlationID string) {
	t.Helper()
	databaseURL := os.Getenv("WERK_TEST_BACKUP_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("WERK_TEST_BACKUP_DATABASE_URL is not set")
	}
	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	connection, err := pool.Acquire(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer connection.Release()
	if _, err := connection.Exec(ctx, `SET ROLE werk_backup_reader`); err != nil {
		t.Fatal(err)
	}
	rows, err := connection.Query(ctx, `
		SELECT event_type, count(*)
		FROM werk_core.security_audit_events
		WHERE correlation_id = $1::uuid
		GROUP BY event_type
	`, correlationID)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	counts := make(map[string]int)
	for rows.Next() {
		var eventType string
		var count int
		if err := rows.Scan(&eventType, &count); err != nil {
			t.Fatal(err)
		}
		counts[eventType] = count
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	for _, eventType := range []string{
		"identity.login.succeeded.v1",
		"identity.login.second-factor-required.v1",
		"identity.password.changed.v1",
		"identity.logout.succeeded.v1",
		"identity.mfa.authentication-succeeded.v1",
	} {
		if counts[eventType] == 0 {
			t.Errorf("missing security audit event %q; counts = %#v", eventType, counts)
		}
	}
	var auditCount int
	var queuedCount int
	if err := connection.QueryRow(ctx, `
		SELECT count(*), count(queue.audit_event_id)
		FROM werk_core.security_audit_events AS audit
		LEFT JOIN werk_core.security_audit_export_queue AS queue
		  ON queue.audit_event_id = audit.id
		WHERE audit.correlation_id = $1::uuid
	`, correlationID).Scan(&auditCount, &queuedCount); err != nil {
		t.Fatal(err)
	}
	if auditCount == 0 || queuedCount != auditCount {
		t.Fatalf("audit export queue coverage = %d/%d", queuedCount, auditCount)
	}
}
