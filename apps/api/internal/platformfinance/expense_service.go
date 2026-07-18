package platformfinance

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"lapangango-api/internal/audit"
)

type ExpenseService interface {
	ListExpenses(ctx context.Context, query ExpenseListQuery) (*ExpensePage, error)
	CreateDraft(ctx context.Context, req CreateExpenseRequest, idempotencyKey, actorID, actorRole, ipAddress, userAgent string) (*PlatformExpense, bool, error)
	CancelDraft(ctx context.Context, expenseID, reason, idempotencyKey, actorID, actorRole, ipAddress, userAgent string) (*PlatformExpense, bool, error)
	ApproveDraft(ctx context.Context, expenseID, idempotencyKey, actorID, actorRole, ipAddress, userAgent string) (*PlatformExpense, bool, error)
	PostExpense(ctx context.Context, expenseID, idempotencyKey, actorID, actorRole, ipAddress, userAgent string) (*PlatformExpense, bool, error)
	VoidExpense(ctx context.Context, expenseID, reason, idempotencyKey, actorID, actorRole, ipAddress, userAgent string) (*PlatformExpense, bool, error)
}

type expenseService struct {
	repo         ExpenseRepository
	dbPool       *pgxpool.Pool
	auditService audit.PlatformService
	journal      JournalService
}

func NewExpenseService(repo ExpenseRepository, dbPool *pgxpool.Pool, auditService audit.PlatformService, journalServices ...JournalService) ExpenseService {
	var journal JournalService
	if len(journalServices) > 0 {
		journal = journalServices[0]
	}
	return &expenseService{repo: repo, dbPool: dbPool, auditService: auditService, journal: journal}
}

func (s *expenseService) ListExpenses(ctx context.Context, query ExpenseListQuery) (*ExpensePage, error) {
	normalized, err := normalizeExpenseListQuery(query)
	if err != nil {
		return nil, err
	}
	return s.repo.ListExpenses(ctx, normalized)
}

func (s *expenseService) CreateDraft(ctx context.Context, req CreateExpenseRequest, idempotencyKey, actorID, actorRole, ipAddress, userAgent string) (*PlatformExpense, bool, error) {
	idempotencyKey = strings.TrimSpace(idempotencyKey)
	if idempotencyKey == "" {
		return nil, false, ErrExpenseMissingKey
	}
	if len(idempotencyKey) > 255 {
		return nil, false, ErrExpenseInvalidKey
	}
	if strings.TrimSpace(actorID) == "" {
		return nil, false, ErrExpenseValidation
	}
	if s == nil || s.dbPool == nil || s.repo == nil || s.auditService == nil {
		return nil, false, ErrExpensePersistence
	}

	normalized, amount, occurredAt, fieldErrors, err := req.NormalizeAndValidate(time.Now())
	if err != nil {
		return nil, false, &ExpenseValidationError{Fields: fieldErrors}
	}
	requestHash := expenseRequestHash(normalized)
	tx, err := s.dbPool.Begin(ctx)
	if err != nil {
		return nil, false, err
	}
	defer tx.Rollback(ctx)
	if err := s.repo.LockIdempotency(ctx, tx, actorID, "CREATE", idempotencyKey); err != nil {
		return nil, false, err
	}
	record, err := s.repo.GetIdempotency(ctx, tx, actorID, "CREATE", idempotencyKey)
	if err != nil {
		return nil, false, err
	}
	if record != nil {
		if record.RequestHash != requestHash {
			return nil, false, ErrExpenseConflict
		}
		var item PlatformExpense
		if err := json.Unmarshal(record.ResponseBody, &item); err != nil || item.ID == "" {
			return nil, false, ErrExpenseIntegrity
		}
		return &item, true, nil
	}

	item, err := s.repo.CreateDraft(ctx, tx, actorID, normalized, amount, occurredAt)
	if err != nil {
		return nil, false, err
	}
	if err := s.auditService.Record(ctx, tx, audit.CreatePlatformAuditLogParams{
		ActorUserID: &actorID, ActorRole: actorRole, Action: audit.ActionPlatformExpenseCreated,
		EntityType: audit.EntityPlatformExpense, EntityID: &item.ID, CorrelationID: &idempotencyKey,
		Metadata: map[string]any{
			"category": item.Category, "amount_rupiah": item.AmountRupiah, "currency": item.Currency,
			"occurred_at": item.OccurredAt.Format(time.RFC3339Nano), "payment_account": item.PaymentAccount,
			"vendor": optionalExpenseAuditValue(item.Vendor), "external_reference": optionalExpenseAuditValue(item.ExternalReference),
		}, IPAddress: expenseStringPointer(ipAddress), UserAgent: expenseStringPointer(userAgent),
	}); err != nil {
		return nil, false, err
	}
	responseBody, err := json.Marshal(item)
	if err != nil {
		return nil, false, err
	}
	if err := s.repo.InsertIdempotency(ctx, tx, actorID, "CREATE", idempotencyKey, requestHash, item.ID, 201, responseBody); err != nil {
		return nil, false, err
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, false, err
	}
	return item, false, nil
}

