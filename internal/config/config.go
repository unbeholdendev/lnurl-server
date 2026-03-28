package config

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

type Config struct {
	Server    ServerConfig             `yaml:"server"`
	LND       LNDConfig                `yaml:"lnd"`
	Ntfy      *NtfyConfig              `yaml:"ntfy,omitempty"`
	Pay       *PayConfig               `yaml:"pay,omitempty"`
	Withdraw  []WithdrawConfig         `yaml:"withdraw,omitempty"`
	Addresses map[string]AddressConfig `yaml:"addresses,omitempty"`
}

type ServerConfig struct {
	Host           string   `yaml:"host"`
	Port           int      `yaml:"port"`
	AllowedDomains []string `yaml:"allowed_domains"`
}

type LNDConfig struct {
	Host         string `yaml:"host"`
	TLSCertPath  string `yaml:"tls_cert_path"`
	MacaroonPath string `yaml:"macaroon_path"`
}

type NtfyConfig struct {
	URL   string `yaml:"url"`
	Topic string `yaml:"topic"`
	Token string `yaml:"token,omitempty"`
}

type PayConfig struct {
	Description string `yaml:"description"`
	MinSendable int64  `yaml:"min_sendable"`
	MaxSendable int64  `yaml:"max_sendable"`
}

type WithdrawConfig struct {
	// Single voucher fields
	ID string `yaml:"id,omitempty"`
	// Batch fields
	Prefix string `yaml:"prefix,omitempty"`
	Count  int    `yaml:"count,omitempty"`
	// Common fields
	Description string `yaml:"description"`
	Amount      int64  `yaml:"amount"`
}

type AddressConfig struct {
	Description string `yaml:"description"`
	MinSendable int64  `yaml:"min_sendable"`
	MaxSendable int64  `yaml:"max_sendable"`
}

// Voucher is an expanded single withdraw voucher (after batch expansion).
type Voucher struct {
	ID          string
	Token       string // URL-safe opaque token derived from ID
	Description string
	Amount      int64 // millisatoshis
}

// tokenForVoucher generates a deterministic, unguessable URL token from a voucher ID.
func tokenForVoucher(id string) string {
	hash := sha256.Sum256([]byte(id))
	return hex.EncodeToString(hash[:16]) // 32 hex chars, 128 bits
}

func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read config: %w", err)
	}
	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parse config: %w", err)
	}
	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("validate config: %w", err)
	}
	return &cfg, nil
}

func (c *Config) Validate() error {
	if c.Server.Port == 0 {
		return fmt.Errorf("server.port is required")
	}
	if len(c.Server.AllowedDomains) == 0 {
		return fmt.Errorf("server.allowed_domains must have at least one entry")
	}
	if c.LND.Host == "" {
		return fmt.Errorf("lnd.host is required")
	}
	if c.LND.TLSCertPath == "" {
		return fmt.Errorf("lnd.tls_cert_path is required")
	}
	if c.LND.MacaroonPath == "" {
		return fmt.Errorf("lnd.macaroon_path is required")
	}
	if c.Pay != nil {
		if c.Pay.MinSendable <= 0 {
			return fmt.Errorf("pay.min_sendable must be positive")
		}
		if c.Pay.MaxSendable <= 0 {
			return fmt.Errorf("pay.max_sendable must be positive")
		}
		if c.Pay.MinSendable > c.Pay.MaxSendable {
			return fmt.Errorf("pay.min_sendable must be <= pay.max_sendable")
		}
	}
	for i, w := range c.Withdraw {
		if w.ID == "" && w.Prefix == "" {
			return fmt.Errorf("withdraw[%d]: must have either id or prefix", i)
		}
		if w.ID != "" && w.Prefix != "" {
			return fmt.Errorf("withdraw[%d]: cannot have both id and prefix", i)
		}
		if w.Prefix != "" && w.Count <= 0 {
			return fmt.Errorf("withdraw[%d]: prefix requires count > 0", i)
		}
		if w.Amount <= 0 {
			return fmt.Errorf("withdraw[%d]: amount must be positive", i)
		}
	}
	for name, addr := range c.Addresses {
		if addr.MinSendable <= 0 {
			return fmt.Errorf("addresses.%s.min_sendable must be positive", name)
		}
		if addr.MaxSendable <= 0 {
			return fmt.Errorf("addresses.%s.max_sendable must be positive", name)
		}
		if addr.MinSendable > addr.MaxSendable {
			return fmt.Errorf("addresses.%s.min_sendable must be <= max_sendable", name)
		}
	}
	return nil
}

// ExpandVouchers expands batch withdraw entries into individual Voucher structs.
func (c *Config) ExpandVouchers() []Voucher {
	var vouchers []Voucher
	for _, w := range c.Withdraw {
		if w.ID != "" {
			vouchers = append(vouchers, Voucher{
				ID:          w.ID,
				Token:       tokenForVoucher(w.ID),
				Description: w.Description,
				Amount:      w.Amount,
			})
		} else if w.Prefix != "" {
			for i := 1; i <= w.Count; i++ {
				id := fmt.Sprintf("%s-%03d", w.Prefix, i)
				vouchers = append(vouchers, Voucher{
					ID:          id,
					Token:       tokenForVoucher(id),
					Description: w.Description,
					Amount:      w.Amount,
				})
			}
		}
	}
	return vouchers
}
