package platformfinance

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

const (
	TermsSourcePolicy             = "POLICY"
	TermsSourceLegacyNoCommission = "LEGACY_NO_COMMISSION"

	SnapshotFinanceModeSimulation = "SIMULATION"

	BookingFeeCalculationVersionV1     = "booking-fee-v1"
	LegacyBackfillCalculationVersionV1 = "legacy-backfill-v1"

	SnapshotCurrencyIDR         = "IDR"
	SnapshotCurrencyExponentIDR = int16(0)
)

var (
	ErrInvalidBookingFeeSnapshot       = errors.New("INVALID_BOOKING_FEE_SNAPSHOT")
	ErrUnsupportedSnapshotFinanceMode  = errors.New("UNSUPPORTED_SNAPSHOT_FINANCE_MODE")
	ErrBookingFeeSnapshotAlreadyExists = errors.New("BOOKING_FEE_SNAPSHOT_ALREADY_EXISTS")
	ErrBookingFeeSnapshotNotFound      = errors.New("BOOKING_FEE_SNAPSHOT_NOT_FOUND")
)

type CreateBookingFeeSnapshotParams struct {
	BookingID                   string
	OwnerProfileID              string
	VenueID                     string
	CommercialTermID            *string
	TermsSource                 string
	BookingChannel              BookingChannel
	FinanceMode                 string
	OriginalPriceRupiah         int64
	OwnerPriceAdjustmentRupiah  int64
	PriceAdjustmentReason       *string
	FinalBookingPriceRupiah     int64
	CustomerServiceFeeRupiah    int64
	CustomerChargeAmountRupiah  int64
	CommissionBasisAmountRupiah int64
	CommissionBps               int
	CommissionAmountRupiah      int64
	OwnerNetAmountRupiah        int64
	CalculationVersion          string
}

type BookingFeeSnapshot struct {
	BookingID                   string
	OwnerProfileID              string
	VenueID                     string
	CommercialTermID            *string
	TermsSource                 string
	BookingChannel              BookingChannel
	FinanceMode                 string
	Currency                    string
	CurrencyExponent            int16
	OriginalPriceRupiah         int64
	OwnerPriceAdjustmentRupiah  int64
	PriceAdjustmentReason       *string
	FinalBookingPriceRupiah     int64
	CustomerServiceFeeRupiah    int64
	CustomerChargeAmountRupiah  int64
	CommissionBasisAmountRupiah int64
	CommissionBps               int
	CommissionAmountRupiah      int64
	OwnerNetAmountRupiah        int64
	CalculationVersion          string
	CreatedAt                   time.Time
}

