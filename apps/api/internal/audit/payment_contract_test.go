package audit

import (
	"testing"

	"github.com/google/uuid"
)

func TestPaymentAuditContract(t *testing.T) {
	entityID := uuid.NewString()
	correlationID := "payment:create:" + entityID

	tests := []struct {
		name    string
		params  CreatePlatformAuditLogParams
		wantErr bool
	}{
		{
			name: "payment attempt created",
			params: CreatePlatformAuditLogParams{
				ActorRole:     "SYSTEM",
				Action:        ActionPaymentStateTransition,
				EntityType:    EntityPaymentAttempt,
				EntityID:      &entityID,
				CorrelationID: &correlationID,
				Metadata: map[string]any{
					"to_state":     "CREATED",
					"attempt_no":   1,
					"late_capture": false,
				},
			},
		},
		{
			name: "late capture reconciliation exception",
			params: CreatePlatformAuditLogParams{
				ActorRole:     "SYSTEM",
				Action:        ActionReconciliationException,
				EntityType:    EntityPaymentAttempt,
				EntityID:      &entityID,
				CorrelationID: &correlationID,
				Metadata: map[string]any{
					"from_state": "EXPIRED",
					"to_state":   "CAPTURED",
					"attempt_no": 1,
					"reason":     "LATE_CAPTURE",
				},
			},
		},
		{
			name: "missing attempt command invariant",
			params: CreatePlatformAuditLogParams{
				ActorRole:     "SYSTEM",
				Action:        ActionPaymentCommandInvariantViolation,
				EntityType:    EntityPaymentAttempt,
				EntityID:      &entityID,
				CorrelationID: &correlationID,
				Metadata: map[string]any{
					"command_type": "PAYMENT_INQUIRY",
					"reason":       "ATTEMPT_NOT_FOUND",
				},
			},
		},
		{
			name: "command invariant reason is fixed",
			params: CreatePlatformAuditLogParams{
				ActorRole:     "SYSTEM",
				Action:        ActionPaymentCommandInvariantViolation,
				EntityType:    EntityPaymentAttempt,
				EntityID:      &entityID,
				CorrelationID: &correlationID,
				Metadata: map[string]any{
					"command_type": "PAYMENT_CREATE",
					"reason":       "OTHER",
				},
			},
			wantErr: true,
		},
		{
			name: "command invariant rejects global audit reason",
			params: CreatePlatformAuditLogParams{
				ActorRole:     "SYSTEM",
				Action:        ActionPaymentCommandInvariantViolation,
				EntityType:    EntityPaymentAttempt,
				EntityID:      &entityID,
				CorrelationID: &correlationID,
				Metadata: map[string]any{
					"command_type": "PAYMENT_CREATE",
					"reason":       "LIVE_NOT_ALLOWED",
				},
			},
			wantErr: true,
		},
		{
			name: "payment action requires payment entity",
			params: CreatePlatformAuditLogParams{
				ActorRole:     "SYSTEM",
				Action:        ActionPaymentStateTransition,
				EntityType:    EntityPlatformExpense,
				EntityID:      &entityID,
				CorrelationID: &correlationID,
				Metadata: map[string]any{
					"to_state":     "CREATED",
					"attempt_no":   1,
					"late_capture": false,
				},
			},
			wantErr: true,
		},
		{
			name: "invalid payment state rejected",
			params: CreatePlatformAuditLogParams{
				ActorRole:     "SYSTEM",
				Action:        ActionPaymentStateTransition,
				EntityType:    EntityPaymentAttempt,
				EntityID:      &entityID,
				CorrelationID: &correlationID,
				Metadata: map[string]any{
					"to_state":     "PAID",
					"attempt_no":   1,
					"late_capture": false,
				},
			},
			wantErr: true,
		},
		{
			name: "reconciliation reason is fixed",
			params: CreatePlatformAuditLogParams{
				ActorRole:     "SYSTEM",
				Action:        ActionReconciliationException,
				EntityType:    EntityPaymentAttempt,
				EntityID:      &entityID,
				CorrelationID: &correlationID,
				Metadata: map[string]any{
					"from_state": "EXPIRED",
					"to_state":   "CAPTURED",
					"attempt_no": 1,
					"reason":     "OTHER",
				},
			},
			wantErr: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.params.Validate()
			if tc.wantErr && err == nil {
				t.Fatal("expected validation error")
			}
			if !tc.wantErr && err != nil {
				t.Fatalf("unexpected validation error: %v", err)
			}
		})
	}
}

func TestSanitizePaymentAuditMetadata(t *testing.T) {
	metadata := SanitizePlatformAuditMetadata(ActionPaymentStateTransition, map[string]any{
		"from_state":   "PENDING",
		"to_state":     "CAPTURED",
		"attempt_no":   float64(2),
		"late_capture": false,
		"payload":      "must not be returned",
	})

	if len(metadata) != 4 {
		t.Fatalf("expected four safe payment keys, got %#v", metadata)
	}
	if metadata["attempt_no"] != 2 {
		t.Fatalf("attempt_no was not normalized: %#v", metadata["attempt_no"])
	}
}

func TestSanitizePaymentInvariantReasonIsActionSpecific(t *testing.T) {
	metadata := SanitizePlatformAuditMetadata(ActionPaymentCommandInvariantViolation, map[string]any{
		"command_type": "PAYMENT_CREATE",
		"reason":       "LIVE_NOT_ALLOWED",
	})
	if _, ok := metadata["reason"]; ok {
		t.Fatalf("global audit reason leaked into payment invariant metadata: %#v", metadata)
	}
}

func TestPaymentCreateAuditActionsAreBounded(t *testing.T) {
	entityID := uuid.NewString()
	correlationID := "pa-create-audit"
	fingerprint := "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
	valid := []CreatePlatformAuditLogParams{
		{
			ActorRole: "CUSTOMER", Action: ActionPaymentCommandEnqueued, EntityType: EntityPaymentAttempt,
			EntityID: &entityID, CorrelationID: &correlationID,
			Metadata: map[string]any{"attempt_no": 1, "command_type": "PAYMENT_CREATE"},
		},
		{
			ActorRole: "SYSTEM", Action: ActionPaymentCommandEnqueued, EntityType: EntityPaymentAttempt,
			EntityID: &entityID, CorrelationID: &correlationID,
			Metadata: map[string]any{"attempt_no": 1, "command_type": "PAYMENT_INQUIRY"},
		},
		{
			ActorRole: "CUSTOMER", Action: ActionPaymentCreateFlagOffRejected, EntityType: EntityPaymentAttempt,
			CorrelationID: &correlationID,
			Metadata:      map[string]any{"reason": "CREATE_DISABLED", "requested_method": "QRIS", "request_fingerprint": fingerprint},
		},
	}
	for _, params := range valid {
		if err := params.Validate(); err != nil {
			t.Fatalf("valid payment create audit rejected: %v", err)
		}
	}
	invalid := valid[0]
	invalid.Metadata["command_type"] = "UNKNOWN_COMMAND"
	if err := invalid.Validate(); err == nil {
		t.Fatal("invalid provider command audit accepted")
	}
}
