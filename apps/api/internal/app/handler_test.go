package app

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/dytonpictures/werk-plattform/apps/api/internal/config"
)

type fakeDB struct{ err error }

func (db fakeDB) Ping(context.Context) error { return db.err }

func TestHealth(t *testing.T) {
	handler := NewHandler(config.Config{}, fakeDB{}, slog.New(slog.NewTextHandler(io.Discard, nil)))
	request := httptest.NewRequest(http.MethodGet, "/health", nil)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d", response.Code)
	}
}

func TestReadyFailsWhenDatabaseFails(t *testing.T) {
	handler := NewHandler(config.Config{}, fakeDB{err: errors.New("offline")}, slog.New(slog.NewTextHandler(io.Discard, nil)))
	request := httptest.NewRequest(http.MethodGet, "/ready", nil)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d", response.Code)
	}
}
