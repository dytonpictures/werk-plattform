// Package storage defines provider-independent contracts for tenant-bound
// blobs and their opaque physical locations. It contains no S3, HTTP, ticket,
// or database implementation.
package storage

import (
	"crypto/rand"
	"errors"
	"fmt"
	"math"
	"mime"
	"strings"
	"time"

	"github.com/dytonpictures/werk/internal/core/identity"
	"github.com/dytonpictures/werk/internal/core/resource"
	"github.com/dytonpictures/werk/internal/core/tenancy"
)

var ErrInvalid = errors.New("invalid storage model")

const maximumProviderChecksumLength = 256

type BlobID [16]byte

func (id BlobID) IsZero() bool { return id == BlobID{} }

type LocationID [16]byte

func (id LocationID) IsZero() bool { return id == LocationID{} }

// OpaqueKey is a random technical locator. It cannot contain a file name,
// document title, login name, or a client-selected object path.
type OpaqueKey [16]byte

func (key OpaqueKey) IsZero() bool { return key == OpaqueKey{} }

type Digest [32]byte

func (digest Digest) IsZero() bool { return digest == Digest{} }

type BlobState string

const (
	BlobQuarantined BlobState = "quarantined"
	BlobAvailable   BlobState = "available"
	BlobRejected    BlobState = "rejected"
	BlobMissing     BlobState = "missing"
	BlobUnknown     BlobState = "unknown"
)

type ContentDescriptor struct {
	SizeBytes uint64
	SHA256    Digest
	MediaType string
}

func (descriptor ContentDescriptor) Validate() error {
	if descriptor.SizeBytes > math.MaxInt64 || descriptor.SHA256.IsZero() {
		return ErrInvalid
	}
	mediaType, _, err := mime.ParseMediaType(strings.TrimSpace(descriptor.MediaType))
	if err != nil || mediaType == "" || len(mediaType) > 255 {
		return ErrInvalid
	}
	return nil
}

func (blob Blob) Reject() (Blob, error) {
	if blob.Validate() != nil || blob.State != BlobQuarantined {
		return Blob{}, ErrInvalid
	}
	rejected := blob
	rejected.State = BlobRejected
	rejected.Version++
	if err := rejected.Validate(); err != nil {
		return Blob{}, err
	}
	return rejected, nil
}

type Blob struct {
	ID         BlobID
	TenantID   tenancy.TenantID
	State      BlobState
	Content    ContentDescriptor
	CreatedBy  identity.AccountID
	CreatedAt  time.Time
	VerifiedAt *time.Time
	Version    uint64
}

func NewQuarantinedBlob(tenantID tenancy.TenantID, createdBy identity.AccountID, createdAt time.Time) (Blob, error) {
	id, err := newIdentifier[BlobID]()
	if err != nil {
		return Blob{}, err
	}
	blob := Blob{
		ID: id, TenantID: tenantID, State: BlobQuarantined,
		CreatedBy: createdBy, CreatedAt: createdAt.UTC(), Version: 1,
	}
	if err := blob.Validate(); err != nil {
		return Blob{}, err
	}
	return blob, nil
}

// Seal returns a new immutable-content snapshot. Only server-verified content
// may be supplied here; a client hash is not a canonical integrity result.
func (blob Blob) Seal(content ContentDescriptor, verifiedAt time.Time) (Blob, error) {
	if blob.Validate() != nil || blob.State != BlobQuarantined || content.Validate() != nil || verifiedAt.IsZero() || verifiedAt.Before(blob.CreatedAt) {
		return Blob{}, ErrInvalid
	}
	mediaType, _, _ := mime.ParseMediaType(strings.TrimSpace(content.MediaType))
	sealed := blob
	sealed.State = BlobAvailable
	sealed.Content = content
	sealed.Content.MediaType = mediaType
	verified := verifiedAt.UTC()
	sealed.VerifiedAt = &verified
	sealed.Version++
	if err := sealed.Validate(); err != nil {
		return Blob{}, err
	}
	return sealed, nil
}

// MarkUnknown records an indeterminate provider check. The verified content
// descriptor remains authoritative, but reads must fail closed until a later
// check restores availability or confirms that the object is missing.
func (blob Blob) MarkUnknown() (Blob, error) {
	if blob.Validate() != nil || blob.State != BlobAvailable {
		return Blob{}, ErrInvalid
	}
	unknown := blob
	unknown.State = BlobUnknown
	unknown.Version++
	if err := unknown.Validate(); err != nil {
		return Blob{}, err
	}
	return unknown, nil
}

// MarkMissing records a confirmed absence of every usable physical location.
// Content metadata is retained for audit, restore and reconciliation.
func (blob Blob) MarkMissing() (Blob, error) {
	if blob.Validate() != nil || (blob.State != BlobAvailable && blob.State != BlobUnknown) {
		return Blob{}, ErrInvalid
	}
	missing := blob
	missing.State = BlobMissing
	missing.Version++
	if err := missing.Validate(); err != nil {
		return Blob{}, err
	}
	return missing, nil
}

