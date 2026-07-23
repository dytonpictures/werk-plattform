// Package documents defines the provider-independent document aggregate. It
// owns published immutable versions and classification history, but no object
// store credentials, transfer tickets, or mutable collaboration working copies.
package documents

import (
	"crypto/rand"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/dytonpictures/werk/internal/core/identity"
	"github.com/dytonpictures/werk/internal/core/resource"
	"github.com/dytonpictures/werk/internal/core/storage"
	"github.com/dytonpictures/werk/internal/core/tenancy"
)

var ErrInvalid = errors.New("invalid document model")

const maximumTitleLength = 240

type DocumentID [16]byte

func (id DocumentID) IsZero() bool { return id == DocumentID{} }

type VersionID [16]byte

func (id VersionID) IsZero() bool { return id == VersionID{} }

type ClassificationRevisionID [16]byte

func (id ClassificationRevisionID) IsZero() bool { return id == ClassificationRevisionID{} }

type Status string

const (
	StatusActive   Status = "active"
	StatusArchived Status = "archived"
)

type VersionSource string

const (
	SourceUpload        VersionSource = "upload"
	SourceImport        VersionSource = "import"
	SourceCollaboration VersionSource = "collaboration"
	SourceSignature     VersionSource = "signature"
)

type Classification string

const (
	ClassificationInternal     Classification = "internal"
	ClassificationConfidential Classification = "confidential"
	ClassificationRestricted   Classification = "restricted"
)

type Document struct {
	ID           DocumentID
	TenantID     tenancy.TenantID
	Title        string
	Status       Status
	SourceModule string
	CreatedBy    identity.AccountID
	CreatedAt    time.Time
	UpdatedAt    time.Time
	Version      uint64
}

type Version struct {
	ID            VersionID
	TenantID      tenancy.TenantID
	DocumentID    DocumentID
	VersionNumber uint64
	BlobID        storage.BlobID
	Source        VersionSource
	CreatedBy     identity.AccountID
	PublishedAt   time.Time
}

type ClassificationRevision struct {
	ID             ClassificationRevisionID
	TenantID       tenancy.TenantID
	DocumentID     DocumentID
	Revision       uint64
	Classification Classification
	RetentionClass string
	RetentionUntil *time.Time
	LegalHold      bool
	RecordedBy     identity.AccountID
	RecordedAt     time.Time
}

// PublishNew creates the first visible aggregate only from an already sealed,
// tenant-matching blob. Pending uploads are storage state, not draft versions.
func PublishNew(tenantID tenancy.TenantID, title, sourceModule string, blob storage.Blob, source VersionSource,
	classification Classification, retentionClass string, retentionUntil *time.Time, legalHold bool,
	actor identity.AccountID, publishedAt time.Time) (Document, Version, ClassificationRevision, error) {
	if blob.Validate() != nil || blob.State != storage.BlobAvailable || blob.TenantID != tenantID || actor.IsZero() || publishedAt.IsZero() {
		return Document{}, Version{}, ClassificationRevision{}, ErrInvalid
	}
	documentID, err := newIdentifier[DocumentID]()
	if err != nil {
		return Document{}, Version{}, ClassificationRevision{}, err
	}
	versionID, err := newIdentifier[VersionID]()
	if err != nil {
		return Document{}, Version{}, ClassificationRevision{}, err
	}
	classificationID, err := newIdentifier[ClassificationRevisionID]()
	if err != nil {
		return Document{}, Version{}, ClassificationRevision{}, err
	}
	now := publishedAt.UTC()
	document := Document{
		ID: documentID, TenantID: tenantID, Title: strings.TrimSpace(title), Status: StatusActive,
		SourceModule: strings.TrimSpace(sourceModule), CreatedBy: actor, CreatedAt: now, UpdatedAt: now, Version: 1,
	}
	version := Version{
		ID: versionID, TenantID: tenantID, DocumentID: documentID, VersionNumber: 1,
		BlobID: blob.ID, Source: source, CreatedBy: actor, PublishedAt: now,
	}
	revision := ClassificationRevision{
		ID: classificationID, TenantID: tenantID, DocumentID: documentID, Revision: 1,
		Classification: classification, RetentionClass: strings.TrimSpace(retentionClass),
		RetentionUntil: normalizeTimePointer(retentionUntil), LegalHold: legalHold, RecordedBy: actor, RecordedAt: now,
	}
	if document.Validate() != nil || version.Validate() != nil || revision.Validate() != nil {
		return Document{}, Version{}, ClassificationRevision{}, ErrInvalid
	}
	return document, version, revision, nil
}

