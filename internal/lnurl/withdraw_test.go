package lnurl

import (
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"lnurl-server/internal/config"
)

func TestSaveStateConcurrency(t *testing.T) {
	dir := t.TempDir()
	statePath := filepath.Join(dir, "state.json")

	h := NewWithdrawHandler(
		[]config.Voucher{
			{ID: "test-001", Token: "abc123", Domain: "example.com", Description: "test", Amount: 1000},
			{ID: "test-002", Token: "def456", Domain: "example.com", Description: "test", Amount: 1000},
		},
		nil, // lnd client not needed for state tests
		nil, // ntfy not needed
		statePath,
	)

	// Concurrently claim vouchers and save state
	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			id := "test-001"
			if i%2 == 0 {
				id = "test-002"
			}
			h.mu.Lock()
			h.claimed[id] = ClaimedInfo{
				ClaimedAt:  time.Now().UTC(),
				AmountMsat: 1000,
			}
			h.mu.Unlock()
			_ = h.saveState()
		}(i)
	}
	wg.Wait()

	// Verify state file is valid JSON
	data, err := os.ReadFile(statePath)
	if err != nil {
		t.Fatalf("failed to read state file: %v", err)
	}
	if len(data) == 0 {
		t.Fatal("state file is empty")
	}
}

func TestK1Expiration(t *testing.T) {
	dir := t.TempDir()
	statePath := filepath.Join(dir, "state.json")

	h := NewWithdrawHandler(
		[]config.Voucher{
			{ID: "test-001", Token: "abc123", Domain: "example.com", Description: "test", Amount: 1000},
		},
		nil, nil, statePath,
	)

	// Manually insert an expired k1
	h.mu.Lock()
	h.k1s["expired-k1"] = k1Entry{
		voucherID: "test-001",
		createdAt: time.Now().Add(-10 * time.Minute),
	}
	h.k1s["fresh-k1"] = k1Entry{
		voucherID: "test-001",
		createdAt: time.Now(),
	}
	h.mu.Unlock()

	h.cleanupExpiredK1s()

	h.mu.Lock()
	defer h.mu.Unlock()

	if _, exists := h.k1s["expired-k1"]; exists {
		t.Error("expired k1 should have been removed")
	}
	if _, exists := h.k1s["fresh-k1"]; !exists {
		t.Error("fresh k1 should still exist")
	}
}

func TestK1PerVoucherEviction(t *testing.T) {
	dir := t.TempDir()
	statePath := filepath.Join(dir, "state.json")

	h := NewWithdrawHandler(
		[]config.Voucher{
			{ID: "test-001", Token: "abc123", Domain: "example.com", Description: "test", Amount: 1000},
		},
		nil, nil, statePath,
	)

	// Add a k1 for the voucher
	h.mu.Lock()
	h.k1s["old-k1"] = k1Entry{voucherID: "test-001", createdAt: time.Now().Add(-1 * time.Minute)}
	h.voucherToK1["test-001"] = "old-k1"
	h.mu.Unlock()

	// Issue a new k1 — should evict the old one
	newK1 := h.addK1("test-001")

	h.mu.Lock()
	defer h.mu.Unlock()

	if _, exists := h.k1s["old-k1"]; exists {
		t.Error("old k1 for same voucher should have been evicted")
	}
	if _, exists := h.k1s[newK1]; !exists {
		t.Error("new k1 should exist")
	}
}
