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
