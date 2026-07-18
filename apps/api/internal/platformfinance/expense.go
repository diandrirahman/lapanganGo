package platformfinance

import (
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"
)

const (
	ExpenseCurrencyIDR     = "IDR"
	ExpenseDefaultPage     = 1
	ExpenseDefaultLimit    = 20
	ExpenseMaxLimit        = 100
	ExpenseMaxAmountRupiah = int64(1_000_000_000)
	ExpenseBackdateWindow  = 90 * 24 * time.Hour
)

var (
	ErrExpenseValidation  = errors.New("EXPENSE_VALIDATION_ERROR")
	ErrExpenseConflict    = errors.New("EXPENSE_CONFLICT")
	ErrExpenseNotFound    = errors.New("EXPENSE_NOT_FOUND")
	ErrExpensePersistence = errors.New("EXPENSE_PERSISTENCE_FAILED")
	ErrExpenseIntegrity   = errors.New("EXPENSE_INTEGRITY_FAILURE")
	ErrExpenseMissingKey  = errors.New("EXPENSE_IDEMPOTENCY_KEY_REQUIRED")
	ErrExpenseInvalidKey  = errors.New("EXPENSE_IDEMPOTENCY_KEY_INVALID")
)

const ExpenseMaxReasonBytes = 500

var expenseAmountPattern = regexp.MustCompile(`^[0-9]+$`)

var expenseCategories = map[string]bool{
	"INFRASTRUCTURE": true, "MARKETING": true, "CUSTOMER_SUPPORT": true,
	"SALARY_CONTRACTOR": true, "LEGAL_COMPLIANCE": true,
	"PAYMENT_OPERATIONS": true, "OFFICE_ADMIN": true, "OTHER": true,
}

var expensePaymentAccounts = map[string]bool{
	"FUNDING_CLEARING": true,
	"ACCOUNTS_PAYABLE": true,
}

var expenseStatuses = map[string]bool{
	"DRAFT": true, "APPROVED": true, "POSTED": true, "VOID": true, "CANCELLED": true,
}

type CreateExpenseRequest struct {
	AmountRupiah      string `json:"amount_rupiah"`
	Currency          string `json:"currency"`
	OccurredAt        string `json:"occurred_at"`
	Category          string `json:"category"`
	PaymentAccount    string `json:"payment_account"`
	Vendor            string `json:"vendor"`
	ExternalReference string `json:"external_reference"`
	Description       string `json:"description"`
}

type ExpenseCancelRequest struct {
	Reason string `json:"reason"`
}

func normalizeExpenseReason(reason string) (string, error) {
	reason = strings.TrimSpace(reason)
	if reason == "" || len([]byte(reason)) > ExpenseMaxReasonBytes || expenseReasonContainsSecret(reason) {
		return "", &ExpenseValidationError{Fields: map[string]string{"reason": "is required and must be at most 500 bytes"}}
	}
	return reason, nil
}

func expenseReasonContainsSecret(reason string) bool {
	lower := strings.ToLower(reason)
	for _, marker := range []string{"secret", "token", "password", "authorization", "credential", "bearer"} {
		if strings.Contains(lower, marker) {
			return true
		}
	}
	return false
}

type ExpenseListQuery struct {
	Status   string `form:"status"`
	Category string `form:"category"`
	Page     int    `form:"page"`
	Limit    int    `form:"limit"`
}

type PlatformExpense struct {
	ID                string     `json:"id"`
	Category          string     `json:"category"`
	Vendor            *string    `json:"vendor"`
	AmountRupiah      string     `json:"amount_rupiah"`
	Currency          string     `json:"currency"`
	OccurredAt        time.Time  `json:"occurred_at"`
	PaymentAccount    string     `json:"payment_account"`
	ExternalReference *string    `json:"external_reference"`
	Description       string     `json:"description"`
	Status            string     `json:"status"`
	PostedJournalID   *string    `json:"posted_journal_id"`
	VoidJournalID     *string    `json:"void_journal_id"`
	CreatedByUserID   string     `json:"created_by_user_id"`
	ApprovedByUserID  *string    `json:"approved_by_user_id"`
	PostedByUserID    *string    `json:"posted_by_user_id"`
	VoidedByUserID    *string    `json:"voided_by_user_id"`
	CancelledByUserID *string    `json:"cancelled_by_user_id"`
	CancelReason      *string    `json:"cancel_reason"`
	VoidReason        *string    `json:"void_reason"`
	CreatedAt         time.Time  `json:"created_at"`
	ApprovedAt        *time.Time `json:"approved_at"`
	PostedAt          *time.Time `json:"posted_at"`
	VoidedAt          *time.Time `json:"voided_at"`
	CancelledAt       *time.Time `json:"cancelled_at"`
}

