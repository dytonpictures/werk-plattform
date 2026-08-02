// Package businessobjectstore persists and resolves minimized, tenant-bound
// BusinessObjectView projections. The publisher deliberately operates on the
// caller's transaction so the owning mutation, projection, audit, and outbox
// can share one PostgreSQL commit.
package businessobjectstore

import (
	"context"
	"errors"
	"math"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/dytonpictures/werk/internal/core/businessobject"
	"github.com/dytonpictures/werk/internal/core/compliance"
	"github.com/dytonpictures/werk/internal/core/identity"
	"github.com/dytonpictures/werk/internal/core/resource"
	"github.com/dytonpictures/werk/internal/core/tenancy"
	"github.com/dytonpictures/werk/internal/platform/database"
)

const (
	WorkspaceResourceKind    resource.Kind = resource.KindWorkspace
	WorkspaceOwnerModule                   = "core.workspace"
	WorkspaceReadPermission                = "core.workspace.access"
	WorkspaceContractVersion uint64        = 1
)

var (
	ErrInvalidRequest      = errors.New("invalid business object store request")
	ErrContractUnavailable = errors.New("business object view contract unavailable")
	ErrNotFound            = errors.New("business object view not found")
	ErrStaleSourceVersion  = errors.New("stale business object source version")
	ErrConflictingRetry    = errors.New("conflicting business object retry")
)

// Resolved is the server-side result needed by an authorization adapter. The
// permission is registry-derived and must never be accepted from or exposed to
// an HTTP client.
type Resolved struct {
	View            businessobject.View
	ReadPermission  string
	ContractVersion uint64
	Version         uint64
}

// Publisher writes through a caller-owned admin tenant transaction. V1 is
// intentionally stateless and bound to the workspace projection; it cannot
// open, commit, or roll back a transaction itself.
type Publisher struct{}

func NewPublisher() *Publisher { return &Publisher{} }

type Reader struct {
	database *database.WorkDB
}

func NewReader(workDatabase *database.WorkDB) (*Reader, error) {
	if workDatabase == nil {
		return nil, errors.New("work database is required")
	}
	return &Reader{database: workDatabase}, nil
}

// Resolve loads one active projection in the Work actor's explicit tenant. It
// resolves the read permission and complete Core contract from server-owned
// registry state; missing or inactive registrations fail closed as not found.
func (reader *Reader) Resolve(ctx context.Context, actor identity.AuthenticatedActor, ref resource.Ref) (Resolved, error) {
	if reader == nil || reader.database == nil ||
		identity.AuthorizeAccessPlane(actor, identity.AccessPlaneWork) != nil ||
		actor.TenantID == nil || !workspaceRefMatchesTenant(ref, *actor.TenantID) {
		return Resolved{}, identity.ErrAccessDenied
	}

	var resolved Resolved
	err := reader.database.WithinTenantRead(ctx, *actor.TenantID, func(ctx context.Context, tx database.TenantTx) error {
		contract, err := resolveActiveWorkspaceContract(ctx, tx)
		if err != nil {
			if errors.Is(err, ErrContractUnavailable) {
				return ErrNotFound
			}
			return err
		}

		var ownerModule string
		var title string
		var classification *string
		var sourceVersion int64
		var version int64
		var updatedAt time.Time
		var contractVersion int64
		err = tx.QueryRow(ctx, `
			SELECT owner_module, title, classification, source_version, version,
			       updated_at, contract_version
			FROM werk_core.business_object_views
			WHERE tenant_id = $1::uuid
			  AND resource_kind = $2
			  AND resource_id = $3
			  AND state = 'active'
		`, actor.TenantID.String(), string(ref.Kind), ref.ID).Scan(
			&ownerModule, &title, &classification, &sourceVersion, &version,
			&updatedAt, &contractVersion,
		)
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrNotFound
		}
		if err != nil {
			return err
		}
		if sourceVersion <= 0 || version <= 0 || contractVersion <= 0 || uint64(contractVersion) != contract.version ||
			ownerModule != contract.model.ResourceType.OwnerModule {
			return ErrNotFound
		}

		classificationLevel, err := parseClassification(classification)
		if err != nil {
			return ErrNotFound
		}
		view, err := businessobject.NewView(
			contract.model, *actor.TenantID, ref, title, classificationLevel,
			uint64(sourceVersion), updatedAt.UTC(),
		)
		if err != nil {
			return ErrNotFound
		}
		resolved = Resolved{
			View: view, ReadPermission: contract.permission,
			ContractVersion: contract.version, Version: uint64(version),
		}
		return nil
	})
	if err != nil {
		return Resolved{}, err
	}
	return resolved, nil
}

