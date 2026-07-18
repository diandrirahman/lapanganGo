package platformfinance

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

type ExpenseDBTX interface {
	Exec(context.Context, string, ...any) (pgconn.CommandTag, error)
	Query(context.Context, string, ...any) (pgx.Rows, error)
	QueryRow(context.Context, string, ...any) pgx.Row
}

type ExpenseIdempotencyRecord struct {
	RequestHash    string
	ExpenseID      string
	ResponseStatus int
	ResponseBody   []byte
}

type ExpenseRepository interface {
	ListExpenses(ctx context.Context, query ExpenseListQuery) (*ExpensePage, error)
	LockIdempotency(ctx context.Context, db ExpenseDBTX, actorID, action, key string) error
	GetIdempotency(ctx context.Context, db ExpenseDBTX, actorID, action, key string) (*ExpenseIdempotencyRecord, error)
	CreateDraft(ctx context.Context, db ExpenseDBTX, actorID string, req CreateExpenseRequest, amount int64, occurredAt time.Time) (*PlatformExpense, error)
	GetExpenseForUpdate(ctx context.Context, db ExpenseDBTX, expenseID string) (*PlatformExpense, error)
	DatabaseNow(ctx context.Context, db ExpenseDBTX) (time.Time, error)
	CancelDraft(ctx context.Context, db ExpenseDBTX, expenseID, actorID, reason string) (*PlatformExpense, error)
	ApproveDraft(ctx context.Context, db ExpenseDBTX, expenseID, actorID string) (*PlatformExpense, error)
	InsertIdempotency(ctx context.Context, db ExpenseDBTX, actorID, action, key, requestHash, expenseID string, responseStatus int, responseBody []byte) error
}

type expenseRepository struct{ db *pgxpool.Pool }

func NewExpenseRepository(db *pgxpool.Pool) ExpenseRepository { return &expenseRepository{db: db} }

func (r *expenseRepository) ListExpenses(ctx context.Context, query ExpenseListQuery) (*ExpensePage, error) {
	if r == nil || r.db == nil {
		return nil, ErrExpensePersistence
	}
	offset := (query.Page - 1) * query.Limit
	where := []string{"1=1"}
	args := make([]any, 0, 4)
	if query.Status != "" {
		args = append(args, query.Status)
		where = append(where, fmt.Sprintf("status = $%d", len(args)))
	}
	if query.Category != "" {
		args = append(args, query.Category)
		where = append(where, fmt.Sprintf("category = $%d", len(args)))
	}
	args = append(args, query.Limit, offset)
	limitIndex, offsetIndex := len(args)-1, len(args)
	rows, err := r.db.Query(ctx, fmt.Sprintf(`
		SELECT id, category, vendor, amount_rupiah, currency, occurred_at,
		       payment_account, external_reference, description, status,
		       posted_journal_id, void_journal_id, created_by_user_id,
		       approved_by_user_id, posted_by_user_id, voided_by_user_id,
		       cancelled_by_user_id, cancel_reason, void_reason, created_at,
		       approved_at, posted_at, voided_at, cancelled_at,
		       COUNT(*) OVER() AS total_items
		FROM platform_expenses
		WHERE %s
		ORDER BY occurred_at DESC, created_at DESC, id DESC
		LIMIT $%d OFFSET $%d`, strings.Join(where, " AND "), limitIndex, offsetIndex), args...)
	if err != nil {
		return nil, mapExpenseRepositoryError(err)
	}
	defer rows.Close()
	page := &ExpensePage{Items: make([]PlatformExpense, 0, query.Limit), Page: query.Page, Limit: query.Limit}
	for rows.Next() {
		item, total, scanErr := scanPlatformExpense(rows)
		if scanErr != nil {
			return nil, mapExpenseRepositoryError(scanErr)
		}
		page.Items = append(page.Items, item)
		page.TotalItems = total
	}
	if err := rows.Err(); err != nil {
		return nil, mapExpenseRepositoryError(err)
	}
	if page.TotalItems > 0 {
		page.TotalPages = (page.TotalItems + query.Limit - 1) / query.Limit
	}
	return page, nil
}