type ExpensePage struct {
	Items      []PlatformExpense `json:"data"`
	TotalItems int               `json:"total_items"`
	TotalPages int               `json:"total_pages"`
	Page       int               `json:"page"`
	Limit      int               `json:"limit"`
}

func (r CreateExpenseRequest) NormalizeAndValidate(now time.Time) (normalized CreateExpenseRequest, amount int64, occurredAt time.Time, fieldErrors map[string]string, err error) {
	fieldErrors = make(map[string]string)
	normalized = r
	normalized.AmountRupiah = strings.TrimSpace(r.AmountRupiah)
	normalized.Currency = strings.TrimSpace(r.Currency)
	normalized.Category = strings.ToUpper(strings.TrimSpace(r.Category))
	normalized.PaymentAccount = strings.ToUpper(strings.TrimSpace(r.PaymentAccount))
	normalized.Vendor = strings.TrimSpace(r.Vendor)
	normalized.ExternalReference = strings.TrimSpace(r.ExternalReference)
	normalized.Description = strings.TrimSpace(r.Description)

	if normalized.AmountRupiah == "" || !expenseAmountPattern.MatchString(normalized.AmountRupiah) {
		fieldErrors["amount_rupiah"] = "must be a positive integer string"
	} else if parsed, parseErr := strconv.ParseInt(normalized.AmountRupiah, 10, 64); parseErr != nil || parsed < 1 || parsed > ExpenseMaxAmountRupiah {
		fieldErrors["amount_rupiah"] = fmt.Sprintf("must be between 1 and %d", ExpenseMaxAmountRupiah)
	} else {
		amount = parsed
	}
	if normalized.Currency != ExpenseCurrencyIDR {
		fieldErrors["currency"] = "must be IDR"
	}
	if !expenseCategories[normalized.Category] {
		fieldErrors["category"] = "unsupported expense category"
	}
	if !expensePaymentAccounts[normalized.PaymentAccount] {
		fieldErrors["payment_account"] = "unsupported payment account"
	}
	if normalized.Vendor != "" && len([]byte(normalized.Vendor)) > 160 {
		fieldErrors["vendor"] = "must be at most 160 bytes"
	}
	if normalized.ExternalReference != "" {
		if len([]byte(normalized.ExternalReference)) > 191 {
			fieldErrors["external_reference"] = "must be at most 191 bytes"
		} else if normalized.Vendor == "" {
			fieldErrors["external_reference"] = "requires vendor"
		}
	}
	if normalized.Description == "" || len([]byte(normalized.Description)) > 500 {
		fieldErrors["description"] = "is required and must be at most 500 bytes"
	}
	if normalized.OccurredAt == "" {
		fieldErrors["occurred_at"] = "must be an RFC3339 timestamp"
	} else if parsed, parseErr := time.Parse(time.RFC3339Nano, normalized.OccurredAt); parseErr != nil {
		fieldErrors["occurred_at"] = "must be an RFC3339 timestamp"
	} else {
		occurredAt = parsed
		if occurredAt.After(now) {
			fieldErrors["occurred_at"] = "cannot be future-dated"
		} else if occurredAt.Before(now.Add(-ExpenseBackdateWindow)) {
			fieldErrors["occurred_at"] = "cannot be more than 90 days in the past"
		}
	}
	if len(fieldErrors) > 0 {
		return normalized, amount, occurredAt, fieldErrors, ErrExpenseValidation
	}
	return normalized, amount, occurredAt.UTC().Truncate(time.Microsecond), fieldErrors, nil
}

func normalizeExpenseListQuery(query ExpenseListQuery) (ExpenseListQuery, error) {
	query.Status = strings.ToUpper(strings.TrimSpace(query.Status))
	query.Category = strings.ToUpper(strings.TrimSpace(query.Category))
	if query.Page == 0 {
		query.Page = ExpenseDefaultPage
	}
	if query.Limit == 0 {
		query.Limit = ExpenseDefaultLimit
	}
	if query.Page < 1 || query.Limit < 1 {
		return ExpenseListQuery{}, ErrExpenseValidation
	}
	if query.Limit > ExpenseMaxLimit {
		query.Limit = ExpenseMaxLimit
	}
	if query.Status != "" && !expenseStatuses[query.Status] {
		return ExpenseListQuery{}, ErrExpenseValidation
	}
	if query.Category != "" && !expenseCategories[query.Category] {
		return ExpenseListQuery{}, ErrExpenseValidation
	}
	return query, nil
}
