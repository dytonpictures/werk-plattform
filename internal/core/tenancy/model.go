package tenancy

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"golang.org/x/text/language"
)

const (
	maximumNameLength     = 200
	maximumUnitTypeLength = 64
)

type TenantID [16]byte

type UnitID [16]byte

type TenantStatus string

const (
	TenantStatusActive    TenantStatus = "active"
	TenantStatusSuspended TenantStatus = "suspended"
	TenantStatusArchived  TenantStatus = "archived"
)

type UnitStatus string

const (
	UnitStatusActive   UnitStatus = "active"
	UnitStatusArchived UnitStatus = "archived"
)

type UnitType string

type Tenant struct {
	ID              TenantID
	Name            string
	Status          TenantStatus
	DefaultLocale   string
	DefaultTimezone string
	CreatedAt       time.Time
	UpdatedAt       time.Time
	Version         uint64
}

type OrganizationalUnit struct {
	ID        UnitID
	TenantID  TenantID
	ParentID  *UnitID
	Type      UnitType
	Name      string
	Status    UnitStatus
	CreatedAt time.Time
	UpdatedAt time.Time
	Version   uint64
}

func NewTenant(name, locale, timezone string) (Tenant, error) {
	id, err := NewTenantID()
	if err != nil {
		return Tenant{}, err
	}
	now := time.Now().UTC()
	tenant := Tenant{
		ID:              id,
		Name:            strings.TrimSpace(name),
		Status:          TenantStatusActive,
		DefaultLocale:   strings.TrimSpace(locale),
		DefaultTimezone: strings.TrimSpace(timezone),
		CreatedAt:       now,
		UpdatedAt:       now,
		Version:         1,
	}
	if err := tenant.Validate(); err != nil {
		return Tenant{}, err
	}
	return tenant, nil
}

func (tenant Tenant) Validate() error {
	if tenant.ID.IsZero() {
		return errors.New("tenant ID is required")
	}
	if err := validateName(tenant.Name); err != nil {
		return fmt.Errorf("tenant name: %w", err)
	}
	if !tenant.Status.Valid() {
		return fmt.Errorf("invalid tenant status %q", tenant.Status)
	}
	if tenant.DefaultLocale == "" || strings.ContainsAny(tenant.DefaultLocale, "_ \t\r\n") {
		return fmt.Errorf("invalid BCP-47 locale %q", tenant.DefaultLocale)
	}
	parsedLocale, err := language.Parse(tenant.DefaultLocale)
	if err != nil {
		return fmt.Errorf("invalid BCP-47 locale %q: %w", tenant.DefaultLocale, err)
	}
	if parsedLocale == language.Und && !strings.EqualFold(tenant.DefaultLocale, "und") {
		return fmt.Errorf("invalid BCP-47 locale %q", tenant.DefaultLocale)
	}
	if tenant.DefaultTimezone == "Local" {
		return errors.New("tenant timezone must be an explicit IANA timezone")
	}
	if _, err := time.LoadLocation(tenant.DefaultTimezone); err != nil {
		return fmt.Errorf("invalid IANA timezone %q: %w", tenant.DefaultTimezone, err)
	}
	if tenant.Version == 0 {
		return errors.New("tenant version must be greater than zero")
	}
	return nil
}

func NewOrganizationalUnit(tenantID TenantID, parentID *UnitID, unitType UnitType, name string) (OrganizationalUnit, error) {
	id, err := NewUnitID()
	if err != nil {
		return OrganizationalUnit{}, err
	}
	now := time.Now().UTC()
	unit := OrganizationalUnit{
		ID:        id,
		TenantID:  tenantID,
		ParentID:  parentID,
		Type:      UnitType(strings.TrimSpace(string(unitType))),
		Name:      strings.TrimSpace(name),
		Status:    UnitStatusActive,
		CreatedAt: now,
		UpdatedAt: now,
		Version:   1,
	}
	if err := unit.Validate(); err != nil {
		return OrganizationalUnit{}, err
	}
	return unit, nil
}

