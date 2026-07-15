package platformfinance

import (
	"bytes"
	"context"
	"errors"
	"math/big"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

// Classification
type PostCutoverP0Classification string

const (
	ClassificationRepairablePolicyOnline PostCutoverP0Classification = "REPAIRABLE_POLICY_ONLINE"
	ClassificationRepairablePolicyWalkIn PostCutoverP0Classification = "REPAIRABLE_POLICY_WALK_IN"
	ClassificationManualDecisionRequired PostCutoverP0Classification = "MANUAL_DECISION_REQUIRED"
)

// Reason
type PostCutoverP0Reason string

const (
	ReasonMissingEffectiveTerm        PostCutoverP0Reason = "MISSING_EFFECTIVE_TERM"
	ReasonDuplicateEffectiveTerm      PostCutoverP0Reason = "DUPLICATE_EFFECTIVE_TERM"
	ReasonUnsupportedFinanceMode      PostCutoverP0Reason = "UNSUPPORTED_FINANCE_MODE"
	ReasonUnsupportedCollectionMethod PostCutoverP0Reason = "UNSUPPORTED_COLLECTION_METHOD"
	ReasonInvalidTermPhase            PostCutoverP0Reason = "INVALID_TERM_PHASE"
	ReasonInvalidCommissionBps        PostCutoverP0Reason = "INVALID_COMMISSION_BPS"
	ReasonInvalidResolvedTerm         PostCutoverP0Reason = "INVALID_RESOLVED_TERM"

	ReasonMissingCourtReference PostCutoverP0Reason = "MISSING_COURT_REFERENCE"
	ReasonMissingVenueReference PostCutoverP0Reason = "MISSING_VENUE_REFERENCE"
	ReasonMissingOwnerReference PostCutoverP0Reason = "MISSING_OWNER_REFERENCE"
	ReasonUnknownBookingChannel PostCutoverP0Reason = "UNKNOWN_BOOKING_CHANNEL"

	ReasonMissingOfflineFact         PostCutoverP0Reason = "MISSING_OFFLINE_FACT"
	ReasonDuplicateOfflineFact       PostCutoverP0Reason = "DUPLICATE_OFFLINE_FACT"
	ReasonOfflineSystemPriceMismatch PostCutoverP0Reason = "OFFLINE_SYSTEM_PRICE_MISMATCH"
	ReasonOfflineFinalPriceMismatch  PostCutoverP0Reason = "OFFLINE_FINAL_PRICE_MISMATCH"

	ReasonMissingMoneyValue    PostCutoverP0Reason = "MISSING_MONEY_VALUE"
	ReasonFractionalMoneyValue PostCutoverP0Reason = "FRACTIONAL_MONEY_VALUE"
	ReasonNegativeMoneyValue   PostCutoverP0Reason = "NEGATIVE_MONEY_VALUE"
	ReasonMoneyOverflow        PostCutoverP0Reason = "MONEY_OVERFLOW"

	ReasonDiscountArithmeticMismatch PostCutoverP0Reason = "DISCOUNT_ARITHMETIC_MISMATCH"
	ReasonBookingTotalFinalMismatch  PostCutoverP0Reason = "BOOKING_TOTAL_FINAL_MISMATCH"
	ReasonPromoFactMissing           PostCutoverP0Reason = "PROMO_FACT_MISSING"
	ReasonOnlinePositiveAdjustment   PostCutoverP0Reason = "ONLINE_POSITIVE_ADJUSTMENT"
	ReasonAdjustmentWithoutReason    PostCutoverP0Reason = "ADJUSTMENT_WITHOUT_REASON"
	ReasonCalculatorResultMismatch   PostCutoverP0Reason = "CALCULATOR_RESULT_MISMATCH"
)

// Params & Results
type PostCutoverDetectorParams struct {
	CutoverAt      time.Time
	AfterBookingID *uuid.UUID
	BatchSize      int
}

type PostCutoverClassificationResult struct {
	Classification   PostCutoverP0Classification
	Reason           PostCutoverP0Reason
	OperationalError error
}

type PostCutoverCandidate struct {
	ID             uuid.UUID
	CreatedAt      time.Time
	CourtID        *uuid.UUID
	VenueID        *uuid.UUID
	OwnerProfileID *uuid.UUID

	OriginalPrice  pgtype.Numeric
	FinalPrice     pgtype.Numeric
	TotalPrice     pgtype.Numeric
	DiscountAmount pgtype.Numeric

	HasOfflineRecord   bool
	OfflineRowCount    int
	OfflineSystemPrice pgtype.Numeric
	OfflineFinalPrice  pgtype.Numeric
	HasOverrideReason  bool

	HasPromoID   bool
	HasPromoCode bool
}

type PostCutoverCandidateBatch struct {
	Candidates []PostCutoverCandidate
	NextCursor *uuid.UUID
	HasMore    bool
}

type PostCutoverPreflight struct {
	CutoverAt time.Time
}

type PostCutoverPopulation struct {
	PostCutoverBookingTotal    int64
	PostCutoverWithSnapshot    int64
	PostCutoverMissingSnapshot int64
}

// Interface
type PostCutoverQueryer interface {
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
}

var ErrPostCutoverDetectorIntegrity = errors.New("post cutover detector integrity failure")

// Preflight
func LoadPostCutoverDetectorPreflight(ctx context.Context, db PostCutoverQueryer) (PostCutoverPreflight, error) {
	// 1. Transaction Read Only & Isolation
	var txReadOnly string
	var txIsolation string
	err := db.QueryRow(ctx, `SHOW transaction_read_only`).Scan(&txReadOnly)
	if err != nil {
		return PostCutoverPreflight{}, err
	}
	if txReadOnly != "on" {
		return PostCutoverPreflight{}, ErrPostCutoverDetectorIntegrity
	}

	err = db.QueryRow(ctx, `SHOW transaction_isolation`).Scan(&txIsolation)
	if err != nil {
		return PostCutoverPreflight{}, err
	}
	if txIsolation != "repeatable read" {
		return PostCutoverPreflight{}, ErrPostCutoverDetectorIntegrity
	}

	// 2. Migration Valid
	var schemaVersion int
	var schemaDirty bool
	err = db.QueryRow(ctx, `SELECT version, dirty FROM schema_migrations LIMIT 1`).Scan(&schemaVersion, &schemaDirty)
	if err != nil {
		return PostCutoverPreflight{}, err
	}
	if schemaVersion < 21 || schemaDirty {
		return PostCutoverPreflight{}, ErrPostCutoverDetectorIntegrity
	}

	// 3. Cutover Count = 1
	var cutoverCount int
	err = db.QueryRow(ctx, `SELECT COUNT(*) FROM platform_finance_cutovers`).Scan(&cutoverCount)
	if err != nil {
		return PostCutoverPreflight{}, err
	}
	if cutoverCount != 1 {
		return PostCutoverPreflight{}, ErrPostCutoverDetectorIntegrity
	}

	// 4. Exact Cutover Row Check
	var cutoverAt time.Time
	var calcVersion string
	var releaseRef string
	var actorID uuid.UUID
	var cutoverCreatedAt time.Time
	err = db.QueryRow(ctx, `
		SELECT
			snapshot_cutover_at,
			calculation_version,
			release_reference,
			created_by_user_id,
			created_at
		FROM platform_finance_cutovers
		LIMIT 1
	`).Scan(&cutoverAt, &calcVersion, &releaseRef, &actorID, &cutoverCreatedAt)
	if err != nil {
		return PostCutoverPreflight{}, err
	}

	if cutoverAt.IsZero() || calcVersion != ActiveBookingFeeCalculationVersion || strings.TrimSpace(releaseRef) == "" || actorID == uuid.Nil {
		return PostCutoverPreflight{}, ErrPostCutoverDetectorIntegrity
	}
	var databaseNow time.Time
	if err := db.QueryRow(ctx, `SELECT clock_timestamp()`).Scan(&databaseNow); err != nil {
		return PostCutoverPreflight{}, err
	}
	if cutoverAt.After(databaseNow) || cutoverCreatedAt.Before(cutoverAt) {
		return PostCutoverPreflight{}, ErrPostCutoverDetectorIntegrity
	}

	// 5. Trigger Integrity
	triggerValid, err := VerifyTriggerIntegrity(ctx, db)
	if err != nil {
		return PostCutoverPreflight{}, err
	}
	if !triggerValid {
		return PostCutoverPreflight{}, ErrPostCutoverDetectorIntegrity
	}

	return PostCutoverPreflight{
		CutoverAt: cutoverAt,
	}, nil
}

// Population
func LoadPostCutoverPopulation(ctx context.Context, db PostCutoverQueryer, cutoverAt time.Time) (PostCutoverPopulation, error) {
	if db == nil || cutoverAt.IsZero() {
		return PostCutoverPopulation{}, ErrPostCutoverDetectorIntegrity
	}
	var pop PostCutoverPopulation
	err := db.QueryRow(ctx, `
		SELECT
			COUNT(*) AS post_cutover_booking_total,
			COUNT(*) FILTER (
				WHERE bfs.booking_id IS NOT NULL
			) AS post_cutover_with_snapshot,
			COUNT(*) FILTER (
				WHERE bfs.booking_id IS NULL
			) AS post_cutover_missing_snapshot
		FROM bookings b
		LEFT JOIN booking_fee_snapshots bfs
		  ON bfs.booking_id = b.id
		WHERE b.created_at >= $1
	`, cutoverAt.UTC()).Scan(&pop.PostCutoverBookingTotal, &pop.PostCutoverWithSnapshot, &pop.PostCutoverMissingSnapshot)
	if err != nil {
		return PostCutoverPopulation{}, err
	}
	return pop, nil
}

// Fetch Candidates
func FetchPostCutoverP0Candidates(ctx context.Context, db PostCutoverQueryer, params PostCutoverDetectorParams) (PostCutoverCandidateBatch, error) {
	if db == nil || params.CutoverAt.IsZero() || params.BatchSize < 1 || params.BatchSize > 1000 {
		return PostCutoverCandidateBatch{}, ErrPostCutoverDetectorIntegrity
	}

	q := `
		SELECT
			b.id,
			b.created_at,
			b.court_id,
			c.venue_id,
			v.owner_profile_id,

			b.original_price,
			b.final_price,
			b.total_price,
			b.discount_amount,

			offline_fact.offline_row_count IS NOT NULL AND offline_fact.offline_row_count > 0 AS has_offline_record,
			COALESCE(offline_fact.offline_row_count, 0) AS offline_row_count,
			offline_fact.system_price,
			offline_fact.final_price AS offline_final_price,
			COALESCE(offline_fact.has_override_reason, false) AS has_override_reason,

			b.promo_id IS NOT NULL AS has_promo_id,
			CASE
				WHEN b.promo_code IS NOT NULL
				 AND BTRIM(b.promo_code) <> ''
				THEN true
				ELSE false
			END AS has_promo_code

		FROM bookings b
		LEFT JOIN courts c
		  ON c.id = b.court_id
		LEFT JOIN venues v
		  ON v.id = c.venue_id
		LEFT JOIN LATERAL (
			SELECT
				COUNT(*) AS offline_row_count,
				MAX(system_price) AS system_price,
				MAX(final_price) AS final_price,
				BOOL_OR(
					price_override_reason IS NOT NULL
					AND BTRIM(price_override_reason) <> ''
				) AS has_override_reason
			FROM offline_booking_customers obc
			WHERE obc.booking_id = b.id
		) offline_fact ON true
		WHERE b.created_at >= $1
		  AND ($2::uuid IS NULL OR b.id > $2::uuid)
		  AND NOT EXISTS (
			  SELECT 1
			  FROM booking_fee_snapshots bfs
			  WHERE bfs.booking_id = b.id
		  )
		ORDER BY b.id ASC
		LIMIT $3
	`
	rows, err := db.Query(ctx, q, params.CutoverAt.UTC(), params.AfterBookingID, params.BatchSize)
	if err != nil {
		return PostCutoverCandidateBatch{}, err
	}
	defer rows.Close()

	var candidates []PostCutoverCandidate
	var previousID *uuid.UUID
	for rows.Next() {
		var c PostCutoverCandidate
		err := rows.Scan(
			&c.ID,
			&c.CreatedAt,
			&c.CourtID,
			&c.VenueID,
			&c.OwnerProfileID,
			&c.OriginalPrice,
			&c.FinalPrice,
			&c.TotalPrice,
			&c.DiscountAmount,
			&c.HasOfflineRecord,
			&c.OfflineRowCount,
			&c.OfflineSystemPrice,
			&c.OfflineFinalPrice,
			&c.HasOverrideReason,
			&c.HasPromoID,
			&c.HasPromoCode,
		)
		if err != nil {
			return PostCutoverCandidateBatch{}, err
		}

		// Guard cursor always advancing, including within a returned batch.
		if params.AfterBookingID != nil && bytes.Compare(c.ID[:], params.AfterBookingID[:]) <= 0 {
			return PostCutoverCandidateBatch{}, ErrPostCutoverDetectorIntegrity
		}
		if previousID != nil && bytes.Compare(c.ID[:], previousID[:]) <= 0 {
			return PostCutoverCandidateBatch{}, ErrPostCutoverDetectorIntegrity
		}

		candidates = append(candidates, c)
		id := c.ID
		previousID = &id
	}
	if err := rows.Err(); err != nil {
		return PostCutoverCandidateBatch{}, err
	}

	batch := PostCutoverCandidateBatch{
		Candidates: candidates,
	}

	if len(candidates) == params.BatchSize {
		batch.HasMore = true
		lastID := candidates[len(candidates)-1].ID
		batch.NextCursor = &lastID
	}

	return batch, nil
}

func parseMoney(n pgtype.Numeric) (int64, PostCutoverP0Reason, bool) {
	if !n.Valid {
		return 0, ReasonMissingMoneyValue, false
	}
	if n.NaN || n.InfinityModifier != pgtype.Finite || n.Int == nil {
		return 0, ReasonFractionalMoneyValue, false // Treat NaN/Inf as fractional/invalid
	}
	// NUMERIC(12,2) whole-rupiah values may have a negative exponent. For
	// example, 100000.00 can be represented as Int=10000000, Exp=-2. It is
	// fractional only when division by the scale leaves a non-zero remainder.
	value := new(big.Int).Set(n.Int)
	exp := int64(n.Exp)
	if exp < 0 {
		scale := new(big.Int).Exp(big.NewInt(10), big.NewInt(-exp), nil)
		quotient, remainder := new(big.Int), new(big.Int)
		quotient.QuoRem(value, scale, remainder)
		if remainder.Sign() != 0 {
			return 0, ReasonFractionalMoneyValue, false
		}
		value = quotient
	} else if exp > 0 {
		scale := new(big.Int).Exp(big.NewInt(10), big.NewInt(exp), nil)
		value.Mul(value, scale)
	}
	if !value.IsInt64() {
		return 0, ReasonMoneyOverflow, false
	}
	val := value.Int64()
	if val < 0 {
		return 0, ReasonNegativeMoneyValue, false
	}
	if val > 9_999_999_999 {
		return 0, ReasonMoneyOverflow, false
	}
	return val, "", true
}

func ClassifyPostCutoverP0Candidate(candidate PostCutoverCandidate, term *CommercialTerm, resolverErr error) PostCutoverClassificationResult {
	// 1. References Null checks
	if candidate.CourtID == nil {
		return PostCutoverClassificationResult{Classification: ClassificationManualDecisionRequired, Reason: ReasonMissingCourtReference}
	}
	if candidate.VenueID == nil {
		return PostCutoverClassificationResult{Classification: ClassificationManualDecisionRequired, Reason: ReasonMissingVenueReference}
	}
	if candidate.OwnerProfileID == nil {
		return PostCutoverClassificationResult{Classification: ClassificationManualDecisionRequired, Reason: ReasonMissingOwnerReference}
	}

	// 2. Channel Identification
	var channel BookingChannel
	if candidate.OfflineRowCount == 0 {
		channel = BookingChannelMarketplaceOnline
	} else if candidate.OfflineRowCount == 1 {
		channel = BookingChannelOwnerWalkIn
	} else {
		return PostCutoverClassificationResult{Classification: ClassificationManualDecisionRequired, Reason: ReasonDuplicateOfflineFact}
	}

	// 3. Resolver Sentinel Error Mapping & Term Validation
	if resolverErr != nil {
		if errors.Is(resolverErr, ErrMissingEffectiveCommercialTerm) {
			return PostCutoverClassificationResult{Classification: ClassificationManualDecisionRequired, Reason: ReasonMissingEffectiveTerm}
		}
		if errors.Is(resolverErr, ErrDuplicateCommercialTerm) {
			return PostCutoverClassificationResult{Classification: ClassificationManualDecisionRequired, Reason: ReasonDuplicateEffectiveTerm}
		}
		if errors.Is(resolverErr, ErrUnsupportedCommercialTermFinanceMode) {
			return PostCutoverClassificationResult{Classification: ClassificationManualDecisionRequired, Reason: ReasonUnsupportedFinanceMode}
		}
		if errors.Is(resolverErr, ErrInvalidResolvedCommercialTerm) {
			return PostCutoverClassificationResult{Classification: ClassificationManualDecisionRequired, Reason: ReasonInvalidResolvedTerm}
		}
		return PostCutoverClassificationResult{OperationalError: resolverErr}
	}

	// 4. Term Validation
	if term == nil || strings.TrimSpace(term.ID) == "" {
		return PostCutoverClassificationResult{Classification: ClassificationManualDecisionRequired, Reason: ReasonInvalidResolvedTerm}
	}
	if term.ValidFrom.IsZero() {
		return PostCutoverClassificationResult{Classification: ClassificationManualDecisionRequired, Reason: ReasonInvalidResolvedTerm}
	}
	if term.ValidUntil != nil && !term.ValidUntil.After(term.ValidFrom) {
		return PostCutoverClassificationResult{Classification: ClassificationManualDecisionRequired, Reason: ReasonInvalidResolvedTerm}
	}
	if term.FinanceMode != "SIMULATION" {
		return PostCutoverClassificationResult{Classification: ClassificationManualDecisionRequired, Reason: ReasonUnsupportedFinanceMode}
	}
	if term.CollectionMethod != "NONE" {
		return PostCutoverClassificationResult{Classification: ClassificationManualDecisionRequired, Reason: ReasonUnsupportedCollectionMethod}
	}
	if term.Phase != "TRIAL" && term.Phase != "INTRODUCTORY" && term.Phase != "STANDARD" && term.Phase != "CUSTOM" {
		return PostCutoverClassificationResult{Classification: ClassificationManualDecisionRequired, Reason: ReasonInvalidTermPhase}
	}
	if term.CommissionBps < 0 || term.CommissionBps > 3000 {
		return PostCutoverClassificationResult{Classification: ClassificationManualDecisionRequired, Reason: ReasonInvalidCommissionBps}
	}

	// 5. Money Parsing
	originalPrice, reason, ok := parseMoney(candidate.OriginalPrice)
	if !ok {
		return PostCutoverClassificationResult{Classification: ClassificationManualDecisionRequired, Reason: reason}
	}
	finalPrice, reason, ok := parseMoney(candidate.FinalPrice)
	if !ok {
		return PostCutoverClassificationResult{Classification: ClassificationManualDecisionRequired, Reason: reason}
	}
	totalPrice, reason, ok := parseMoney(candidate.TotalPrice)
	if !ok {
		return PostCutoverClassificationResult{Classification: ClassificationManualDecisionRequired, Reason: reason}
	}

	// 6. Online specific logic
	if channel == BookingChannelMarketplaceOnline {
		discountAmount, reason, ok := parseMoney(candidate.DiscountAmount)
		if !ok {
			return PostCutoverClassificationResult{Classification: ClassificationManualDecisionRequired, Reason: reason}
		}

		adj := finalPrice - originalPrice
		if adj > 0 {
			return PostCutoverClassificationResult{Classification: ClassificationManualDecisionRequired, Reason: ReasonOnlinePositiveAdjustment}
		}

		if originalPrice-discountAmount != finalPrice {
			return PostCutoverClassificationResult{Classification: ClassificationManualDecisionRequired, Reason: ReasonDiscountArithmeticMismatch}
		}
		if finalPrice != totalPrice {
			return PostCutoverClassificationResult{Classification: ClassificationManualDecisionRequired, Reason: ReasonBookingTotalFinalMismatch}
		}
		if adj != 0 {
			if !candidate.HasPromoID || !candidate.HasPromoCode {
				return PostCutoverClassificationResult{Classification: ClassificationManualDecisionRequired, Reason: ReasonPromoFactMissing}
			}
		}

		// Run calculator
		calcRes, err := CalculateBookingFees(CalculatorParams{
			OriginalPriceRupiah:        originalPrice,
			OwnerPriceAdjustmentRupiah: adj,
			PriceAdjustmentReason:      "PROMO", // Calculator requires a non-empty reason for non-zero adjustments.
			CommissionBps:              term.CommissionBps,
			BookingChannel:             BookingChannelMarketplaceOnline,
			CustomerServiceFeeRupiah:   0,
		})
		if err != nil {
			return PostCutoverClassificationResult{Classification: ClassificationManualDecisionRequired, Reason: ReasonCalculatorResultMismatch}
		}
		if !calculatorResultMatches(calcRes, finalPrice, term.CommissionBps, BookingChannelMarketplaceOnline) {
			return PostCutoverClassificationResult{Classification: ClassificationManualDecisionRequired, Reason: ReasonCalculatorResultMismatch}
		}

		return PostCutoverClassificationResult{Classification: ClassificationRepairablePolicyOnline, Reason: ""}
	}

	// 7. Offline specific logic
	if channel == BookingChannelOwnerWalkIn {
		sysPrice, reason, ok := parseMoney(candidate.OfflineSystemPrice)
		if !ok {
			if reason == ReasonMissingMoneyValue {
				reason = ReasonMissingOfflineFact
			}
			return PostCutoverClassificationResult{Classification: ClassificationManualDecisionRequired, Reason: reason}
		}
		offFinalPrice, reason, ok := parseMoney(candidate.OfflineFinalPrice)
		if !ok {
			if reason == ReasonMissingMoneyValue {
				reason = ReasonMissingOfflineFact
			}
			return PostCutoverClassificationResult{Classification: ClassificationManualDecisionRequired, Reason: reason}
		}

		if sysPrice != originalPrice {
			return PostCutoverClassificationResult{Classification: ClassificationManualDecisionRequired, Reason: ReasonOfflineSystemPriceMismatch}
		}
		if offFinalPrice != finalPrice {
			return PostCutoverClassificationResult{Classification: ClassificationManualDecisionRequired, Reason: ReasonOfflineFinalPriceMismatch}
		}
		if finalPrice != totalPrice {
			return PostCutoverClassificationResult{Classification: ClassificationManualDecisionRequired, Reason: ReasonBookingTotalFinalMismatch}
		}

		adj := finalPrice - sysPrice
		if adj != 0 && !candidate.HasOverrideReason {
			return PostCutoverClassificationResult{Classification: ClassificationManualDecisionRequired, Reason: ReasonAdjustmentWithoutReason}
		}

		// Run calculator
		calcRes, err := CalculateBookingFees(CalculatorParams{
			OriginalPriceRupiah:        originalPrice,
			OwnerPriceAdjustmentRupiah: adj,
			PriceAdjustmentReason:      "OFFLINE",          // Calculator requires a non-empty reason for non-zero adjustments.
			CommissionBps:              term.CommissionBps, // Although channel walkin ignores it, we pass it.
			BookingChannel:             BookingChannelOwnerWalkIn,
			CustomerServiceFeeRupiah:   0,
		})
		if err != nil {
			return PostCutoverClassificationResult{Classification: ClassificationManualDecisionRequired, Reason: ReasonCalculatorResultMismatch}
		}
		if !calculatorResultMatches(calcRes, finalPrice, term.CommissionBps, BookingChannelOwnerWalkIn) {
			return PostCutoverClassificationResult{Classification: ClassificationManualDecisionRequired, Reason: ReasonCalculatorResultMismatch}
		}

		return PostCutoverClassificationResult{Classification: ClassificationRepairablePolicyWalkIn, Reason: ""}
	}

	return PostCutoverClassificationResult{Classification: ClassificationManualDecisionRequired, Reason: ReasonUnknownBookingChannel}
}

func calculatorResultMatches(result CalculatorResult, finalPrice int64, requestedBPS int, channel BookingChannel) bool {
	effectiveBPS := requestedBPS
	if channel == BookingChannelOwnerWalkIn {
		effectiveBPS = 0
	}
	expectedCommission := (finalPrice*int64(effectiveBPS) + 5_000) / 10_000
	return result.FinalBookingPriceRupiah == finalPrice &&
		result.CustomerChargeAmountRupiah == finalPrice &&
		result.CommissionBasisAmountRupiah == finalPrice &&
		result.CommissionBps == effectiveBPS &&
		result.CommissionAmountRupiah == expectedCommission &&
		result.OwnerNetAmountRupiah == finalPrice-expectedCommission
}
