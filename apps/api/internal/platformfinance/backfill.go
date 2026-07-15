package platformfinance

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"math/big"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

var (
	ErrBackfillInvalidInput       = errors.New("invalid backfill input parameters")
	ErrBackfillMissingCutover     = errors.New("cutover record not found or timestamp is null")
	ErrBackfillMultipleCutover    = errors.New("multiple cutover records found, which is invalid")
	ErrBackfillIntegrity          = errors.New("cursor integrity failure")
	ErrBackfillSnapshotConflict   = errors.New("snapshot idempotent conflict mismatch")
	ErrBackfillCalculationInvalid = errors.New("calculation error or constraint violation")
)

type LegacyBackfillQueryer interface {
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

type LegacyBackfillCandidate struct {
	BookingID             uuid.UUID
	BookingCreatedAt      time.Time
	OwnerProfileID        uuid.UUID
	VenueID               uuid.UUID
	BookingChannel        string
	BookingOriginalPrice  pgtype.Numeric
	BookingFinalPrice     pgtype.Numeric
	BookingTotalPrice     pgtype.Numeric
	BookingDiscountAmount pgtype.Numeric
	OfflineSystemPrice    pgtype.Numeric
	OfflineFinalPrice     pgtype.Numeric
	OfflineOverrideReason *string
	PromoID               *uuid.UUID
	PromoCode             *string
}

type LegacyBackfillBatch struct {
	Count        int
	OnlineCount  int
	OfflineCount int
	NextCursor   *uuid.UUID
	HasMore      bool
	Candidates   []LegacyBackfillCandidate
}

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

func FetchLegacyBackfillCandidates(ctx context.Context, db LegacyBackfillQueryer, cutoverAt time.Time, batchSize int, afterBookingID *uuid.UUID, lockRows bool) (LegacyBackfillBatch, error) {
	if db == nil {
		return LegacyBackfillBatch{}, ErrBackfillInvalidInput
	}
	if cutoverAt.IsZero() {
		return LegacyBackfillBatch{}, ErrBackfillInvalidInput
	}
	if batchSize < 1 || batchSize > 1000 {
		return LegacyBackfillBatch{}, ErrBackfillInvalidInput
	}

	// Fetch candidates with required joins for full DTO.
	// Only fetch EXACTLY batchSize. If we get batchSize rows, HasMore is true.
	// Apply mode locks booking rows here, then locks and verifies offline price
	// rows after this candidate result set has been consumed and closed.
	query := `
SELECT
    b.id,
    b.created_at,
    v.owner_profile_id,
    c.venue_id,
    CASE
        WHEN obc.booking_id IS NULL THEN 'MARKETPLACE_ONLINE'
        ELSE 'OWNER_WALK_IN'
    END AS booking_channel,
    b.original_price,
    b.final_price,
    b.total_price,
    b.discount_amount,
    obc.system_price,
    obc.final_price AS offline_final_price,
    obc.price_override_reason,
    b.promo_id,
    b.promo_code
FROM bookings b
JOIN courts c ON b.court_id = c.id
JOIN venues v ON c.venue_id = v.id
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
LIMIT $3
`
	if lockRows {
		query += " FOR SHARE OF b"
	}

	rows, err := db.Query(ctx, query, cutoverAt, afterBookingID, batchSize)
	if err != nil {
		return LegacyBackfillBatch{}, fmt.Errorf("query execution failed: %w", err)
	}
	defer rows.Close()

	var batch LegacyBackfillBatch
	var lastID uuid.UUID

	for rows.Next() {
		var cand LegacyBackfillCandidate

		if err := rows.Scan(
			&cand.BookingID,
			&cand.BookingCreatedAt,
			&cand.OwnerProfileID,
			&cand.VenueID,
			&cand.BookingChannel,
			&cand.BookingOriginalPrice,
			&cand.BookingFinalPrice,
			&cand.BookingTotalPrice,
			&cand.BookingDiscountAmount,
			&cand.OfflineSystemPrice,
			&cand.OfflineFinalPrice,
			&cand.OfflineOverrideReason,
			&cand.PromoID,
			&cand.PromoCode,
		); err != nil {
			return LegacyBackfillBatch{}, fmt.Errorf("scan failed: %w", err)
		}

		if cand.BookingChannel == "MARKETPLACE_ONLINE" {
			batch.OnlineCount++
		} else if cand.BookingChannel == "OWNER_WALK_IN" {
			batch.OfflineCount++
		} else {
			return LegacyBackfillBatch{}, fmt.Errorf("unknown channel value: %s", cand.BookingChannel)
		}

		batch.Candidates = append(batch.Candidates, cand)
		lastID = cand.BookingID
		batch.Count++
	}

	if err := rows.Err(); err != nil {
		return LegacyBackfillBatch{}, fmt.Errorf("row iteration error: %w", err)
	}
	rows.Close()

	if lockRows && batch.OfflineCount > 0 {
		for _, cand := range batch.Candidates {
			if cand.BookingChannel == "OWNER_WALK_IN" {
				var lockedSystem, lockedFinal pgtype.Numeric
				var lockedReason pgtype.Text
				err := db.QueryRow(ctx, "SELECT system_price, final_price, price_override_reason FROM offline_booking_customers WHERE booking_id = $1 FOR SHARE", cand.BookingID).Scan(&lockedSystem, &lockedFinal, &lockedReason)
				if err != nil {
					if err == pgx.ErrNoRows {
						return LegacyBackfillBatch{}, fmt.Errorf("%w: missing offline pricing record for booking %s", ErrBackfillIntegrity, cand.BookingID.String())
					}
					return LegacyBackfillBatch{}, fmt.Errorf("failed to query offline_booking_customers: %w", err)
				}
				// Verify exactly matches the candidate
				sysMatch, errSys := equalExactRupiah(cand.OfflineSystemPrice, lockedSystem)
				if errSys != nil || !sysMatch {
					return LegacyBackfillBatch{}, fmt.Errorf("%w: offline pricing record changed during lock for booking %s", ErrBackfillIntegrity, cand.BookingID.String())
				}

				finMatch, errFin := equalExactRupiah(cand.OfflineFinalPrice, lockedFinal)
				if errFin != nil || !finMatch {
					return LegacyBackfillBatch{}, fmt.Errorf("%w: offline pricing record changed during lock for booking %s", ErrBackfillIntegrity, cand.BookingID.String())
				}

				if !equalNullableText(cand.OfflineOverrideReason, lockedReason) {
					return LegacyBackfillBatch{}, fmt.Errorf("%w: offline pricing reason changed during lock for booking %s", ErrBackfillIntegrity, cand.BookingID.String())
				}
			}
		}
	}

	if batch.Count > 0 {
		if afterBookingID != nil && bytes.Compare(lastID[:], (*afterBookingID)[:]) <= 0 {
			return LegacyBackfillBatch{}, ErrBackfillIntegrity
		}
		cursor := lastID
		batch.NextCursor = &cursor
	}

	if batch.Count == batchSize {
		batch.HasMore = true
	}

	return batch, nil
}

func equalExactRupiah(a pgtype.Numeric, b pgtype.Numeric) (bool, error) {
	if a.Valid != b.Valid {
		return false, nil
	}
	if !a.Valid {
		return true, nil
	}

	aValue, err := parseNumericExact(a)
	if err != nil {
		return false, err
	}

	bValue, err := parseNumericExact(b)
	if err != nil {
		return false, err
	}

	return aValue == bValue, nil
}

func equalNullableText(candidate *string, locked pgtype.Text) bool {
	candValid := candidate != nil
	if candValid != locked.Valid {
		return false
	}
	if !candValid {
		return true
	}
	return *candidate == locked.String
}

func parseNumericExact(n pgtype.Numeric) (int64, error) {
	if !n.Valid || n.NaN || n.InfinityModifier != pgtype.Finite || n.Int == nil {
		return 0, fmt.Errorf("%w: numeric value is invalid or null", ErrBackfillCalculationInvalid)
	}

	v, err := n.Int64Value()
	if err != nil {
		return 0, fmt.Errorf("%w: cannot convert to int64: %v", ErrBackfillCalculationInvalid, err)
	}
	if !v.Valid {
		return 0, fmt.Errorf("%w: cannot convert to int64, invalid value", ErrBackfillCalculationInvalid)
	}

	val := v.Int64
	if n.Exp > 0 {
		multiplier := new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(n.Exp)), nil)
		res := new(big.Int).Mul(n.Int, multiplier)
		if !res.IsInt64() {
			return 0, fmt.Errorf("%w: positive exponent overflow", ErrBackfillCalculationInvalid)
		}
		val = res.Int64()
	}

	if val < 0 || val > 9999999999 {
		return 0, fmt.Errorf("%w: price out of bounds: %d", ErrBackfillCalculationInvalid, val)
	}
	return val, nil
}

