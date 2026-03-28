package server

import (
	"bytes"
	"log"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestRequestLogging_LogsRequestDetails(t *testing.T) {
	var buf bytes.Buffer
	logger := log.New(&buf, "", 0)

	handler := requestLogging(logger, false, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("ok"))
	}))

	req := httptest.NewRequest("GET", "/health", nil)
	req.RemoteAddr = "1.2.3.4:5678"
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	logLine := buf.String()
	if !strings.Contains(logLine, "GET") {
		t.Errorf("log should contain method, got: %s", logLine)
	}
	if !strings.Contains(logLine, "/health") {
		t.Errorf("log should contain path, got: %s", logLine)
	}
	if !strings.Contains(logLine, "200") {
		t.Errorf("log should contain status code, got: %s", logLine)
	}
	if !strings.Contains(logLine, "1.2.3.4") {
		t.Errorf("log should contain client IP, got: %s", logLine)
	}
}

func TestRequestLogging_CapturesNon200Status(t *testing.T) {
	var buf bytes.Buffer
	logger := log.New(&buf, "", 0)

	handler := requestLogging(logger, false, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))

	req := httptest.NewRequest("GET", "/missing", nil)
	req.RemoteAddr = "1.2.3.4:5678"
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	logLine := buf.String()
	if !strings.Contains(logLine, "404") {
		t.Errorf("log should contain 404 status, got: %s", logLine)
	}
}

func TestStatusResponseWriter_DefaultStatus(t *testing.T) {
	rec := httptest.NewRecorder()
	rw := &statusResponseWriter{ResponseWriter: rec}

	// Write body without calling WriteHeader — should default to 200
	rw.Write([]byte("hello"))

	if rw.statusCode() != http.StatusOK {
		t.Errorf("expected default status 200, got %d", rw.statusCode())
	}
}
