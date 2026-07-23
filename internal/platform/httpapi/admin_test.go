package httpapi

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/dytonpictures/werk/internal/platform/adminstore"
)

func TestSecurityAuditCursorRoundTrip(t *testing.T) {
	want := adminstore.SecurityAuditCursor{
		OccurredAt: time.Date(2026, time.July, 21, 12, 34, 56, 123456000, time.UTC),
		ID:         "0196f000-0000-7000-8000-000000000801",
	}
	encoded, err := encodeSecurityAuditCursor(want)
	if err != nil {
		t.Fatal(err)
	}
	got, err := decodeSecurityAuditCursor(encoded)
	if err != nil {
		t.Fatal(err)
	}
	if got.ID != want.ID || !got.OccurredAt.Equal(want.OccurredAt) {
		t.Fatalf("cursor = %#v, want %#v", got, want)
	}
}

func TestSecurityAuditQueryRejectsMalformedLimitAndCursor(t *testing.T) {
	for _, target := range []string{
		"/admin/v1/security-audit?limit=many",
		"/admin/v1/security-audit?cursor=not-base64!",
		"/admin/v1/security-audit?outcome=failed&outcome=denied",
	} {
		request := httptest.NewRequest(http.MethodGet, target, nil)
		if _, err := securityAuditQueryFromRequest(request); err == nil {
			t.Fatalf("query %q was accepted", target)
		}
	}
}

func TestRequireExpectedVersionUsesStrongNumericETag(t *testing.T) {
	tests := []struct {
		name       string
		ifMatch    string
		wantStatus int
		wantCode   string
		want       uint64
	}{
		{name: "missing", wantStatus: http.StatusPreconditionRequired, wantCode: "version-required"},
		{name: "weak", ifMatch: `W/"7"`, wantStatus: http.StatusBadRequest, wantCode: "invalid-version"},
		{name: "list", ifMatch: `"7", "8"`, wantStatus: http.StatusBadRequest, wantCode: "invalid-version"},
		{name: "zero", ifMatch: `"0"`, wantStatus: http.StatusBadRequest, wantCode: "invalid-version"},
		{name: "valid", ifMatch: `"7"`, wantStatus: http.StatusNoContent, want: 7},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var got uint64
			handler := requestIdentityMiddleware(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
				version, ok := requireExpectedVersion(writer, request)
				if !ok {
					return
				}
				got = version
				writer.WriteHeader(http.StatusNoContent)
			}))
			request := httptest.NewRequest(http.MethodPut, "/admin/v1/resource", nil)
			if test.ifMatch != "" {
				request.Header.Set("If-Match", test.ifMatch)
			}
			response := httptest.NewRecorder()
			handler.ServeHTTP(response, request)
			if response.Code != test.wantStatus {
				t.Fatalf("status = %d, want %d", response.Code, test.wantStatus)
			}
			if test.wantCode != "" {
				assertProblem(t, response, test.wantCode)
			}
			if got != test.want {
				t.Fatalf("version = %d, want %d", got, test.want)
			}
		})
	}
}

func TestAdminUpdateErrorsHaveStableHTTPStatus(t *testing.T) {
	tests := []struct {
		name       string
		err        error
		wantStatus int
		wantCode   string
	}{
		{name: "not found", err: adminstore.ErrNotFound, wantStatus: http.StatusNotFound, wantCode: "tenant-not-found"},
		{name: "stale", err: adminstore.ErrVersionConflict, wantStatus: http.StatusPreconditionFailed, wantCode: "version-conflict"},
		{name: "immutable", err: adminstore.ErrImmutable, wantStatus: http.StatusConflict, wantCode: "immutable-resource"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			handler := requestIdentityMiddleware(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
				writeAdminUpdateProblem(writer, request, test.err, "tenant")
			}))
			response := request(t, handler, http.MethodPut, "/admin/v1/tenants/id", "")
			if response.Code != test.wantStatus {
				t.Fatalf("status = %d, want %d", response.Code, test.wantStatus)
			}
			assertProblem(t, response, test.wantCode)
		})
	}
}