func (reader *Reader) Get(ctx context.Context, actor identity.AuthenticatedActor, ref resource.Ref) (businessobject.View, error) {
	resolved, err := reader.Resolve(ctx, actor, ref)
	if err != nil {
		return businessobject.View{}, err
	}
	return resolved.View, nil
}

// PublishWorkspace is the narrow V1 owner contract used by tenant
// administration. It derives reference, owner, classification baseline and
// registry contract server-side, so callers cannot forge projection identity.
func (publisher *Publisher) PublishWorkspace(ctx context.Context, tx database.TenantTx,
	tenantID tenancy.TenantID, title string, sourceVersion uint64, updatedAt time.Time) error {
	if publisher == nil || tx == nil || tenantID.IsZero() || sourceVersion == 0 ||
		sourceVersion > math.MaxInt64 || updatedAt.IsZero() {
		return ErrInvalidRequest
	}
	contract, err := resolveActiveWorkspaceContract(ctx, tx)
	if err != nil {
		return err
	}
	ref := resource.TenantRef(tenantID, WorkspaceResourceKind, tenantID.String())
	view, err := businessobject.NewView(
		contract.model, tenantID, ref, title, nil, sourceVersion, updatedAt.UTC(),
	)
	if err != nil {
		return err
	}
	return publisher.Publish(ctx, tx, tenantID, view)
}

// WithdrawWorkspace removes navigational data while retaining a versioned
// tombstone. Only a strictly newer owner revision can publish it again.
func (publisher *Publisher) WithdrawWorkspace(ctx context.Context, tx database.TenantTx,
	tenantID tenancy.TenantID, sourceVersion uint64, updatedAt time.Time) error {
	ref := resource.TenantRef(tenantID, WorkspaceResourceKind, tenantID.String())
	return publisher.Withdraw(ctx, tx, tenantID, ref, sourceVersion, updatedAt)
}

// Publish creates or replaces the active workspace projection. A retry with an
// identical source version and identical contents is a no-op. The same version
// with different contents and every lower version fail closed.
func (publisher *Publisher) Publish(ctx context.Context, tx database.TenantTx, tenantID tenancy.TenantID, view businessobject.View) error {
	if publisher == nil || tx == nil || tenantID.IsZero() ||
		!workspaceRefMatchesTenant(view.Ref, tenantID) || view.SourceVersion > math.MaxInt64 {
		return ErrInvalidRequest
	}
	contract, err := resolveActiveWorkspaceContract(ctx, tx)
	if err != nil {
		return err
	}
	if view.Validate(contract.model, tenantID) != nil {
		return businessobject.ErrInvalid
	}

	existing, found, err := loadProjectionForUpdate(ctx, tx, tenantID, view.Ref)
	if err != nil {
		return err
	}
	if found {
		switch {
		case view.SourceVersion < existing.sourceVersion:
			return ErrStaleSourceVersion
		case view.SourceVersion == existing.sourceVersion:
			if existing.identicalActive(view, contract) {
				return nil
			}
			return ErrConflictingRetry
		case view.UpdatedAt.Before(existing.updatedAt):
			return ErrStaleSourceVersion
		}
		command, err := tx.Exec(ctx, `
			UPDATE werk_core.business_object_views
			SET contract_version = $4,
			    owner_module = $5,
			    title = $6,
			    classification = $7,
			    state = 'active',
			    source_version = $8,
			    version = version + 1,
			    updated_at = $9
			WHERE tenant_id = $1::uuid
			  AND resource_kind = $2
			  AND resource_id = $3
			  AND source_version = $10
		`, tenantID.String(), string(view.Ref.Kind), view.Ref.ID,
			contract.version, contract.model.ResourceType.OwnerModule,
			view.Title, classificationValue(view.Classification), view.SourceVersion,
			view.UpdatedAt, existing.sourceVersion)
		if err != nil {
			return err
		}
		if command.RowsAffected() != 1 {
			return ErrStaleSourceVersion
		}
		return nil
	}

	command, err := tx.Exec(ctx, `
		INSERT INTO werk_core.business_object_views (
			tenant_id, resource_kind, resource_id, contract_version,
			owner_module, title, classification, state, source_version, updated_at
		) VALUES (
			$1::uuid, $2, $3, $4, $5, $6, $7, 'active', $8, $9
		)
	`, tenantID.String(), string(view.Ref.Kind), view.Ref.ID,
		contract.version, contract.model.ResourceType.OwnerModule,
		view.Title, classificationValue(view.Classification), view.SourceVersion,
		view.UpdatedAt)
	if err != nil {
		return err
	}
	if command.RowsAffected() != 1 {
		return ErrConflictingRetry
	}
	return nil
}

