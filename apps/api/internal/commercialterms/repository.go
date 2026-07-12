package commercialterms

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"lapangango-api/internal/audit"
)

type DBTX interface {
	Exec(context.Context, string, ...any) (pgconn.CommandTag, error)
	Query(context.Context, string, ...any) (pgx.Rows, error)
	QueryRow(context.Context, string, ...any) pgx.Row
}

type Repository interface {
	GetTerms(ctx context.Context, query GetTermsQuery) (PaginatedTermsResponse, error)
	GetAuditByCorrelationID(ctx context.Context, db DBTX, correlationID string, action string) (map[string]any, string, error)
	GetOpenEndedTermByScope(ctx context.Context, db DBTX, scopeKey string) (*CommercialTerm, error)
	GetOwnerForShare(ctx context.Context, db DBTX, ownerID string) error
	SupersedeTerm(ctx context.Context, db DBTX, id string, validUntil time.Time) error
	CreateTerm(ctx context.Context, db DBTX, term *CommercialTerm) error
	LockIdempotency(ctx context.Context, db DBTX, key string) error
	LockScope(ctx context.Context, db DBTX, scopeKey string) error
	GetTransactionTimestamp(ctx context.Context, db DBTX) (time.Time, error)
	GetTermByID(ctx context.Context, db DBTX, id string) (CommercialTerm, error)
}

type repository struct {
	db DBTX
}

func NewRepository(db DBTX) Repository {
	return &repository{db: db}
}

func (r *repository) GetTerms(ctx context.Context, query GetTermsQuery) (PaginatedTermsResponse, error) {
	page := query.Page
	if page < 1 {
		page = 1
	}
	limit := query.Limit
	if limit < 1 {
		limit = 10
	}
	offset := (page - 1) * limit

	asOf := time.Now()

	whereClause := "1=1"
	whereArgs := []any{}

	if query.Scope == "GLOBAL" {
		whereClause += " AND owner_profile_id IS NULL"
	} else if query.Scope == "OWNER" {
		whereClause += " AND owner_profile_id IS NOT NULL"
		if query.OwnerProfileID != "" {
			whereArgs = append(whereArgs, query.OwnerProfileID)
			whereClause += fmt.Sprintf(" AND owner_profile_id = $%d", len(whereArgs))
		}
	}

	if query.Status != "" {
		whereArgs = append(whereArgs, asOf)
		nowIdx := len(whereArgs)
		if query.Status == "CURRENT" {
			whereClause += fmt.Sprintf(" AND valid_from <= $%d AND (valid_until > $%d OR valid_until IS NULL)", nowIdx, nowIdx)
		} else if query.Status == "SCHEDULED" {
			whereClause += fmt.Sprintf(" AND valid_from > $%d", nowIdx)
		} else if query.Status == "HISTORICAL" {
			whereClause += fmt.Sprintf(" AND valid_until <= $%d", nowIdx)
		}
	}

	countQuery := fmt.Sprintf("SELECT count(*) FROM platform_commercial_terms WHERE %s", whereClause)
	var totalItems int
	err := r.db.QueryRow(ctx, countQuery, whereArgs...).Scan(&totalItems)
	if err != nil {
		return PaginatedTermsResponse{}, err
	}

	totalPages := (totalItems + limit - 1) / limit
	if totalPages == 0 {
		totalPages = 1
	}

	selectArgs := make([]any, len(whereArgs))
	copy(selectArgs, whereArgs)

	selectArgs = append(selectArgs, asOf)
	nowParamForSelect := fmt.Sprintf("$%d", len(selectArgs))

	// Determine status inline in SELECT to populate the DTO status field correctly
	statusExpr := fmt.Sprintf(`
		CASE
			WHEN valid_until <= %[1]s THEN 'HISTORICAL'
			WHEN valid_from > %[1]s THEN 'SCHEDULED'
			ELSE 'CURRENT'
		END
	`, nowParamForSelect)

	selectArgs = append(selectArgs, limit, offset)
	limitParam := fmt.Sprintf("$%d", len(selectArgs)-1)
	offsetParam := fmt.Sprintf("$%d", len(selectArgs))

	selectQuery := fmt.Sprintf(`
		SELECT
			id, owner_profile_id, scope_key, label, phase, finance_mode, collection_method, commission_bps, valid_from, valid_until, supersedes_id, created_by_user_id, created_at,
			%s AS status
		FROM platform_commercial_terms
		WHERE %s
		ORDER BY valid_from DESC, created_at DESC, id DESC
		LIMIT %s OFFSET %s
	`, statusExpr, whereClause, limitParam, offsetParam)

	rows, err := r.db.Query(ctx, selectQuery, selectArgs...)
	if err != nil {
		return PaginatedTermsResponse{}, err
	}
	defer rows.Close()

	terms := make([]CommercialTerm, 0)
	for rows.Next() {
		var t CommercialTerm
		var createdBy *string
		err := rows.Scan(
			&t.ID,
			&t.OwnerProfileID,
			&t.ScopeKey,
			&t.Label,
			&t.Phase,
			&t.FinanceMode,
			&t.CollectionMethod,
			&t.CommissionBps,
			&t.ValidFrom,
			&t.ValidUntil,
			&t.SupersedesID,
			&createdBy,
			&t.CreatedAt,
			&t.Status,
		)
		if err != nil {
			return PaginatedTermsResponse{}, err
		}
		if createdBy != nil {
			t.CreatedByUserID = *createdBy
		}
		terms = append(terms, t)
	}

	if err := rows.Err(); err != nil {
		return PaginatedTermsResponse{}, err
	}

	return PaginatedTermsResponse{
		Data:       terms,
		TotalItems: totalItems,
		TotalPages: totalPages,
		Page:       page,
		Limit:      limit,
	}, nil
}

