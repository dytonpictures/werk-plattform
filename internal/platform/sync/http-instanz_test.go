package platformsync

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestHTTPInstanceHealthReportsOnlyLivenessMetadata(t *testing.T) {
	at := time.Date(2026, 7, 22, 17, 30, 0, 0, time.UTC)
	instance, err := NewInstance("instance.primary", "realm.secret", ProfileDualCloud, CoordinationPlatformWitness, "2026.7.22")
	if err != nil {
		t.Fatal(err)
	}
	handler, err := newHTTPInstanceHealthHandler(instance, func() time.Time { return at })
	if err != nil {
		t.Fatal(err)
	}
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/health/instance", nil))
	if response.Code != http.StatusOK || response.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("status = %d, headers = %#v", response.Code, response.Header())
	}
	var view instanceHealthResponse
	if err := json.Unmarshal(response.Body.Bytes(), &view); err != nil {
		t.Fatal(err)
	}
	if view.SchemaVersion != instanceHealthSchemaV1 || view.Status != "live" ||
		view.InstanceID != instance.ID || view.Profile != instance.DeploymentProfile ||
		view.BuildVersion != instance.BuildVersion || !view.ObservedAt.Equal(at) {
		t.Fatalf("view = %#v", view)
	}
	for _, forbidden := range []string{"realm", "generation", "policy_revision", "lease", "fencing", "coordination", "authority_domain"} {
		if strings.Contains(response.Body.String(), forbidden) {
			t.Fatalf("health response contains forbidden authority state %q: %s", forbidden, response.Body.String())
		}
	}
}

func TestHTTPInstanceHealthSupportsHeadAndRejectsMutationMethods(t *testing.T) {
	instance, err := NewInstance("instance.primary", "realm.main", ProfileSingle, CoordinationLocal, "dev")
	if err != nil {
		t.Fatal(err)
	}
	handler, err := NewHTTPInstanceHealthHandler(instance)
	if err != nil {
		t.Fatal(err)
	}

	head := httptest.NewRecorder()
	handler.ServeHTTP(head, httptest.NewRequest(http.MethodHead, "/health/instance", nil))
	if head.Code != http.StatusOK || head.Body.Len() != 0 {
		t.Fatalf("HEAD status = %d, body = %q", head.Code, head.Body.String())
	}

	post := httptest.NewRecorder()
	handler.ServeHTTP(post, httptest.NewRequest(http.MethodPost, "/health/instance", nil))
	if post.Code != http.StatusMethodNotAllowed || post.Header().Get("Allow") != "GET, HEAD" {
		t.Fatalf("POST status = %d, allow = %q", post.Code, post.Header().Get("Allow"))
	}
}

func TestHTTPInstanceHealthRejectsInvalidInstance(t *testing.T) {
	if _, err := NewHTTPInstanceHealthHandler(Instance{}); err != ErrInvalidHealthHandler {
		t.Fatalf("error = %v", err)
	}
}