// Withdraw removes an object's navigational contents while preserving its
// source version. A later, strictly newer Publish may reactivate the workspace
// after the owning tenant has itself become active again.
func (publisher *Publisher) Withdraw(ctx context.Context, tx database.TenantTx, tenantID tenancy.TenantID,
	ref resource.Ref, sourceVersion uint64, updatedAt time.Time) error {
	if publisher == nil || tx == nil || tenantID.IsZero() ||
		!workspaceRefMatchesTenant(ref, tenantID) || sourceVersion == 0 ||
		sourceVersion > math.MaxInt64 || updatedAt.IsZero() {
		return ErrInvalidRequest
	}
	updatedAt = updatedAt.UTC()
	contract, err := resolveActiveWorkspaceContract(ctx, tx)
	if err != nil {
		return err
	}
	existing, found, err := loadProjectionForUpdate(ctx, tx, tenantID, ref)
	if err != nil {
		return err
	}
	if !found {
		return ErrNotFound
	}
	switch {
	case sourceVersion < existing.sourceVersion:
		return ErrStaleSourceVersion
	case sourceVersion == existing.sourceVersion:
		if existing.identicalWithdrawn(updatedAt, contract) {
			return nil
		}
		return ErrConflictingRetry
	case updatedAt.Before(existing.updatedAt):
		return ErrStaleSourceVersion
	}

	command, err := tx.Exec(ctx, `
		UPDATE werk_core.business_object_views
		SET contract_version = $4,
		    owner_module = $5,
		    title = NULL,
		    classification = NULL,
		    state = 'withdrawn',
		    source_version = $6,
		    version = version + 1,
		    updated_at = $7
		WHERE tenant_id = $1::uuid
		  AND resource_kind = $2
		  AND resource_id = $3
		  AND source_version = $8
	`, tenantID.String(), string(ref.Kind), ref.ID,
		contract.version, contract.model.ResourceType.OwnerModule,
		sourceVersion, updatedAt, existing.sourceVersion)
	if err != nil {
		return err
	}
	if command.RowsAffected() != 1 {
		return ErrStaleSourceVersion
	}
	return nil
}

type resolvedContract struct {
	model      businessobject.Contract
	version    uint64
	permission string
}

func resolveActiveWorkspaceContract(ctx context.Context, tx database.TenantTx) (resolvedContract, error) {
	if tx == nil {
		return resolvedContract{}, ErrContractUnavailable
	}
	var resourceKind, ownerModule, permissionKey string
	var moduleKind, resourceBoundary string
	var personalData, confidentiality string
	var contractVersion, moduleVersion, resourceVersion, profileVersion int64
	var profileProcessingRequired bool
	err := tx.QueryRow(ctx, `
		SELECT resource_kind, contract_version, owner_module, read_permission_key,
		       module_kind, module_contract_version, resource_boundary,
		       resource_contract_version, personal_data_category,
		       confidentiality_level, processing_activity_required,
		       data_profile_contract_version
		FROM werk_core.active_business_object_view_contracts
		WHERE resource_kind = $1
		  AND contract_version = $2
		  AND owner_module = $3
	`, string(WorkspaceResourceKind), WorkspaceContractVersion, WorkspaceOwnerModule).Scan(
		&resourceKind, &contractVersion, &ownerModule, &permissionKey,
		&moduleKind, &moduleVersion, &resourceBoundary, &resourceVersion,
		&personalData, &confidentiality, &profileProcessingRequired, &profileVersion,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return resolvedContract{}, ErrContractUnavailable
	}
	if err != nil {
		return resolvedContract{}, err
	}
	if contractVersion <= 0 || moduleVersion <= 0 || resourceVersion <= 0 || profileVersion <= 0 {
		return resolvedContract{}, ErrContractUnavailable
	}
	contract := businessobject.Contract{
		Module: resource.ModuleRegistration{
			Key: ownerModule, Kind: resource.ModuleKind(moduleKind),
			Status: resource.RegistrationActive, Version: uint64(moduleVersion),
		},
		ResourceType: resource.TypeRegistration{
			Kind: resource.Kind(resourceKind), OwnerModule: ownerModule,
			Boundary: resource.Boundary(resourceBoundary),
			Status:   resource.RegistrationActive, Version: uint64(resourceVersion),
		},
		DataProfile: compliance.ResourceDataProfile{
			ResourceKind:               resource.Kind(resourceKind),
			PersonalData:               compliance.PersonalDataCategory(personalData),
			Confidentiality:            compliance.ConfidentialityLevel(confidentiality),
			ProcessingActivityRequired: profileProcessingRequired,
			Status:                     resource.RegistrationActive, Version: uint64(profileVersion),
		},
	}
	if contract.Validate() != nil || uint64(contractVersion) != WorkspaceContractVersion ||
		contract.ResourceType.Kind != WorkspaceResourceKind ||
		contract.ResourceType.OwnerModule != WorkspaceOwnerModule ||
		permissionKey != WorkspaceReadPermission {
		return resolvedContract{}, ErrContractUnavailable
	}
	return resolvedContract{model: contract, version: uint64(contractVersion), permission: permissionKey}, nil
}

