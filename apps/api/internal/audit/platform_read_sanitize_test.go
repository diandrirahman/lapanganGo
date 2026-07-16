package audit

import "testing"

func TestSanitizePlatformAuditMetadata(t *testing.T) {
	metadata := SanitizePlatformAuditMetadata(ActionPlatformCommercialTermCreated, map[string]any{
		"commission_bps":      float64(700),
		"label":               "Standard",
		"valid_from":          "2026-07-16T00:00:00Z",
		"phase":               "STANDARD",
		"request_fingerprint": "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
		"payload":             "must not be returned",
		"nested":              map[string]any{"secret": "must not be returned"},
	})

	if len(metadata) != 4 {
		t.Fatalf("expected four safe keys, got %d: %#v", len(metadata), metadata)
	}
	if _, ok := metadata["request_fingerprint"]; ok {
		t.Fatal("request fingerprint must not be returned")
	}
	if _, ok := metadata["payload"]; ok {
		t.Fatal("unknown sensitive key must not be returned")
	}
	if _, ok := metadata["nested"]; ok {
		t.Fatal("nested metadata must not be returned")
	}
}

func TestSanitizePlatformAuditMetadataInvalidValuesAreOmitted(t *testing.T) {
	metadata := SanitizePlatformAuditMetadata(ActionPlatformCommercialTermLiveRejected, map[string]any{
		"reason": "not-an-allowed-reason",
	})
	if len(metadata) != 0 {
		t.Fatalf("expected invalid reason to be omitted, got %#v", metadata)
	}
}

func TestSanitizeAuditMetadataRejectsSensitiveAndNestedValues(t *testing.T) {
	metadata := SanitizeAuditMetadata(map[string]any{
		"new_status": "ACTIVE",
		"payload":    "raw payload",
		"nested":     map[string]any{"safe": "value"},
		"token_hint": "secret",
	})
	if len(metadata) != 1 || metadata["new_status"] != "ACTIVE" {
		t.Fatalf("unexpected sanitized legacy metadata: %#v", metadata)
	}
}