type expenseActionFingerprint struct {
	ExpenseID string `json:"expense_id"`
	Reason    string `json:"reason,omitempty"`
}

func expenseActionHash(expenseID, reason string) string {
	payload, _ := json.Marshal(expenseActionFingerprint{ExpenseID: expenseID, Reason: reason})
	digest := sha256.Sum256(payload)
	return hex.EncodeToString(digest[:])
}

func (s *expenseService) validateActionInputs(expenseID, idempotencyKey, actorID string) (string, error) {
	idempotencyKey = strings.TrimSpace(idempotencyKey)
	if idempotencyKey == "" {
		return "", ErrExpenseMissingKey
	}
	if len(idempotencyKey) > 255 {
		return "", ErrExpenseInvalidKey
	}
	if strings.TrimSpace(actorID) == "" || strings.TrimSpace(expenseID) == "" {
		return "", ErrExpenseValidation
	}
	if _, err := uuid.Parse(expenseID); err != nil {
		return "", ErrExpenseValidation
	}
	if s == nil || s.dbPool == nil || s.repo == nil || s.auditService == nil {
		return "", ErrExpensePersistence
	}
	return idempotencyKey, nil
}

func (s *expenseService) replayExpenseAction(record *ExpenseIdempotencyRecord, requestHash, expenseID string) (*PlatformExpense, bool, error) {
	if record.RequestHash != requestHash {
		return nil, false, ErrExpenseConflict
	}
	var item PlatformExpense
	if err := json.Unmarshal(record.ResponseBody, &item); err != nil || item.ID == "" || item.ID != expenseID {
		return nil, false, ErrExpenseIntegrity
	}
	return &item, true, nil
}

func (s *expenseService) CancelDraft(ctx context.Context, expenseID, reason, idempotencyKey, actorID, actorRole, ipAddress, userAgent string) (*PlatformExpense, bool, error) {
	var err error
	reason, err = normalizeExpenseReason(reason)
	if err != nil {
		return nil, false, err
	}
	idempotencyKey, err = s.validateActionInputs(expenseID, idempotencyKey, actorID)
	if err != nil {
		return nil, false, err
	}
	requestHash := expenseActionHash(expenseID, reason)
	tx, err := s.dbPool.Begin(ctx)
	if err != nil {
		return nil, false, err
	}
	defer tx.Rollback(ctx)
	if err := s.repo.LockIdempotency(ctx, tx, actorID, "CANCEL", idempotencyKey); err != nil {
		return nil, false, err
	}
	record, err := s.repo.GetIdempotency(ctx, tx, actorID, "CANCEL", idempotencyKey)
	if err != nil {
		return nil, false, err
	}
	if record != nil {
		return s.replayExpenseAction(record, requestHash, expenseID)
	}
	locked, err := s.repo.GetExpenseForUpdate(ctx, tx, expenseID)
	if err != nil {
		return nil, false, err
	}
	if locked.Status != "DRAFT" {
		return nil, false, ErrExpenseConflict
	}
	item, err := s.repo.CancelDraft(ctx, tx, expenseID, actorID, reason)
	if err != nil {
		return nil, false, err
	}
	if err := s.auditService.Record(ctx, tx, audit.CreatePlatformAuditLogParams{
		ActorUserID: &actorID, ActorRole: actorRole, Action: audit.ActionPlatformExpenseCancelled,
		EntityType: audit.EntityPlatformExpense, EntityID: &item.ID, CorrelationID: &idempotencyKey,
		Metadata: map[string]any{"reason": reason}, IPAddress: expenseStringPointer(ipAddress), UserAgent: expenseStringPointer(userAgent),
	}); err != nil {
		return nil, false, err
	}
	responseBody, err := json.Marshal(item)
	if err != nil {
		return nil, false, err
	}
	if err := s.repo.InsertIdempotency(ctx, tx, actorID, "CANCEL", idempotencyKey, requestHash, item.ID, 200, responseBody); err != nil {
		return nil, false, err
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, false, err
	}
	return item, false, nil
}

