package platformfinance

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type JournalReadRepository interface {
	ListJournals(ctx context.Context, query JournalListQuery) (*journalReadRepositoryData, error)
	GetSummary(ctx context.Context, query JournalListQuery) (*journalReadSummaryData, error)
}

type journalReadRepository struct {
	db *pgxpool.Pool
}

func NewJournalReadRepository(db *pgxpool.Pool) JournalReadRepository {
	return &journalReadRepository{db: db}
}

func (r *journalReadRepository) ListJournals(ctx context.Context, query JournalListQuery) (*journalReadRepositoryData, error) {
	if r == nil || r.db == nil {
		return nil, ErrJournalPersistence
	}
	tx, err := r.db.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
	if err != nil {
		return nil, mapJournalReadRepositoryError(err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	cte, args, nextArg := buildJournalReadCTE(query, 1)
	var totalItems, invalidItems int64
	if err := tx.QueryRow(ctx, cte+`
SELECT
	(SELECT COUNT(*) FROM filtered_journals),
	(SELECT COUNT(*) FROM journal_totals
	 WHERE entry_count < 2 OR debit_total_rupiah <> credit_total_rupiah)
`, args...).Scan(&totalItems, &invalidItems); err != nil {
		return nil, mapJournalReadRepositoryError(err)
	}
	if invalidItems > 0 {
		return nil, ErrJournalIntegrity
	}

	offset := (query.Page - 1) * query.Limit
	rows, err := tx.Query(ctx, cte+fmt.Sprintf(`
SELECT
	 fj.id, fj.event_key, fj.event_type,
	 fj.booking_id, fj.owner_profile_id, fj.venue_id, fj.currency,
	 fj.effective_at, fj.posted_at,
	 fj.reverses_journal_id, fj.reversal_reason,
	 reverse_journal.id,
	 jt.entry_count, jt.debit_total_rupiah, jt.credit_total_rupiah
FROM filtered_journals fj
JOIN journal_totals jt ON jt.id = fj.id
LEFT JOIN platform_journals reverse_journal
	ON reverse_journal.reverses_journal_id = fj.id
ORDER BY fj.effective_at DESC, fj.id DESC
LIMIT $%d OFFSET $%d
`, nextArg, nextArg+1), append(args, query.Limit, offset)...)
	if err != nil {
		return nil, mapJournalReadRepositoryError(err)
	}
	defer rows.Close()

	items := make([]journalReadRow, 0, query.Limit)
	for rows.Next() {
		var row journalReadRow
		if err := rows.Scan(
			&row.ID,
			&row.EventKey,
			&row.EventType,
			&row.BookingID,
			&row.OwnerProfileID,
			&row.VenueID,
			&row.Currency,
			&row.EffectiveAt,
			&row.PostedAt,
			&row.ReversesJournalID,
			&row.ReversalReason,
			&row.ReversedByJournalID,
			&row.EntryCount,
			&row.DebitTotalRupiah,
			&row.CreditTotalRupiah,
		); err != nil {
			return nil, mapJournalReadRepositoryError(err)
		}
		if row.EntryCount < 2 || row.DebitTotalRupiah != row.CreditTotalRupiah {
			return nil, ErrJournalIntegrity
		}
		items = append(items, row)
	}
	if err := rows.Err(); err != nil {
		return nil, mapJournalReadRepositoryError(err)
	}
	return &journalReadRepositoryData{Items: items, TotalItems: totalItems}, nil
}

func (r *journalReadRepository) GetSummary(ctx context.Context, query JournalListQuery) (*journalReadSummaryData, error) {
	if r == nil || r.db == nil {
		return nil, ErrJournalPersistence
	}
	tx, err := r.db.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
	if err != nil {
		return nil, mapJournalReadRepositoryError(err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	cte, args, _ := buildJournalReadCTE(query, 1)
	var result journalReadSummaryData
	var invalidItems int64
	err = tx.QueryRow(ctx, cte+`
SELECT
	COUNT(*)::bigint,
	COUNT(*) FILTER (WHERE fj.reverses_journal_id IS NOT NULL)::bigint,
	CAST(COALESCE(SUM(jt.debit_total_rupiah), 0) AS bigint),
	CAST(COALESCE(SUM(jt.credit_total_rupiah), 0) AS bigint),
	COUNT(*) FILTER (WHERE jt.entry_count < 2 OR jt.debit_total_rupiah <> jt.credit_total_rupiah)::bigint
FROM filtered_journals fj
JOIN journal_totals jt ON jt.id = fj.id
`, args...).Scan(
		&result.JournalCount,
		&result.ReversalCount,
		&result.TotalDebitRupiah,
		&result.TotalCreditRupiah,
		&invalidItems,
	)
	if err != nil {
		return nil, mapJournalReadRepositoryError(err)
	}
	if invalidItems > 0 {
		return nil, ErrJournalIntegrity
	}
	return &result, nil
}

func buildJournalReadCTE(query JournalListQuery, firstArg int) (string, []any, int) {
	clauses := []string{"TRUE"}
	args := make([]any, 0, 7)
	nextArg := firstArg
	add := func(clause string, value any) {
		clauses = append(clauses, fmt.Sprintf(clause, nextArg))
		args = append(args, value)
		nextArg++
	}
	if query.EffectiveFrom != nil {
		add("j.effective_at >= $%d", *query.EffectiveFrom)
	}
	if query.EffectiveTo != nil {
		add("j.effective_at < $%d", *query.EffectiveTo)
	}
	if query.EventType != "" {
		add("j.event_type = $%d", query.EventType)
	}
	if query.AccountCode != "" {
		add("EXISTS (SELECT 1 FROM platform_ledger_entries account_filter WHERE account_filter.journal_id = j.id AND account_filter.account_code = $%d)", query.AccountCode)
	}
	if query.JournalID != "" {
		add("j.id = $%d", query.JournalID)
	}
	if query.OwnerProfileID != "" {
		add("j.owner_profile_id = $%d", query.OwnerProfileID)
	}
	if query.VenueID != "" {
		add("j.venue_id = $%d", query.VenueID)
	}
	if query.BookingID != "" {
		add("j.booking_id = $%d", query.BookingID)
	}
	return `WITH filtered_journals AS (
	SELECT j.*
	FROM platform_journals j
	WHERE ` + strings.Join(clauses, " AND ") + `
), journal_totals AS (
	SELECT
		fj.id,
		COUNT(e.id)::bigint AS entry_count,
		CAST(COALESCE(SUM(e.amount_rupiah) FILTER (WHERE e.side = 'DEBIT'), 0) AS bigint) AS debit_total_rupiah,
		CAST(COALESCE(SUM(e.amount_rupiah) FILTER (WHERE e.side = 'CREDIT'), 0) AS bigint) AS credit_total_rupiah
	FROM filtered_journals fj
	LEFT JOIN platform_ledger_entries e ON e.journal_id = fj.id
	GROUP BY fj.id
)
`, args, nextArg
}

func mapJournalReadRepositoryError(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return err
	}
	return mapJournalRepositoryError(err)
}
