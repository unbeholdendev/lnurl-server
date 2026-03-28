package lnurl

import (
	"crypto/sha256"
	"fmt"
	"log"
	"net/http"
	"strconv"

	"lnurl-server/internal/config"
	"lnurl-server/internal/lnd"
	"lnurl-server/internal/notifier"
)

// AddressHandler handles Lightning Address (LUD-16) requests.
type AddressHandler struct {
	addresses map[string]config.AddressEntry
	wildcard  *config.AddressEntry // catch-all for unknown usernames
	lnd       *lnd.Client
	ntfy      *notifier.Ntfy
}

// NewAddressHandler creates a new AddressHandler. Returns nil if no addresses configured.
func NewAddressHandler(addresses map[string]config.AddressEntry, lndClient *lnd.Client, ntfy *notifier.Ntfy) *AddressHandler {
	if len(addresses) == 0 {
		return nil
	}

	h := &AddressHandler{
		addresses: addresses,
		lnd:       lndClient,
		ntfy:      ntfy,
	}

	// Extract wildcard entry if present
	if wc, ok := addresses["*"]; ok {
		h.wildcard = &wc
	}

	return h
}

// resolveAddress looks up the address entry for a username.
// Explicit entries take priority over the wildcard.
func (h *AddressHandler) resolveAddress(username string) (config.AddressEntry, bool) {
	if addr, ok := h.addresses[username]; ok && username != "*" {
		return addr, true
	}
	if h.wildcard != nil {
		return *h.wildcard, true
	}
	return config.AddressEntry{}, false
}

// HandleWellKnown handles GET /.well-known/lnurlp/{username}.
func (h *AddressHandler) HandleWellKnown(w http.ResponseWriter, r *http.Request) {
	username := r.PathValue("username")
	addr, ok := h.resolveAddress(username)
	if !ok {
		http.NotFound(w, r)
		return
	}

	metadata := addressMetadata(addr.Description, username, addr.Domain)

	resp := map[string]any{
		"callback":    fmt.Sprintf("https://%s/.well-known/lnurlp/%s/invoice", addr.Domain, username),
		"minSendable": addr.MinSendable,
		"maxSendable": addr.MaxSendable,
		"metadata":    metadata,
		"tag":         "payRequest",
	}
	writeJSON(w, resp)
}

// HandleInvoice handles GET /.well-known/lnurlp/{username}/invoice?amount=X.
func (h *AddressHandler) HandleInvoice(w http.ResponseWriter, r *http.Request) {
	username := r.PathValue("username")
	addr, ok := h.resolveAddress(username)
	if !ok {
		http.NotFound(w, r)
		return
	}

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
	if amountMsat < addr.MinSendable || amountMsat > addr.MaxSendable {
		writeError(w, fmt.Sprintf("amount must be between %d and %d msats", addr.MinSendable, addr.MaxSendable))
		return
	}

	metadata := addressMetadata(addr.Description, username, addr.Domain)
	descHash := sha256.Sum256([]byte(metadata))

	bolt11, err := h.lnd.AddInvoice(r.Context(), amountMsat, descHash[:])
	if err != nil {
		log.Printf("lightning-address: add invoice failed for %s: %v", username, err)
		writeError(w, "failed to create invoice")
		return
	}

	// Notify on invoice creation
	address := fmt.Sprintf("%s@%s", username, addr.Domain)
	h.ntfy.NotifyInvoiceCreated(amountMsat/1000, address)
	log.Printf("invoice created: %d sats for %s", amountMsat/1000, address)

	resp := map[string]any{
		"pr":     bolt11,
		"routes": []any{},
	}
	writeJSON(w, resp)
}

func addressMetadata(description, username, host string) string {
	return fmt.Sprintf(`[["text/plain","%s"],["text/identifier","%s@%s"]]`, description, username, host)
}