func (s *expenseService) ApproveDraft(ctx context.Context, expenseID, idempotencyKey, actorID, actorRole, ipAddress, userAgent string) (*PlatformExpense, bool, error) {
	var err error
	idempotencyKey, err = s.validateActionInputs(expenseID, idempotencyKey, actorID)
	if err != nil {
		return nil, false, err
	}
	requestHash := expenseActionHash(expenseID, "")
	tx, err := s.dbPool.Begin(ctx)
	if err != nil {
		return nil, false, err
	}
	defer tx.Rollback(ctx)
	if err := s.repo.LockIdempotency(ctx, tx, actorID, "APPROVE", idempotencyKey); err != nil {
		return nil, false, err
	}
	record, err := s.repo.GetIdempotency(ctx, tx, actorID, "APPROVE", idempotencyKey)
	if err != nil {
		return nil, false, err
	}
	if record != nil {
		return s.replayExpenseAction(record, requestHash, expenseID)
	}
	locked, err := s.repo.GetExpenseForUpdate(ctx, tx, expenseID)
	if err != nil {
		return nil, false, err
	}
	if locked.Status != "DRAFT" {
		return nil, false, ErrExpenseConflict
	}
	databaseNow, err := s.repo.DatabaseNow(ctx, tx)
	if err != nil {
		return nil, false, err
	}
	if locked.OccurredAt.After(databaseNow) {
		return nil, false, &ExpenseValidationError{Fields: map[string]string{"occurred_at": "cannot be future-dated"}}
	}
	item, err := s.repo.ApproveDraft(ctx, tx, expenseID, actorID)
	if err != nil {
		return nil, false, err
	}
	if err := s.auditService.Record(ctx, tx, audit.CreatePlatformAuditLogParams{
		ActorUserID: &actorID, ActorRole: actorRole, Action: audit.ActionPlatformExpenseApproved,
		EntityType: audit.EntityPlatformExpense, EntityID: &item.ID, CorrelationID: &idempotencyKey,
		Metadata: map[string]any{"transition": "DRAFT_TO_APPROVED"}, IPAddress: expenseStringPointer(ipAddress), UserAgent: expenseStringPointer(userAgent),
	}); err != nil {
		return nil, false, err
	}
	responseBody, err := json.Marshal(item)
	if err != nil {
		return nil, false, err
	}
	if err := s.repo.InsertIdempotency(ctx, tx, actorID, "APPROVE", idempotencyKey, requestHash, item.ID, 200, responseBody); err != nil {
		return nil, false, err
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, false, err
	}
	return item, false, nil
}

