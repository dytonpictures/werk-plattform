// Package documentstore exposes tenant-bound, provider-independent document
// metadata to the work API. The read slice is limited to documents created by
// the authenticated actor or exposed through an active, direct document-local
// visibility binding.
package documentstore

import (
	"context"
	"errors"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"

	"github.com/dytonpictures/werk/internal/core/documents"
	"github.com/dytonpictures/werk/internal/core/identity"
	"github.com/dytonpictures/werk/internal/platform/database"
)

const (
	DefaultLimit      = 25
	MaximumLimit      = 100
	maximumSearchSize = 120
	VisibilityScope   = "created-or-directly-shared-with-me"
)

var (
	ErrInvalidQuery = errors.New("invalid document query")
	ErrNotFound     = errors.New("document not found")
)

type Service struct {
	database *database.WorkDB
}

type Cursor struct {
	UpdatedAt time.Time `json:"updated_at"`
	ID        string    `json:"id"`
}

type ListQuery struct {
	Limit          int
	Search         string
	Status         string
	Classification string
	AccessReason   string
	Cursor         *Cursor
}

type ClassificationView struct {
	Revision       uint64     `json:"revision"`
	Level          string     `json:"level"`
	RetentionClass string     `json:"retention_class"`
	RetentionUntil *time.Time `json:"retention_until,omitempty"`
	LegalHold      bool       `json:"legal_hold"`
}

type LatestVersionView struct {
	ID            string    `json:"id"`
	VersionNumber uint64    `json:"version_number"`
	Source        string    `json:"source"`
	PublishedAt   time.Time `json:"published_at"`
}

type Summary struct {
	ID             string             `json:"id"`
	Title          string             `json:"title"`
	Status         string             `json:"status"`
	SourceModule   string             `json:"source_module"`
	CreatedAt      time.Time          `json:"created_at"`
	UpdatedAt      time.Time          `json:"updated_at"`
	Version        uint64             `json:"version"`
	AccessReason   string             `json:"access_reason"`
	LatestVersion  LatestVersionView  `json:"latest_version"`
	Classification ClassificationView `json:"classification"`
}

type Page struct {
	Items      []Summary
	NextCursor *Cursor
}

type VersionView struct {
	ID            string    `json:"id"`
	VersionNumber uint64    `json:"version_number"`
	Source        string    `json:"source"`
	PublishedAt   time.Time `json:"published_at"`
}

type Detail struct {
	Summary
	Versions []VersionView `json:"versions"`
}

func New(db *database.WorkDB) (*Service, error) {
	if db == nil {
		return nil, errors.New("work database is required")
	}
	return &Service{database: db}, nil
}

func NormalizeListQuery(query ListQuery) (ListQuery, error) {
	query.Search = strings.TrimSpace(query.Search)
	query.Status = strings.TrimSpace(query.Status)
	query.Classification = strings.TrimSpace(query.Classification)
	query.AccessReason = strings.TrimSpace(query.AccessReason)
	if query.Limit == 0 {
		query.Limit = DefaultLimit
	}
	if query.Limit < 1 || query.Limit > MaximumLimit || utf8.RuneCountInString(query.Search) > maximumSearchSize {
		return ListQuery{}, ErrInvalidQuery
	}
	if query.Status != "" && query.Status != string(documents.StatusActive) && query.Status != string(documents.StatusArchived) {
		return ListQuery{}, ErrInvalidQuery
	}
	if query.Classification != "" && query.Classification != string(documents.ClassificationInternal) &&
		query.Classification != string(documents.ClassificationConfidential) &&
		query.Classification != string(documents.ClassificationRestricted) {
		return ListQuery{}, ErrInvalidQuery
	}
	if query.AccessReason != "" && query.AccessReason != string(documents.AccessReasonCreatedByMe) &&
		query.AccessReason != string(documents.AccessReasonSharedDirectlyWithMe) {
		return ListQuery{}, ErrInvalidQuery
	}
	if query.Cursor != nil && (query.Cursor.UpdatedAt.IsZero() || !ValidDocumentID(query.Cursor.ID)) {
		return ListQuery{}, ErrInvalidQuery
	}
	return query, nil
}

