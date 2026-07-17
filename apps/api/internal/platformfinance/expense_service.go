package platformfinance

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"lapangango-api/internal/audit"
)

type ExpenseService interface {
	ListExpenses(ctx context.Context, query ExpenseListQuery) (*ExpensePage, error)
	CreateDraft(ctx context.Context, req CreateExpenseRequest, idempotencyKey, actorID, actorRole, ipAddress, userAgent string) (*PlatformExpense, bool, error)
}

type expenseService struct {
	repo         ExpenseRepository
	dbPool       *pgxpool.Pool
	auditService audit.PlatformService
}

func NewExpenseService(repo ExpenseRepository, dbPool *pgxpool.Pool, auditService audit.PlatformService) ExpenseService {
	return &expenseService{repo: repo, dbPool: dbPool, auditService: auditService}
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