func (s *expenseService) PostExpense(ctx context.Context, expenseID, idempotencyKey, actorID, actorRole, ipAddress, userAgent string) (*PlatformExpense, bool, error) {
	if s == nil || s.journal == nil {
		return nil, false, ErrExpensePersistence
	}
	var err error
	idempotencyKey, err = s.validateActionInputs(expenseID, idempotencyKey, actorID)
	if err != nil {
		return nil, false, err
	}
	requestHash := expenseActionHash(expenseID, "")
	tx, err := s.dbPool.Begin(ctx)
	if err != nil {
		return nil, false, err
	}
	defer tx.Rollback(ctx)
	if err := s.repo.LockIdempotency(ctx, tx, actorID, "POST", idempotencyKey); err != nil {
		return nil, false, err
	}
	record, err := s.repo.GetIdempotency(ctx, tx, actorID, "POST", idempotencyKey)
	if err != nil {
		return nil, false, err
	}
	if record != nil {
		return s.replayExpenseAction(record, requestHash, expenseID)
	}
	locked, err := s.repo.GetExpenseForUpdate(ctx, tx, expenseID)
	if err != nil {
		return nil, false, err
	}
	if locked.Status != "APPROVED" {
		return nil, false, ErrExpenseConflict
	}
	databaseNow, err := s.repo.DatabaseNow(ctx, tx)
	if err != nil {
		return nil, false, err
	}
	if locked.OccurredAt.After(databaseNow) {
		return nil, false, &ExpenseValidationError{Fields: map[string]string{"occurred_at": "cannot be future-dated"}}
	}
	amount, err := parseExpenseAmount(locked.AmountRupiah)
	if err != nil {
		return nil, false, err
	}
	postedJournal, err := s.journal.PostJournal(ctx, tx, PostJournalParams{
		EventKey:        "expense.posted:" + locked.ID,
		EventType:       "PLATFORM_EXPENSE_POSTED",
		EffectiveAt:     locked.OccurredAt,
		CreatedByUserID: &actorID,
		Description:     expenseDescriptionPointer(locked.Description),
		Metadata: map[string]string{
			"source_type":      "PLATFORM_EXPENSE",
			"source_reference": locked.ID,
		},
		Entries: []PostJournalEntry{
			{AccountCode: "OPEX_" + locked.Category, Side: JournalSideDebit, AmountRupiah: amount},
			{AccountCode: locked.PaymentAccount, Side: JournalSideCredit, AmountRupiah: amount},
		},
	})
	if err != nil {
		return nil, false, err
	}
	item, err := s.repo.PostApproved(ctx, tx, locked.ID, postedJournal.ID, actorID)
	if err != nil {
		return nil, false, err
	}
	if err := s.auditService.Record(ctx, tx, audit.CreatePlatformAuditLogParams{
		ActorUserID: &actorID, ActorRole: actorRole, Action: audit.ActionPlatformExpensePosted,
		EntityType: audit.EntityPlatformExpense, EntityID: &item.ID, CorrelationID: &idempotencyKey,
		Metadata: map[string]any{
			"category": locked.Category, "amount_rupiah": locked.AmountRupiah, "currency": locked.Currency,
			"occurred_at": locked.OccurredAt.Format(time.RFC3339Nano), "payment_account": locked.PaymentAccount,
			"posted_journal_id": postedJournal.ID,
		}, IPAddress: expenseStringPointer(ipAddress), UserAgent: expenseStringPointer(userAgent),
	}); err != nil {
		return nil, false, err
	}
	responseBody, err := json.Marshal(item)
	if err != nil {
		return nil, false, err
	}
	if err := s.repo.InsertIdempotency(ctx, tx, actorID, "POST", idempotencyKey, requestHash, item.ID, 200, responseBody); err != nil {
		return nil, false, err
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, false, err
	}
	return item, false, nil
}

