// Package party models tenant-owned people and organizations shared by
// applications without owning their application-specific roles.
package party

import (
	"crypto/rand"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/dytonpictures/werk/internal/core/tenancy"
)

const (
	maximumDisplayNameLength = 200
	maximumNamePartLength    = 120
	maximumMembershipLength  = 64
)

type ID [16]byte

func (id ID) IsZero() bool {
	return id == ID{}
}

type Type string

const (
	TypePerson       Type = "person"
	TypeOrganization Type = "organization"
)

type Status string

const (
	StatusActive   Status = "active"
	StatusArchived Status = "archived"
)

type Party struct {
	ID          ID
	TenantID    tenancy.TenantID
	Type        Type
	DisplayName string
	Status      Status
	CreatedAt   time.Time
	UpdatedAt   time.Time
	Version     uint64
}

type Person struct {
	PartyID    ID
	GivenName  string
	FamilyName string
}

type Organization struct {
	PartyID   ID
	LegalName string
}

type Membership struct {
	ID               ID
	TenantID         tenancy.TenantID
	PartyID          ID
	OrganizationalID tenancy.UnitID
	MembershipType   string
	ValidFrom        time.Time
	ValidUntil       *time.Time
}

func NewPerson(tenantID tenancy.TenantID, givenName, familyName string) (Party, Person, error) {
	trimmedGivenName := strings.TrimSpace(givenName)
	trimmedFamilyName := strings.TrimSpace(familyName)
	party, err := newParty(tenantID, TypePerson, strings.Join([]string{trimmedGivenName, trimmedFamilyName}, " "))
	if err != nil {
		return Party{}, Person{}, err
	}
	person := Person{PartyID: party.ID, GivenName: trimmedGivenName, FamilyName: trimmedFamilyName}
	if err := person.Validate(); err != nil {
		return Party{}, Person{}, err
	}
	return party, person, nil
}

func NewOrganization(tenantID tenancy.TenantID, legalName string) (Party, Organization, error) {
	organization := Organization{LegalName: strings.TrimSpace(legalName)}
	if err := organization.Validate(); err != nil {
		return Party{}, Organization{}, err
	}
	party, err := newParty(tenantID, TypeOrganization, organization.LegalName)
	if err != nil {
		return Party{}, Organization{}, err
	}
	organization.PartyID = party.ID
	return party, organization, nil
}

func NewMembership(tenantID tenancy.TenantID, partyID ID, organizationalID tenancy.UnitID, membershipType string, validFrom time.Time, validUntil *time.Time) (Membership, error) {
	id, err := newID()
	if err != nil {
		return Membership{}, err
	}
	membership := Membership{
		ID:               id,
		TenantID:         tenantID,
		PartyID:          partyID,
		OrganizationalID: organizationalID,
		MembershipType:   strings.TrimSpace(membershipType),
		ValidFrom:        validFrom.UTC(),
		ValidUntil:       validUntil,
	}
	if membership.ValidUntil != nil {
		until := membership.ValidUntil.UTC()
		membership.ValidUntil = &until
	}
	if err := membership.Validate(); err != nil {
		return Membership{}, err
	}
	return membership, nil
}

func (party Party) Validate() error {
	if party.ID.IsZero() {
		return errors.New("party ID is required")
	}
	if party.TenantID.IsZero() {
		return errors.New("party tenant ID is required")
	}
	if party.Type != TypePerson && party.Type != TypeOrganization {
		return fmt.Errorf("invalid party type %q", party.Type)
	}
	if err := validateText(party.DisplayName, maximumDisplayNameLength, "party display name"); err != nil {
		return err
	}
	if party.Status != StatusActive && party.Status != StatusArchived {
		return fmt.Errorf("invalid party status %q", party.Status)
	}
	if party.Version == 0 {
		return errors.New("party version must be greater than zero")
	}
	return nil
}

func (person Person) Validate() error {
	if person.PartyID.IsZero() {
		return errors.New("person party ID is required")
	}
	if err := validateText(person.GivenName, maximumNamePartLength, "person given name"); err != nil {
		return err
	}
	return validateText(person.FamilyName, maximumNamePartLength, "person family name")
}

func (organization Organization) Validate() error {
	if organization.PartyID.IsZero() {
		return errors.New("organization party ID is required")
	}
	return validateText(organization.LegalName, maximumDisplayNameLength, "organization legal name")
}

func (membership Membership) Validate() error {
	if membership.ID.IsZero() {
		return errors.New("membership ID is required")
	}
	if membership.TenantID.IsZero() {
		return errors.New("membership tenant ID is required")
	}
	if membership.PartyID.IsZero() {
		return errors.New("membership party ID is required")
	}
	if membership.OrganizationalID.IsZero() {
		return errors.New("membership organizational unit ID is required")
	}
	if err := validateKey(membership.MembershipType); err != nil {
		return fmt.Errorf("membership type: %w", err)
	}
	if membership.ValidFrom.IsZero() {
		return errors.New("membership valid-from time is required")
	}
	if membership.ValidUntil != nil && membership.ValidUntil.Before(membership.ValidFrom) {
		return errors.New("membership valid-until must not precede valid-from")
	}
	return nil
}

func newParty(tenantID tenancy.TenantID, partyType Type, displayName string) (Party, error) {
	id, err := newID()
	if err != nil {
		return Party{}, err
	}
	now := time.Now().UTC()
	party := Party{
		ID:          id,
		TenantID:    tenantID,
		Type:        partyType,
		DisplayName: displayName,
		Status:      StatusActive,
		CreatedAt:   now,
		UpdatedAt:   now,
		Version:     1,
	}
	if err := party.Validate(); err != nil {
		return Party{}, err
	}
	return party, nil
}

func validateText(value string, maximum int, field string) error {
	if strings.TrimSpace(value) == "" {
		return fmt.Errorf("%s is required", field)
	}
	if utf8.RuneCountInString(value) > maximum {
		return fmt.Errorf("%s exceeds %d characters", field, maximum)
	}
	return nil
}

func validateKey(value string) error {
	if value == "" {
		return errors.New("key is required")
	}
	if len(value) > maximumMembershipLength {
		return fmt.Errorf("key exceeds %d bytes", maximumMembershipLength)
	}
	for index, character := range value {
		if character >= 'a' && character <= 'z' {
			continue
		}
		if index > 0 && ((character >= '0' && character <= '9') || character == '.' || character == '_' || character == '-') {
			continue
		}
		return errors.New("key must be a lowercase stable key")
	}
	return nil
}

func newID() (ID, error) {
	var id ID
	milliseconds := uint64(time.Now().UnixMilli())
	id[0] = byte(milliseconds >> 40)
	id[1] = byte(milliseconds >> 32)
	id[2] = byte(milliseconds >> 24)
	id[3] = byte(milliseconds >> 16)
	id[4] = byte(milliseconds >> 8)
	id[5] = byte(milliseconds)
	if _, err := rand.Read(id[6:]); err != nil {
		return ID{}, fmt.Errorf("generate party UUIDv7 randomness: %w", err)
	}
	id[6] = (id[6] & 0x0f) | 0x70
	id[8] = (id[8] & 0x3f) | 0x80
	return id, nil
}