func (r *expenseRepository) LockIdempotency(ctx context.Context, db ExpenseDBTX, actorID, action, key string) error {
	_, err := db.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1, 0))`, actorID+":"+action+":"+key)
	return mapExpenseRepositoryError(err)
}

func (r *expenseRepository) GetIdempotency(ctx context.Context, db ExpenseDBTX, actorID, action, key string) (*ExpenseIdempotencyRecord, error) {
	var record ExpenseIdempotencyRecord
	err := db.QueryRow(ctx, `
		SELECT request_hash, expense_id, response_status, response_body
		FROM platform_expense_idempotency
		WHERE actor_user_id = $1 AND action = $2 AND idempotency_key = $3
		FOR UPDATE`, actorID, action, key).Scan(&record.RequestHash, &record.ExpenseID, &record.ResponseStatus, &record.ResponseBody)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, mapExpenseRepositoryError(err)
	}
	return &record, nil
}

func (r *expenseRepository) CreateDraft(ctx context.Context, db ExpenseDBTX, actorID string, req CreateExpenseRequest, amount int64, occurredAt time.Time) (*PlatformExpense, error) {
	row := db.QueryRow(ctx, `
		INSERT INTO platform_expenses (
			category, vendor, amount_rupiah, currency, occurred_at, payment_account,
			external_reference, description, status, created_by_user_id
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,'DRAFT',$9)
		RETURNING id, category, vendor, amount_rupiah, currency, occurred_at,
		          payment_account, external_reference, description, status,
		          posted_journal_id, void_journal_id, created_by_user_id,
		          approved_by_user_id, posted_by_user_id, voided_by_user_id,
		          cancelled_by_user_id, cancel_reason, void_reason, created_at,
		          approved_at, posted_at, voided_at, cancelled_at, 0::bigint`,
		req.Category, nullableExpenseString(req.Vendor), amount, req.Currency, occurredAt,
		req.PaymentAccount, nullableExpenseString(req.ExternalReference), req.Description, actorID)
	item, _, err := scanPlatformExpense(row)
	if err != nil {
		return nil, mapExpenseRepositoryError(err)
	}
	return &item, nil
}

const expenseSelectColumns = `id, category, vendor, amount_rupiah, currency, occurred_at,
       payment_account, external_reference, description, status,
       posted_journal_id, void_journal_id, created_by_user_id,
       approved_by_user_id, posted_by_user_id, voided_by_user_id,
       cancelled_by_user_id, cancel_reason, void_reason, created_at,
       approved_at, posted_at, voided_at, cancelled_at, 0::bigint`

func (r *expenseRepository) GetExpenseForUpdate(ctx context.Context, db ExpenseDBTX, expenseID string) (*PlatformExpense, error) {
	row := db.QueryRow(ctx, `SELECT `+expenseSelectColumns+` FROM platform_expenses WHERE id = $1 FOR UPDATE`, expenseID)
	item, _, err := scanPlatformExpense(row)
	if err != nil {
		return nil, mapExpenseRepositoryError(err)
	}
	return &item, nil
}

func (r *expenseRepository) DatabaseNow(ctx context.Context, db ExpenseDBTX) (time.Time, error) {
	var now time.Time
	if err := db.QueryRow(ctx, `SELECT clock_timestamp()`).Scan(&now); err != nil {
		return time.Time{}, mapExpenseRepositoryError(err)
	}
	return now, nil
}

func (r *expenseRepository) CancelDraft(ctx context.Context, db ExpenseDBTX, expenseID, actorID, reason string) (*PlatformExpense, error) {
	row := db.QueryRow(ctx, `
		UPDATE platform_expenses
		SET status = 'CANCELLED', cancelled_at = clock_timestamp(), cancelled_by_user_id = $2, cancel_reason = $3
		WHERE id = $1 AND status = 'DRAFT'
		RETURNING `+expenseSelectColumns, expenseID, actorID, reason)
	item, _, err := scanPlatformExpense(row)
	if err != nil {
		return nil, mapExpenseRepositoryError(err)
	}
	return &item, nil
}

func (r *expenseRepository) ApproveDraft(ctx context.Context, db ExpenseDBTX, expenseID, actorID string) (*PlatformExpense, error) {
	row := db.QueryRow(ctx, `
		UPDATE platform_expenses
		SET status = 'APPROVED', approved_at = clock_timestamp(), approved_by_user_id = $2
		WHERE id = $1 AND status = 'DRAFT'
		RETURNING `+expenseSelectColumns, expenseID, actorID)
	item, _, err := scanPlatformExpense(row)
	if err != nil {
		return nil, mapExpenseRepositoryError(err)
	}
	return &item, nil
}

func (r *expenseRepository) InsertIdempotency(ctx context.Context, db ExpenseDBTX, actorID, action, key, requestHash, expenseID string, responseStatus int, responseBody []byte) error {
	_, err := db.Exec(ctx, `
		INSERT INTO platform_expense_idempotency (
			actor_user_id, action, idempotency_key, request_hash, expense_id, response_status, response_body
		) VALUES ($1,$2,$3,$4,$5,$6,$7::jsonb)`, actorID, action, key, requestHash, expenseID, responseStatus, string(responseBody))
	return mapExpenseRepositoryError(err)
}

type expenseRowScanner interface{ Scan(dest ...any) error }

func scanPlatformExpense(row expenseRowScanner) (PlatformExpense, int, error) {
	var item PlatformExpense
	var amount int64
	var total int64
	err := row.Scan(
		&item.ID, &item.Category, &item.Vendor, &amount, &item.Currency, &item.OccurredAt,
		&item.PaymentAccount, &item.ExternalReference, &item.Description, &item.Status,
		&item.PostedJournalID, &item.VoidJournalID, &item.CreatedByUserID,
		&item.ApprovedByUserID, &item.PostedByUserID, &item.VoidedByUserID,
		&item.CancelledByUserID, &item.CancelReason, &item.VoidReason, &item.CreatedAt,
		&item.ApprovedAt, &item.PostedAt, &item.VoidedAt, &item.CancelledAt, &total,
	)
	if err != nil {
		return PlatformExpense{}, 0, err
	}
	if amount < 1 || amount > ExpenseMaxAmountRupiah || total < 0 || total > int64(^uint(0)>>1) {
		return PlatformExpense{}, 0, ErrExpenseIntegrity
	}
	item.AmountRupiah = strconv.FormatInt(amount, 10)
	return item, int(total), nil
}

func nullableExpenseString(value string) *string {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}
	return &value
}

func mapExpenseRepositoryError(err error) error {
	if err == nil {
		return nil
	}
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		if pgErr.Code == "23505" {
			return ErrExpenseConflict
		}
		if pgErr.Code == "23514" || pgErr.Code == "22003" {
			return ErrExpenseValidation
		}
	}
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrExpenseNotFound
	}
	return err
}
