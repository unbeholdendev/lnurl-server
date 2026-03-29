package server

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"lnurl-server/internal/config"
)

func TestDomainValidation_MatchingHostAllowed(t *testing.T) {
	cfg := &config.Config{
		LND: config.LNDConfig{Host: "localhost:10009"},
		Addresses: map[string]config.AddressEntry{
			"test": {Domain: "pay.example.com", MinSendable: 1000, MaxSendable: 1000000, Description: "test"},
		},
	}

	s := &Server{cfg: cfg, mux: http.NewServeMux()}
	handler := s.domainValidation(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest("GET", "/health", nil)
	req.Host = "pay.example.com"
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected 200 for matching host, got %d", rec.Code)
	}
}

func TestDomainValidation_UnknownHostReturns404(t *testing.T) {
	cfg := &config.Config{
		LND: config.LNDConfig{Host: "localhost:10009"},
		Addresses: map[string]config.AddressEntry{
			"test": {Domain: "pay.example.com", MinSendable: 1000, MaxSendable: 1000000, Description: "test"},
		},
	}

	s := &Server{cfg: cfg, mux: http.NewServeMux()}
	handler := s.domainValidation(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest("GET", "/some-path", nil)
	req.Host = "unknown.example.com"
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Errorf("expected 404 for unknown host, got %d", rec.Code)
	}
}

func TestDomainValidation_XForwardedHostOverridesHost(t *testing.T) {
	cfg := &config.Config{
		LND: config.LNDConfig{Host: "localhost:10009"},
		Addresses: map[string]config.AddressEntry{
			"test": {Domain: "pay.example.com", MinSendable: 1000, MaxSendable: 1000000, Description: "test"},
		},
	}

	s := &Server{cfg: cfg, mux: http.NewServeMux()}
	handler := s.domainValidation(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	// Proxy forwards with internal Host but sets X-Forwarded-Host to the real domain
	req := httptest.NewRequest("GET", "/health", nil)
	req.Host = "10.0.0.1:8080"
	req.Header.Set("X-Forwarded-Host", "pay.example.com")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected 200 via X-Forwarded-Host, got %d", rec.Code)
	}
}
