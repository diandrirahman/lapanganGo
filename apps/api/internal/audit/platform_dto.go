package audit

import (
	"encoding/hex"
	"errors"
	"math"
	"regexp"
	"strings"
	"time"

	"github.com/google/uuid"
)

const (
	ActionPlatformCommercialTermCreated      = "PLATFORM_COMMERCIAL_TERM_CREATED"
	ActionPlatformCommercialTermSuperseded   = "PLATFORM_COMMERCIAL_TERM_SUPERSEDED"
	ActionPlatformCommercialTermLiveRejected = "PLATFORM_COMMERCIAL_TERM_LIVE_REJECTED"
	ActionPlatformFinanceJournalReversed     = "PLATFORM_FINANCE_JOURNAL_REVERSED"
	ActionPlatformFinanceLiveWriteRejected   = "PLATFORM_FINANCE_LIVE_WRITE_REJECTED"
	ActionPlatformExpenseCreated             = "PLATFORM_EXPENSE_CREATED"
	ActionPlatformExpenseCancelled           = "PLATFORM_EXPENSE_CANCELLED"
	ActionPlatformExpenseApproved            = "PLATFORM_EXPENSE_APPROVED"
	ActionPlatformExpensePosted              = "PLATFORM_EXPENSE_POSTED"
	ActionPlatformExpenseVoided              = "PLATFORM_EXPENSE_VOIDED"
	ActionPaymentStateTransition             = "payment_state_transition"
	ActionReconciliationException            = "reconciliation_exception"
	ActionPaymentAttemptCreated              = "PAYMENT_ATTEMPT_CREATED"
	ActionPaymentCommandEnqueued             = "PAYMENT_COMMAND_ENQUEUED"
	ActionPaymentCommandInvariantViolation   = "PAYMENT_COMMAND_INVARIANT_VIOLATION"
	ActionPaymentCreateFlagOffRejected       = "PAYMENT_CREATE_FLAG_OFF_REJECTED"

	EntityPlatformCommercialTerm = "PLATFORM_COMMERCIAL_TERM"
	EntityPlatformFinanceJournal = "PLATFORM_FINANCE_JOURNAL"
	EntityPlatformExpense        = "PLATFORM_EXPENSE"
	EntityPaymentAttempt         = "PAYMENT_ATTEMPT"
)

var allowedPlatformActions = map[string]bool{
	ActionPlatformCommercialTermCreated:      true,
	ActionPlatformCommercialTermSuperseded:   true,
	ActionPlatformCommercialTermLiveRejected: true,
	ActionPlatformFinanceJournalReversed:     true,
	ActionPlatformFinanceLiveWriteRejected:   true,
	ActionPlatformExpenseCreated:             true,
	ActionPlatformExpenseCancelled:           true,
	ActionPlatformExpenseApproved:            true,
	ActionPlatformExpensePosted:              true,
	ActionPlatformExpenseVoided:              true,
	ActionPaymentStateTransition:             true,
	ActionReconciliationException:            true,
	ActionPaymentAttemptCreated:              true,
	ActionPaymentCommandEnqueued:             true,
	ActionPaymentCommandInvariantViolation:   true,
	ActionPaymentCreateFlagOffRejected:       true,
}

var allowedPlatformEntities = map[string]bool{
	EntityPlatformCommercialTerm: true,
	EntityPlatformFinanceJournal: true,
	EntityPlatformExpense:        true,
	EntityPaymentAttempt:         true,
}

