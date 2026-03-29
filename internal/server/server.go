package server

import (
	"context"
	"log"
	"net/http"
	"sync"

	"lnurl-server/internal/config"
	"lnurl-server/internal/lnurl"
)

const listenAddr = "0.0.0.0:8080"

// Server is the main HTTP server.
type Server struct {
	cfg        *config.Config
	mux        *http.ServeMux
	withdraw   *lnurl.WithdrawHandler
	address    *lnurl.AddressHandler
	rl         *rateLimiter
	done       chan struct{}
	closeOnce  sync.Once
	httpServer *http.Server
}

// New creates a new Server with all routes registered.
func New(cfg *config.Config, withdraw *lnurl.WithdrawHandler, address *lnurl.AddressHandler) *Server {
	done := make(chan struct{})
	rl := newRateLimiter(10.0/60.0, 10) // 10 requests/min per IP
	go rl.startCleanup(done)

	s := &Server{
		cfg:      cfg,
		mux:      http.NewServeMux(),
		withdraw: withdraw,
		address:  address,
		rl:       rl,
		done:     done,
	}
	s.registerRoutes()
	return s
}

func (s *Server) registerRoutes() {
	// LNURL-withdraw
	if s.withdraw != nil {
		s.mux.HandleFunc("GET /lnurl/withdraw/{token}", s.withdraw.HandleFirst)
		s.mux.HandleFunc("GET /lnurl/withdraw/{token}/claim", s.withdraw.HandleClaim)
	}

	// Lightning Address
	if s.address != nil {
		s.mux.HandleFunc("GET /.well-known/lnurlp/{username}", s.address.HandleWellKnown)
		s.mux.HandleFunc("GET /.well-known/lnurlp/{username}/invoice", s.address.HandleInvoice)
	}

	// Health check
	s.mux.HandleFunc("GET /health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("ok"))
	})
}

// domainValidation is middleware that checks the Host header against allowed domains.
// Allowed domains are derived from the per-section domain configs.
func (s *Server) domainValidation(next http.Handler) http.Handler {
	allowed := s.cfg.AllowedDomains()

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/health" {
			next.ServeHTTP(w, r)
			return
		}
		host := r.Host
		if fwd := r.Header.Get("X-Forwarded-Host"); fwd != "" {
			host = fwd
		}
		if !allowed[host] {
			http.NotFound(w, r)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// ListenAndServe starts the HTTP server. The server can be stopped
// by calling Shutdown.
func (s *Server) ListenAndServe() error {
	handler := s.domainValidation(s.mux)
	handler = requestLogging(log.Default(), handler)
	handler = s.rl.middleware(handler)

	s.httpServer = &http.Server{
		Addr:    listenAddr,
		Handler: handler,
	}

	log.Printf("starting server on %s", listenAddr)
	return s.httpServer.ListenAndServe()
}

// Shutdown gracefully shuts down the HTTP server and stops background goroutines.
func (s *Server) Shutdown(ctx context.Context) error {
	s.Close()
	if s.httpServer != nil {
		return s.httpServer.Shutdown(ctx)
	}
	return nil
}

// Close stops background goroutines. Safe to call multiple times.
func (s *Server) Close() {
	s.closeOnce.Do(func() {
		close(s.done)
	})
}