func (s *expenseService) VoidExpense(ctx context.Context, expenseID, reason, idempotencyKey, actorID, actorRole, ipAddress, userAgent string) (*PlatformExpense, bool, error) {
	if s == nil || s.journal == nil {
		return nil, false, ErrExpensePersistence
	}
	var err error
	reason, err = normalizeExpenseReason(reason)
	if err != nil {
		return nil, false, err
	}
	idempotencyKey, err = s.validateActionInputs(expenseID, idempotencyKey, actorID)
	if err != nil {
		return nil, false, err
	}
	requestHash := expenseActionHash(expenseID, reason)
	tx, err := s.dbPool.Begin(ctx)
	if err != nil {
		return nil, false, err
	}
	defer tx.Rollback(ctx)
	if err := s.repo.LockIdempotency(ctx, tx, actorID, "VOID", idempotencyKey); err != nil {
		return nil, false, err
	}
	record, err := s.repo.GetIdempotency(ctx, tx, actorID, "VOID", idempotencyKey)
	if err != nil {
		return nil, false, err
	}
	if record != nil {
		return s.replayExpenseAction(record, requestHash, expenseID)
	}
	locked, err := s.repo.GetExpenseForUpdate(ctx, tx, expenseID)
	if err != nil {
		return nil, false, err
	}
	if locked.Status != "POSTED" || locked.PostedJournalID == nil {
		return nil, false, ErrExpenseConflict
	}
	voidedAt, err := s.repo.DatabaseNow(ctx, tx)
	if err != nil {
		return nil, false, err
	}
	reversal, err := s.journal.ReverseJournal(ctx, tx, ReverseJournalParams{
		JournalID:       *locked.PostedJournalID,
		Reason:          reason,
		EffectiveAt:     voidedAt,
		CreatedByUserID: &actorID,
		Metadata: map[string]string{
			"source_type":      "PLATFORM_EXPENSE_VOID",
			"source_reference": locked.ID,
		},
	})
	if err != nil {
		return nil, false, err
	}
	item, err := s.repo.VoidPosted(ctx, tx, locked.ID, reversal.ID, actorID, reason, voidedAt)
	if err != nil {
		return nil, false, err
	}
	if err := s.auditService.Record(ctx, tx, audit.CreatePlatformAuditLogParams{
		ActorUserID: &actorID, ActorRole: actorRole, Action: audit.ActionPlatformExpenseVoided,
		EntityType: audit.EntityPlatformExpense, EntityID: &item.ID, CorrelationID: &idempotencyKey,
		Metadata: map[string]any{
			"reason": reason, "source_journal_id": *locked.PostedJournalID,
			"void_journal_id": reversal.ID, "effective_at": reversal.EffectiveAt.Format(time.RFC3339Nano),
		}, IPAddress: expenseStringPointer(ipAddress), UserAgent: expenseStringPointer(userAgent),
	}); err != nil {
		return nil, false, err
	}
	if err := s.recordJournalReversalAudit(ctx, tx, reversal, *locked.PostedJournalID, actorID, actorRole, ipAddress, userAgent); err != nil {
		return nil, false, err
	}
	responseBody, err := json.Marshal(item)
	if err != nil {
		return nil, false, err
	}
	if err := s.repo.InsertIdempotency(ctx, tx, actorID, "VOID", idempotencyKey, requestHash, item.ID, 200, responseBody); err != nil {
		return nil, false, err
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, false, err
	}
	return item, false, nil
}

func parseExpenseAmount(value string) (int64, error) {
	amount, err := strconv.ParseInt(value, 10, 64)
	if err != nil || amount < 1 || amount > ExpenseMaxAmountRupiah {
		return 0, ErrExpenseIntegrity
	}
	return amount, nil
}

func expenseDescriptionPointer(value string) *string {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}
	return &value
}

func (s *expenseService) recordJournalReversalAudit(ctx context.Context, tx pgx.Tx, reversal *PostedJournal, sourceJournalID, actorID, actorRole, ipAddress, userAgent string) error {
	correlationID := reversal.EventKey
	marker, err := findFinanceAuditMarker(ctx, tx, audit.ActionPlatformFinanceJournalReversed, correlationID)
	if err != nil {
		return err
	}
	if marker != nil {
		return validateReversalAuditMarker(marker, reversal, sourceJournalID, correlationID)
	}
	entityID := reversal.ID
	correlation := correlationID
	return s.auditService.Record(ctx, tx, audit.CreatePlatformAuditLogParams{
		ActorUserID:    &actorID,
		ActorRole:      actorRole,
		Action:         audit.ActionPlatformFinanceJournalReversed,
		EntityType:     audit.EntityPlatformFinanceJournal,
		EntityID:       &entityID,
		OwnerProfileID: reversal.OwnerProfileID,
		VenueID:        reversal.VenueID,
		CorrelationID:  &correlation,
		Metadata: map[string]any{
			"source_journal_id": sourceJournalID,
			"effective_at":      reversal.EffectiveAt.UTC().Format(time.RFC3339Nano),
		},
		IPAddress: expenseStringPointer(ipAddress),
		UserAgent: expenseStringPointer(userAgent),
	})
}

type ExpenseValidationError struct{ Fields map[string]string }

func (e *ExpenseValidationError) Error() string { return ErrExpenseValidation.Error() }

func expenseRequestHash(req CreateExpenseRequest) string {
	payload, _ := json.Marshal(req)
	digest := sha256.Sum256(payload)
	return hex.EncodeToString(digest[:])
}

func optionalExpenseAuditValue(value *string) any {
	if value == nil {
		return ""
	}
	return *value
}

func expenseStringPointer(value string) *string {
	if value == "" {
		return nil
	}
	return &value
}
