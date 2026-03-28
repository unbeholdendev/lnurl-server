package lnurl

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strconv"

	"lnurl-server/internal/config"
	"lnurl-server/internal/lnd"
)

// PayHandler handles LNURL-pay protocol requests.
type PayHandler struct {
	cfg    *config.PayConfig
	lnd    *lnd.Client
	domain string
}

// NewPayHandler creates a new PayHandler. Returns nil if pay is not configured.
func NewPayHandler(cfg *config.PayConfig, lndClient *lnd.Client) *PayHandler {
	if cfg == nil {
		return nil
	}
	return &PayHandler{
		cfg:    cfg,
		lnd:    lndClient,
		domain: cfg.Domain,
	}
}

// HandleFirst handles the initial LNURL-pay request (GET /lnurl/pay).
func (h *PayHandler) HandleFirst(w http.ResponseWriter, r *http.Request) {
	metadata := h.metadata()

	resp := map[string]interface{}{
		"callback":    fmt.Sprintf("https://%s/lnurl/pay/invoice", h.domain),
		"minSendable": h.cfg.MinSendable,
		"maxSendable": h.cfg.MaxSendable,
		"metadata":    metadata,
		"tag":         "payRequest",
	}
	writeJSON(w, resp)
}

// HandleInvoice handles the invoice creation request (GET /lnurl/pay/invoice?amount=X).
func (h *PayHandler) HandleInvoice(w http.ResponseWriter, r *http.Request) {
	amountStr := r.URL.Query().Get("amount")
	if amountStr == "" {
		writeError(w, "missing amount parameter")
		return
	}
	amountMsat, err := strconv.ParseInt(amountStr, 10, 64)
	if err != nil {
		writeError(w, "invalid amount")
		return
	}
	if amountMsat < h.cfg.MinSendable || amountMsat > h.cfg.MaxSendable {
		writeError(w, fmt.Sprintf("amount must be between %d and %d msats", h.cfg.MinSendable, h.cfg.MaxSendable))
		return
	}

	descHash := sha256.Sum256([]byte(h.metadata()))
	bolt11, err := h.lnd.AddInvoice(r.Context(), amountMsat, descHash[:])
	if err != nil {
		log.Printf("lnurl-pay: add invoice failed: %v", err)
		writeError(w, "failed to create invoice")
		return
	}

	resp := map[string]interface{}{
		"pr":     bolt11,
		"routes": []interface{}{},
	}
	writeJSON(w, resp)
}

func (h *PayHandler) metadata() string {
	return fmt.Sprintf(`[["text/plain","%s"]]`, h.cfg.Description)
}

// MetadataHash returns the SHA256 hash of the pay metadata string.
// Used by the invoice subscriber to match settled invoices back to this endpoint.
func (h *PayHandler) MetadataHash() [32]byte {
	return sha256.Sum256([]byte(h.metadata()))
}

// requestHost returns the public-facing host from the request.
func requestHost(r *http.Request) string {
	if fwd := r.Header.Get("X-Forwarded-Host"); fwd != "" {
		return fwd
	}
	return r.Host
}

func writeJSON(w http.ResponseWriter, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, reason string) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{
		"status": "ERROR",
		"reason": reason,
	})
}