// RestoreAvailability is valid only after the storage layer has verified an
// existing or repaired location. It does not replace the sealed descriptor.
func (blob Blob) RestoreAvailability() (Blob, error) {
	if blob.Validate() != nil || (blob.State != BlobUnknown && blob.State != BlobMissing) {
		return Blob{}, ErrInvalid
	}
	available := blob
	available.State = BlobAvailable
	available.Version++
	if err := available.Validate(); err != nil {
		return Blob{}, err
	}
	return available, nil
}

func (blob Blob) Validate() error {
	if blob.ID.IsZero() || blob.TenantID.IsZero() || blob.CreatedBy.IsZero() || blob.CreatedAt.IsZero() || blob.Version == 0 {
		return ErrInvalid
	}
	switch blob.State {
	case BlobQuarantined:
		if blob.Content != (ContentDescriptor{}) || blob.VerifiedAt != nil || blob.Version != 1 {
			return ErrInvalid
		}
	case BlobAvailable:
		if blob.Content.Validate() != nil || blob.VerifiedAt == nil || blob.VerifiedAt.Before(blob.CreatedAt) || blob.Version < 2 {
			return ErrInvalid
		}
	case BlobMissing, BlobUnknown:
		if blob.Content.Validate() != nil || blob.VerifiedAt == nil || blob.VerifiedAt.Before(blob.CreatedAt) || blob.Version < 3 {
			return ErrInvalid
		}
	case BlobRejected:
		if blob.VerifiedAt != nil || blob.Content != (ContentDescriptor{}) || blob.Version < 2 {
			return ErrInvalid
		}
	default:
		return ErrInvalid
	}
	return nil
}

func (location BlobLocation) MarkMissing() (BlobLocation, error) {
	if location.Validate() != nil || location.State != LocationAvailable {
		return BlobLocation{}, ErrInvalid
	}
	missing := location
	missing.State = LocationMissing
	missing.Version++
	if err := missing.Validate(); err != nil {
		return BlobLocation{}, err
	}
	return missing, nil
}

type LocationState string

const (
	LocationQuarantined LocationState = "quarantined"
	LocationAvailable   LocationState = "available"
	LocationMissing     LocationState = "missing"
)

type BlobLocation struct {
	ID               LocationID
	TenantID         tenancy.TenantID
	BlobID           BlobID
	ProviderKey      string
	OpaqueKey        OpaqueKey
	State            LocationState
	ProviderChecksum string
	CreatedAt        time.Time
	ActivatedAt      *time.Time
	Version          uint64
}

func NewQuarantinedLocation(blob Blob, providerKey string, createdAt time.Time) (BlobLocation, error) {
	if blob.Validate() != nil || blob.State != BlobQuarantined || !resource.ValidKey(strings.TrimSpace(providerKey)) || createdAt.IsZero() {
		return BlobLocation{}, ErrInvalid
	}
	id, err := newIdentifier[LocationID]()
	if err != nil {
		return BlobLocation{}, err
	}
	opaqueKey, err := newIdentifier[OpaqueKey]()
	if err != nil {
		return BlobLocation{}, err
	}
	location := BlobLocation{
		ID: id, TenantID: blob.TenantID, BlobID: blob.ID,
		ProviderKey: strings.TrimSpace(providerKey), OpaqueKey: opaqueKey,
		State: LocationQuarantined, CreatedAt: createdAt.UTC(), Version: 1,
	}
	if err := location.Validate(); err != nil {
		return BlobLocation{}, err
	}
	return location, nil
}

func (location BlobLocation) Activate(providerChecksum string, activatedAt time.Time) (BlobLocation, error) {
	if location.Validate() != nil || location.State != LocationQuarantined || activatedAt.IsZero() || activatedAt.Before(location.CreatedAt) || len(providerChecksum) > maximumProviderChecksumLength {
		return BlobLocation{}, ErrInvalid
	}
	active := location
	active.State = LocationAvailable
	active.ProviderChecksum = strings.TrimSpace(providerChecksum)
	activated := activatedAt.UTC()
	active.ActivatedAt = &activated
	active.Version++
	if err := active.Validate(); err != nil {
		return BlobLocation{}, err
	}
	return active, nil
}

func (location BlobLocation) Validate() error {
	if location.ID.IsZero() || location.TenantID.IsZero() || location.BlobID.IsZero() || location.OpaqueKey.IsZero() ||
		!resource.ValidKey(location.ProviderKey) || location.CreatedAt.IsZero() || location.Version == 0 ||
		len(location.ProviderChecksum) > maximumProviderChecksumLength {
		return ErrInvalid
	}
	switch location.State {
	case LocationQuarantined:
		if location.ActivatedAt != nil || location.ProviderChecksum != "" || location.Version != 1 {
			return ErrInvalid
		}
	case LocationAvailable:
		if location.ActivatedAt == nil || location.ActivatedAt.Before(location.CreatedAt) || location.Version < 2 {
			return ErrInvalid
		}
	case LocationMissing:
		if location.ActivatedAt == nil || location.ActivatedAt.Before(location.CreatedAt) || location.Version < 3 {
			return ErrInvalid
		}
	default:
		return ErrInvalid
	}
	return nil
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
		return T{}, fmt.Errorf("generate storage UUIDv7 randomness: %w", err)
	}
	identifier[6] = (identifier[6] & 0x0f) | 0x70
	identifier[8] = (identifier[8] & 0x3f) | 0x80
	return identifier, nil
}
