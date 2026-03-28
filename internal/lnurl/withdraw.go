package lnurl

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"time"

	"lnurl-server/internal/config"
	"lnurl-server/internal/lnd"
	"lnurl-server/internal/notifier"
)

// WithdrawHandler handles LNURL-withdraw protocol requests.
type WithdrawHandler struct {
	vouchers    map[string]config.Voucher // id -> voucher
	tokenToID   map[string]string         // token -> id (for URL lookup)
	allVouchers []config.Voucher          // ordered list for printing
	lnd         *lnd.Client
	ntfy        *notifier.Ntfy
	statePath   string

	mu      sync.Mutex
	claimed map[string]ClaimedInfo // id -> claim info
	k1s     map[string]string      // k1 -> voucher id
}

// ClaimedInfo records when a voucher was claimed.
type ClaimedInfo struct {
	ClaimedAt  time.Time `json:"claimed_at"`
	AmountMsat int64     `json:"amount_msat"`
}

type stateFile struct {
	Claimed map[string]ClaimedInfo `json:"claimed"`
}

// NewWithdrawHandler creates a new WithdrawHandler. Returns nil if no vouchers configured.
func NewWithdrawHandler(vouchers []config.Voucher, lndClient *lnd.Client, ntfy *notifier.Ntfy, statePath string) *WithdrawHandler {
	if len(vouchers) == 0 {
		return nil
	}

	voucherMap := make(map[string]config.Voucher)
	tokenToID := make(map[string]string)
	for _, v := range vouchers {
		voucherMap[v.ID] = v
		tokenToID[v.Token] = v.ID
	}

	h := &WithdrawHandler{
		vouchers:    voucherMap,
		tokenToID:   tokenToID,
		allVouchers: vouchers,
		lnd:         lndClient,
		ntfy:        ntfy,
		statePath:   statePath,
		claimed:     make(map[string]ClaimedInfo),
		k1s:         make(map[string]string),
	}

	if err := h.loadState(); err != nil {
		log.Printf("withdraw: failed to load state (starting fresh): %v", err)
	}

	return h
}

// HandleFirst handles GET /lnurl/withdraw/{token}.
func (h *WithdrawHandler) HandleFirst(w http.ResponseWriter, r *http.Request) {
	token := r.PathValue("token")
	id, ok := h.tokenToID[token]
	if !ok {
		http.NotFound(w, r)
		return
	}
	voucher := h.vouchers[id]

	h.mu.Lock()
	_, alreadyClaimed := h.claimed[id]
	h.mu.Unlock()

	if alreadyClaimed {
		writeError(w, "this voucher has already been claimed")
		return
	}

	// Generate k1
	k1Bytes := make([]byte, 32)
	if _, err := rand.Read(k1Bytes); err != nil {
		log.Printf("withdraw: failed to generate k1: %v", err)
		writeError(w, "internal error")
		return
	}
	k1 := hex.EncodeToString(k1Bytes)

	h.mu.Lock()
	h.k1s[k1] = id
	h.mu.Unlock()

	resp := map[string]any{
		"callback":           fmt.Sprintf("https://%s/lnurl/withdraw/%s/claim", voucher.Domain, token),
		"k1":                 k1,
		"minWithdrawable":    voucher.Amount,
		"maxWithdrawable":    voucher.Amount,
		"defaultDescription": voucher.Description,
		"tag":                "withdrawRequest",
	}
	writeJSON(w, resp)
}

// HandleClaim handles GET /lnurl/withdraw/{token}/claim?k1=X&pr=X.
func (h *WithdrawHandler) HandleClaim(w http.ResponseWriter, r *http.Request) {
	token := r.PathValue("token")
	k1 := r.URL.Query().Get("k1")
	pr := r.URL.Query().Get("pr")

	if k1 == "" || pr == "" {
		writeError(w, "missing k1 or pr parameter")
		return
	}

	id, ok := h.tokenToID[token]
	if !ok {
		http.NotFound(w, r)
		return
	}
	voucher := h.vouchers[id]

	h.mu.Lock()
	// Validate k1
	expectedID, k1Valid := h.k1s[k1]
	if !k1Valid || expectedID != id {
		h.mu.Unlock()
		writeError(w, "invalid k1")
		return
	}
	// Check not already claimed
	if _, alreadyClaimed := h.claimed[id]; alreadyClaimed {
		h.mu.Unlock()
		writeError(w, "this voucher has already been claimed")
		return
	}
	// Mark as claimed immediately to prevent double-claim
	h.claimed[id] = ClaimedInfo{
		ClaimedAt:  time.Now().UTC(),
		AmountMsat: voucher.Amount,
	}
	delete(h.k1s, k1)
	h.mu.Unlock()

	// Pay the invoice (max fee: 1% of amount or 10 sats, whichever is higher)
	maxFeeSat := max(voucher.Amount/100000, 10) // 1% in sats, minimum 10
	_, err := h.lnd.SendPayment(r.Context(), pr, maxFeeSat)
	if err != nil {
		// Undo the claim on failure
		h.mu.Lock()
		delete(h.claimed, id)
		h.mu.Unlock()
		log.Printf("withdraw: payment failed for voucher %s: %v", id, err)
		writeError(w, "payment failed")
		return
	}

	// Persist state
	if err := h.saveState(); err != nil {
		log.Printf("withdraw: failed to save state: %v", err)
	}

	// Notify
	h.ntfy.NotifyVoucherClaimed(id, voucher.Amount/1000)

	resp := map[string]string{
		"status": "OK",
	}
	writeJSON(w, resp)
}

func (h *WithdrawHandler) loadState() error {
	data, err := os.ReadFile(h.statePath)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	var state stateFile
	if err := json.Unmarshal(data, &state); err != nil {
		return err
	}
	if state.Claimed != nil {
		h.claimed = state.Claimed
	}
	return nil
}

func (h *WithdrawHandler) saveState() error {
	h.mu.Lock()
	state := stateFile{Claimed: h.claimed}
	h.mu.Unlock()

	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return err
	}

	// Atomic write: write to temp file, then rename
	dir := filepath.Dir(h.statePath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	tmp := h.statePath + ".tmp"
	if err := os.WriteFile(tmp, data, 0644); err != nil {
		return err
	}
	return os.Rename(tmp, h.statePath)
}

// UnclaimedVouchers returns all vouchers that have not been claimed, with their full URLs.
func (h *WithdrawHandler) UnclaimedVouchers() []VoucherStatus {
	h.mu.Lock()
	defer h.mu.Unlock()

	var result []VoucherStatus
	for _, v := range h.allVouchers {
		if _, claimed := h.claimed[v.ID]; !claimed {
			result = append(result, VoucherStatus{
				ID:     v.ID,
				Amount: v.Amount,
				URL:    fmt.Sprintf("https://%s/lnurl/withdraw/%s", v.Domain, v.Token),
			})
		}
	}
	return result
}

// VoucherStatus represents an unclaimed voucher with its public URL.
type VoucherStatus struct {
	ID     string
	Amount int64
	URL    string
}
