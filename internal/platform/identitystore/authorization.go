package identitystore

import (
	"context"
	"crypto/sha256"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	coreauth "github.com/dytonpictures/werk/internal/core/authorization"
	"github.com/dytonpictures/werk/internal/core/compliance"
	"github.com/dytonpictures/werk/internal/core/identity"
	"github.com/dytonpictures/werk/internal/core/resource"
	"github.com/dytonpictures/werk/internal/core/tenancy"
	"github.com/dytonpictures/werk/internal/platform/database"
)

func (service *Service) ResolveActor(ctx context.Context, token string, plane identity.AccessPlane) (identity.AuthenticatedActor, error) {
	if token == "" {
		return identity.AuthenticatedActor{}, identity.ErrSessionInvalid
	}
	hash := sha256.Sum256([]byte(token))
	var sessionID, accountID [16]byte
	var accountClass, audience, assurance, authenticationKind string
	var mustChangePassword bool
	var tenantValue pgtype.UUID
	var expiresAt time.Time
	err := service.database.WithinRead(ctx, func(ctx context.Context, tx database.TenantTx) error {
		return tx.QueryRow(ctx, `
			SELECT session.id, account.id, account.account_class, session.audience,
			       session.authentication_assurance, session.authentication_kind,
			       session.tenant_id, session.expires_at,
			       account.must_change_password
			FROM werk_core.sessions AS session
			JOIN werk_core.accounts AS account ON account.id = session.account_id
			LEFT JOIN werk_core.tenants AS tenant ON tenant.id = account.tenant_id
			WHERE session.token_hash = $1 AND session.revoked_at IS NULL
			  AND session.expires_at > $2 AND account.status = 'active'
			  AND (account.tenant_id IS NULL OR tenant.status = 'active')
		`, hash[:], service.now()).Scan(&sessionID, &accountID, &accountClass, &audience, &assurance, &authenticationKind, &tenantValue, &expiresAt, &mustChangePassword)
	})
	if err != nil || mustChangePassword {
		return identity.AuthenticatedActor{}, identity.ErrSessionInvalid
	}
	actor := identity.AuthenticatedActor{AccountID: identity.AccountID(accountID), AccountClass: identity.AccountClass(accountClass), Audience: identity.Audience(audience), Kind: identity.AuthenticationKind(authenticationKind), Assurance: identity.AuthenticationAssurance(assurance)}
	var tenantID *tenancy.TenantID
	if tenantValue.Valid {
		value := tenancy.TenantID(tenantValue.Bytes)
		actor.TenantID = &value
		tenantID = &value
	}
	return identity.ResolveSession(identity.SessionRecord{ID: identity.SessionID(sessionID), Account: actor, Audience: identity.Audience(audience), TenantID: tenantID, ExpiresAt: expiresAt}, plane, service.now())
}

