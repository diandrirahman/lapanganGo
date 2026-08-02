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
	Verification    string         `json:"verification"`
	Processing      string         `json:"processing"`
	Duplicate       string         `json:"duplicate"`
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

func TestXenditWebhookFixturesV1ReplayIdentityIsConsistent(t *testing.T) {
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

	original := fixtureByID(t, manifest, "capture-succeeded")
	conflict := fixtureByID(t, manifest, "duplicate-conflict")
	amountMismatch := fixtureByID(t, manifest, "amount-mismatch")

	originalKey, originalPrimary := canonicalCaptureFixtureIdentity(t, fixtureDir, original.File)
	conflictKey, conflictPrimary := canonicalCaptureFixtureIdentity(t, fixtureDir, conflict.File)
	mismatchKey, mismatchPrimary := canonicalCaptureFixtureIdentity(t, fixtureDir, amountMismatch.File)

	if originalKey != original.EventKey || originalPrimary != original.PrimaryObjectID {
		t.Fatalf("capture fixture identity = %q/%q; want %q/%q", originalKey, originalPrimary, original.EventKey, original.PrimaryObjectID)
	}
	if conflictKey != conflict.EventKey || conflictPrimary != conflict.PrimaryObjectID {
		t.Fatalf("conflicting replay identity = %q/%q; want %q/%q", conflictKey, conflictPrimary, conflict.EventKey, conflict.PrimaryObjectID)
	}
	if originalKey != conflictKey || originalPrimary != conflictPrimary {
		t.Fatalf("conflicting replay did not retain canonical capture identity: original=%q/%q conflict=%q/%q", originalKey, originalPrimary, conflictKey, conflictPrimary)
	}
	if original.Hash == conflict.Hash {
		t.Fatal("conflicting replay retained the original exact raw-body hash")
	}
	if mismatchKey != amountMismatch.EventKey || mismatchPrimary != amountMismatch.PrimaryObjectID || mismatchKey == originalKey {
		t.Fatalf("amount mismatch identity is not independently canonical: key=%q primary=%q", mismatchKey, mismatchPrimary)
	}
	if conflict.Verification != "QUARANTINED" || conflict.Processing != "TERMINAL" || conflict.Duplicate != "SAME_KEY_DIFFERENT_HASH_QUARANTINE" {
		t.Fatalf("conflicting replay expectation is not quarantined terminal: %+v", conflict)
	}
}

func TestXenditPaymentSessionFixturesV2AreDeterministicAndRedacted(t *testing.T) {
	_, sourceFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("could not locate fixture test")
	}
	fixtureDir := filepath.Join(filepath.Dir(sourceFile), "testdata", "xendit_payment_sessions_v2")
	manifestBytes, err := os.ReadFile(filepath.Join(fixtureDir, "manifest.json"))
	if err != nil {
		t.Fatal(err)
	}
	var manifest webhookFixtureManifest
	if err := json.Unmarshal(manifestBytes, &manifest); err != nil {
		t.Fatalf("parse V2 manifest: %v", err)
	}
	if manifest.FixtureVersion != "XENDIT_PAYMENT_SESSION_FIXTURES_V2" || len(manifest.Fixtures) != 3 {
		t.Fatalf("unexpected Payment Session fixture freeze: version=%q count=%d", manifest.FixtureVersion, len(manifest.Fixtures))
	}

	for _, fixture := range manifest.Fixtures {
		t.Run(fixture.ID, func(t *testing.T) {
			raw, err := os.ReadFile(filepath.Join(fixtureDir, fixture.File))
			if err != nil {
				t.Fatal(err)
			}
			digest := sha256.Sum256(raw)
			if got := hex.EncodeToString(digest[:]); got != fixture.Hash {
				t.Fatalf("raw hash = %s; want %s", got, fixture.Hash)
			}
			if !json.Valid(raw) {
				t.Fatal("fixture must contain valid JSON")
			}
			key, primary := canonicalPaymentSessionFixtureIdentity(t, raw)
			if key != fixture.EventKey || primary != fixture.PrimaryObjectID {
				t.Fatalf("Payment Session identity = %q/%q; want %q/%q", key, primary, fixture.EventKey, fixture.PrimaryObjectID)
			}
			assertNormalizedPayloadIsRedacted(t, fixture.Normalized)
		})
	}

	original := fixtureByID(t, manifest, "session-completed-actual")
	duplicate := fixtureByID(t, manifest, "session-completed-duplicate")
	if original.EventKey != duplicate.EventKey || original.Hash != duplicate.Hash || duplicate.Processing != "DUPLICATE" || duplicate.Duplicate != "SAME_KEY_SAME_HASH_NOOP" {
		t.Fatalf("Payment Session duplicate contract is inconsistent: original=%+v duplicate=%+v", original, duplicate)
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

func fixtureByID(t *testing.T, manifest webhookFixtureManifest, id string) webhookFixture {
	t.Helper()
	for _, fixture := range manifest.Fixtures {
		if fixture.ID == id {
			return fixture
		}
	}
	t.Fatalf("fixture %q not found", id)
	return webhookFixture{}
}

func canonicalCaptureFixtureIdentity(t *testing.T, fixtureDir, file string) (string, string) {
	t.Helper()
	raw, err := fixtureRawBytes(fixtureDir, file)
	if err != nil {
		t.Fatal(err)
	}
	var payload struct {
		Event string `json:"event"`
		Data  struct {
			PaymentID string `json:"payment_id"`
		} `json:"data"`
	}
	if err := json.Unmarshal(raw, &payload); err != nil || payload.Event != "payment.capture" || payload.Data.PaymentID == "" {
		t.Fatalf("parse canonical capture fixture %q: %v", file, err)
	}
	firstKey := "XENDIT|" + payload.Event + "|" + payload.Data.PaymentID
	var repeated struct {
		Event string `json:"event"`
		Data  struct {
			PaymentID string `json:"payment_id"`
		} `json:"data"`
	}
	if err := json.Unmarshal(raw, &repeated); err != nil {
		t.Fatalf("repeat parse canonical capture fixture %q: %v", file, err)
	}
	secondKey := "XENDIT|" + repeated.Event + "|" + repeated.Data.PaymentID
	if firstKey != secondKey || payload.Data.PaymentID != repeated.Data.PaymentID {
		t.Fatalf("canonical capture parsing is non-deterministic for %q", file)
	}
	return firstKey, payload.Data.PaymentID
}

func canonicalPaymentSessionFixtureIdentity(t *testing.T, raw []byte) (string, string) {
	t.Helper()
	var payload struct {
		Event string `json:"event"`
		Data  struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err := json.Unmarshal(raw, &payload); err != nil || (payload.Event != "payment_session.completed" && payload.Event != "payment_session.expired") || payload.Data.ID == "" {
		t.Fatalf("parse canonical Payment Session fixture: %v", err)
	}
	return "XENDIT|" + payload.Event + "|" + payload.Data.ID, payload.Data.ID
}