type SnapshotDBTX interface {
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

type BookingFeeSnapshotRepository interface {
	InsertSnapshot(ctx context.Context, db SnapshotDBTX, params CreateBookingFeeSnapshotParams) (*BookingFeeSnapshot, error)
	GetSnapshot(ctx context.Context, db SnapshotDBTX, bookingID string) (*BookingFeeSnapshot, error)
}

type LegacyBackfillSnapshotRepository interface {
	InsertIdempotentBackfillSnapshot(ctx context.Context, db SnapshotDBTX, params CreateBookingFeeSnapshotParams) (*BookingFeeSnapshot, bool, error)
}

type bookingFeeSnapshotRepository struct{}

func NewBookingFeeSnapshotRepository() BookingFeeSnapshotRepository {
	return &bookingFeeSnapshotRepository{}
}

func NewLegacyBackfillSnapshotRepository() LegacyBackfillSnapshotRepository {
	return &bookingFeeSnapshotRepository{}
}

func (r *bookingFeeSnapshotRepository) InsertSnapshot(ctx context.Context, db SnapshotDBTX, params CreateBookingFeeSnapshotParams) (*BookingFeeSnapshot, error) {
	if err := validateSnapshotParams(params); err != nil {
		return nil, err
	}

	sql := `
		INSERT INTO booking_fee_snapshots (
			booking_id, owner_profile_id, venue_id, commercial_term_id,
			terms_source, booking_channel, finance_mode, currency, currency_exponent,
			original_price_rupiah, owner_price_adjustment_rupiah, price_adjustment_reason, final_booking_price_rupiah,
			customer_service_fee_rupiah, customer_charge_amount_rupiah, commission_basis_amount_rupiah, commission_bps, commission_amount_rupiah, owner_net_amount_rupiah,
			calculation_version
		) VALUES (
			$1, $2, $3, $4,
			$5, $6, $7, $8, $9,
			$10, $11, $12, $13,
			$14, $15, $16, $17, $18, $19,
			$20
		) RETURNING
			booking_id, owner_profile_id, venue_id, commercial_term_id,
			terms_source, booking_channel, finance_mode, currency, currency_exponent,
			original_price_rupiah, owner_price_adjustment_rupiah, price_adjustment_reason, final_booking_price_rupiah,
			customer_service_fee_rupiah, customer_charge_amount_rupiah, commission_basis_amount_rupiah, commission_bps, commission_amount_rupiah, owner_net_amount_rupiah,
			calculation_version, created_at
	`

	row := db.QueryRow(ctx, sql,
		params.BookingID, params.OwnerProfileID, params.VenueID, params.CommercialTermID,
		params.TermsSource, params.BookingChannel, params.FinanceMode, SnapshotCurrencyIDR, SnapshotCurrencyExponentIDR,
		params.OriginalPriceRupiah, params.OwnerPriceAdjustmentRupiah, params.PriceAdjustmentReason, params.FinalBookingPriceRupiah,
		params.CustomerServiceFeeRupiah, params.CustomerChargeAmountRupiah, params.CommissionBasisAmountRupiah, params.CommissionBps, params.CommissionAmountRupiah, params.OwnerNetAmountRupiah,
		params.CalculationVersion,
	)

	return scanSnapshotRow(row)
}

func (r *bookingFeeSnapshotRepository) InsertIdempotentBackfillSnapshot(ctx context.Context, db SnapshotDBTX, params CreateBookingFeeSnapshotParams) (*BookingFeeSnapshot, bool, error) {
	if err := validateSnapshotParams(params); err != nil {
		return nil, false, err
	}

	sqlInsert := `
		INSERT INTO booking_fee_snapshots (
			booking_id, owner_profile_id, venue_id, commercial_term_id,
			terms_source, booking_channel, finance_mode, currency, currency_exponent,
			original_price_rupiah, owner_price_adjustment_rupiah, price_adjustment_reason, final_booking_price_rupiah,
			customer_service_fee_rupiah, customer_charge_amount_rupiah, commission_basis_amount_rupiah, commission_bps, commission_amount_rupiah, owner_net_amount_rupiah,
			calculation_version
		) VALUES (
			$1, $2, $3, $4,
			$5, $6, $7, $8, $9,
			$10, $11, $12, $13,
			$14, $15, $16, $17, $18, $19,
			$20
		) ON CONFLICT (booking_id) DO NOTHING
		RETURNING booking_id
	`
	var insertedID string
	err := db.QueryRow(ctx, sqlInsert,
		params.BookingID, params.OwnerProfileID, params.VenueID, params.CommercialTermID,
		params.TermsSource, params.BookingChannel, params.FinanceMode, SnapshotCurrencyIDR, SnapshotCurrencyExponentIDR,
		params.OriginalPriceRupiah, params.OwnerPriceAdjustmentRupiah, params.PriceAdjustmentReason, params.FinalBookingPriceRupiah,
		params.CustomerServiceFeeRupiah, params.CustomerChargeAmountRupiah, params.CommissionBasisAmountRupiah, params.CommissionBps, params.CommissionAmountRupiah, params.OwnerNetAmountRupiah,
		params.CalculationVersion,
	).Scan(&insertedID)

	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			// No row returned -> Duplicate exists. Fetch and verify.
			existing, getErr := r.GetSnapshot(ctx, db, params.BookingID)
			if getErr != nil {
				return nil, false, fmt.Errorf("idempotent conflict check failed to get existing: %w", getErr)
			}

			// Exact comparison (ignoring created_at)
			if existing.OwnerProfileID != params.OwnerProfileID ||
				existing.VenueID != params.VenueID ||
				(existing.CommercialTermID != nil && params.CommercialTermID == nil) ||
				(existing.CommercialTermID == nil && params.CommercialTermID != nil) ||
				(existing.CommercialTermID != nil && params.CommercialTermID != nil && *existing.CommercialTermID != *params.CommercialTermID) ||
				existing.TermsSource != params.TermsSource ||
				existing.BookingChannel != params.BookingChannel ||
				existing.FinanceMode != params.FinanceMode ||
				existing.Currency != SnapshotCurrencyIDR ||
				existing.CurrencyExponent != SnapshotCurrencyExponentIDR ||
				existing.OriginalPriceRupiah != params.OriginalPriceRupiah ||
				existing.OwnerPriceAdjustmentRupiah != params.OwnerPriceAdjustmentRupiah ||
				(existing.PriceAdjustmentReason != nil && params.PriceAdjustmentReason == nil) ||
				(existing.PriceAdjustmentReason == nil && params.PriceAdjustmentReason != nil) ||
				(existing.PriceAdjustmentReason != nil && params.PriceAdjustmentReason != nil && *existing.PriceAdjustmentReason != *params.PriceAdjustmentReason) ||
				existing.FinalBookingPriceRupiah != params.FinalBookingPriceRupiah ||
				existing.CustomerServiceFeeRupiah != params.CustomerServiceFeeRupiah ||
				existing.CustomerChargeAmountRupiah != params.CustomerChargeAmountRupiah ||
				existing.CommissionBasisAmountRupiah != params.CommissionBasisAmountRupiah ||
				existing.CommissionBps != params.CommissionBps ||
				existing.CommissionAmountRupiah != params.CommissionAmountRupiah ||
				existing.OwnerNetAmountRupiah != params.OwnerNetAmountRupiah ||
				existing.CalculationVersion != params.CalculationVersion {

				return nil, false, ErrBackfillSnapshotConflict
			}

			// Exact match -> idempotent no-op.
			return existing, false, nil
		}
		return nil, false, err
	}

	// It was successfully inserted. We can just return the snapshot or fetch it.
	snap, err := r.GetSnapshot(ctx, db, params.BookingID)
	return snap, true, err
}

