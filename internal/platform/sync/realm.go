package platformsync

import (
	"errors"
	"strings"
	"unicode"
)

var (
	ErrInvalidRealm    = errors.New("invalid platform realm")
	ErrInvalidInstance = errors.New("invalid platform instance")
)

// Realm identifies one logical installation and platform authority boundary.
// It is operational metadata, not a credential and not a second data store.
type Realm struct {
	ID string
}

type DeploymentProfile string

const (
	ProfileSingle    DeploymentProfile = "single"
	ProfileDualCloud DeploymentProfile = "dual-cloud"
	ProfileHybrid    DeploymentProfile = "hybrid"
)

// Instance identifies one runtime node inside a Realm. Authority generation,
// policy revision, leases, and fencing deliberately remain outside this stable
// descriptor because they are changing security state.
type Instance struct {
	ID                    string
	Realm                 Realm
	DeploymentProfile     DeploymentProfile
	AuthorityCoordination AuthorityCoordination
	BuildVersion          string
}

func NewRealm(id string) (Realm, error) {
	realm := Realm{ID: id}
	if !realm.Valid() {
		return Realm{}, ErrInvalidRealm
	}
	return realm, nil
}

func (realm Realm) Valid() bool {
	return stableAuthorityIDPattern.MatchString(realm.ID)
}

func NewInstance(
	instanceID string,
	realmID string,
	profile DeploymentProfile,
	coordination AuthorityCoordination,
	buildVersion string,
) (Instance, error) {
	realm, err := NewRealm(realmID)
	if err != nil {
		return Instance{}, err
	}
	instance := Instance{
		ID:                    instanceID,
		Realm:                 realm,
		DeploymentProfile:     profile,
		AuthorityCoordination: coordination,
		BuildVersion:          buildVersion,
	}
	if !instance.Valid() {
		return Instance{}, ErrInvalidInstance
	}
	return instance, nil
}

func (instance Instance) Valid() bool {
	return stableAuthorityIDPattern.MatchString(instance.ID) &&
		instance.Realm.Valid() &&
		validDeploymentProfile(instance.DeploymentProfile) &&
		validAuthorityCoordination(instance.AuthorityCoordination) &&
		validBuildVersion(instance.BuildVersion)
}

func validDeploymentProfile(profile DeploymentProfile) bool {
	switch profile {
	case ProfileSingle, ProfileDualCloud, ProfileHybrid:
		return true
	default:
		return false
	}
}

func validAuthorityCoordination(coordination AuthorityCoordination) bool {
	switch coordination {
	case CoordinationLocal, CoordinationSharedDatabase, CoordinationPlatformWitness:
		return true
	default:
		return false
	}
}

func validBuildVersion(version string) bool {
	if version == "" || len(version) > 128 || strings.TrimSpace(version) != version {
		return false
	}
	for _, character := range version {
		if unicode.IsControl(character) {
			return false
		}
	}
	return true
}