func CalculateLegacySnapshot(c LegacyBackfillCandidate, cutoverAt time.Time) (BookingFeeSnapshot, error) {
	if c.OwnerProfileID == uuid.Nil {
		return BookingFeeSnapshot{}, fmt.Errorf("%w: empty OwnerProfileID", ErrBackfillCalculationInvalid)
	}
	if c.VenueID == uuid.Nil {
		return BookingFeeSnapshot{}, fmt.Errorf("%w: empty VenueID", ErrBackfillCalculationInvalid)
	}
	if !c.BookingCreatedAt.Before(cutoverAt) {
		return BookingFeeSnapshot{}, fmt.Errorf("%w: post-cutover candidate", ErrBackfillCalculationInvalid)
	}

	var parsedOnlineOriginal, parsedOnlineFinal, parsedOnlineTotal, parsedOnlineDiscount *int64
	var parsedOfflineSystem, parsedOfflineFinal *int64

	// Parse all numeric fields that are valid
	if c.BookingOriginalPrice.Valid {
		val, err := parseNumericExact(c.BookingOriginalPrice)
		if err != nil {
			return BookingFeeSnapshot{}, fmt.Errorf("BookingOriginalPrice invalid: %w", err)
		}
		parsedOnlineOriginal = &val
	}
	if c.BookingFinalPrice.Valid {
		val, err := parseNumericExact(c.BookingFinalPrice)
		if err != nil {
			return BookingFeeSnapshot{}, fmt.Errorf("BookingFinalPrice invalid: %w", err)
		}
		parsedOnlineFinal = &val
	}
	if c.BookingTotalPrice.Valid {
		val, err := parseNumericExact(c.BookingTotalPrice)
		if err != nil {
			return BookingFeeSnapshot{}, fmt.Errorf("BookingTotalPrice invalid: %w", err)
		}
		parsedOnlineTotal = &val
	}
	if c.BookingDiscountAmount.Valid {
		val, err := parseNumericExact(c.BookingDiscountAmount)
		if err != nil {
			return BookingFeeSnapshot{}, fmt.Errorf("BookingDiscountAmount invalid: %w", err)
		}
		parsedOnlineDiscount = &val
	}
	if c.OfflineSystemPrice.Valid {
		val, err := parseNumericExact(c.OfflineSystemPrice)
		if err != nil {
			return BookingFeeSnapshot{}, fmt.Errorf("OfflineSystemPrice invalid: %w", err)
		}
		parsedOfflineSystem = &val
	}
	if c.OfflineFinalPrice.Valid {
		val, err := parseNumericExact(c.OfflineFinalPrice)
		if err != nil {
			return BookingFeeSnapshot{}, fmt.Errorf("OfflineFinalPrice invalid: %w", err)
		}
		parsedOfflineFinal = &val
	}

	var originalPrice, finalPrice int64

	// Extract and validate base prices depending on channel and availability
	if c.BookingChannel == "MARKETPLACE_ONLINE" {
		if parsedOnlineOriginal != nil {
			originalPrice = *parsedOnlineOriginal
		} else if parsedOnlineTotal != nil {
			originalPrice = *parsedOnlineTotal
		} else {
			return BookingFeeSnapshot{}, fmt.Errorf("%w: missing original or total price for online booking", ErrBackfillCalculationInvalid)
		}

		if parsedOnlineFinal != nil {
			finalPrice = *parsedOnlineFinal
		} else if parsedOnlineTotal != nil {
			finalPrice = *parsedOnlineTotal
		} else {
			return BookingFeeSnapshot{}, fmt.Errorf("%w: missing final or total price for online booking", ErrBackfillCalculationInvalid)
		}

		// Cross-field validation for online
		if parsedOnlineOriginal != nil && parsedOnlineFinal != nil && parsedOnlineDiscount != nil && parsedOnlineTotal != nil {
			if finalPrice != originalPrice-(*parsedOnlineDiscount) {
				return BookingFeeSnapshot{}, fmt.Errorf("%w: online price math mismatch (final != original - discount)", ErrBackfillCalculationInvalid)
			}
			if *parsedOnlineTotal != finalPrice {
				return BookingFeeSnapshot{}, fmt.Errorf("%w: online price math mismatch (total != final)", ErrBackfillCalculationInvalid)
			}
		}
	} else if c.BookingChannel == "OWNER_WALK_IN" {
		// Fallback order: offline sources first, then booking sources
		if parsedOfflineSystem != nil {
			originalPrice = *parsedOfflineSystem
		} else if parsedOnlineOriginal != nil {
			originalPrice = *parsedOnlineOriginal
		} else if parsedOnlineTotal != nil {
			originalPrice = *parsedOnlineTotal
		} else {
			return BookingFeeSnapshot{}, fmt.Errorf("%w: missing original price source for offline booking", ErrBackfillCalculationInvalid)
		}

		if parsedOfflineFinal != nil {
			finalPrice = *parsedOfflineFinal
		} else if parsedOnlineFinal != nil {
			finalPrice = *parsedOnlineFinal
		} else if parsedOnlineTotal != nil {
			finalPrice = *parsedOnlineTotal
		} else {
			return BookingFeeSnapshot{}, fmt.Errorf("%w: missing final price source for offline booking", ErrBackfillCalculationInvalid)
		}

		// Cross-field validation for offline
		if parsedOfflineSystem != nil && parsedOnlineOriginal != nil {
			if originalPrice != *parsedOnlineOriginal {
				return BookingFeeSnapshot{}, fmt.Errorf("%w: offline system price != booking original price", ErrBackfillCalculationInvalid)
			}
		}
		if parsedOfflineFinal != nil && parsedOnlineTotal != nil {
			if finalPrice != *parsedOnlineTotal {
				return BookingFeeSnapshot{}, fmt.Errorf("%w: offline final price != booking total price", ErrBackfillCalculationInvalid)
			}
		}
	} else {
		return BookingFeeSnapshot{}, fmt.Errorf("%w: unknown channel", ErrBackfillCalculationInvalid)
	}

	if originalPrice < 0 || originalPrice > 9999999999 {
		return BookingFeeSnapshot{}, fmt.Errorf("%w: original price out of bounds: %d", ErrBackfillCalculationInvalid, originalPrice)
	}
	if finalPrice <= 0 || finalPrice > 9999999999 {
		return BookingFeeSnapshot{}, fmt.Errorf("%w: final price out of bounds: %d", ErrBackfillCalculationInvalid, finalPrice)
	}

	adjustment := finalPrice - originalPrice
	var reason *string

	if adjustment != 0 {
		if c.BookingChannel == "OWNER_WALK_IN" {
			if c.OfflineOverrideReason == nil {
				return BookingFeeSnapshot{}, fmt.Errorf("%w: missing offline price override reason for adjustment", ErrBackfillCalculationInvalid)
			}
			trimmed := strings.TrimSpace(*c.OfflineOverrideReason)
			if trimmed == "" {
				return BookingFeeSnapshot{}, fmt.Errorf("%w: empty offline price override reason for adjustment", ErrBackfillCalculationInvalid)
			}
			if utf8.RuneCountInString(trimmed) > 1024 {
				trimmed = string([]rune(trimmed)[:1024])
			}
			reason = &trimmed
		} else {
			if c.PromoCode == nil {
				return BookingFeeSnapshot{}, fmt.Errorf("%w: missing promo code for online adjustment", ErrBackfillCalculationInvalid)
			}
			trimmed := strings.TrimSpace(*c.PromoCode)
			if trimmed == "" {
				return BookingFeeSnapshot{}, fmt.Errorf("%w: empty promo code for online adjustment", ErrBackfillCalculationInvalid)
			}
			res := fmt.Sprintf("PROMO:%s", strings.ToUpper(trimmed))
			if utf8.RuneCountInString(res) > 255 {
				res = string([]rune(res)[:255])
			}
			reason = &res
		}
	}

	snap := BookingFeeSnapshot{
		BookingID:                   c.BookingID.String(),
		OwnerProfileID:              c.OwnerProfileID.String(),
		VenueID:                     c.VenueID.String(),
		CommercialTermID:            nil,
		TermsSource:                 "LEGACY_NO_COMMISSION",
		BookingChannel:              BookingChannel(c.BookingChannel),
		FinanceMode:                 "SIMULATION",
		Currency:                    "IDR",
		CurrencyExponent:            0,
		OriginalPriceRupiah:         originalPrice,
		OwnerPriceAdjustmentRupiah:  adjustment,
		FinalBookingPriceRupiah:     finalPrice,
		CustomerServiceFeeRupiah:    0,
		CustomerChargeAmountRupiah:  finalPrice,
		CommissionBasisAmountRupiah: finalPrice,
		CommissionBps:               0,
		CommissionAmountRupiah:      0,
		OwnerNetAmountRupiah:        finalPrice,
		CalculationVersion:          "legacy-backfill-v1",
		PriceAdjustmentReason:       reason,
		CreatedAt:                   time.Now().UTC(),
	}

	return snap, nil
}