var platformActionEntity = map[string]string{
	ActionPlatformCommercialTermCreated:      EntityPlatformCommercialTerm,
	ActionPlatformCommercialTermSuperseded:   EntityPlatformCommercialTerm,
	ActionPlatformCommercialTermLiveRejected: EntityPlatformCommercialTerm,
	ActionPlatformFinanceJournalReversed:     EntityPlatformFinanceJournal,
	ActionPlatformFinanceLiveWriteRejected:   EntityPlatformFinanceJournal,
	ActionPlatformExpenseCreated:             EntityPlatformExpense,
	ActionPlatformExpenseCancelled:           EntityPlatformExpense,
	ActionPlatformExpenseApproved:            EntityPlatformExpense,
	ActionPlatformExpensePosted:              EntityPlatformExpense,
	ActionPlatformExpenseVoided:              EntityPlatformExpense,
	ActionPaymentStateTransition:             EntityPaymentAttempt,
	ActionReconciliationException:            EntityPaymentAttempt,
	ActionPaymentAttemptCreated:              EntityPaymentAttempt,
	ActionPaymentCommandEnqueued:             EntityPaymentAttempt,
	ActionPaymentCommandInvariantViolation:   EntityPaymentAttempt,
	ActionPaymentCreateFlagOffRejected:       EntityPaymentAttempt,
}

var allowedMetadataKeysPerAction = map[string]map[string]bool{
	ActionPlatformCommercialTermCreated: {
		"commission_bps": true,
		"label":          true,
		"valid_from":     true,
		"phase":          true,
	},
	ActionPlatformCommercialTermSuperseded: {
		"superseded_term_id": true,
		"new_term_id":        true,
	},
	ActionPlatformCommercialTermLiveRejected: {
		"reason":              true,
		"request_fingerprint": true,
	},
	ActionPlatformFinanceJournalReversed: {
		"source_journal_id": true,
		"effective_at":      true,
	},
	ActionPlatformFinanceLiveWriteRejected: {
		"reason":              true,
		"write_kind":          true,
		"request_fingerprint": true,
	},
	ActionPlatformExpenseCreated: {
		"category":           true,
		"amount_rupiah":      true,
		"currency":           true,
		"occurred_at":        true,
		"payment_account":    true,
		"vendor":             true,
		"external_reference": true,
	},
	ActionPlatformExpenseCancelled: {
		"reason": true,
	},
	ActionPlatformExpenseApproved: {
		"transition": true,
	},
	ActionPlatformExpensePosted: {
		"category":          true,
		"amount_rupiah":     true,
		"currency":          true,
		"occurred_at":       true,
		"payment_account":   true,
		"posted_journal_id": true,
	},
	ActionPlatformExpenseVoided: {
		"reason":            true,
		"source_journal_id": true,
		"void_journal_id":   true,
		"effective_at":      true,
	},
	ActionPaymentStateTransition: {
		"from_state":   true,
		"to_state":     true,
		"attempt_no":   true,
		"late_capture": true,
	},
	ActionReconciliationException: {
		"from_state": true,
		"to_state":   true,
		"attempt_no": true,
		"reason":     true,
	},
	ActionPaymentAttemptCreated: {
		"attempt_no": true,
	},
	ActionPaymentCommandEnqueued: {
		"attempt_no":   true,
		"command_type": true,
	},
	ActionPaymentCommandInvariantViolation: {
		"command_type": true,
		"reason":       true,
	},
	ActionPaymentCreateFlagOffRejected: {
		"reason":              true,
		"requested_method":    true,
		"request_fingerprint": true,
	},
}

var requiredMetadataKeysPerAction = map[string][]string{
	ActionPlatformFinanceJournalReversed:   {"source_journal_id", "effective_at"},
	ActionPlatformFinanceLiveWriteRejected: {"reason", "write_kind", "request_fingerprint"},
	ActionPaymentStateTransition:           {"to_state", "attempt_no", "late_capture"},
	ActionReconciliationException:          {"from_state", "to_state", "attempt_no", "reason"},
	ActionPaymentAttemptCreated:            {"attempt_no"},
	ActionPaymentCommandEnqueued:           {"attempt_no", "command_type"},
	ActionPaymentCommandInvariantViolation: {"command_type", "reason"},
	ActionPaymentCreateFlagOffRejected:     {"reason", "requested_method", "request_fingerprint"},
}

var allowedLiveWriteKinds = map[string]bool{
	"JOURNAL":           true,
	"PAYMENT":           true,
	"COMMISSION":        true,
	"REFUND":            true,
	"PAYOUT":            true,
	"SETTLEMENT":        true,
	"OPERATING_EXPENSE": true,
}

