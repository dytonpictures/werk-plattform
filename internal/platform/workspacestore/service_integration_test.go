package workspacestore

import (
	"context"
	"encoding/hex"
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/dytonpictures/werk/internal/core/identity"
	"github.com/dytonpictures/werk/internal/core/tenancy"
	"github.com/dytonpictures/werk/internal/platform/database"
)

func TestWorkspaceOverviewTenantIsolationIntegration(t *testing.T) {
	workURL := os.Getenv("WERK_TEST_WORK_DATABASE_URL")
	migratorURL := os.Getenv("WERK_TEST_MIGRATOR_DATABASE_URL")
	if workURL == "" || migratorURL == "" {
		t.Skip("integration database URLs are not set")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	owner, err := pgxpool.New(ctx, migratorURL)
	if err != nil {
		t.Fatal(err)
	}
	defer owner.Close()
	connection, err := owner.Acquire(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer connection.Release()
	if _, err := connection.Exec(ctx, `SET ROLE werk_owner`); err != nil {
		t.Fatal(err)
	}
	const (
		tenantA     = "0196f000-0000-7000-8000-000000000801"
		tenantB     = "0196f000-0000-7000-8000-000000000802"
		unitA       = "0196f000-0000-7000-8000-000000000803"
		unitB       = "0196f000-0000-7000-8000-000000000804"
		partyA      = "0196f000-0000-7000-8000-000000000805"
		partyB      = "0196f000-0000-7000-8000-000000000806"
		membershipA = "0196f000-0000-7000-8000-000000000807"
		membershipB = "0196f000-0000-7000-8000-000000000808"
		accountA    = "0196f000-0000-7000-8000-000000000809"
		accountB    = "0196f000-0000-7000-8000-00000000080a"
	)
	statements := []struct {
		query string
		args  []any
	}{
		{`INSERT INTO werk_core.tenants (id,name,status,default_locale,default_timezone) VALUES ($1::uuid,'Workspace A','active','de-DE','Europe/Berlin'),($2::uuid,'Workspace B','active','de-DE','Europe/Berlin')`, []any{tenantA, tenantB}},
		{`INSERT INTO werk_core.organizational_units (id,tenant_id,unit_type,name,status) VALUES ($1::uuid,$2::uuid,'team','Team A','active'),($3::uuid,$4::uuid,'team','Team B','active')`, []any{unitA, tenantA, unitB, tenantB}},
		{`INSERT INTO werk_core.parties (id,tenant_id,party_type,display_name,status) VALUES ($1::uuid,$2::uuid,'person','Worker A','active'),($3::uuid,$4::uuid,'person','Worker B','active')`, []any{partyA, tenantA, partyB, tenantB}},
		{`INSERT INTO werk_core.persons (party_id,tenant_id,given_name,family_name) VALUES ($1::uuid,$2::uuid,'Worker','A'),($3::uuid,$4::uuid,'Worker','B')`, []any{partyA, tenantA, partyB, tenantB}},
		{`INSERT INTO werk_core.memberships (id,tenant_id,party_id,organizational_unit_id,membership_type,valid_from) VALUES ($1::uuid,$2::uuid,$3::uuid,$4::uuid,'team.member',now()),($5::uuid,$6::uuid,$7::uuid,$8::uuid,'team.manager',now())`, []any{membershipA, tenantA, partyA, unitA, membershipB, tenantB, partyB, unitB}},
		{`INSERT INTO werk_core.accounts (id,account_class,tenant_id,person_party_id,login_name,status) VALUES ($1::uuid,'work',$2::uuid,$3::uuid,'workspace-a@werk.test','active'),($4::uuid,'work',$5::uuid,$6::uuid,'workspace-b@werk.test','active')`, []any{accountA, tenantA, partyA, accountB, tenantB, partyB}},
	}
	for _, statement := range statements {
		if _, err := connection.Exec(ctx, statement.query, statement.args...); err != nil {
			t.Fatal(err)
		}
	}
	defer func() {
		cleanupContext, cleanupCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cleanupCancel()
		_, _ = connection.Exec(cleanupContext, `DELETE FROM werk_core.accounts WHERE id IN ($1::uuid,$2::uuid)`, accountA, accountB)
		_, _ = connection.Exec(cleanupContext, `DELETE FROM werk_core.memberships WHERE id IN ($1::uuid,$2::uuid)`, membershipA, membershipB)
		_, _ = connection.Exec(cleanupContext, `DELETE FROM werk_core.persons WHERE party_id IN ($1::uuid,$2::uuid)`, partyA, partyB)
		_, _ = connection.Exec(cleanupContext, `DELETE FROM werk_core.parties WHERE id IN ($1::uuid,$2::uuid)`, partyA, partyB)
		_, _ = connection.Exec(cleanupContext, `DELETE FROM werk_core.organizational_units WHERE id IN ($1::uuid,$2::uuid)`, unitA, unitB)
		_, _ = connection.Exec(cleanupContext, `DELETE FROM werk_core.tenants WHERE id IN ($1::uuid,$2::uuid)`, tenantA, tenantB)
	}()

	workDatabase, err := database.NewWork(ctx, workURL, "werk-workspace-integration")
	if err != nil {
		t.Fatal(err)
	}
	defer workDatabase.Close()
	service, err := New(workDatabase)
	if err != nil {
		t.Fatal(err)
	}
	parsedTenantA, _ := tenancy.ParseTenantID(tenantA)
	actorA := identity.AuthenticatedActor{
		AccountID: accountID(accountA), AccountClass: identity.AccountClassWork,
		Audience: identity.AudienceWork, Kind: identity.AuthenticationInteractive,
		Assurance: identity.AssuranceSingleFactor, TenantID: &parsedTenantA,
	}
	view, err := service.Overview(ctx, actorA)
	if err != nil || view.Tenant.ID != tenantA || view.Tenant.Name != "Workspace A" || view.OrganizationalUnit == nil || view.OrganizationalUnit.ID != unitA || view.MembershipType != "team.member" {
		t.Fatalf("workspace A = %#v, err = %v", view, err)
	}

	foreignAccount := actorA
	foreignAccount.AccountID = accountID(accountB)
	if _, err := service.Overview(ctx, foreignAccount); !errors.Is(err, identity.ErrAccessDenied) {
		t.Fatalf("foreign account error = %v, want access denied", err)
	}
	adminActor := identity.AuthenticatedActor{
		AccountID: identity.AccountID{9}, AccountClass: identity.AccountClassAdmin,
		Audience: identity.AudienceAdmin, Kind: identity.AuthenticationInteractive,
		Assurance: identity.AssuranceMultiFactor,
	}
	if _, err := service.Overview(ctx, adminActor); !errors.Is(err, identity.ErrAccessDenied) {
		t.Fatalf("admin overview error = %v, want access denied", err)
	}
}

func accountID(value string) identity.AccountID {
	raw, _ := hex.DecodeString(strings.ReplaceAll(value, "-", ""))
	var id identity.AccountID
	copy(id[:], raw)
	return id
}
