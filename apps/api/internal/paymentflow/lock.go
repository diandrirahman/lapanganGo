package paymentflow

import (
	"context"

	"github.com/jackc/pgx/v5/pgconn"
)

// Execer is the smallest transaction surface needed by LockBooking.
type Execer interface {
	Exec(context.Context, string, ...any) (pgconn.CommandTag, error)
}

// LockBooking serializes mutually exclusive payment flows for one booking.
// Every caller must acquire it before locking a payment attempt or booking row.
func LockBooking(ctx context.Context, db Execer, bookingID string) error {
	_, err := db.Exec(ctx, `
		SELECT pg_advisory_xact_lock(
			hashtextextended('payments:booking-flow:' || $1::uuid::text, 0)
		)
	`, bookingID)
	return err
}