func (service *Service) List(ctx context.Context, actor identity.AuthenticatedActor, query ListQuery) (Page, error) {
	if err := identity.AuthorizeAccessPlane(actor, identity.AccessPlaneWork); err != nil || actor.TenantID == nil {
		return Page{}, identity.ErrAccessDenied
	}
	query, err := NormalizeListQuery(query)
	if err != nil {
		return Page{}, err
	}
	page := Page{Items: make([]Summary, 0, query.Limit)}
	var cursorTime any
	var cursorID any
	if query.Cursor != nil {
		cursorTime = query.Cursor.UpdatedAt.UTC()
		cursorID = query.Cursor.ID
	}
	err = service.database.WithinTenantRead(ctx, *actor.TenantID, func(ctx context.Context, tx database.TenantTx) error {
		rows, err := tx.Query(ctx, `
			SELECT document.id::text, document.title, document.status, document.source_module,
			       document.created_at, document.updated_at, document.version,
			       CASE WHEN document.created_by_account_id=$2::uuid
			            THEN 'created-by-me' ELSE 'shared-directly-with-me' END,
			       latest_version.id::text, latest_version.version_number,
			       latest_version.source, latest_version.published_at,
			       classification.revision, classification.classification,
			       classification.retention_class, classification.retention_until,
			       classification.legal_hold
			FROM werk_core.documents AS document
			JOIN LATERAL (
			  SELECT version.id, version.version_number, version.source, version.published_at
			  FROM werk_core.document_versions AS version
			  WHERE version.tenant_id=document.tenant_id AND version.document_id=document.id
			  ORDER BY version.version_number DESC
			  LIMIT 1
			) AS latest_version ON true
			JOIN LATERAL (
			  SELECT revision.revision, revision.classification, revision.retention_class,
			         revision.retention_until, revision.legal_hold
			  FROM werk_core.document_classification_revisions AS revision
			  WHERE revision.tenant_id=document.tenant_id AND revision.document_id=document.id
			  ORDER BY revision.revision DESC
			  LIMIT 1
			) AS classification ON true
			LEFT JOIN LATERAL (
			  SELECT true AS active
			  FROM werk_core.document_account_visibility_bindings AS binding
			  WHERE binding.tenant_id=document.tenant_id
			    AND binding.document_id=document.id
			    AND binding.grantee_account_id=$2::uuid
			    AND binding.revoked_at IS NULL
			  LIMIT 1
			) AS direct_visibility ON true
			WHERE document.tenant_id=$1::uuid
			  AND (document.created_by_account_id=$2::uuid OR COALESCE(direct_visibility.active, false))
			  AND ($3::text='' OR $3=CASE WHEN document.created_by_account_id=$2::uuid
			       THEN 'created-by-me' ELSE 'shared-directly-with-me' END)
			  AND ($4::text='' OR document.status=$4)
			  AND ($5::text='' OR classification.classification=$5)
			  AND ($6::text='' OR position(lower($6) in lower(document.title)) > 0)
			  AND ($7::timestamptz IS NULL OR document.updated_at < $7
			       OR (document.updated_at=$7 AND document.id < $8::uuid))
			ORDER BY document.updated_at DESC, document.id DESC
			LIMIT $9
		`, actor.TenantID.String(), formatUUID(actor.AccountID), query.AccessReason,
			query.Status, query.Classification, query.Search,
			cursorTime, cursorID, query.Limit+1)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var item Summary
			if err := rows.Scan(
				&item.ID, &item.Title, &item.Status, &item.SourceModule,
				&item.CreatedAt, &item.UpdatedAt, &item.Version, &item.AccessReason,
				&item.LatestVersion.ID, &item.LatestVersion.VersionNumber,
				&item.LatestVersion.Source, &item.LatestVersion.PublishedAt,
				&item.Classification.Revision, &item.Classification.Level,
				&item.Classification.RetentionClass, &item.Classification.RetentionUntil,
				&item.Classification.LegalHold,
			); err != nil {
				return err
			}
			page.Items = append(page.Items, item)
		}
		return rows.Err()
	})
	if err != nil {
		return Page{}, err
	}
	if len(page.Items) > query.Limit {
		page.Items = page.Items[:query.Limit]
		last := page.Items[len(page.Items)-1]
		page.NextCursor = &Cursor{UpdatedAt: last.UpdatedAt, ID: last.ID}
	}
	return page, nil
}

