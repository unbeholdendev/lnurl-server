package config

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

type Config struct {
	Server    ServerConfig     `yaml:"server"`
	LND       LNDConfig        `yaml:"lnd"`
	Ntfy      *NtfyConfig      `yaml:"ntfy,omitempty"`
	Pay       *PayConfig       `yaml:"pay,omitempty"`
	Withdraw  *WithdrawSection `yaml:"withdraw,omitempty"`
	Addresses *AddressSection  `yaml:"addresses,omitempty"`
}

type ServerConfig struct {
	Host string `yaml:"host"`
	Port int    `yaml:"port"`
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
	Domain      string `yaml:"domain"`
	Description string `yaml:"description"`
	MinSendable int64  `yaml:"min_sendable"`
	MaxSendable int64  `yaml:"max_sendable"`
}

type WithdrawSection struct {
	Domain   string           `yaml:"domain"`
	Vouchers []WithdrawConfig `yaml:"vouchers"`
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

type AddressSection struct {
	Domain string                  `yaml:"domain"`
	Users  map[string]AddressEntry `yaml:"users"`
}

type AddressEntry struct {
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
	return hex.EncodeToString(hash[:6]) // 12 hex chars, 48 bits
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

// AllowedDomains returns the set of all configured domains across sections.
func (c *Config) AllowedDomains() map[string]bool {
	domains := make(map[string]bool)
	if c.Pay != nil && c.Pay.Domain != "" {
		domains[c.Pay.Domain] = true
	}
	if c.Withdraw != nil && c.Withdraw.Domain != "" {
		domains[c.Withdraw.Domain] = true
	}
	if c.Addresses != nil && c.Addresses.Domain != "" {
		domains[c.Addresses.Domain] = true
	}
	return domains
}

func (c *Config) Validate() error {
	if c.Server.Port == 0 {
		return fmt.Errorf("server.port is required")
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
		if c.Pay.Domain == "" {
			return fmt.Errorf("pay.domain is required")
		}
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
	if c.Withdraw != nil {
		if c.Withdraw.Domain == "" {
			return fmt.Errorf("withdraw.domain is required")
		}
		for i, w := range c.Withdraw.Vouchers {
			if w.ID == "" && w.Prefix == "" {
				return fmt.Errorf("withdraw.vouchers[%d]: must have either id or prefix", i)
			}
			if w.ID != "" && w.Prefix != "" {
				return fmt.Errorf("withdraw.vouchers[%d]: cannot have both id and prefix", i)
			}
			if w.Prefix != "" && w.Count <= 0 {
				return fmt.Errorf("withdraw.vouchers[%d]: prefix requires count > 0", i)
			}
			if w.Amount <= 0 {
				return fmt.Errorf("withdraw.vouchers[%d]: amount must be positive", i)
			}
		}
	}
	if c.Addresses != nil {
		if c.Addresses.Domain == "" {
			return fmt.Errorf("addresses.domain is required")
		}
		for name, addr := range c.Addresses.Users {
			if addr.MinSendable <= 0 {
				return fmt.Errorf("addresses.users.%s.min_sendable must be positive", name)
			}
			if addr.MaxSendable <= 0 {
				return fmt.Errorf("addresses.users.%s.max_sendable must be positive", name)
			}
			if addr.MinSendable > addr.MaxSendable {
				return fmt.Errorf("addresses.users.%s.min_sendable must be <= max_sendable", name)
			}
		}
	}
	return nil
}

// ExpandVouchers expands batch withdraw entries into individual Voucher structs.
func (c *Config) ExpandVouchers() []Voucher {
	if c.Withdraw == nil {
		return nil
	}
	var vouchers []Voucher
	for _, w := range c.Withdraw.Vouchers {
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