type storedProjection struct {
	contractVersion uint64
	version         uint64
	ownerModule     string
	title           *string
	classification  *string
	state           string
	sourceVersion   uint64
	updatedAt       time.Time
}

func loadProjectionForUpdate(ctx context.Context, tx database.TenantTx, tenantID tenancy.TenantID,
	ref resource.Ref) (storedProjection, bool, error) {
	var stored storedProjection
	var contractVersion, version, sourceVersion int64
	err := tx.QueryRow(ctx, `
		SELECT contract_version, version, owner_module, title, classification,
		       state, source_version, updated_at
		FROM werk_core.business_object_views
		WHERE tenant_id = $1::uuid
		  AND resource_kind = $2
		  AND resource_id = $3
		FOR UPDATE
	`, tenantID.String(), string(ref.Kind), ref.ID).Scan(
		&contractVersion, &version, &stored.ownerModule, &stored.title, &stored.classification,
		&stored.state, &sourceVersion, &stored.updatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return storedProjection{}, false, nil
	}
	if err != nil {
		return storedProjection{}, false, err
	}
	if contractVersion <= 0 || version <= 0 || sourceVersion <= 0 {
		return storedProjection{}, false, ErrConflictingRetry
	}
	stored.contractVersion = uint64(contractVersion)
	stored.version = uint64(version)
	stored.sourceVersion = uint64(sourceVersion)
	return stored, true, nil
}

func (stored storedProjection) identicalActive(view businessobject.View, contract resolvedContract) bool {
	return stored.state == "active" && stored.contractVersion == contract.version &&
		stored.ownerModule == contract.model.ResourceType.OwnerModule &&
		stored.title != nil && *stored.title == view.Title &&
		equalClassification(stored.classification, view.Classification) &&
		stored.updatedAt.Equal(view.UpdatedAt)
}

func (stored storedProjection) identicalWithdrawn(updatedAt time.Time, contract resolvedContract) bool {
	return stored.state == "withdrawn" && stored.contractVersion == contract.version &&
		stored.ownerModule == contract.model.ResourceType.OwnerModule &&
		stored.title == nil && stored.classification == nil && stored.updatedAt.Equal(updatedAt)
}

func workspaceRefMatchesTenant(ref resource.Ref, tenantID tenancy.TenantID) bool {
	return !tenantID.IsZero() && ref.Validate() == nil &&
		ref.Boundary == resource.BoundaryTenant && ref.TenantID != nil &&
		*ref.TenantID == tenantID && ref.Kind == WorkspaceResourceKind &&
		ref.ID == tenantID.String()
}

func parseClassification(value *string) (*compliance.ConfidentialityLevel, error) {
	if value == nil {
		return nil, nil
	}
	classification := compliance.ConfidentialityLevel(*value)
	switch classification {
	case compliance.ConfidentialityPublic, compliance.ConfidentialityInternal,
		compliance.ConfidentialityConfidential, compliance.ConfidentialityRestricted:
		return &classification, nil
	default:
		return nil, ErrInvalidRequest
	}
}

func classificationValue(value *compliance.ConfidentialityLevel) any {
	if value == nil {
		return nil
	}
	return string(*value)
}

func equalClassification(stored *string, expected *compliance.ConfidentialityLevel) bool {
	if stored == nil || expected == nil {
		return stored == nil && expected == nil
	}
	return *stored == string(*expected)
}
