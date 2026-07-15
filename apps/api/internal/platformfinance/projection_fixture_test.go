package platformfinance

import (
	"context"
	"os"
	"testing"
	"time"

	"lapangango-api/internal/database"
)

func TestProjectionReadModel_HistoricalSnapshotMixedAndRefund(t *testing.T) {
	if os.Getenv("TEST_INTEGRATION") != "1" {
		t.Skip("projection fixture requires TEST_INTEGRATION=1")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL is required")
	}
	pool, err := database.NewPostgresPool(ctx, dsn)
	if err != nil {
		t.Skip("database unavailable")
	}
	defer pool.Close()
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)
	_, err = tx.Exec(ctx, `
CREATE TEMP TABLE platform_finance_cutovers (id smallint primary key, snapshot_cutover_at timestamptz);
CREATE TEMP TABLE owner_profiles (id uuid primary key, user_id uuid not null, business_name text not null);
CREATE TEMP TABLE venues (id uuid primary key, owner_profile_id uuid not null, name text not null);
CREATE TEMP TABLE courts (id uuid primary key, venue_id uuid not null);
CREATE TEMP TABLE bookings (id uuid primary key, court_id uuid not null, created_at timestamptz not null);
CREATE TEMP TABLE offline_booking_customers (booking_id uuid primary key);
CREATE TEMP TABLE booking_fee_snapshots (booking_id uuid primary key, commercial_term_id uuid, terms_source text, booking_channel text, finance_mode text, commission_amount_rupiah bigint, final_booking_price_rupiah bigint, commission_bps integer);
CREATE TEMP TABLE owner_finance_transactions (booking_id uuid, owner_id uuid, venue_id uuid, type text, source text, amount numeric, created_at timestamptz);
INSERT INTO platform_finance_cutovers VALUES (1, '2026-01-01T00:00:00Z');
INSERT INTO owner_profiles VALUES ('22222222-2222-2222-2222-222222222222','11111111-1111-1111-1111-111111111111','Fixture Owner');
INSERT INTO venues VALUES ('33333333-3333-3333-3333-333333333333','22222222-2222-2222-2222-222222222222','Fixture Venue');
INSERT INTO courts VALUES ('44444444-4444-4444-4444-444444444444','33333333-3333-3333-3333-333333333333');
INSERT INTO bookings VALUES
 ('aaaaaaaa-0000-0000-0000-000000000001','44444444-4444-4444-4444-444444444444','2025-12-31T12:00:00Z'),
 ('aaaaaaaa-0000-0000-0000-000000000002','44444444-4444-4444-4444-444444444444','2026-01-02T12:00:00Z'),
 ('aaaaaaaa-0000-0000-0000-000000000003','44444444-4444-4444-4444-444444444444','2026-01-03T12:00:00Z');
INSERT INTO offline_booking_customers VALUES ('aaaaaaaa-0000-0000-0000-000000000003');
INSERT INTO booking_fee_snapshots VALUES ('aaaaaaaa-0000-0000-0000-000000000002','55555555-5555-5555-5555-555555555555','POLICY','MARKETPLACE_ONLINE','SIMULATION',14000,200000,700);
INSERT INTO booking_fee_snapshots VALUES ('aaaaaaaa-0000-0000-0000-000000000003','55555555-5555-5555-5555-555555555555','POLICY','OWNER_WALK_IN','SIMULATION',0,150000,0);
INSERT INTO owner_finance_transactions VALUES
 ('aaaaaaaa-0000-0000-0000-000000000001','11111111-1111-1111-1111-111111111111','33333333-3333-3333-3333-333333333333','INCOME','BOOKING',100000,'2025-12-31T13:00:00Z'),
 ('aaaaaaaa-0000-0000-0000-000000000002','11111111-1111-1111-1111-111111111111','33333333-3333-3333-3333-333333333333','INCOME','BOOKING',200000,'2026-01-02T13:00:00Z'),
 ('aaaaaaaa-0000-0000-0000-000000000002','11111111-1111-1111-1111-111111111111','33333333-3333-3333-3333-333333333333','EXPENSE','REFUND',200000,'2026-01-03T13:00:00Z');
`)
	if err != nil {
		t.Fatal(err)
	}
	incomes, refunds, _, _, err := (&repository{}).loadProjectionEvents(ctx, tx, time.Date(2025, 12, 30, 0, 0, 0, 0, time.UTC), time.Date(2026, 1, 5, 0, 0, 0, 0, time.UTC), "", "")
	if err != nil {
		t.Fatal(err)
	}
	if len(incomes) != 2 || len(refunds) != 1 {
		t.Fatalf("events income=%d refund=%d", len(incomes), len(refunds))
	}
	total, _, _ := aggregateProjection(incomes, refunds)
	if total.Gross != 300000 || total.Refund != 200000 || total.CommGross != 21000 || total.CommRefund != 14000 {
		t.Fatalf("aggregate=%#v", total)
	}
	if total.LegacyCount != 1 || total.SnapshotCount != 1 || projectionBasis(total.LegacyCount, total.SnapshotCount, ProjectionBasisHistorical) != ProjectionBasisMixed {
		t.Fatalf("source aggregate=%#v", total)
	}
	filteredIncome, filteredRefunds, _, _, err := (&repository{}).loadProjectionEvents(ctx, tx,
		time.Date(2025, 12, 30, 0, 0, 0, 0, time.UTC),
		time.Date(2026, 1, 5, 0, 0, 0, 0, time.UTC),
		"22222222-2222-2222-2222-222222222222", "33333333-3333-3333-3333-333333333333")
	if err != nil {
		t.Fatal(err)
	}
	if len(filteredIncome) != 2 || len(filteredRefunds) != 1 {
		t.Fatalf("filtered events income=%d refund=%d", len(filteredIncome), len(filteredRefunds))
	}
	noMatchIncome, noMatchRefunds, _, _, err := (&repository{}).loadProjectionEvents(ctx, tx,
		time.Date(2025, 12, 30, 0, 0, 0, 0, time.UTC),
		time.Date(2026, 1, 5, 0, 0, 0, 0, time.UTC),
		"99999999-9999-9999-9999-999999999999", "")
	if err != nil {
		t.Fatal(err)
	}
	if len(noMatchIncome) != 0 || len(noMatchRefunds) != 0 {
		t.Fatalf("owner filter leak income=%d refund=%d", len(noMatchIncome), len(noMatchRefunds))
	}
	cells, err := (&repository{}).loadProjectionBreakdownCells(ctx, tx,
		time.Date(2026, 1, 3, 0, 0, 0, 0, time.UTC),
		time.Date(2026, 1, 4, 0, 0, 0, 0, time.UTC),
		"", "")
	if err != nil {
		t.Fatal(err)
	}
	if len(cells) != 1 || cells[0].Gross != 0 || cells[0].Refund != 200000 || cells[0].NetCommission != -14000 || !cells[0].SnapshotPresent {
		t.Fatalf("refund-only breakdown cells=%#v", cells)
	}
}
