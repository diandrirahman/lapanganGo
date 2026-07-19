package platformfinance

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"lapangango-api/internal/database"

	"github.com/jackc/pgx/v5"
)

const (
	fixtureOwnerUserID    = "11111111-1111-1111-1111-111111111111"
	fixtureOwnerProfileID = "22222222-2222-2222-2222-222222222222"
	fixtureVenueID        = "33333333-3333-3333-3333-333333333333"
	fixtureCourtID        = "44444444-4444-4444-4444-444444444444"
)

func TestRepositoryControlledFinanceFixtures(t *testing.T) {
	if os.Getenv("TEST_INTEGRATION") != "1" {
		t.Skip("integration fixture requires TEST_INTEGRATION=1")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		dsn = "postgres://lapangango_user:lapangango_password@localhost:5432/lapangango_db?sslmode=disable"
	}
	pool, err := database.NewPostgresPool(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()

	location := GetJakartaLocation()
	start := time.Date(2030, time.January, 10, 0, 0, 0, 0, location).UTC()
	endExclusive := time.Date(2030, time.January, 11, 0, 0, 0, 0, location).UTC()

	t.Run("canonical source excludes offline manual confirmed and invalid rows", func(t *testing.T) {
		tx := beginFixtureTx(t, ctx, pool)
		defer tx.Rollback(ctx)

		insertBooking(t, ctx, tx, "aaaaaaaa-0000-0000-0000-000000000001", "COMPLETED", start.Add(time.Hour))
		insertLedger(t, ctx, tx, "aaaaaaaa-0000-0000-0000-000000000001", "INCOME", "BOOKING", "49", start.Add(time.Hour))

		// The original income is outside the report; the refund event is inside.
		insertBooking(t, ctx, tx, "aaaaaaaa-0000-0000-0000-000000000002", "CANCELLED", start.Add(-48*time.Hour))
		insertLedger(t, ctx, tx, "aaaaaaaa-0000-0000-0000-000000000002", "INCOME", "BOOKING", "50", start.Add(-24*time.Hour))
		insertLedger(t, ctx, tx, "aaaaaaaa-0000-0000-0000-000000000002", "EXPENSE", "REFUND", "50", start.Add(2*time.Hour))

		insertBooking(t, ctx, tx, "aaaaaaaa-0000-0000-0000-000000000003", "COMPLETED", start.Add(3*time.Hour))
		insertLedger(t, ctx, tx, "aaaaaaaa-0000-0000-0000-000000000003", "INCOME", "BOOKING", "1000", start.Add(3*time.Hour))
		insertLedger(t, ctx, tx, "aaaaaaaa-0000-0000-0000-000000000003", "EXPENSE", "REFUND", "1000", start.Add(4*time.Hour))
		if _, err := tx.Exec(ctx, `INSERT INTO offline_booking_customers (booking_id) VALUES ($1)`, "aaaaaaaa-0000-0000-0000-000000000003"); err != nil {
			t.Fatal(err)
		}

		insertBooking(t, ctx, tx, "aaaaaaaa-0000-0000-0000-000000000004", "CONFIRMED", start.Add(5*time.Hour))
		insertBooking(t, ctx, tx, "aaaaaaaa-0000-0000-0000-000000000005", "PAID", start.Add(6*time.Hour))
		insertLedger(t, ctx, tx, "", "INCOME", "MANUAL", "900", start.Add(7*time.Hour))
		// A BOOKING-labelled ledger without a booking is data quality only and
		// must never enter canonical money/counts.
		insertLedger(t, ctx, tx, "", "INCOME", "BOOKING", "777", start.Add(8*time.Hour))

		result, err := getSummaryDataInTx(ctx, tx, start, endExclusive, "", "")
		if err != nil {
			t.Fatal(err)
		}
		if result.Gross != 49 || result.RealizedBookingCount != 1 || result.ProjectedCommGross != 3 {
			t.Fatalf("gross result = %#v, want amount=49 count=1 commission=3", result)
		}
		if result.RefundPrincipal != 50 || result.RefundedBookingCount != 1 || result.ProjectedCommRefunded != 4 {
			t.Fatalf("refund result = %#v, want amount=50 count=1 commission=4", result)
		}
		if result.PaidWithoutLedgerCount != 1 || result.LedgerWithoutBookingCount != 1 {
			t.Fatalf("data quality = paid_without=%d ledger_without=%d, want 1/1", result.PaidWithoutLedgerCount, result.LedgerWithoutBookingCount)
		}
		if len(result.TopOwners) != 1 || result.TopOwners[0].Net != -1 || result.TopOwners[0].NetComm != -1 || result.TopOwners[0].BookingCount != 1 {
			t.Fatalf("unexpected owner breakdown: %#v", result.TopOwners)
		}
		if len(result.TopVenues) != 1 || result.TopVenues[0].Net != -1 || result.TopVenues[0].NetComm != -1 || result.TopVenues[0].BookingCount != 1 {
			t.Fatalf("unexpected venue breakdown: %#v", result.TopVenues)
		}

		filtered, err := getSummaryDataInTx(ctx, tx, start, endExclusive, "00000000-0000-0000-0000-000000000000", "")
		if err != nil {
			t.Fatal(err)
		}
		if filtered.Gross != 0 || filtered.PaidWithoutLedgerCount != 0 || filtered.LedgerWithoutBookingCount != 0 {
			t.Fatalf("owner filter leaked data: %#v", filtered)
		}
	})

	t.Run("null refund fails closed", func(t *testing.T) {
		tx := beginFixtureTx(t, ctx, pool)
		defer tx.Rollback(ctx)
		insertLedger(t, ctx, tx, "", "EXPENSE", "REFUND", "100", start)
		_, err := getSummaryDataInTx(ctx, tx, start, endExclusive, "", "")
		if !errors.Is(err, ErrOrphanRefundDetected) {
			t.Fatalf("error = %v, want ErrOrphanRefundDetected", err)
		}
	})

	t.Run("partial refund fails closed", func(t *testing.T) {
		tx := beginFixtureTx(t, ctx, pool)
		defer tx.Rollback(ctx)
		bookingID := "bbbbbbbb-0000-0000-0000-000000000001"
		insertBooking(t, ctx, tx, bookingID, "CANCELLED", start.Add(-48*time.Hour))
		insertLedger(t, ctx, tx, bookingID, "INCOME", "BOOKING", "100", start.Add(-24*time.Hour))
		insertLedger(t, ctx, tx, bookingID, "EXPENSE", "REFUND", "50", start)
		_, err := getSummaryDataInTx(ctx, tx, start, endExclusive, "", "")
		if !errors.Is(err, ErrRefundAmountMismatch) {
			t.Fatalf("error = %v, want ErrRefundAmountMismatch", err)
		}
	})

	t.Run("duplicate original outside period fails closed", func(t *testing.T) {
		tx := beginFixtureTx(t, ctx, pool)
		defer tx.Rollback(ctx)
		bookingID := "cccccccc-0000-0000-0000-000000000001"
		insertBooking(t, ctx, tx, bookingID, "COMPLETED", start.Add(-96*time.Hour))
		insertLedger(t, ctx, tx, bookingID, "INCOME", "BOOKING", "100", start.Add(-72*time.Hour))
		insertLedger(t, ctx, tx, bookingID, "INCOME", "BOOKING", "100", start.Add(-48*time.Hour))
		_, err := getSummaryDataInTx(ctx, tx, start, endExclusive, "", "")
		if !errors.Is(err, ErrDuplicateLedgerDetected) {
			t.Fatalf("error = %v, want ErrDuplicateLedgerDetected", err)
		}
	})

	t.Run("fractional rupiah fails closed", func(t *testing.T) {
		tx := beginFixtureTx(t, ctx, pool)
		defer tx.Rollback(ctx)
		insertLedger(t, ctx, tx, "", "INCOME", "MANUAL", "1.5", start)
		_, err := getSummaryDataInTx(ctx, tx, start, endExclusive, "", "")
		if !errors.Is(err, ErrFractionalLedgerDetected) {
			t.Fatalf("error = %v, want ErrFractionalLedgerDetected", err)
		}
	})

	t.Run("numeric overflow maps to controlled error", func(t *testing.T) {
		tx := beginFixtureTx(t, ctx, pool)
		defer tx.Rollback(ctx)
		bookingID := "dddddddd-0000-0000-0000-000000000001"
		insertBooking(t, ctx, tx, bookingID, "COMPLETED", start)
		insertLedger(t, ctx, tx, bookingID, "INCOME", "BOOKING", "9223372036854775808", start)
		_, err := getSummaryDataInTx(ctx, tx, start, endExclusive, "", "")
		if !errors.Is(err, ErrOverflowDetected) {
			t.Fatalf("error = %v, want ErrOverflowDetected", err)
		}
	})
}

func beginFixtureTx(t *testing.T, ctx context.Context, pool interface {
	Begin(context.Context) (pgx.Tx, error)
}) pgx.Tx {
	t.Helper()
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	_, err = tx.Exec(ctx, `
		CREATE TEMP TABLE owner_profiles (id uuid PRIMARY KEY, user_id uuid NOT NULL, business_name text NOT NULL);
		CREATE TEMP TABLE venues (id uuid PRIMARY KEY, owner_profile_id uuid NOT NULL, name text NOT NULL);
		CREATE TEMP TABLE courts (id uuid PRIMARY KEY, venue_id uuid NOT NULL);
		CREATE TEMP TABLE bookings (
			id uuid PRIMARY KEY,
			court_id uuid NOT NULL,
			status text NOT NULL,
			total_price numeric(12,2) NOT NULL DEFAULT 0,
			created_at timestamptz NOT NULL
		);
		CREATE TEMP TABLE offline_booking_customers (booking_id uuid PRIMARY KEY);
		CREATE TEMP TABLE owner_finance_transactions (
			id bigserial PRIMARY KEY,
			owner_id uuid NOT NULL,
			venue_id uuid,
			booking_id uuid,
			type text NOT NULL,
			source text NOT NULL,
			amount numeric NOT NULL,
			created_at timestamptz NOT NULL
		);
		INSERT INTO owner_profiles (id, user_id, business_name) VALUES
			('22222222-2222-2222-2222-222222222222', '11111111-1111-1111-1111-111111111111', 'Fixture Owner');
		INSERT INTO venues (id, owner_profile_id, name) VALUES
			('33333333-3333-3333-3333-333333333333', '22222222-2222-2222-2222-222222222222', 'Fixture Venue');
		INSERT INTO courts (id, venue_id) VALUES
			('44444444-4444-4444-4444-444444444444', '33333333-3333-3333-3333-333333333333');
	`)
	if err != nil {
		tx.Rollback(ctx)
		t.Fatal(err)
	}
	return tx
}

func insertBooking(t *testing.T, ctx context.Context, tx pgx.Tx, id, status string, createdAt time.Time) {
	t.Helper()
	_, err := tx.Exec(ctx, `
		INSERT INTO bookings (id, court_id, status, created_at)
		VALUES ($1, $2, $3, $4)
	`, id, fixtureCourtID, status, createdAt)
	if err != nil {
		t.Fatal(err)
	}
}

func insertLedger(t *testing.T, ctx context.Context, tx pgx.Tx, bookingID, transactionType, source, amount string, createdAt time.Time) {
	t.Helper()
	var booking any
	if bookingID != "" {
		booking = bookingID
	}
	_, err := tx.Exec(ctx, `
		INSERT INTO owner_finance_transactions (owner_id, venue_id, booking_id, type, source, amount, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
	`, fixtureOwnerUserID, fixtureVenueID, booking, transactionType, source, amount, createdAt)
	if err != nil {
		t.Fatal(err)
	}
}
