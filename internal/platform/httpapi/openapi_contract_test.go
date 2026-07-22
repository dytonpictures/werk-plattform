package httpapi

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestOpenAPIContractDocumentsOperationalRoutesAndBoundaries(t *testing.T) {
	_, filename, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("locate contract test")
	}
	contractPath := filepath.Join(filepath.Dir(filename), "..", "..", "..", "api", "openapi.json")
	contents, err := os.ReadFile(contractPath)
	if err != nil {
		t.Fatalf("read OpenAPI contract: %v", err)
	}

	var contract map[string]any
	if err := json.Unmarshal(contents, &contract); err != nil {
		t.Fatalf("OpenAPI contract is not valid JSON: %v", err)
	}
	if contract["openapi"] != "3.1.0" {
		t.Fatalf("OpenAPI version = %v, want 3.1.0", contract["openapi"])
	}

	paths, ok := contract["paths"].(map[string]any)
	if !ok {
		t.Fatal("OpenAPI contract has no paths object")
	}
	for _, path := range []string{"/health/live", "/health/ready", "/meta", "/metrics"} {
		operations, ok := paths[path].(map[string]any)
		if !ok || operations["get"] == nil {
			t.Errorf("OpenAPI contract does not document GET %s", path)
		}
	}
	for _, path := range []string{
		"/api/v1/auth/mfa/challenge",
		"/api/v1/auth/mfa/totp/enrollment",
		"/api/v1/auth/mfa/totp/confirmation",
	} {
		operations, ok := paths[path].(map[string]any)
		if !ok || operations["post"] == nil {
			t.Errorf("OpenAPI contract does not document POST %s", path)
		}
	}
	if operations, ok := paths["/admin/v1/work-users"].(map[string]any); !ok || operations["get"] == nil || operations["post"] == nil {
		t.Error("OpenAPI contract does not document GET and POST /admin/v1/work-users")
	}
	if operations, ok := paths["/admin/v1/security-audit"].(map[string]any); !ok || operations["get"] == nil {
		t.Error("OpenAPI contract does not document GET /admin/v1/security-audit")
	}
	if operations, ok := paths["/admin/v1/work-roles"].(map[string]any); !ok || operations["get"] == nil || operations["post"] == nil {
		t.Error("OpenAPI contract does not document GET and POST /admin/v1/work-roles")
	}
	for _, path := range []string{
		"/admin/v1/tenants/{tenantId}",
		"/admin/v1/tenants/{tenantId}/organizational-units/{unitId}",
		"/admin/v1/work-roles/{roleId}",
	} {
		operations, ok := paths[path].(map[string]any)
		if !ok || operations["put"] == nil {
			t.Errorf("OpenAPI contract does not document PUT %s", path)
		}
	}
	if operations, ok := paths["/admin/v1/work-users/{accountId}/roles"].(map[string]any); !ok || operations["put"] == nil {
		t.Error("OpenAPI contract does not document PUT /admin/v1/work-users/{accountId}/roles")
	}
	if operations, ok := paths["/api/v1/workspace"].(map[string]any); !ok || operations["get"] == nil {
		t.Error("OpenAPI contract does not document GET /api/v1/workspace")
	}
	for _, path := range []string{"/api/v1/documents", "/api/v1/documents/{documentId}"} {
		operations, ok := paths[path].(map[string]any)
		if !ok || operations["get"] == nil {
			t.Errorf("OpenAPI contract does not document GET %s", path)
		}
	}
	for _, path := range []string{"/admin/v1/tenants", "/admin/v1/tenants/{tenantId}/organizational-units"} {
		operations, ok := paths[path].(map[string]any)
		if !ok || operations["get"] == nil || operations["post"] == nil {
			t.Errorf("OpenAPI contract does not document GET and POST %s", path)
		}
	}
	if paths["/api/v1/meta"] != nil {
		t.Fatal("neutral metadata must not be documented in the work API namespace")
	}

	boundaries, ok := contract["x-werk-api-boundaries"].(map[string]any)
	if !ok || boundaries["work"] != "/api/v1" || boundaries["admin"] != "/admin/v1" || boundaries["service"] != "/service/v1" {
		t.Fatal("OpenAPI contract does not define the three separate API boundaries")
	}
}
