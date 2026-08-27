package web

import (
	"bytes"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"radio-observation-release-gate/internal/application"
	"radio-observation-release-gate/internal/storage"
)

func testHandler(t *testing.T) http.Handler {
	t.Helper()
	repo, err := storage.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	return New(application.New(repo), slog.New(slog.NewTextHandler(io.Discard, nil)))
}

func TestWorkbenchAndCreateRoute(t *testing.T) {
	handler := testHandler(t)
	request := httptest.NewRequest(http.MethodGet, "/", nil)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != 200 || !strings.Contains(response.Body.String(), "<body>") {
		t.Fatalf("invalid workbench response: %d", response.Code)
	}
	payload := map[string]any{"request_id": "request-1", "expected_revision": 0, "actor": "operator", "batch_id": "WEB-1"}
	raw, _ := json.Marshal(payload)
	request = httptest.NewRequest(http.MethodPost, "/api/batches", bytes.NewReader(raw))
	request.Header.Set("Content-Type", "application/json")
	response = httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != 201 {
		t.Fatalf("create returned %d: %s", response.Code, response.Body.String())
	}
	request = httptest.NewRequest(http.MethodGet, "/api/batches/WEB-1", nil)
	response = httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != 200 || !strings.Contains(response.Body.String(), "eligibility") {
		t.Fatalf("detail returned %d: %s", response.Code, response.Body.String())
	}
}

func TestJSONValidationAndContentType(t *testing.T) {
	handler := testHandler(t)
	request := httptest.NewRequest(http.MethodPost, "/api/batches", strings.NewReader(`{"unknown":true}`))
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != 400 || !strings.Contains(response.Body.String(), "invalid_json") {
		t.Fatalf("unknown field response: %d %s", response.Code, response.Body.String())
	}
	request = httptest.NewRequest(http.MethodPost, "/api/batches", strings.NewReader(`{}`))
	request.Header.Set("Content-Type", "text/plain")
	response = httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != 415 {
		t.Fatalf("content type response: %d", response.Code)
	}
}

func TestBatchListQueryValidation(t *testing.T) {
	handler := testHandler(t)
	for _, path := range []string{"/api/batches?state=unknown", "/api/batches?page=-1", "/api/batches?page_size=101"} {
		request := httptest.NewRequest(http.MethodGet, path, nil)
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, request)
		if response.Code != 400 {
			t.Fatalf("%s returned %d: %s", path, response.Code, response.Body.String())
		}
	}
	request := httptest.NewRequest(http.MethodGet, "/api/batches", nil)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != 200 || !strings.Contains(response.Body.String(), "pagination") {
		t.Fatalf("default list failed: %d %s", response.Code, response.Body.String())
	}
}