func IsAllowedPlatformFinanceWriteKind(value string) bool {
	return allowedLiveWriteKinds[value]
}

var forbiddenMetadataKeys = []string{
	"secret", "token", "password", "authorization", "credential", "payload", "pii", "bearer",
}

// Legacy owner audit rows predate the read API and may contain arbitrary JSON.
// Keep the read boundary deny-by-default by projecting only fields emitted by
// the current owner/staff/booking/finance handlers for each action.
var allowedLegacyMetadataKeysPerAction = map[string]map[string]bool{
	ActionStaffCreated:                legacyMetadataKeys("email", "role", "permissions", "venue_ids"),
	ActionStaffUpdated:                legacyMetadataKeys("role", "permissions", "venue_ids"),
	ActionStaffStatusUpdated:          legacyMetadataKeys("old_status", "new_status"),
	ActionStaffVenuesUpdated:          legacyMetadataKeys("old_venue_ids", "new_venue_ids"),
	ActionStaffInviteCreated:          legacyMetadataKeys("email", "role", "permissions", "venue_ids"),
	ActionStaffInviteRegenerated:      legacyMetadataKeys("expires_at"),
	ActionStaffPasswordResetRequested: legacyMetadataKeys("expires_at"),
	ActionStaffPasswordResetCompleted: legacyMetadataKeys(),
	ActionStaffPasswordSetupCompleted: legacyMetadataKeys(),
	ActionBookingPaymentVerified:      legacyMetadataKeys("booking_id", "is_approved", "new_status"),
	ActionBookingPaymentRejected:      legacyMetadataKeys("booking_id", "is_approved", "new_status"),
	ActionBookingMarkedPaid:           legacyMetadataKeys("booking_id", "new_status"),
	ActionBookingCompleted:            legacyMetadataKeys("booking_id", "new_status"),
	ActionBookingCancelRefund:         legacyMetadataKeys("booking_id", "reason", "amount", "venue_id"),
	ActionRefundApproved:              legacyMetadataKeys("refund_request_id", "booking_id", "owner_note"),
	ActionRefundRejected:              legacyMetadataKeys("refund_request_id", "booking_id", "owner_note"),
	ActionFinanceCreated:              legacyMetadataKeys("venue_id", "type", "category", "amount", "transaction_date"),
	ActionFinanceUpdated:              legacyMetadataKeys("before", "after", "changed_fields"),
	ActionFinanceDeleted:              legacyMetadataKeys("deleted_transaction"),
	"UPDATE_OWNER_STATUS":             legacyMetadataKeys("new_status"),
	"UPDATE_VENUE_STATUS":             legacyMetadataKeys("new_status"),
}

func legacyMetadataKeys(keys ...string) map[string]bool {
	allowed := make(map[string]bool, len(keys))
	for _, key := range keys {
		allowed[key] = true
	}
	return allowed
}

type CreatePlatformAuditLogParams struct {
	ActorUserID    *string
	ActorRole      string
	Action         string
	EntityType     string
	EntityID       *string
	OwnerProfileID *string
	VenueID        *string
	CorrelationID  *string
	Metadata       map[string]any
	IPAddress      *string
	UserAgent      *string
}

