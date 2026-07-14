package bookings

import (
	"context"
	"github.com/jackc/pgx/v5"
	"lapangango-api/internal/platformfinance"
)

type mockOrchestrator struct{}

func (m *mockOrchestrator) CreateBookingWithSnapshot(ctx context.Context, tx pgx.Tx, req SnapshotOrchestrationRequest, cb InsertBookingWithCanonicalPricingFunc) (Booking, *platformfinance.BookingFeeSnapshot, error) {
	b, err := cb(ctx, tx, CanonicalBookingPricing{
		OriginalPriceRupiah:        req.OriginalPriceRupiah,
		OwnerPriceAdjustmentRupiah: req.OwnerPriceAdjustmentRupiah,
		FinalBookingPriceRupiah:    req.OriginalPriceRupiah + req.OwnerPriceAdjustmentRupiah,
	})
	return b, nil, err
}