func (r *bookingFeeSnapshotRepository) GetSnapshot(ctx context.Context, db SnapshotDBTX, bookingID string) (*BookingFeeSnapshot, error) {
	sql := `
		SELECT
			booking_id, owner_profile_id, venue_id, commercial_term_id,
			terms_source, booking_channel, finance_mode, currency, currency_exponent,
			original_price_rupiah, owner_price_adjustment_rupiah, price_adjustment_reason, final_booking_price_rupiah,
			customer_service_fee_rupiah, customer_charge_amount_rupiah, commission_basis_amount_rupiah, commission_bps, commission_amount_rupiah, owner_net_amount_rupiah,
			calculation_version, created_at
		FROM booking_fee_snapshots
		WHERE booking_id = $1
	`
	return scanSnapshotRow(db.QueryRow(ctx, sql, bookingID))
}

func scanSnapshotRow(row pgx.Row) (*BookingFeeSnapshot, error) {
	var s BookingFeeSnapshot
	var bookingChannel string

	err := row.Scan(
		&s.BookingID, &s.OwnerProfileID, &s.VenueID, &s.CommercialTermID,
		&s.TermsSource, &bookingChannel, &s.FinanceMode, &s.Currency, &s.CurrencyExponent,
		&s.OriginalPriceRupiah, &s.OwnerPriceAdjustmentRupiah, &s.PriceAdjustmentReason, &s.FinalBookingPriceRupiah,
		&s.CustomerServiceFeeRupiah, &s.CustomerChargeAmountRupiah, &s.CommissionBasisAmountRupiah, &s.CommissionBps, &s.CommissionAmountRupiah, &s.OwnerNetAmountRupiah,
		&s.CalculationVersion, &s.CreatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrBookingFeeSnapshotNotFound
		}
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) {
			if pgErr.Code == "23505" { // unique_violation
				return nil, ErrBookingFeeSnapshotAlreadyExists
			}
		}
		return nil, err
	}
	s.BookingChannel = BookingChannel(bookingChannel)
	return &s, nil
}

func validateSnapshotParams(p CreateBookingFeeSnapshotParams) error {
	if _, err := uuid.Parse(p.BookingID); err != nil {
		return ErrInvalidBookingFeeSnapshot
	}
	if _, err := uuid.Parse(p.OwnerProfileID); err != nil {
		return ErrInvalidBookingFeeSnapshot
	}
	if _, err := uuid.Parse(p.VenueID); err != nil {
		return ErrInvalidBookingFeeSnapshot
	}

	if p.BookingChannel != BookingChannelMarketplaceOnline && p.BookingChannel != BookingChannelOwnerWalkIn {
		return ErrInvalidBookingFeeSnapshot
	}

	if p.FinanceMode != SnapshotFinanceModeSimulation {
		return ErrUnsupportedSnapshotFinanceMode
	}

	if p.BookingChannel == BookingChannelOwnerWalkIn {
		if p.CommissionBps != 0 || p.CommissionAmountRupiah != 0 {
			return ErrInvalidBookingFeeSnapshot
		}
	}

	if p.OwnerPriceAdjustmentRupiah != 0 {
		if p.PriceAdjustmentReason == nil || strings.TrimSpace(*p.PriceAdjustmentReason) == "" {
			return ErrInvalidBookingFeeSnapshot
		}
	}

	if p.TermsSource == TermsSourcePolicy {
		if p.CommercialTermID == nil {
			return ErrInvalidBookingFeeSnapshot
		}
		if _, err := uuid.Parse(*p.CommercialTermID); err != nil {
			return ErrInvalidBookingFeeSnapshot
		}
		if p.CalculationVersion != BookingFeeCalculationVersionV1 {
			return ErrInvalidBookingFeeSnapshot
		}
	} else if p.TermsSource == TermsSourceLegacyNoCommission {
		if p.CommercialTermID != nil {
			return ErrInvalidBookingFeeSnapshot
		}
		if p.CalculationVersion != LegacyBackfillCalculationVersionV1 {
			return ErrInvalidBookingFeeSnapshot
		}
		if p.CommissionBps != 0 || p.CommissionAmountRupiah != 0 {
			return ErrInvalidBookingFeeSnapshot
		}
	} else {
		return ErrInvalidBookingFeeSnapshot
	}

	return nil
}