func (p *CreatePlatformAuditLogParams) Validate() error {
	if !allowedPlatformActions[p.Action] {
		return errors.New("invalid platform audit action")
	}
	if !allowedPlatformEntities[p.EntityType] {
		return errors.New("invalid platform audit entity type")
	}
	if expectedEntity := platformActionEntity[p.Action]; p.EntityType != expectedEntity {
		return errors.New("platform audit action/entity mismatch")
	}
	if p.ActorRole == "" {
		return errors.New("actor role is required")
	}
	if p.ActorRole != strings.TrimSpace(p.ActorRole) {
		return errors.New("actor role must not contain surrounding whitespace")
	}
	if p.ActorUserID != nil {
		if _, err := uuid.Parse(*p.ActorUserID); err != nil {
			return errors.New("actor user id must be a valid UUID")
		}
	}
	if p.EntityID != nil {
		if _, err := uuid.Parse(*p.EntityID); err != nil {
			return errors.New("entity id must be a valid UUID")
		}
	}
	if p.OwnerProfileID != nil {
		if _, err := uuid.Parse(*p.OwnerProfileID); err != nil {
			return errors.New("owner profile id must be a valid UUID")
		}
	}
	if p.VenueID != nil {
		if _, err := uuid.Parse(*p.VenueID); err != nil {
			return errors.New("venue id must be a valid UUID")
		}
	}
	if p.CorrelationID != nil && strings.TrimSpace(*p.CorrelationID) == "" {
		return errors.New("correlation id must not be empty")
	}
	if p.Action == ActionPlatformFinanceJournalReversed && p.EntityID == nil {
		return errors.New("reversal audit entity id is required")
	}
	if p.Action == ActionPlatformFinanceLiveWriteRejected && p.EntityID != nil {
		return errors.New("live rejection audit entity id must be nil")
	}
	if (p.Action == ActionPlatformExpenseCreated || p.Action == ActionPlatformExpenseCancelled || p.Action == ActionPlatformExpenseApproved || p.Action == ActionPlatformExpensePosted || p.Action == ActionPlatformExpenseVoided) && p.EntityID == nil {
		return errors.New("expense audit entity id is required")
	}
	if (p.Action == ActionPlatformExpenseCancelled || p.Action == ActionPlatformExpenseApproved || p.Action == ActionPlatformExpensePosted || p.Action == ActionPlatformExpenseVoided) && p.CorrelationID == nil {
		return errors.New("correlation id is required for expense action")
	}
	if (p.Action == ActionPaymentStateTransition || p.Action == ActionReconciliationException || p.Action == ActionPaymentAttemptCreated || p.Action == ActionPaymentCommandEnqueued || p.Action == ActionPaymentCommandInvariantViolation) && (p.EntityID == nil || p.CorrelationID == nil) {
		return errors.New("payment audit entity and correlation id are required")
	}
	if p.Action == ActionPaymentCreateFlagOffRejected && p.CorrelationID == nil {
		return errors.New("correlation id is required for payment create rejection audit")
	}
	if (p.Action == ActionPlatformFinanceJournalReversed || p.Action == ActionPlatformFinanceLiveWriteRejected) && p.CorrelationID == nil {
		return errors.New("correlation id is required for finance audit action")
	}
	if _, required := requiredMetadataKeysPerAction[p.Action]; required && p.Metadata == nil {
		return errors.New("metadata is required for this platform audit action")
	}

	allowedKeys := allowedMetadataKeysPerAction[p.Action]

	if p.Metadata != nil {
		for key, val := range p.Metadata {
			if !allowedKeys[key] {
				return errors.New("metadata key not allowed for this action: " + key)
			}

			// Value validation
			switch key {
			case "commission_bps":
				switch v := val.(type) {
				case float64:
					if v < 0 || v > 3000 {
						return errors.New("commission_bps out of bounds")
					}
				case int:
					if v < 0 || v > 3000 {
						return errors.New("commission_bps out of bounds")
					}
				default:
					return errors.New("commission_bps must be a number")
				}
			case "label":
				v, ok := val.(string)
				if !ok || len(v) > 200 {
					return errors.New("label must be string <= 200 chars")
				}
				if containsSecret(v) {
					return errors.New("label contains sensitive information")
				}
			case "valid_from":
				v, ok := val.(string)
				if !ok {
					return errors.New("valid_from must be a date string")
				}
				if containsSecret(v) {
					return errors.New("valid_from contains sensitive information")
				}
			case "phase":
				v, ok := val.(string)
				if !ok || (v != "TRIAL" && v != "INTRODUCTORY" && v != "STANDARD" && v != "CUSTOM") {
					return errors.New("phase must be valid enum")
				}
			case "superseded_term_id", "new_term_id":
				v, ok := val.(string)
				if !ok {
					return errors.New(key + " must be a UUID string")
				}
				if _, err := uuid.Parse(v); err != nil {
					return errors.New(key + " must be a valid UUID")
				}
			case "reason":
				v, ok := val.(string)
				if !ok {
					return errors.New("reason must be string")
				}
				if !isAllowedReasonForAction(p.Action, v) {
					return errors.New("reason must be an allowed code")
				}
			case "from_state", "to_state":
				v, ok := val.(string)
				if !ok || !isPaymentAttemptAuditState(v) {
					return errors.New(key + " must be a supported payment attempt state")
				}
			case "attempt_no":
				v, ok := val.(int)
				if !ok || v <= 0 || v > math.MaxInt16 {
					return errors.New("attempt_no must be a positive small integer")
				}
			case "late_capture":
				if _, ok := val.(bool); !ok {
					return errors.New("late_capture must be boolean")
				}
			case "command_type":
				v, ok := val.(string)
				if !ok || (v != "PAYMENT_CREATE" && v != "PAYMENT_INQUIRY") {
					return errors.New("command_type must be a supported payment command")
				}
			case "requested_method":
				v, ok := val.(string)
				if !ok || (v != "BCA_VA" && v != "QRIS" && v != "CARD") {
					return errors.New("requested_method must be an allowed payment method")
				}
			case "transition":
				v, ok := val.(string)
				if !ok || v != "DRAFT_TO_APPROVED" {
					return errors.New("transition must be DRAFT_TO_APPROVED")
				}
			case "request_fingerprint":
				v, ok := val.(string)
				if !ok || len(v) != 64 {
					return errors.New("request_fingerprint must be a 64-character SHA-256 hex string")
				}
				if _, err := hex.DecodeString(v); err != nil {
					return errors.New("request_fingerprint must be valid hexadecimal")
				}
			case "source_journal_id":
				v, ok := val.(string)
				if !ok {
					return errors.New("source_journal_id must be a UUID string")
				}
				if _, err := uuid.Parse(v); err != nil {
					return errors.New("source_journal_id must be a valid UUID")
				}
			case "effective_at":
				v, ok := val.(string)
				if !ok {
					return errors.New("effective_at must be an RFC3339 timestamp")
				}
				if _, err := time.Parse(time.RFC3339Nano, v); err != nil {
					return errors.New("effective_at must be an RFC3339 timestamp")
				}
			case "posted_journal_id", "void_journal_id":
				v, ok := val.(string)
				if !ok {
					return errors.New(key + " must be a UUID string")
				}
				if _, err := uuid.Parse(v); err != nil {
					return errors.New(key + " must be a valid UUID")
				}
			case "write_kind":
				v, ok := val.(string)
				if !ok || !allowedLiveWriteKinds[v] {
					return errors.New("write_kind must be a supported finance write kind")
				}
			case "category":
				v, ok := val.(string)
				if !ok || !expenseAuditCategories[v] {
					return errors.New("category must be a supported platform expense category")
				}
			case "amount_rupiah":
				v, ok := val.(string)
				if !ok || !expenseAuditDigits.MatchString(v) || len(v) > 19 {
					return errors.New("amount_rupiah must be a digit string")
				}
			case "currency":
				if val != "IDR" {
					return errors.New("currency must be IDR")
				}
			case "occurred_at":
				v, ok := val.(string)
				if !ok {
					return errors.New("occurred_at must be an RFC3339 timestamp")
				}
				if _, err := time.Parse(time.RFC3339Nano, v); err != nil {
					return errors.New("occurred_at must be an RFC3339 timestamp")
				}
			case "payment_account":
				v, ok := val.(string)
				if !ok || (v != "FUNDING_CLEARING" && v != "ACCOUNTS_PAYABLE") {
					return errors.New("payment_account must be a supported account")
				}
			case "vendor", "external_reference":
				v, ok := val.(string)
				if !ok || len([]byte(v)) > 191 || containsSecret(v) {
					return errors.New(key + " must be a safe bounded string")
				}
			}
		}
	} else {
		p.Metadata = make(map[string]any)
	}

	for _, key := range requiredMetadataKeysPerAction[p.Action] {
		if _, ok := p.Metadata[key]; !ok {
			return errors.New("required metadata key missing: " + key)
		}
	}
	if p.Action == ActionPlatformFinanceLiveWriteRejected {
		if p.Metadata["reason"] != "LIVE_NOT_ALLOWED" {
			return errors.New("reason must be LIVE_NOT_ALLOWED for finance LIVE rejection")
		}
	}

	return nil
}

