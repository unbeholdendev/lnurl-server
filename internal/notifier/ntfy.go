package notifier

import (
	"bytes"
	"fmt"
	"log"
	"net/http"
	"time"

	"lnurl-server/internal/config"
)

// Ntfy sends push notifications via an ntfy server.
type Ntfy struct {
	url    string
	topic  string
	token  string
	client *http.Client
}

// NewNtfy creates a new Ntfy notifier from config. Returns nil if ntfy is not configured.
func NewNtfy(cfg *config.NtfyConfig) *Ntfy {
	if cfg == nil {
		return nil
	}
	return &Ntfy{
		url:   cfg.URL,
		topic: cfg.Topic,
		token: cfg.Token,
		client: &http.Client{
			Timeout: 10 * time.Second,
		},
	}
}

// NotifyInvoiceCreated sends a notification when an invoice is created.
func (n *Ntfy) NotifyInvoiceCreated(amountSats int64, address string) {
	if n == nil {
		return
	}
	body := fmt.Sprintf("Invoice created: %d sats for %s", amountSats, address)
	n.send("Invoice created", body)
}

// NotifyPayment sends a notification for a settled payment.
func (n *Ntfy) NotifyPayment(amountSats int64) {
	if n == nil {
		return
	}
	body := fmt.Sprintf("Received %d sats", amountSats)
	n.send("Payment received", body)
}

// NotifyVoucherClaimed sends a notification for a claimed withdraw voucher.
func (n *Ntfy) NotifyVoucherClaimed(voucherID string, amountSats int64) {
	if n == nil {
		return
	}
	body := fmt.Sprintf("Voucher %s claimed (%d sats)", voucherID, amountSats)
	n.send("Voucher claimed", body)
}

func (n *Ntfy) send(title, body string) {
	url := fmt.Sprintf("%s/%s", n.url, n.topic)
	req, err := http.NewRequest("POST", url, bytes.NewBufferString(body))
	if err != nil {
		log.Printf("ntfy: failed to create request: %v", err)
		return
	}
	req.Header.Set("Title", title)
	if n.token != "" {
		req.Header.Set("Authorization", "Bearer "+n.token)
	}

	resp, err := n.client.Do(req)
	if err != nil {
		log.Printf("ntfy: failed to send notification: %v", err)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		log.Printf("ntfy: unexpected status code: %d", resp.StatusCode)
	}
}
