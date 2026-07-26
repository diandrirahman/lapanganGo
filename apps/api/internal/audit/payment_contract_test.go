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
