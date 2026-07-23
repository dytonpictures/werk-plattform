package events

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/dytonpictures/werk/internal/core/tenancy"
)

func TestEventValidation(t *testing.T) {
	event := Event{
		ID: [16]byte{1}, TenantID: tenancy.TenantID{2}, Type: "core.example.created.v1",
		Producer: "core.example", SubjectKind: "example.item", SubjectID: [16]byte{3},
		PartitionKey: "item:3", OccurredAt: time.Now().UTC(), CorrelationID: [16]byte{4},
		Payload: json.RawMessage(`{"name":"WERK"}`),
	}
	if err := event.Validate(); err != nil {
		t.Fatal(err)
	}
	event.Type = "unversioned"
	if err := event.Validate(); err == nil {
		t.Fatal("unversioned event type was accepted")
	}
}

func TestEventTagsReceiveConservativeDefaults(t *testing.T) {
	tags := NormalizeTags(map[string]string{"organization.unit": "finance"})
	if tags[TagDataClassification] != "restricted" || tags[TagRetentionClass] != "domain-event" {
		t.Fatalf("unexpected default tags: %#v", tags)
	}
	if err := ValidateTags(tags); err != nil {
		t.Fatalf("valid tags rejected: %v", err)
	}
	tags["free form"] = "not-allowed"
	if err := ValidateTags(tags); err == nil {
		t.Fatal("invalid tag key was accepted")
	}
}