func PublishNextVersion(document Document, currentVersion uint64, blob storage.Blob, source VersionSource,
	actor identity.AccountID, publishedAt time.Time) (Version, error) {
	if document.Validate() != nil || document.Status != StatusActive || currentVersion == 0 || currentVersion == ^uint64(0) ||
		blob.Validate() != nil || blob.State != storage.BlobAvailable || blob.TenantID != document.TenantID || actor.IsZero() || publishedAt.IsZero() {
		return Version{}, ErrInvalid
	}
	id, err := newIdentifier[VersionID]()
	if err != nil {
		return Version{}, err
	}
	version := Version{
		ID: id, TenantID: document.TenantID, DocumentID: document.ID,
		VersionNumber: currentVersion + 1, BlobID: blob.ID, Source: source,
		CreatedBy: actor, PublishedAt: publishedAt.UTC(),
	}
	if err := version.Validate(); err != nil {
		return Version{}, err
	}
	return version, nil
}

func RecordClassification(document Document, currentRevision uint64, classification Classification, retentionClass string,
	retentionUntil *time.Time, legalHold bool, actor identity.AccountID, recordedAt time.Time) (ClassificationRevision, error) {
	if document.Validate() != nil || currentRevision == 0 || currentRevision == ^uint64(0) || actor.IsZero() || recordedAt.IsZero() {
		return ClassificationRevision{}, ErrInvalid
	}
	id, err := newIdentifier[ClassificationRevisionID]()
	if err != nil {
		return ClassificationRevision{}, err
	}
	revision := ClassificationRevision{
		ID: id, TenantID: document.TenantID, DocumentID: document.ID, Revision: currentRevision + 1,
		Classification: classification, RetentionClass: strings.TrimSpace(retentionClass),
		RetentionUntil: normalizeTimePointer(retentionUntil), LegalHold: legalHold,
		RecordedBy: actor, RecordedAt: recordedAt.UTC(),
	}
	if err := revision.Validate(); err != nil {
		return ClassificationRevision{}, err
	}
	return revision, nil
}

func (document Document) Validate() error {
	if document.ID.IsZero() || document.TenantID.IsZero() || document.CreatedBy.IsZero() || document.CreatedAt.IsZero() ||
		document.UpdatedAt.IsZero() || document.UpdatedAt.Before(document.CreatedAt) || document.Version == 0 ||
		!resource.ValidKey(document.SourceModule) {
		return ErrInvalid
	}
	if strings.TrimSpace(document.Title) == "" || utf8.RuneCountInString(document.Title) > maximumTitleLength {
		return ErrInvalid
	}
	if document.Status != StatusActive && document.Status != StatusArchived {
		return ErrInvalid
	}
	return nil
}

func (version Version) Validate() error {
	if version.ID.IsZero() || version.TenantID.IsZero() || version.DocumentID.IsZero() || version.BlobID.IsZero() ||
		version.VersionNumber == 0 || version.CreatedBy.IsZero() || version.PublishedAt.IsZero() || !validSource(version.Source) {
		return ErrInvalid
	}
	return nil
}

func (revision ClassificationRevision) Validate() error {
	if revision.ID.IsZero() || revision.TenantID.IsZero() || revision.DocumentID.IsZero() || revision.Revision == 0 ||
		revision.RecordedBy.IsZero() || revision.RecordedAt.IsZero() || !validClassification(revision.Classification) ||
		!resource.ValidKey(revision.RetentionClass) {
		return ErrInvalid
	}
	return nil
}

func validSource(source VersionSource) bool {
	switch source {
	case SourceUpload, SourceImport, SourceCollaboration, SourceSignature:
		return true
	default:
		return false
	}
}

func validClassification(classification Classification) bool {
	switch classification {
	case ClassificationInternal, ClassificationConfidential, ClassificationRestricted:
		return true
	default:
		return false
	}
}

func normalizeTimePointer(value *time.Time) *time.Time {
	if value == nil {
		return nil
	}
	normalized := value.UTC()
	return &normalized
}

func newIdentifier[T ~[16]byte]() (T, error) {
	var identifier T
	milliseconds := uint64(time.Now().UnixMilli())
	identifier[0] = byte(milliseconds >> 40)
	identifier[1] = byte(milliseconds >> 32)
	identifier[2] = byte(milliseconds >> 24)
	identifier[3] = byte(milliseconds >> 16)
	identifier[4] = byte(milliseconds >> 8)
	identifier[5] = byte(milliseconds)
	if _, err := rand.Read(identifier[6:]); err != nil {
		return T{}, fmt.Errorf("generate document UUIDv7 randomness: %w", err)
	}
	identifier[6] = (identifier[6] & 0x0f) | 0x70
	identifier[8] = (identifier[8] & 0x3f) | 0x80
	return identifier, nil
}
