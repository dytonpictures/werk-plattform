package httpapi

import (
	"net/http"
	"testing"

	"github.com/dytonpictures/werk/internal/core/identity"
)

func TestRequireAccessPlaneFailsClosedWithoutActor(t *testing.T) {
	handler := requestIdentityMiddleware(RequireAccessPlane(identity.AccessPlaneWork)(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		t.Fatal("handler must not run")
	})))
	response := request(t, handler, http.MethodGet, "/api/v1/test", "")
	if response.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusForbidden)
	}
	assertProblem(t, response, "access-denied")
}
