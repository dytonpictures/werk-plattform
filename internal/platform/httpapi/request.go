package httpapi

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
)

const maximumJSONBodyBytes = 64 << 10

// decodeJSON applies one global input policy: bounded body size, known fields
// only, and exactly one JSON document.
func decodeJSON(writer http.ResponseWriter, request *http.Request, destination any) error {
	request.Body = http.MaxBytesReader(writer, request.Body, maximumJSONBodyBytes)
	decoder := json.NewDecoder(request.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(destination); err != nil {
		return err
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		if err == nil {
			return errors.New("request body must contain exactly one JSON document")
		}
		return err
	}
	return nil
}
