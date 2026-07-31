package payments

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

type webhookFixtureManifest struct {
	FixtureVersion string           `json:"fixture_version"`
	Fixtures       []webhookFixture `json:"fixtures"`
}

type webhookFixture struct {
	ID              string         `json:"id"`
	File            string         `json:"file"`
	Family          string         `json:"family"`
	EventType       string         `json:"event_type"`
	EventKey        string         `json:"event_key"`
	PrimaryObjectID string         `json:"primary_object_id"`
	Hash            string         `json:"hash"`
	Normalized      map[string]any `json:"normalized"`
}

func TestXenditWebhookFixturesV1AreDeterministicAndRedacted(t *testing.T) {
	_, sourceFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("could not locate fixture test")
	}
	fixtureDir := filepath.Join(filepath.Dir(sourceFile), "testdata", "xendit_webhooks_v1")
	manifestBytes, err := os.ReadFile(filepath.Join(fixtureDir, "manifest.json"))
	if err != nil {
		t.Fatal(err)
	}
	var manifest webhookFixtureManifest
	if err := json.Unmarshal(manifestBytes, &manifest); err != nil {
		t.Fatalf("parse manifest: %v", err)
	}
	if manifest.FixtureVersion != "XENDIT_WEBHOOK_FIXTURES_V1" || len(manifest.Fixtures) != 29 {
		t.Fatalf("unexpected fixture freeze: version=%q count=%d", manifest.FixtureVersion, len(manifest.Fixtures))
	}

	for _, fixture := range manifest.Fixtures {
		t.Run(fixture.ID, func(t *testing.T) {
			raw, err := fixtureRawBytes(fixtureDir, fixture.File)
			if err != nil {
				t.Fatal(err)
			}
			digest := sha256.Sum256(raw)
			if got := hex.EncodeToString(digest[:]); got != fixture.Hash {
				t.Fatalf("raw hash = %s; want %s", got, fixture.Hash)
			}
			if fixture.EventKey == "" || fixture.PrimaryObjectID == "" || !strings.HasPrefix(fixture.EventKey, "XENDIT|") {
				t.Fatalf("fixture identity is not deterministic: key=%q primary=%q", fixture.EventKey, fixture.PrimaryObjectID)
			}
			if fixture.ID == "malformed-json" || fixture.ID == "oversized-body" {
				return
			}
			if !json.Valid(raw) {
				t.Fatal("fixture must contain valid JSON")
			}
			assertNormalizedPayloadIsRedacted(t, fixture.Normalized)
		})
	}
}

func fixtureRawBytes(dir, name string) ([]byte, error) {
	if name != "oversized_body.spec" {
		return os.ReadFile(filepath.Join(dir, name))
	}
	// The spec is deliberately tiny in Git while producing the exact 256 KiB +
	// one-byte ingress vector without relying on a generated current timestamp.
	return []byte(strings.Repeat("a", 262145)), nil
}

func assertNormalizedPayloadIsRedacted(t *testing.T, normalized map[string]any) {
	t.Helper()
	encoded, err := json.Marshal(normalized)
	if err != nil {
		t.Fatal(err)
	}
	lower := strings.ToLower(string(encoded))
	for _, forbidden := range []string{"callback", "authorization", "token", "pan", "cvv", "email", "phone", "name", "checkout_url"} {
		if strings.Contains(lower, forbidden) {
			t.Fatalf("normalized payload retained forbidden field %q", forbidden)
		}
	}
}
