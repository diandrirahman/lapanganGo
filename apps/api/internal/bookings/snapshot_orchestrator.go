package bookings

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"lapangango-api/internal/platformfinance"
)

type SnapshotOrchestrationRequest struct {
	OwnerProfileID             string
	VenueID                    string
	EffectiveAt                time.Time
	Channel                    platformfinance.BookingChannel
	OriginalPriceRupiah        int64
	OwnerPriceAdjustmentRupiah int64
	PriceAdjustmentReason      string
}

type CanonicalBookingPricing struct {
	OriginalPriceRupiah         int64
	OwnerPriceAdjustmentRupiah  int64
	PriceAdjustmentReason       string
	FinalBookingPriceRupiah     int64
	CustomerChargeAmountRupiah  int64
	CommissionBasisAmountRupiah int64
	EffectiveCommissionBps      int
	CommissionAmountRupiah      int64
	OwnerNetAmountRupiah        int64
}

type InsertBookingWithCanonicalPricingFunc func(
	ctx context.Context,
	tx pgx.Tx,
	pricing CanonicalBookingPricing,
) (Booking, error)

type ResolveEffectiveTermFunc func(
	ctx context.Context,
	db platformfinance.CommercialTermQueryer,
	ownerProfileID string,
	effectiveAt time.Time,
) (*platformfinance.CommercialTerm, error)

type CalculateBookingFeesFunc func(
	platformfinance.CalculatorParams,
) (platformfinance.CalculatorResult, error)

type SnapshotOrchestrator interface {
	CreateBookingWithSnapshot(
		ctx context.Context,
		tx pgx.Tx,
		req SnapshotOrchestrationRequest,
		insertBooking InsertBookingWithCanonicalPricingFunc,
	) (Booking, *platformfinance.BookingFeeSnapshot, error)
}

type snapshotOrchestrator struct {
	resolver     ResolveEffectiveTermFunc
	calculator   CalculateBookingFeesFunc
	snapshotRepo platformfinance.BookingFeeSnapshotRepository
}

func NewSnapshotOrchestrator(
	resolver ResolveEffectiveTermFunc,
	calculator CalculateBookingFeesFunc,
	snapshotRepo platformfinance.BookingFeeSnapshotRepository,
) (SnapshotOrchestrator, error) {
	if resolver == nil {
		return nil, errors.New("resolver is nil")
	}
	if calculator == nil {
		return nil, errors.New("calculator is nil")
	}
	if snapshotRepo == nil {
		return nil, errors.New("snapshotRepo is nil")
	}
	return &snapshotOrchestrator{
		resolver:     resolver,
		calculator:   calculator,
		snapshotRepo: snapshotRepo,
	}, nil
}

func (o *snapshotOrchestrator) CreateBookingWithSnapshot(
	ctx context.Context,
	tx pgx.Tx,
	req SnapshotOrchestrationRequest,
	insertBooking InsertBookingWithCanonicalPricingFunc,
) (Booking, *platformfinance.BookingFeeSnapshot, error) {

	if req.EffectiveAt.IsZero() {
		return Booking{}, nil, errors.New("invalid request: zero EffectiveAt")
	}
	if insertBooking == nil {
		return Booking{}, nil, errors.New("invalid request: nil insert callback")
	}
	if req.Channel != platformfinance.BookingChannelMarketplaceOnline && req.Channel != platformfinance.BookingChannelOwnerWalkIn {
		return Booking{}, nil, errors.New("invalid request: unsupported channel")
	}

	normalizedReason := strings.TrimSpace(req.PriceAdjustmentReason)

	resolvedTerm, err := o.resolver(ctx, tx, req.OwnerProfileID, req.EffectiveAt.UTC())
	if err != nil {
		return Booking{}, nil, fmt.Errorf("failed to resolve commercial term: %w", err)
	}
	if resolvedTerm == nil {
		return Booking{}, nil, errors.New("resolved commercial term is nil")
	}
	if resolvedTerm.ID == "" {
		return Booking{}, nil, errors.New("resolved commercial term ID is missing")
	}

	calcParams := platformfinance.CalculatorParams{
		OriginalPriceRupiah:        req.OriginalPriceRupiah,
		OwnerPriceAdjustmentRupiah: req.OwnerPriceAdjustmentRupiah,
		PriceAdjustmentReason:      normalizedReason,
		CommissionBps:              resolvedTerm.CommissionBps,
		BookingChannel:             req.Channel,
		CustomerServiceFeeRupiah:   0,
	}

	calcResult, err := o.calculator(calcParams)
	if err != nil {
		return Booking{}, nil, fmt.Errorf("failed to calculate booking fees: %w", err)
	}

	pricing := CanonicalBookingPricing{
		OriginalPriceRupiah:         req.OriginalPriceRupiah,
		OwnerPriceAdjustmentRupiah:  req.OwnerPriceAdjustmentRupiah,
		PriceAdjustmentReason:       normalizedReason,
		FinalBookingPriceRupiah:     calcResult.FinalBookingPriceRupiah,
		CustomerChargeAmountRupiah:  calcResult.CustomerChargeAmountRupiah,
		CommissionBasisAmountRupiah: calcResult.CommissionBasisAmountRupiah,
		EffectiveCommissionBps:      calcResult.CommissionBps,
		CommissionAmountRupiah:      calcResult.CommissionAmountRupiah,
		OwnerNetAmountRupiah:        calcResult.OwnerNetAmountRupiah,
	}

	booking, err := insertBooking(ctx, tx, pricing)
	if err != nil {
		return Booking{}, nil, fmt.Errorf("failed to insert booking: %w", err)
	}

	var reasonPtr *string
	if normalizedReason != "" {
		reasonPtr = &normalizedReason
	}

	termID := resolvedTerm.ID

	snapshotParams := platformfinance.CreateBookingFeeSnapshotParams{
		BookingID:                   booking.ID,
		OwnerProfileID:              req.OwnerProfileID,
		VenueID:                     req.VenueID,
		CommercialTermID:            &termID,
		TermsSource:                 platformfinance.TermsSourcePolicy,
		BookingChannel:              req.Channel,
		FinanceMode:                 resolvedTerm.FinanceMode,
		OriginalPriceRupiah:         req.OriginalPriceRupiah,
		OwnerPriceAdjustmentRupiah:  req.OwnerPriceAdjustmentRupiah,
		PriceAdjustmentReason:       reasonPtr,
		FinalBookingPriceRupiah:     calcResult.FinalBookingPriceRupiah,
		CustomerServiceFeeRupiah:    0,
		CustomerChargeAmountRupiah:  calcResult.CustomerChargeAmountRupiah,
		CommissionBasisAmountRupiah: calcResult.CommissionBasisAmountRupiah,
		CommissionBps:               calcResult.CommissionBps,
		CommissionAmountRupiah:      calcResult.CommissionAmountRupiah,
		OwnerNetAmountRupiah:        calcResult.OwnerNetAmountRupiah,
		CalculationVersion:          platformfinance.BookingFeeCalculationVersionV1,
	}

	snapshot, err := o.snapshotRepo.InsertSnapshot(ctx, tx, snapshotParams)
	if err != nil {
		return Booking{}, nil, fmt.Errorf("failed to insert snapshot: %w", err)
	}
	if snapshot == nil {
		return Booking{}, nil, errors.New("snapshot repository returned nil snapshot")
	}

	return booking, snapshot, nil
}