func (service *Service) Authorize(ctx context.Context, actor identity.AuthenticatedActor, permission string, target coreauth.Resource) error {
	grants := make([]coreauth.Grant, 0, 4)
	registeredBoundary := resource.Boundary("")
	dataProfile := compliance.ResourceDataProfile{}
	processingPolicy := compliance.ProcessingPolicy{}
	err := service.database.WithinRead(ctx, func(ctx context.Context, tx database.TenantTx) error {
		rows, err := tx.Query(ctx, `
			SELECT resource_type.boundary,
			       data_profile.personal_data_category, data_profile.confidentiality_level,
			       data_profile.processing_activity_required, data_profile.contract_version, data_profile.status,
			       processing.processing_required, processing.activity_key, processing.purpose_key,
			       processing.legal_basis_ref, processing.contract_version, processing.status,
			       permission.access_plane, permission.permission_key,
			       assignment.scope_type, assignment.scope_tenant_id,
			       COALESCE(assignment.scope_id, ''), assignment.valid_from, assignment.valid_until
			FROM werk_core.role_assignments AS assignment
			JOIN werk_core.roles AS role ON role.id = assignment.role_id AND role.status = 'active'
			JOIN werk_core.role_permissions AS mapping ON mapping.role_id = role.id
			JOIN werk_core.permissions AS permission ON permission.id = mapping.permission_id AND permission.status = 'active'
			JOIN werk_core.platform_modules AS permission_module
			  ON permission_module.module_key = permission.owning_module AND permission_module.status = 'active'
			JOIN werk_core.permission_resource_types AS target ON target.permission_id = permission.id
			JOIN werk_core.resource_type_registrations AS resource_type
			  ON resource_type.resource_kind = target.resource_kind AND resource_type.status = 'active'
			JOIN werk_core.resource_data_profiles AS data_profile
			  ON data_profile.resource_kind = resource_type.resource_kind AND data_profile.status = 'active'
			JOIN werk_core.permission_processing_policies AS processing
			  ON processing.permission_id = permission.id
			 AND processing.resource_kind = resource_type.resource_kind
			 AND processing.status = 'active'
			JOIN werk_core.platform_modules AS resource_module
			  ON resource_module.module_key = resource_type.owner_module AND resource_module.status = 'active'
			WHERE assignment.account_id = $1::uuid AND permission.permission_key = $2
			  AND target.resource_kind = $3
		`, formatUUID(actor.AccountID), permission, target.Reference.Kind)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var boundary, personalData, confidentiality, profileStatus string
			var plane, key, scope, scopeID, processingStatus string
			var profileProcessingRequired, processingRequired bool
			var profileVersion, processingVersion int64
			var activityKey, purposeKey, legalBasisRef pgtype.Text
			var tenantValue pgtype.UUID
			var validFrom time.Time
			var validUntil *time.Time
			if err := rows.Scan(
				&boundary,
				&personalData, &confidentiality, &profileProcessingRequired, &profileVersion, &profileStatus,
				&processingRequired, &activityKey, &purposeKey, &legalBasisRef, &processingVersion, &processingStatus,
				&plane, &key, &scope, &tenantValue, &scopeID, &validFrom, &validUntil,
			); err != nil {
				return err
			}
			registeredBoundary = resource.Boundary(boundary)
			dataProfile = compliance.ResourceDataProfile{
				ResourceKind: target.Reference.Kind, PersonalData: compliance.PersonalDataCategory(personalData),
				Confidentiality:            compliance.ConfidentialityLevel(confidentiality),
				ProcessingActivityRequired: profileProcessingRequired,
				Status:                     resource.RegistrationStatus(profileStatus),
				Version:                    uint64(profileVersion),
			}
			processingPolicy = compliance.ProcessingPolicy{
				Permission: key, ResourceKind: target.Reference.Kind, Required: processingRequired,
				Status: resource.RegistrationStatus(processingStatus), Version: uint64(processingVersion),
			}
			if activityKey.Valid && purposeKey.Valid && legalBasisRef.Valid {
				processingPolicy.Context = compliance.ProcessingContext{
					ActivityKey: activityKey.String, PurposeKey: purposeKey.String, LegalBasisRef: legalBasisRef.String,
				}
			}
			grant := coreauth.Grant{AccessPlane: identity.AccessPlane(plane), Permission: key, Scope: coreauth.ScopeType(scope), ScopeID: scopeID, ValidFrom: validFrom, ValidUntil: validUntil}
			if tenantValue.Valid {
				value := tenancy.TenantID(tenantValue.Bytes)
				grant.TenantID = &value
			}
			grants = append(grants, grant)
		}
		return rows.Err()
	})
	if err != nil {
		return coreauth.ErrDenied
	}
	if registeredBoundary == "" || registeredBoundary != target.Reference.Boundary {
		return coreauth.ErrDenied
	}
	return coreauth.Authorize(coreauth.PolicyRequest{
		Actor: actor, Permission: permission, Target: target, Grants: grants,
		DataProfile: dataProfile, ProcessingPolicy: processingPolicy, EvaluatedAt: service.now(),
	})
}