func (service *Service) Detail(ctx context.Context, actor identity.AuthenticatedActor, documentID string) (Detail, error) {
	if err := identity.AuthorizeAccessPlane(actor, identity.AccessPlaneWork); err != nil || actor.TenantID == nil {
		return Detail{}, identity.ErrAccessDenied
	}
	if !ValidDocumentID(documentID) {
		return Detail{}, ErrNotFound
	}
	detail := Detail{Versions: make([]VersionView, 0)}
	err := service.database.WithinTenantRead(ctx, *actor.TenantID, func(ctx context.Context, tx database.TenantTx) error {
		err := tx.QueryRow(ctx, `
			SELECT document.id::text, document.title, document.status, document.source_module,
			       document.created_at, document.updated_at, document.version,
			       CASE WHEN document.created_by_account_id=$3::uuid
			            THEN 'created-by-me' ELSE 'shared-directly-with-me' END,
			       latest_version.id::text, latest_version.version_number,
			       latest_version.source, latest_version.published_at,
			       classification.revision, classification.classification,
			       classification.retention_class, classification.retention_until,
			       classification.legal_hold
			FROM werk_core.documents AS document
			JOIN LATERAL (
			  SELECT version.id, version.version_number, version.source, version.published_at
			  FROM werk_core.document_versions AS version
			  WHERE version.tenant_id=document.tenant_id AND version.document_id=document.id
			  ORDER BY version.version_number DESC
			  LIMIT 1
			) AS latest_version ON true
			JOIN LATERAL (
			  SELECT revision.revision, revision.classification, revision.retention_class,
			         revision.retention_until, revision.legal_hold
			  FROM werk_core.document_classification_revisions AS revision
			  WHERE revision.tenant_id=document.tenant_id AND revision.document_id=document.id
			  ORDER BY revision.revision DESC
			  LIMIT 1
			) AS classification ON true
			LEFT JOIN LATERAL (
			  SELECT true AS active
			  FROM werk_core.document_account_visibility_bindings AS binding
			  WHERE binding.tenant_id=document.tenant_id
			    AND binding.document_id=document.id
			    AND binding.grantee_account_id=$3::uuid
			    AND binding.revoked_at IS NULL
			  LIMIT 1
			) AS direct_visibility ON true
			WHERE document.tenant_id=$1::uuid AND document.id=$2::uuid
			  AND (document.created_by_account_id=$3::uuid OR COALESCE(direct_visibility.active, false))
		`, actor.TenantID.String(), documentID, formatUUID(actor.AccountID)).Scan(
			&detail.ID, &detail.Title, &detail.Status, &detail.SourceModule,
			&detail.CreatedAt, &detail.UpdatedAt, &detail.Version, &detail.AccessReason,
			&detail.LatestVersion.ID, &detail.LatestVersion.VersionNumber,
			&detail.LatestVersion.Source, &detail.LatestVersion.PublishedAt,
			&detail.Classification.Revision, &detail.Classification.Level,
			&detail.Classification.RetentionClass, &detail.Classification.RetentionUntil,
			&detail.Classification.LegalHold,
		)
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrNotFound
		}
		if err != nil {
			return err
		}
		rows, err := tx.Query(ctx, `
			SELECT version.id::text, version.version_number, version.source, version.published_at
			FROM werk_core.document_versions AS version
			WHERE version.tenant_id=$1::uuid AND version.document_id=$2::uuid
			ORDER BY version.version_number DESC
		`, actor.TenantID.String(), documentID)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var version VersionView
			if err := rows.Scan(&version.ID, &version.VersionNumber, &version.Source, &version.PublishedAt); err != nil {
				return err
			}
			detail.Versions = append(detail.Versions, version)
		}
		return rows.Err()
	})
	if err != nil {
		return Detail{}, err
	}
	return detail, nil
}

func ValidDocumentID(value string) bool {
	if len(value) != 36 {
		return false
	}
	for index, character := range value {
		if index == 8 || index == 13 || index == 18 || index == 23 {
			if character != '-' {
				return false
			}
			continue
		}
		if !((character >= '0' && character <= '9') || (character >= 'a' && character <= 'f') || (character >= 'A' && character <= 'F')) {
			return false
		}
	}
	return true
}

func formatUUID(value identity.AccountID) string {
	const hex = "0123456789abcdef"
	buffer := make([]byte, 36)
	positions := []int{0, 2, 4, 6, 9, 11, 14, 16, 19, 21, 24, 26, 28, 30, 32, 34}
	for index, current := range value {
		position := positions[index]
		buffer[position] = hex[current>>4]
		buffer[position+1] = hex[current&0x0f]
	}
	buffer[8], buffer[13], buffer[18], buffer[23] = '-', '-', '-', '-'
	return string(buffer)
}
