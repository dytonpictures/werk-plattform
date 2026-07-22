package httpapi

import (
	"encoding/json"
	"fmt"
	"net/http"
)

type problem struct {
	Type          string `json:"type"`
	Title         string `json:"title"`
	Status        int    `json:"status"`
	Detail        string `json:"detail"`
	Instance      string `json:"instance"`
	Code          string `json:"code"`
	RequestID     string `json:"request_id"`
	CorrelationID string `json:"correlation_id"`
}

func writeProblem(writer http.ResponseWriter, request *http.Request, status int, code, title, detail string) {
	requestID := requestIDFromContext(request.Context())
	correlationID := correlationIDFromContext(request.Context())
	value := problem{
		Type:          "urn:werk:problem:" + code,
		Title:         title,
		Status:        status,
		Detail:        detail,
		Instance:      fmt.Sprintf("urn:werk:request:%s", requestID),
		Code:          code,
		RequestID:     requestID,
		CorrelationID: correlationID,
	}

	writer.Header().Set("Content-Type", "application/problem+json; charset=utf-8")
	writer.WriteHeader(status)
	_ = json.NewEncoder(writer).Encode(value)
}