func containsSecret(s string) bool {
	lower := strings.ToLower(s)
	for _, f := range forbiddenMetadataKeys {
		if strings.Contains(lower, f) {
			return true
		}
	}
	return false
}

// SanitizePlatformAuditMetadata projects platform audit metadata onto the
// allowlisted, scalar fields that are safe for an administrative read API.
// Invalid or unknown values are omitted rather than being returned verbatim.
func SanitizePlatformAuditMetadata(action string, metadata map[string]any) map[string]any {
	out := make(map[string]any)
	allowedKeys := allowedMetadataKeysPerAction[action]
	for key, value := range metadata {
		if !allowedKeys[key] || key == "request_fingerprint" {
			continue
		}

		switch key {
		case "commission_bps":
			if bps, ok := scalarCommissionBPS(value); ok {
				out[key] = bps
			}
		case "label":
			if value, ok := value.(string); ok && len(value) <= 200 && !containsSecret(value) {
				out[key] = value
			}
		case "valid_from":
			if value, ok := value.(string); ok && !containsSecret(value) {
				out[key] = value
			}
		case "phase":
			if value, ok := value.(string); ok && (value == "TRIAL" || value == "INTRODUCTORY" || value == "STANDARD" || value == "CUSTOM") {
				out[key] = value
			}
		case "superseded_term_id", "new_term_id":
			if value, ok := value.(string); ok {
				if _, err := uuid.Parse(value); err == nil {
					out[key] = value
				}
			}
		case "reason":
			if value, ok := value.(string); ok && isAllowedReasonForAction(action, value) {
				out[key] = value
			}
		case "from_state", "to_state":
			if value, ok := value.(string); ok && isPaymentAttemptAuditState(value) {
				out[key] = value
			}
		case "attempt_no":
			if value, ok := scalarPositiveSmallInt(value); ok {
				out[key] = value
			}
		case "late_capture":
			if value, ok := value.(bool); ok {
				out[key] = value
			}
		case "command_type":
			if value == "PAYMENT_CREATE" || value == "PAYMENT_INQUIRY" {
				out[key] = value
			}
		case "requested_method":
			if value, ok := value.(string); ok && (value == "BCA_VA" || value == "QRIS" || value == "CARD") {
				out[key] = value
			}
		case "source_journal_id":
			if value, ok := value.(string); ok {
				if _, err := uuid.Parse(value); err == nil {
					out[key] = value
				}
			}
		case "effective_at":
			if value, ok := value.(string); ok {
				if _, err := time.Parse(time.RFC3339Nano, value); err == nil {
					out[key] = value
				}
			}
		case "posted_journal_id", "void_journal_id":
			if value, ok := value.(string); ok {
				if _, err := uuid.Parse(value); err == nil {
					out[key] = value
				}
			}
		case "write_kind":
			if value, ok := value.(string); ok && allowedLiveWriteKinds[value] {
				out[key] = value
			}
		case "transition":
			if value == "DRAFT_TO_APPROVED" {
				out[key] = value
			}
		case "category":
			if value, ok := value.(string); ok && expenseAuditCategories[value] {
				out[key] = value
			}
		case "amount_rupiah":
			if value, ok := value.(string); ok && value != "" && len(value) <= 19 && expenseAuditDigits.MatchString(value) {
				out[key] = value
			}
		case "currency":
			if value == "IDR" {
				out[key] = value
			}
		case "occurred_at":
			if value, ok := value.(string); ok {
				if _, err := time.Parse(time.RFC3339Nano, value); err == nil {
					out[key] = value
				}
			}
		case "payment_account":
			if value, ok := value.(string); ok && (value == "FUNDING_CLEARING" || value == "ACCOUNTS_PAYABLE") {
				out[key] = value
			}
		case "vendor", "external_reference":
			if value, ok := value.(string); ok && len([]byte(value)) <= 191 && !containsSecret(value) {
				out[key] = value
			}
		}
	}
	return out
}

