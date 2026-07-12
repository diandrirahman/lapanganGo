package commercialterms

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

type DBTX interface {
	Exec(context.Context, string, ...any) (pgconn.CommandTag, error)
	Query(context.Context, string, ...any) (pgx.Rows, error)
	QueryRow(context.Context, string, ...any) pgx.Row
}

type Repository interface {
	GetTerms(ctx context.Context, query GetTermsQuery) (PaginatedTermsResponse, error)
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
