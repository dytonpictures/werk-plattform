package platformsync

import (
	"errors"
	"testing"
)

func TestNewInstanceValidatesStableRuntimeIdentity(t *testing.T) {
	instance, err := NewInstance("instance.primary", "realm.main", ProfileSingle, CoordinationSharedDatabase, "2026.7.22")
	if err != nil {
		t.Fatal(err)
	}
	if !instance.Valid() || instance.Realm.ID != "realm.main" {
		t.Fatalf("instance = %#v", instance)
	}
}

func TestNewInstanceRejectsInvalidConfiguration(t *testing.T) {
	tests := []struct {
		name         string
		instanceID   string
		realmID      string
		profile      DeploymentProfile
		coordination AuthorityCoordination
		buildVersion string
		wantError    error
	}{
		{name: "instance", instanceID: "INVALID", realmID: "realm.main", profile: ProfileSingle, coordination: CoordinationLocal, buildVersion: "dev", wantError: ErrInvalidInstance},
		{name: "realm", instanceID: "instance.primary", realmID: "INVALID", profile: ProfileSingle, coordination: CoordinationLocal, buildVersion: "dev", wantError: ErrInvalidRealm},
		{name: "profile", instanceID: "instance.primary", realmID: "realm.main", profile: "cluster", coordination: CoordinationLocal, buildVersion: "dev", wantError: ErrInvalidInstance},
		{name: "coordination", instanceID: "instance.primary", realmID: "realm.main", profile: ProfileSingle, coordination: "active-active", buildVersion: "dev", wantError: ErrInvalidInstance},
		{name: "version", instanceID: "instance.primary", realmID: "realm.main", profile: ProfileSingle, coordination: CoordinationLocal, buildVersion: " dev ", wantError: ErrInvalidInstance},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := NewInstance(test.instanceID, test.realmID, test.profile, test.coordination, test.buildVersion)
			if !errors.Is(err, test.wantError) {
				t.Fatalf("error = %v, want %v", err, test.wantError)
			}
		})
	}
}
