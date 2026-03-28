package lnurl

import (
	"crypto/sha256"
	"fmt"
	"log"
	"net/http"
	"strconv"

	"lnurl-server/internal/config"
	"lnurl-server/internal/lnd"
)

// AddressHandler handles Lightning Address (LUD-16) requests.
type AddressHandler struct {
	addresses map[string]config.AddressEntry
	lnd       *lnd.Client
}

// NewAddressHandler creates a new AddressHandler. Returns nil if no addresses configured.
func NewAddressHandler(addresses map[string]config.AddressEntry, lndClient *lnd.Client) *AddressHandler {
	if len(addresses) == 0 {
		return nil
	}
	return &AddressHandler{
		addresses: addresses,
		lnd:       lndClient,
	}
}

// HandleWellKnown handles GET /.well-known/lnurlp/{username}.
func (h *AddressHandler) HandleWellKnown(w http.ResponseWriter, r *http.Request) {
	username := r.PathValue("username")
	addr, ok := h.addresses[username]
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
	addr, ok := h.addresses[username]
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

	resp := map[string]any{
		"pr":     bolt11,
		"routes": []any{},
	}
	writeJSON(w, resp)
}

// MetadataHashes returns a map of description hash -> "username@domain" for all addresses.
func (h *AddressHandler) MetadataHashes() map[[32]byte]string {
	result := make(map[[32]byte]string)
	for username, addr := range h.addresses {
		metadata := addressMetadata(addr.Description, username, addr.Domain)
		hash := sha256.Sum256([]byte(metadata))
		result[hash] = fmt.Sprintf("%s@%s", username, addr.Domain)
	}
	return result
}

func addressMetadata(description, username, host string) string {
	return fmt.Sprintf(`[["text/plain","%s"],["text/identifier","%s@%s"]]`, description, username, host)
}
