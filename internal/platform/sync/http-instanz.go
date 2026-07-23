package platformsync

import (
	"encoding/json"
	"errors"
	"net/http"
	"time"
)

const instanceHealthSchemaV1 = "platform.instance-health.v1"

var ErrInvalidHealthHandler = errors.New("invalid instance health handler")

// HTTPInstanceHealthHandler exposes liveness metadata for one runtime instance.
// Liveness may start failover evaluation, but can never grant a lease, increase
// authority generation, verify fencing, or authorize a policy request.
type HTTPInstanceHealthHandler struct {
	instance Instance
	now      func() time.Time
}

type instanceHealthResponse struct {
	SchemaVersion string            `json:"schema_version"`
	Status        string            `json:"status"`
	InstanceID    string            `json:"instance_id"`
	Profile       DeploymentProfile `json:"deployment_profile"`
	BuildVersion  string            `json:"build_version"`
	ObservedAt    time.Time         `json:"observed_at"`
}

func NewHTTPInstanceHealthHandler(instance Instance) (*HTTPInstanceHealthHandler, error) {
	return newHTTPInstanceHealthHandler(instance, time.Now)
}

func newHTTPInstanceHealthHandler(instance Instance, now func() time.Time) (*HTTPInstanceHealthHandler, error) {
	if !instance.Valid() || now == nil {
		return nil, ErrInvalidHealthHandler
	}
	return &HTTPInstanceHealthHandler{instance: instance, now: now}, nil
}

func (handler *HTTPInstanceHealthHandler) ServeHTTP(writer http.ResponseWriter, request *http.Request) {
	writer.Header().Set("Cache-Control", "no-store")
	writer.Header().Set("X-Content-Type-Options", "nosniff")
	if handler == nil || !handler.instance.Valid() || handler.now == nil {
		http.Error(writer, "instance health unavailable", http.StatusServiceUnavailable)
		return
	}
	if request.Method != http.MethodGet && request.Method != http.MethodHead {
		writer.Header().Set("Allow", "GET, HEAD")
		http.Error(writer, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	writer.Header().Set("Content-Type", "application/json; charset=utf-8")
	writer.WriteHeader(http.StatusOK)
	if request.Method == http.MethodHead {
		return
	}
	_ = json.NewEncoder(writer).Encode(instanceHealthResponse{
		SchemaVersion: instanceHealthSchemaV1,
		Status:        "live",
		InstanceID:    handler.instance.ID,
		Profile:       handler.instance.DeploymentProfile,
		BuildVersion:  handler.instance.BuildVersion,
		ObservedAt:    handler.now().UTC(),
	})
}
