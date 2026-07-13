package platformfinance_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"reflect"
	"strconv"
	"testing"
	"time"

	"lapangango-api/internal/database"
	"lapangango-api/internal/platformfinance"
)

func TestManualReconciliation(t *testing.T) {
	if os.Getenv("TEST_INTEGRATION") != "1" {
		t.Skip("Skipping integration test")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
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

	repo := platformfinance.NewRepository(pool)
	svc := platformfinance.NewService(repo)

	// Summary
	query := platformfinance.FinanceQuery{
		StartDate: "2026-06-01",
		EndDate:   "2026-07-31",
	}
	res, err := svc.GetSummary(ctx, query)
	if err != nil {
		t.Fatal(err)
	}
	b, _ := json.MarshalIndent(res, "", "  ")
	fmt.Println("=== SUMMARY ===")
	fmt.Println(string(b))

	var trendGross, trendRefund, trendNet, trendCommission int64
	for _, item := range res.Trend {
		trendGross += mustMoney(t, item.OnlineGMVGross)
		trendRefund += mustMoney(t, item.RefundPrincipal)
		trendNet += mustMoney(t, item.OnlineGMVNet)
		trendCommission += mustMoney(t, item.ProjectedCommission)
	}
	assertMoneyEqual(t, "trend gross", res.Metrics.OnlineGMVGross, trendGross)
	assertMoneyEqual(t, "trend refund", res.Metrics.RefundPrincipal, trendRefund)
	assertMoneyEqual(t, "trend net", res.Metrics.OnlineGMVNet, trendNet)
	assertMoneyEqual(t, "trend commission", res.Metrics.ProjectedCommission, trendCommission)

	// Breakdown Owner
	bQuery := platformfinance.FinanceBreakdownQuery{
		FinanceQuery: query,
		Dimension:    "owner",
		Page:         1,
		Limit:        20,
	}
	bRes, err := svc.GetBreakdown(ctx, bQuery)
	if err != nil {
		t.Fatal(err)
	}
	b2, _ := json.MarshalIndent(bRes, "", "  ")
	fmt.Println("=== BREAKDOWN OWNER ===")
	fmt.Println(string(b2))
	if bRes.Mode != "SIMULATION" {
		t.Fatalf("owner breakdown mode = %q", bRes.Mode)
	}

	venueQuery := bQuery
	venueQuery.Dimension = "venue"
	venueRes, err := svc.GetBreakdown(ctx, venueQuery)
	if err != nil {
		t.Fatal(err)
	}
	b3, _ := json.MarshalIndent(venueRes, "", "  ")
	fmt.Println("=== BREAKDOWN VENUE ===")
	fmt.Println(string(b3))
	if venueRes.Mode != "SIMULATION" {
		t.Fatalf("venue breakdown mode = %q", venueRes.Mode)
	}

	owners := collectOwners(t, ctx, svc, query)
	venues := collectVenues(t, ctx, svc, query)
	reconcileOwners(t, res, owners)
	reconcileVenues(t, res, venues)
	assertTopOwnerBasis(t, res.TopOwnerBreakdown, owners)
	assertTopVenueBasis(t, res.TopVenueBreakdown, venues)

	// Same request must keep deterministic ordering while data is unchanged.
	again, err := svc.GetBreakdown(ctx, bQuery)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(bRes.Data, again.Data) {
		t.Fatal("owner breakdown ordering is not deterministic")
	}

	// Prove owner+venue scoping and mismatch rejection using current fixtures.
	if len(res.TopVenueBreakdown) > 0 {
		selected := res.TopVenueBreakdown[0]
		filtered, err := svc.GetSummary(ctx, platformfinance.FinanceQuery{
			StartDate: query.StartDate, EndDate: query.EndDate,
			OwnerProfileID: selected.OwnerProfileID, VenueID: selected.VenueID,
		})
		if err != nil {
			t.Fatal(err)
		}
		for _, item := range filtered.TopOwnerBreakdown {
			if item.OwnerProfileID != selected.OwnerProfileID {
				t.Fatalf("owner filter leaked owner %s", item.OwnerProfileID)
			}
		}
		for _, item := range filtered.TopVenueBreakdown {
			if item.VenueID != selected.VenueID || item.OwnerProfileID != selected.OwnerProfileID {
				t.Fatalf("venue filter leaked venue %#v", item)
			}
		}

		for _, owner := range res.TopOwnerBreakdown {
			if owner.OwnerProfileID == selected.OwnerProfileID {
				continue
			}
			_, err := svc.GetSummary(ctx, platformfinance.FinanceQuery{
				StartDate: query.StartDate, EndDate: query.EndDate,
				OwnerProfileID: owner.OwnerProfileID, VenueID: selected.VenueID,
			})
			if !errors.Is(err, platformfinance.ErrOwnerVenueMismatch) {
				t.Fatalf("owner/venue mismatch error = %v", err)
			}
			break
		}
	}
}

func mustMoney(t *testing.T, value string) int64 {
	t.Helper()
	v, err := strconv.ParseInt(value, 10, 64)
	if err != nil {
		t.Fatalf("invalid integer-rupiah string %q: %v", value, err)
	}
	return v
}

func assertMoneyEqual(t *testing.T, label, expected string, actual int64) {
	t.Helper()
	if want := mustMoney(t, expected); want != actual {
		t.Fatalf("%s = %d, want %d", label, actual, want)
	}
}

func collectOwners(t *testing.T, ctx context.Context, svc platformfinance.Service, query platformfinance.FinanceQuery) []platformfinance.TopOwnerItem {
	t.Helper()
	var all []platformfinance.TopOwnerItem
	for page := 1; ; page++ {
		res, err := svc.GetBreakdown(ctx, platformfinance.FinanceBreakdownQuery{FinanceQuery: query, Dimension: "owner", Page: page, Limit: 100})
		if err != nil {
			t.Fatal(err)
		}
		for _, raw := range res.Data.([]any) {
			all = append(all, raw.(platformfinance.TopOwnerItem))
		}
		if page >= res.TotalPages {
			break
		}
	}
	return all
}

func collectVenues(t *testing.T, ctx context.Context, svc platformfinance.Service, query platformfinance.FinanceQuery) []platformfinance.TopVenueItem {
	t.Helper()
	var all []platformfinance.TopVenueItem
	for page := 1; ; page++ {
		res, err := svc.GetBreakdown(ctx, platformfinance.FinanceBreakdownQuery{FinanceQuery: query, Dimension: "venue", Page: page, Limit: 100})
		if err != nil {
			t.Fatal(err)
		}
		for _, raw := range res.Data.([]any) {
			all = append(all, raw.(platformfinance.TopVenueItem))
		}
		if page >= res.TotalPages {
			break
		}
	}
	return all
}

func reconcileOwners(t *testing.T, summary *platformfinance.SummaryResponse, items []platformfinance.TopOwnerItem) {
	t.Helper()
	var net, commission int64
	var bookings int
	for _, item := range items {
		net += mustMoney(t, item.OnlineGMVNet)
		commission += mustMoney(t, item.ProjectedCommission)
		bookings += item.RealizedOnlineBookingCount
	}
	assertMoneyEqual(t, "owner breakdown net", summary.Metrics.OnlineGMVNet, net)
	assertMoneyEqual(t, "owner breakdown commission", summary.Metrics.ProjectedCommission, commission)
	if bookings != summary.Metrics.RealizedOnlineBookingCount {
		t.Fatalf("owner breakdown bookings = %d, want %d", bookings, summary.Metrics.RealizedOnlineBookingCount)
	}
}

func reconcileVenues(t *testing.T, summary *platformfinance.SummaryResponse, items []platformfinance.TopVenueItem) {
	t.Helper()
	var net, commission int64
	var bookings int
	for _, item := range items {
		net += mustMoney(t, item.OnlineGMVNet)
		commission += mustMoney(t, item.ProjectedCommission)
		bookings += item.RealizedOnlineBookingCount
	}
	assertMoneyEqual(t, "venue breakdown net", summary.Metrics.OnlineGMVNet, net)
	assertMoneyEqual(t, "venue breakdown commission", summary.Metrics.ProjectedCommission, commission)
	if bookings != summary.Metrics.RealizedOnlineBookingCount {
		t.Fatalf("venue breakdown bookings = %d, want %d", bookings, summary.Metrics.RealizedOnlineBookingCount)
	}
}

func assertTopOwnerBasis(t *testing.T, top []platformfinance.TopOwnerItem, all []platformfinance.TopOwnerItem) {
	t.Helper()
	if len(top) > len(all) {
		t.Fatalf("top owner length %d exceeds full breakdown %d", len(top), len(all))
	}
	for i := range top {
		if top[i] != all[i] {
			t.Fatalf("top owner %d = %#v, full breakdown = %#v", i, top[i], all[i])
		}
	}
}

func assertTopVenueBasis(t *testing.T, top []platformfinance.TopVenueItem, all []platformfinance.TopVenueItem) {
	t.Helper()
	if len(top) > len(all) {
		t.Fatalf("top venue length %d exceeds full breakdown %d", len(top), len(all))
	}
	for i := range top {
		if top[i] != all[i] {
			t.Fatalf("top venue %d = %#v, full breakdown = %#v", i, top[i], all[i])
		}
	}
}