func (unit OrganizationalUnit) Validate() error {
	if unit.ID.IsZero() {
		return errors.New("organizational unit ID is required")
	}
	if unit.TenantID.IsZero() {
		return errors.New("organizational unit tenant ID is required")
	}
	if unit.ParentID != nil && *unit.ParentID == unit.ID {
		return errors.New("organizational unit cannot be its own parent")
	}
	if err := unit.Type.Validate(); err != nil {
		return err
	}
	if err := validateName(unit.Name); err != nil {
		return fmt.Errorf("organizational unit name: %w", err)
	}
	if !unit.Status.Valid() {
		return fmt.Errorf("invalid organizational unit status %q", unit.Status)
	}
	if unit.Version == 0 {
		return errors.New("organizational unit version must be greater than zero")
	}
	return nil
}

func (status TenantStatus) Valid() bool {
	return status == TenantStatusActive || status == TenantStatusSuspended || status == TenantStatusArchived
}

func (status UnitStatus) Valid() bool {
	return status == UnitStatusActive || status == UnitStatusArchived
}

func (unitType UnitType) Validate() error {
	value := string(unitType)
	if value == "" {
		return errors.New("organizational unit type is required")
	}
	if len(value) > maximumUnitTypeLength {
		return fmt.Errorf("organizational unit type exceeds %d bytes", maximumUnitTypeLength)
	}
	for index, character := range value {
		if character >= 'a' && character <= 'z' {
			continue
		}
		if index > 0 && ((character >= '0' && character <= '9') || character == '.' || character == '_' || character == '-') {
			continue
		}
		return errors.New("organizational unit type must be a lowercase stable key")
	}
	last := value[len(value)-1]
	if last == '.' || last == '_' || last == '-' {
		return errors.New("organizational unit type must end with a letter or digit")
	}
	return nil
}

func NewTenantID() (TenantID, error) {
	value, err := newUUIDv7()
	return TenantID(value), err
}

func NewUnitID() (UnitID, error) {
	value, err := newUUIDv7()
	return UnitID(value), err
}

func ParseTenantID(value string) (TenantID, error) {
	parsed, err := parseUUID(value)
	return TenantID(parsed), err
}

func ParseUnitID(value string) (UnitID, error) {
	parsed, err := parseUUID(value)
	return UnitID(parsed), err
}

func (id TenantID) String() string {
	return formatUUID([16]byte(id))
}

func (id TenantID) IsZero() bool {
	return id == TenantID{}
}

func (id UnitID) String() string {
	return formatUUID([16]byte(id))
}

func (id UnitID) IsZero() bool {
	return id == UnitID{}
}

func validateName(value string) error {
	if strings.TrimSpace(value) == "" {
		return errors.New("name is required")
	}
	if utf8.RuneCountInString(value) > maximumNameLength {
		return fmt.Errorf("name exceeds %d characters", maximumNameLength)
	}
	return nil
}

func newUUIDv7() ([16]byte, error) {
	var value [16]byte
	milliseconds := uint64(time.Now().UnixMilli())
	value[0] = byte(milliseconds >> 40)
	value[1] = byte(milliseconds >> 32)
	value[2] = byte(milliseconds >> 24)
	value[3] = byte(milliseconds >> 16)
	value[4] = byte(milliseconds >> 8)
	value[5] = byte(milliseconds)
	if _, err := rand.Read(value[6:]); err != nil {
		return [16]byte{}, fmt.Errorf("generate UUIDv7 randomness: %w", err)
	}
	value[6] = (value[6] & 0x0f) | 0x70
	value[8] = (value[8] & 0x3f) | 0x80
	return value, nil
}

func parseUUID(value string) ([16]byte, error) {
	if len(value) != 36 || value[8] != '-' || value[13] != '-' || value[18] != '-' || value[23] != '-' {
		return [16]byte{}, errors.New("UUID has an invalid shape")
	}
	compact := strings.ReplaceAll(value, "-", "")
	var parsed [16]byte
	if _, err := hex.Decode(parsed[:], []byte(compact)); err != nil {
		return [16]byte{}, fmt.Errorf("UUID contains invalid hexadecimal data: %w", err)
	}
	version := parsed[6] >> 4
	if version < 1 || version > 8 || parsed[8]&0xc0 != 0x80 {
		return [16]byte{}, errors.New("UUID has an unsupported version or variant")
	}
	if parsed == [16]byte{} {
		return [16]byte{}, errors.New("UUID must not be zero")
	}
	return parsed, nil
}

func formatUUID(value [16]byte) string {
	return fmt.Sprintf("%x-%x-%x-%x-%x", value[0:4], value[4:6], value[6:8], value[8:10], value[10:16])
}
