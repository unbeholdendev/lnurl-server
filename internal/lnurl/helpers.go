package lnurl

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/btcsuite/btcd/btcutil/bech32"
)

func writeJSON(w http.ResponseWriter, v any) {
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

// LNURLEncode bech32-encodes a URL with the "lnurl" human-readable part.
func LNURLEncode(url string) (string, error) {
	s, err := bech32.EncodeFromBase256("lnurl", []byte(url))
	if err != nil {
		return "", err
	}
	return strings.ToUpper(s), nil
}

// publicHost is the domain the payer used. Apps that proxy with their own
// HTTP client can't keep Host, so they send it as X-Forwarded-Host.
func publicHost(r *http.Request) string {
	if h := r.Header.Get("X-Forwarded-Host"); h != "" {
		return h
	}
	return r.Host
}
