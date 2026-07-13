package platformfinance

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

type CommercialTerm struct {
	ID               string
	OwnerProfileID   *string
	Label            string
	Phase            string
	FinanceMode      string
	CollectionMethod string
	CommissionBps    int
	ValidFrom        time.Time
	ValidUntil       *time.Time
}

var (
	ErrInvalidCommercialTermOwner           = errors.New("invalid commercial term owner")
	ErrInvalidCommercialTermTimestamp       = errors.New("invalid commercial term timestamp")
	ErrMissingEffectiveCommercialTerm       = errors.New("missing effective commercial term")
	ErrDuplicateCommercialTerm              = errors.New("duplicate commercial term")
	ErrUnsupportedCommercialTermFinanceMode = errors.New("unsupported commercial term finance mode")
	ErrInvalidResolvedCommercialTerm        = errors.New("invalid resolved commercial term")
)

type CommercialTermQueryer interface {
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
}

type CommercialTermResolver interface {
	ResolveEffectiveTerm(ctx context.Context, ownerProfileID string, bookingAt time.Time) (*CommercialTerm, error)
}

type commercialTermResolver struct {
	db CommercialTermQueryer
}

func NewCommercialTermResolver(db CommercialTermQueryer) CommercialTermResolver {
	return &commercialTermResolver{db: db}
}

func (r *commercialTermResolver) ResolveEffectiveTerm(ctx context.Context, ownerProfileID string, bookingAt time.Time) (*CommercialTerm, error) {
	if _, err := uuid.Parse(ownerProfileID); err != nil {
		return nil, ErrInvalidCommercialTermOwner
	}
	if bookingAt.IsZero() {
		return nil, ErrInvalidCommercialTermTimestamp
	}

	q := `
		SELECT
			id::text,
			owner_profile_id::text,
			label,
			phase,
			finance_mode,
			collection_method,
			commission_bps,
			valid_from,
			valid_until
		FROM platform_commercial_terms
		WHERE (owner_profile_id = $1 OR owner_profile_id IS NULL)
		  AND valid_from <= $2
		  AND (valid_until IS NULL OR valid_until > $2)
		ORDER BY
		  CASE WHEN owner_profile_id IS NULL THEN 1 ELSE 0 END,
		  valid_from DESC,
		  created_at DESC,
		  id DESC
	`

	rows, err := r.db.Query(ctx, q, ownerProfileID, bookingAt.UTC())
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var ownerTerms []*CommercialTerm
	var globalTerms []*CommercialTerm

	for rows.Next() {
		var t CommercialTerm
		if err := rows.Scan(
			&t.ID,
			&t.OwnerProfileID,
			&t.Label,
			&t.Phase,
			&t.FinanceMode,
			&t.CollectionMethod,
			&t.CommissionBps,
			&t.ValidFrom,
			&t.ValidUntil,
		); err != nil {
			return nil, err
		}
		if t.OwnerProfileID != nil && *t.OwnerProfileID == ownerProfileID {
			ownerTerms = append(ownerTerms, &t)
		} else if t.OwnerProfileID == nil {
			globalTerms = append(globalTerms, &t)
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	if len(ownerTerms) > 1 || len(globalTerms) > 1 {
		return nil, ErrDuplicateCommercialTerm
	}

	var winner *CommercialTerm
	if len(ownerTerms) == 1 {
		winner = ownerTerms[0]
	} else if len(globalTerms) == 1 {
		winner = globalTerms[0]
	} else {
		return nil, ErrMissingEffectiveCommercialTerm
	}

	if winner.FinanceMode != "SIMULATION" {
		return nil, ErrUnsupportedCommercialTermFinanceMode
	}

	if winner.CollectionMethod != "NONE" {
		return nil, ErrInvalidResolvedCommercialTerm
	}

	if winner.Phase != "TRIAL" && winner.Phase != "INTRODUCTORY" && winner.Phase != "STANDARD" && winner.Phase != "CUSTOM" {
		return nil, ErrInvalidResolvedCommercialTerm
	}

	if winner.CommissionBps < 0 || winner.CommissionBps > 3000 {
		return nil, ErrInvalidResolvedCommercialTerm
	}

	if winner.ValidUntil != nil && !winner.ValidUntil.After(winner.ValidFrom) {
		return nil, ErrInvalidResolvedCommercialTerm
	}

	return winner, nil
}
