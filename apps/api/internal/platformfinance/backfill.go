package platformfinance

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

var (
	ErrBackfillInvalidInput    = errors.New("invalid backfill input parameters")
	ErrBackfillMissingCutover  = errors.New("cutover record not found or timestamp is null")
	ErrBackfillMultipleCutover = errors.New("multiple cutover records found, which is invalid")
	ErrBackfillIntegrity       = errors.New("cursor integrity failure")
)

// LegacyBackfillQueryer represents the minimal interface required for read-only queries
type LegacyBackfillQueryer interface {
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

type LegacyBackfillBatch struct {
	Count        int
	OnlineCount  int
	OfflineCount int
	NextCursor   *uuid.UUID
	HasMore      bool
}

// LoadStoredCutover fetches the single snapshot_cutover_at from platform_finance_cutovers
func LoadStoredCutover(ctx context.Context, db LegacyBackfillQueryer) (time.Time, error) {
	if db == nil {
		return time.Time{}, ErrBackfillInvalidInput
	}

	var count int
	err := db.QueryRow(ctx, "SELECT COUNT(*) FROM platform_finance_cutovers").Scan(&count)
	if err != nil {
		return time.Time{}, fmt.Errorf("failed to check cutover count: %w", err)
	}
	if count == 0 {
		return time.Time{}, ErrBackfillMissingCutover
	}
	if count > 1 {
		return time.Time{}, ErrBackfillMultipleCutover
	}

	var cutoverAt *time.Time
	err = db.QueryRow(ctx, "SELECT snapshot_cutover_at FROM platform_finance_cutovers LIMIT 1").Scan(&cutoverAt)
	if err != nil {
		return time.Time{}, fmt.Errorf("failed to load cutover timestamp: %w", err)
	}
	if cutoverAt == nil {
		return time.Time{}, ErrBackfillMissingCutover
	}

	return cutoverAt.UTC(), nil
}

// FetchLegacyBackfillCandidates fetches a batch of legacy bookings that don't have fee snapshots
func FetchLegacyBackfillCandidates(ctx context.Context, db LegacyBackfillQueryer, cutoverAt time.Time, batchSize int, afterBookingID *uuid.UUID) (LegacyBackfillBatch, error) {
	if db == nil {
		return LegacyBackfillBatch{}, ErrBackfillInvalidInput
	}
	if cutoverAt.IsZero() {
		return LegacyBackfillBatch{}, ErrBackfillInvalidInput
	}
	if batchSize < 1 || batchSize > 1000 {
		return LegacyBackfillBatch{}, ErrBackfillInvalidInput
	}

	query := `
SELECT
    b.id,
    CASE
        WHEN obc.booking_id IS NULL THEN 'MARKETPLACE_ONLINE'
        ELSE 'OWNER_WALK_IN'
    END AS booking_channel
FROM bookings b
LEFT JOIN offline_booking_customers obc
    ON obc.booking_id = b.id
WHERE b.created_at < $1
  AND ($2::uuid IS NULL OR b.id > $2::uuid)
  AND NOT EXISTS (
      SELECT 1
      FROM booking_fee_snapshots bfs
      WHERE bfs.booking_id = b.id
  )
ORDER BY b.id ASC
LIMIT $3;
`

	// Fetch batchSize + 1 to determine HasMore
	rows, err := db.Query(ctx, query, cutoverAt, afterBookingID, batchSize+1)
	if err != nil {
		return LegacyBackfillBatch{}, fmt.Errorf("query execution failed: %w", err)
	}
	defer rows.Close()

	var batch LegacyBackfillBatch
	var lastID uuid.UUID
	rowCount := 0

	for rows.Next() {
		rowCount++
		var id uuid.UUID
		var channel string

		if err := rows.Scan(&id, &channel); err != nil {
			return LegacyBackfillBatch{}, fmt.Errorf("scan failed: %w", err)
		}

		if rowCount <= batchSize {
			batch.Count++
			if channel == "MARKETPLACE_ONLINE" {
				batch.OnlineCount++
			} else if channel == "OWNER_WALK_IN" {
				batch.OfflineCount++
			} else {
				return LegacyBackfillBatch{}, fmt.Errorf("unknown channel value: %s", channel)
			}
			lastID = id
		} else {
			batch.HasMore = true
		}
	}

	if err := rows.Err(); err != nil {
		return LegacyBackfillBatch{}, fmt.Errorf("row iteration error: %w", err)
	}

	if batch.Count > 0 {
		if afterBookingID != nil && bytes.Compare(lastID[:], (*afterBookingID)[:]) <= 0 {
			return LegacyBackfillBatch{}, ErrBackfillIntegrity
		}
		cursor := lastID
		batch.NextCursor = &cursor
	}

	return batch, nil
}
