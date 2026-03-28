package server

import (
	"fmt"
	"log"
	"net/http"

	"lnurl-server/internal/config"
	"lnurl-server/internal/lnurl"
)

// Server is the main HTTP server.
type Server struct {
	cfg      *config.Config
	mux      *http.ServeMux
	pay      *lnurl.PayHandler
	withdraw *lnurl.WithdrawHandler
	address  *lnurl.AddressHandler
}

// New creates a new Server with all routes registered.
func New(cfg *config.Config, pay *lnurl.PayHandler, withdraw *lnurl.WithdrawHandler, address *lnurl.AddressHandler) *Server {
	s := &Server{
		cfg:      cfg,
		mux:      http.NewServeMux(),
		pay:      pay,
		withdraw: withdraw,
		address:  address,
	}
	s.registerRoutes()
	return s
}

func (s *Server) registerRoutes() {
	// LNURL-pay
	if s.pay != nil {
		s.mux.HandleFunc("GET /lnurl/pay", s.pay.HandleFirst)
		s.mux.HandleFunc("GET /lnurl/pay/invoice", s.pay.HandleInvoice)
	}

	// LNURL-withdraw
	if s.withdraw != nil {
		s.mux.HandleFunc("GET /lnurl/withdraw/{id}", s.withdraw.HandleFirst)
		s.mux.HandleFunc("GET /lnurl/withdraw/{id}/claim", s.withdraw.HandleClaim)
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
func (s *Server) domainValidation(next http.Handler) http.Handler {
	allowed := make(map[string]bool)
	for _, d := range s.cfg.Server.AllowedDomains {
		allowed[d] = true
	}

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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

// ListenAndServe starts the HTTP server.
func (s *Server) ListenAndServe() error {
	handler := s.domainValidation(s.mux)

	addr := fmt.Sprintf("%s:%d", s.cfg.Server.Host, s.cfg.Server.Port)
	log.Printf("starting server on %s", addr)
	return http.ListenAndServe(addr, handler)
}
