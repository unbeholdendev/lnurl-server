package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoad_TrustProxyDefaults(t *testing.T) {
	yaml := `
lnd:
  host: "localhost:10009"
  tls_cert_path: "/tmp/tls.cert"
  macaroon_path: "/tmp/admin.macaroon"
`
	path := writeTestConfig(t, yaml)
	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	if cfg.TrustProxy {
		t.Error("trust_proxy should default to false")
	}
}

func TestLoad_TrustProxyExplicit(t *testing.T) {
	yaml := `
lnd:
  host: "localhost:10009"
  tls_cert_path: "/tmp/tls.cert"
  macaroon_path: "/tmp/admin.macaroon"
trust_proxy: true
`
	path := writeTestConfig(t, yaml)
	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	if !cfg.TrustProxy {
		t.Error("trust_proxy should be true when explicitly set")
	}
}

func writeTestConfig(t *testing.T, content string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("write config: %v", err)
	}
	return path
}
