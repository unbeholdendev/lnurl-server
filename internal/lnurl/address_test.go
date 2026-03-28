package lnurl

import (
	"encoding/json"
	"testing"
)

func TestAddressMetadata_SimpleValues(t *testing.T) {
	meta := addressMetadata("Lightning Address", "satoshi", "pay.example.com")

	var parsed [][]string
	if err := json.Unmarshal([]byte(meta), &parsed); err != nil {
		t.Fatalf("metadata is not valid JSON: %v\nraw: %s", err, meta)
	}

	if len(parsed) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(parsed))
	}
	if parsed[0][0] != "text/plain" || parsed[0][1] != "Lightning Address" {
		t.Errorf("unexpected text/plain entry: %v", parsed[0])
	}
	if parsed[1][0] != "text/identifier" || parsed[1][1] != "satoshi@pay.example.com" {
		t.Errorf("unexpected text/identifier entry: %v", parsed[1])
	}
}

func TestAddressMetadata_SpecialCharsInUsername(t *testing.T) {
	meta := addressMetadata("desc", `user"inject`, "example.com")

	var parsed [][]string
	if err := json.Unmarshal([]byte(meta), &parsed); err != nil {
		t.Fatalf("metadata with special chars is not valid JSON: %v\nraw: %s", err, meta)
	}

	if parsed[1][1] != `user"inject@example.com` {
		t.Errorf("username not preserved correctly: got %s", parsed[1][1])
	}
}

func TestAddressMetadata_SpecialCharsInDescription(t *testing.T) {
	meta := addressMetadata(`He said "hello"`, "user", "example.com")

	var parsed [][]string
	if err := json.Unmarshal([]byte(meta), &parsed); err != nil {
		t.Fatalf("metadata with quotes in description is not valid JSON: %v\nraw: %s", err, meta)
	}

	if parsed[0][1] != `He said "hello"` {
		t.Errorf("description not preserved correctly: got %s", parsed[0][1])
	}
}

func TestAddressMetadata_BackslashInUsername(t *testing.T) {
	meta := addressMetadata("desc", `user\name`, "example.com")

	var parsed [][]string
	if err := json.Unmarshal([]byte(meta), &parsed); err != nil {
		t.Fatalf("metadata with backslash is not valid JSON: %v\nraw: %s", err, meta)
	}

	if parsed[1][1] != `user\name@example.com` {
		t.Errorf("username not preserved correctly: got %s", parsed[1][1])
	}
}