func (r *repository) GetAuditByCorrelationID(ctx context.Context, db DBTX, correlationID string, action string) (map[string]any, string, error) {
	rows, err := db.Query(ctx, "SELECT metadata, entity_id FROM platform_audit_logs WHERE correlation_id = $1 AND action = $2 AND entity_type = 'PLATFORM_COMMERCIAL_TERM'", correlationID, action)
	if err != nil {
		return nil, "", err
	}
	defer rows.Close()

	var metadataJSON []byte
	var entityID *string
	count := 0

	for rows.Next() {
		count++
		if count > 1 {
			return nil, "", errors.New("integrity error: multiple audit logs found for the same idempotency key")
		}
		if err := rows.Scan(&metadataJSON, &entityID); err != nil {
			return nil, "", err
		}
	}

	if err := rows.Err(); err != nil {
		return nil, "", err
	}

	if count == 0 {
		return nil, "", nil
	}

	if entityID == nil {
		if action == audit.ActionPlatformCommercialTermLiveRejected {
			var metadata map[string]any
			if err := json.Unmarshal(metadataJSON, &metadata); err != nil {
				return nil, "", err
			}
			return metadata, "", nil
		}
		return nil, "", errors.New("integrity error: audit log found but entity ID is null")
	}

	var metadata map[string]any
	if err := json.Unmarshal(metadataJSON, &metadata); err != nil {
		return nil, "", err
	}

	return metadata, *entityID, nil
}

