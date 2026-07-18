package audit

import (
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestPlatformFinanceAuditContract(t *testing.T) {
	sourceID := uuid.NewString()
	reversalID := uuid.NewString()
	validEffectiveAt := time.Now().UTC().Format(time.RFC3339Nano)
	fingerprint := "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
	correlationID := "journal.reversed:" + sourceID
	liveCorrelationID := "live-write-test-1"

	tests := []struct {
		name   string
		params CreatePlatformAuditLogParams
		wantOK bool
	}{
		{
			name: "expense creation accepts bounded safe metadata",
			params: CreatePlatformAuditLogParams{
				ActorRole: "SUPER_ADMIN", Action: ActionPlatformExpenseCreated, EntityType: EntityPlatformExpense,
				EntityID: &sourceID, CorrelationID: &liveCorrelationID,
				Metadata: map[string]any{
					"category": "INFRASTRUCTURE", "amount_rupiah": "250000", "currency": "IDR",
					"occurred_at": validEffectiveAt, "payment_account": "FUNDING_CLEARING", "vendor": "Cloud Vendor", "external_reference": "INV-1",
				},
			},
			wantOK: true,
		},
		{
			name: "expense creation rejects secret metadata",
			params: CreatePlatformAuditLogParams{
				ActorRole: "SUPER_ADMIN", Action: ActionPlatformExpenseCreated, EntityType: EntityPlatformExpense,
				EntityID: &sourceID, CorrelationID: &liveCorrelationID,
				Metadata: map[string]any{
					"category": "INFRASTRUCTURE", "amount_rupiah": "250000", "currency": "IDR",
					"occurred_at": validEffectiveAt, "payment_account": "FUNDING_CLEARING", "vendor": "token=bad",
				},
			},
		},
		{
			name: "expense cancellation accepts an operator reason",
			params: CreatePlatformAuditLogParams{
				ActorRole: "SUPER_ADMIN", Action: ActionPlatformExpenseCancelled, EntityType: EntityPlatformExpense,
				EntityID: &sourceID, CorrelationID: &liveCorrelationID,
				Metadata: map[string]any{"reason": "duplicate invoice"},
			},
			wantOK: true,
		},
		{
			name: "expense approval records the exact transition",
			params: CreatePlatformAuditLogParams{
				ActorRole: "SUPER_ADMIN", Action: ActionPlatformExpenseApproved, EntityType: EntityPlatformExpense,
				EntityID: &sourceID, CorrelationID: &liveCorrelationID,
				Metadata: map[string]any{"transition": "DRAFT_TO_APPROVED"},
			},
			wantOK: true,
		},
		{
			name: "expense posting records the journal link",
			params: CreatePlatformAuditLogParams{
				ActorRole: "SUPER_ADMIN", Action: ActionPlatformExpensePosted, EntityType: EntityPlatformExpense,
				EntityID: &sourceID, CorrelationID: &liveCorrelationID,
				Metadata: map[string]any{
					"category": "OFFICE_ADMIN", "amount_rupiah": "125000", "currency": "IDR",
					"occurred_at": validEffectiveAt, "payment_account": "ACCOUNTS_PAYABLE", "posted_journal_id": reversalID,
				},
			},
			wantOK: true,
		},
		{
			name: "expense void records reason and reversal evidence",
			params: CreatePlatformAuditLogParams{
				ActorRole: "SUPER_ADMIN", Action: ActionPlatformExpenseVoided, EntityType: EntityPlatformExpense,
				EntityID: &sourceID, CorrelationID: &liveCorrelationID,
				Metadata: map[string]any{
					"reason": "supplier correction", "source_journal_id": reversalID,
					"void_journal_id": sourceID, "effective_at": validEffectiveAt,
				},
			},
			wantOK: true,
		},
		{
			name: "reversal requires exact safe metadata",
			params: CreatePlatformAuditLogParams{
				ActorRole:     "SYSTEM",
				Action:        ActionPlatformFinanceJournalReversed,
				EntityType:    EntityPlatformFinanceJournal,
				EntityID:      &reversalID,
				CorrelationID: &correlationID,
				Metadata: map[string]any{
					"source_journal_id": sourceID,
					"effective_at":      validEffectiveAt,
				},
			},
			wantOK: true,
		},
		{
			name: "ownerless live rejection is valid",
			params: CreatePlatformAuditLogParams{
				ActorRole:     "SYSTEM",
				Action:        ActionPlatformFinanceLiveWriteRejected,
				EntityType:    EntityPlatformFinanceJournal,
				CorrelationID: &liveCorrelationID,
				Metadata: map[string]any{
					"reason":              "LIVE_NOT_ALLOWED",
					"write_kind":          "JOURNAL",
					"request_fingerprint": fingerprint,
				},
			},
			wantOK: true,
		},
		{
			name: "action and entity cannot cross",
			params: CreatePlatformAuditLogParams{
				ActorRole:     "SYSTEM",
				Action:        ActionPlatformFinanceJournalReversed,
				EntityType:    EntityPlatformCommercialTerm,
				EntityID:      &reversalID,
				CorrelationID: &correlationID,
				Metadata: map[string]any{
					"source_journal_id": sourceID,
					"effective_at":      validEffectiveAt,
				},
			},
		},
		{
			name: "reversal metadata is complete",
			params: CreatePlatformAuditLogParams{
				ActorRole:     "SYSTEM",
				Action:        ActionPlatformFinanceJournalReversed,
				EntityType:    EntityPlatformFinanceJournal,
				EntityID:      &reversalID,
				CorrelationID: &correlationID,
				Metadata:      map[string]any{"source_journal_id": sourceID},
			},
		},
		{
			name: "live rejection metadata is complete",
			params: CreatePlatformAuditLogParams{
				ActorRole:     "SYSTEM",
				Action:        ActionPlatformFinanceLiveWriteRejected,
				EntityType:    EntityPlatformFinanceJournal,
				CorrelationID: &liveCorrelationID,
				Metadata:      map[string]any{"reason": "LIVE_NOT_ALLOWED", "write_kind": "JOURNAL"},
			},
		},
		{
			name: "nested metadata rejected",
			params: CreatePlatformAuditLogParams{
				ActorRole:     "SYSTEM",
				Action:        ActionPlatformFinanceLiveWriteRejected,
				EntityType:    EntityPlatformFinanceJournal,
				CorrelationID: &liveCorrelationID,
				Metadata: map[string]any{
					"reason":              "LIVE_NOT_ALLOWED",
					"write_kind":          "JOURNAL",
					"request_fingerprint": map[string]any{"value": fingerprint},
				},
			},
		},
		{
			name: "secret metadata rejected",
			params: CreatePlatformAuditLogParams{
				ActorRole:     "SYSTEM",
				Action:        ActionPlatformFinanceLiveWriteRejected,
				EntityType:    EntityPlatformFinanceJournal,
				CorrelationID: &liveCorrelationID,
				Metadata: map[string]any{
					"reason":              "LIVE_NOT_ALLOWED",
					"write_kind":          "JOURNAL",
					"request_fingerprint": "secret_token_value",
				},
			},
		},
		{
			name: "invalid timestamp rejected",
			params: CreatePlatformAuditLogParams{
				ActorRole:     "SYSTEM",
				Action:        ActionPlatformFinanceJournalReversed,
				EntityType:    EntityPlatformFinanceJournal,
				CorrelationID: &correlationID,
				EntityID:      &reversalID,
				Metadata: map[string]any{
					"source_journal_id": sourceID,
					"effective_at":      "not-a-time",
				},
			},
		},
		{
			name: "invalid write kind rejected",
			params: CreatePlatformAuditLogParams{
				ActorRole:     "SYSTEM",
				Action:        ActionPlatformFinanceLiveWriteRejected,
				EntityType:    EntityPlatformFinanceJournal,
				CorrelationID: &liveCorrelationID,
				Metadata: map[string]any{
					"reason":              "LIVE_NOT_ALLOWED",
					"write_kind":          "UNKNOWN",
					"request_fingerprint": fingerprint,
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.params.Validate()
			if tt.wantOK && err != nil {
				t.Fatalf("expected valid params, got %v", err)
			}
			if !tt.wantOK && err == nil {
				t.Fatal("expected validation error")
			}
		})
	}
}

func TestSanitizePlatformFinanceMetadata(t *testing.T) {
	metadata := SanitizePlatformAuditMetadata(ActionPlatformFinanceLiveWriteRejected, map[string]any{
		"reason":              "LIVE_NOT_ALLOWED",
		"write_kind":          "JOURNAL",
		"request_fingerprint": "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
		"payload":             "must not appear",
	})
	if len(metadata) != 2 || metadata["reason"] != "LIVE_NOT_ALLOWED" || metadata["write_kind"] != "JOURNAL" {
		t.Fatalf("unexpected safe finance metadata: %#v", metadata)
	}
}