var expenseAuditCategories = map[string]bool{
	"INFRASTRUCTURE": true, "MARKETING": true, "CUSTOMER_SUPPORT": true,
	"SALARY_CONTRACTOR": true, "LEGAL_COMPLIANCE": true, "PAYMENT_OPERATIONS": true,
	"OFFICE_ADMIN": true, "OTHER": true,
}

var expenseAuditDigits = regexp.MustCompile(`^[0-9]+$`)

func isPaymentAttemptAuditState(value string) bool {
	switch value {
	case "CREATED", "PENDING", "CAPTURED", "FAILED", "EXPIRED", "CANCELLED":
		return true
	default:
		return false
	}
}

func isPaymentReconciliationReason(value string) bool {
	switch value {
	case "LATE_CAPTURE", "REFERENCE_MISMATCH", "AMOUNT_MISMATCH", "CURRENCY_MISMATCH", "INQUIRY_UNRESOLVED", "PROVIDER_CONTRACT_BLOCKED":
		return true
	default:
		return false
	}
}

func isPaymentCommandInvariantReason(value string) bool {
	return value == "ATTEMPT_NOT_FOUND"
}

func isAllowedReasonForAction(action, value string) bool {
	switch action {
	case ActionReconciliationException:
		return isPaymentReconciliationReason(value)
	case ActionPaymentCommandInvariantViolation:
		return isPaymentCommandInvariantReason(value)
	case ActionPlatformExpenseCancelled, ActionPlatformExpenseVoided:
		return strings.TrimSpace(value) != "" &&
			len([]byte(value)) <= 500 &&
			!containsSecret(value)
	default:
		switch value {
		case "LIVE_NOT_ALLOWED", "BPS_OUT_OF_BOUNDS", "OVERLAP",
			"INVALID_TIME", "VALIDATION_ERROR", "CREATE_DISABLED":
			return true
		default:
			return false
		}
	}
}