func (r *repository) GetOpenEndedTermByScope(ctx context.Context, db DBTX, scopeKey string) (*CommercialTerm, error) {
	var t CommercialTerm
	var createdBy *string
	err := db.QueryRow(ctx, `
		SELECT id, owner_profile_id, scope_key, label, phase, finance_mode, collection_method, commission_bps, valid_from, valid_until, supersedes_id, created_by_user_id, created_at
		FROM platform_commercial_terms
		WHERE scope_key = $1 AND valid_until IS NULL
		FOR UPDATE
	`, scopeKey).Scan(
		&t.ID, &t.OwnerProfileID, &t.ScopeKey, &t.Label, &t.Phase, &t.FinanceMode, &t.CollectionMethod, &t.CommissionBps, &t.ValidFrom, &t.ValidUntil, &t.SupersedesID, &createdBy, &t.CreatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	if createdBy != nil {
		t.CreatedByUserID = *createdBy
	}
	return &t, nil
}

func (r *repository) GetOwnerForShare(ctx context.Context, db DBTX, ownerID string) error {
	var id string
	err := db.QueryRow(ctx, "SELECT id FROM owner_profiles WHERE id = $1 FOR KEY SHARE", ownerID).Scan(&id)
	return err
}

func (r *repository) SupersedeTerm(ctx context.Context, db DBTX, id string, validUntil time.Time) error {
	tag, err := db.Exec(ctx, "UPDATE platform_commercial_terms SET valid_until = $2 WHERE id = $1 AND valid_until IS NULL", id, validUntil)
	if err != nil {
		return err
	}
	if tag.RowsAffected() != 1 {
		return errors.New("failed to supersede term: expected 1 row affected")
	}
	return nil
}

func (r *repository) CreateTerm(ctx context.Context, db DBTX, term *CommercialTerm) error {
	var createdBy *string
	if term.CreatedByUserID != "" {
		createdBy = &term.CreatedByUserID
	}
	query := `
		INSERT INTO platform_commercial_terms (
			id, owner_profile_id, label, phase, finance_mode, collection_method, commission_bps, valid_from, valid_until, supersedes_id, created_by_user_id
		) VALUES (
			$1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11
		)
		RETURNING created_at, scope_key
	`
	err := db.QueryRow(ctx, query,
		term.ID, term.OwnerProfileID, term.Label, term.Phase, term.FinanceMode, term.CollectionMethod, term.CommissionBps, term.ValidFrom, term.ValidUntil, term.SupersedesID, createdBy,
	).Scan(&term.CreatedAt, &term.ScopeKey)
	return err
}

func (r *repository) LockIdempotency(ctx context.Context, db DBTX, key string) error {
	_, err := db.Exec(ctx, "SELECT pg_advisory_xact_lock(hashtextextended('commercialterms:idempotency:' || $1, 0))", key)
	return err
}

func (r *repository) LockScope(ctx context.Context, db DBTX, scopeKey string) error {
	_, err := db.Exec(ctx, "SELECT pg_advisory_xact_lock(hashtextextended('commercialterms:scope:' || $1, 0))", scopeKey)
	return err
}

func (r *repository) GetTransactionTimestamp(ctx context.Context, db DBTX) (time.Time, error) {
	var ts time.Time
	err := db.QueryRow(ctx, "SELECT transaction_timestamp()").Scan(&ts)
	return ts, err
}

func (r *repository) GetTermByID(ctx context.Context, db DBTX, id string) (CommercialTerm, error) {
	var t CommercialTerm
	var createdBy *string
	err := db.QueryRow(ctx, `
		SELECT id, owner_profile_id, scope_key, label, phase, finance_mode, collection_method, commission_bps, valid_from, valid_until, supersedes_id, created_by_user_id, created_at,
		CASE
			WHEN valid_until <= now() THEN 'HISTORICAL'
			WHEN valid_from > now() THEN 'SCHEDULED'
			ELSE 'CURRENT'
		END AS status
		FROM platform_commercial_terms
		WHERE id = $1
	`, id).Scan(
		&t.ID, &t.OwnerProfileID, &t.ScopeKey, &t.Label, &t.Phase, &t.FinanceMode, &t.CollectionMethod, &t.CommissionBps, &t.ValidFrom, &t.ValidUntil, &t.SupersedesID, &createdBy, &t.CreatedAt, &t.Status,
	)
	if err != nil {
		return CommercialTerm{}, err
	}
	if createdBy != nil {
		t.CreatedByUserID = *createdBy
	}
	return t, nil
}