func scalarPositiveSmallInt(value any) (int, bool) {
	switch v := value.(type) {
	case int:
		return v, v > 0 && v <= math.MaxInt16
	case float64:
		if v <= 0 || v > math.MaxInt16 || math.Trunc(v) != v {
			return 0, false
		}
		return int(v), true
	default:
		return 0, false
	}
}

// SanitizeAuditMetadata applies an action-specific, scalar-only projection to
// legacy owner audit metadata before it crosses the admin read boundary.
func SanitizeAuditMetadata(action string, metadata map[string]any) map[string]any {
	out := make(map[string]any)
	allowedKeys := allowedLegacyMetadataKeysPerAction[action]
	for key, value := range metadata {
		if !allowedKeys[key] || containsSecret(key) || !safeAuditMetadataScalar(value) {
			continue
		}
		out[key] = value
	}
	return out
}

func safeAuditMetadataScalar(value any) bool {
	switch value := value.(type) {
	case nil, bool, int, int64:
		return value != nil
	case float64:
		return !math.IsNaN(value) && !math.IsInf(value, 0)
	case string:
		return len(value) <= 500 && !containsSecret(value)
	default:
		return false
	}
}

func scalarCommissionBPS(value any) (int, bool) {
	switch value := value.(type) {
	case int:
		if value >= 0 && value <= 3000 {
			return value, true
		}
	case int64:
		if value >= 0 && value <= 3000 {
			return int(value), true
		}
	case float64:
		if value >= 0 && value <= 3000 && value == math.Trunc(value) {
			return int(value), true
		}
	}
	return 0, false
}
