package platformfinance

import (
	"context"
	"encoding/json"
	"errors"
	"math"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
)

type mismatchedFilterRepo struct{}

func (mismatchedFilterRepo) OwnerMatchesVenue(context.Context, string, string) (bool, error) {
	return false, nil
}

func (mismatchedFilterRepo) GetSummaryData(context.Context, time.Time, time.Time, string, string) (*SummaryDataResult, error) {
	return nil, errors.New("GetSummaryData must not be reached for mismatched filters")
}

func (mismatchedFilterRepo) GetPaginatedBreakdown(context.Context, time.Time, time.Time, string, string, string, int, int) (*BreakdownResult, error) {
	return nil, errors.New("GetPaginatedBreakdown must not be reached for mismatched filters")
}

type contractResponseRepo struct{}

func (contractResponseRepo) OwnerMatchesVenue(context.Context, string, string) (bool, error) {
	return true, nil
}

func (contractResponseRepo) GetSummaryData(context.Context, time.Time, time.Time, string, string) (*SummaryDataResult, error) {
	return &SummaryDataResult{AsOf: time.Now(), PlatformOperatingExpense: 123}, nil
}

func (contractResponseRepo) GetPaginatedBreakdown(context.Context, time.Time, time.Time, string, string, string, int, int) (*BreakdownResult, error) {
	return &BreakdownResult{AsOf: time.Now(), Rows: []BreakdownRow{}, PlatformOperatingExpense: 123}, nil
}

func TestCalculateBpsUsesExactIntegerRounding(t *testing.T) {
	tests := []struct {
		name        string
		numerator   int64
		denominator int64
		want        int
		wantErr     error
	}{
		{name: "truncates below half", numerator: 1, denominator: 8_000, want: 1},
		{name: "rounds positive half up", numerator: 1, denominator: 20_000, want: 1},
		{name: "rounds negative half away from zero", numerator: -1, denominator: 20_000, want: -1},
		{name: "fails rather than overflowing", numerator: math.MaxInt64, denominator: 1, wantErr: ErrOverflowDetected},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := calculateBps(tt.numerator, tt.denominator)
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("error = %v, want %v", err, tt.wantErr)
			}
			if err == nil && got != tt.want {
				t.Fatalf("bps = %d, want %d", got, tt.want)
			}
		})
	}
}

func TestBuildFiltersResolvesOwnerProfileToUserID(t *testing.T) {
	filters, args := buildFilters("owner-profile-id", "venue-id", 3)
	if !strings.Contains(filters, "t.owner_id = (SELECT user_id FROM owner_profiles WHERE id = $3)") {
		t.Fatalf("owner filter must resolve owner profile ID, got %q", filters)
	}
	if len(args) != 2 || args[0] != "owner-profile-id" || args[1] != "venue-id" {
		t.Fatalf("unexpected filter arguments: %#v", args)
	}
}

func TestParseAndValidateDatesEnforcesInclusive366DayLimit(t *testing.T) {
	if _, _, err := ParseAndValidateDates("2026-01-01", "2027-01-01"); err != nil {
		t.Fatalf("366 inclusive days should be allowed: %v", err)
	}
	if _, _, err := ParseAndValidateDates("2026-01-01", "2027-01-02"); !errors.Is(err, ErrDateRangeTooLarge) {
		t.Fatalf("367 inclusive days error = %v, want ErrDateRangeTooLarge", err)
	}
}

func TestResponseContractUsesCanonicalAvailabilityAndSimulationMode(t *testing.T) {
	svc := NewService(contractResponseRepo{})
	summary, err := svc.GetSummary(context.Background(), FinanceQuery{StartDate: "2026-06-01", EndDate: "2026-06-01"})
	if err != nil {
		t.Fatal(err)
	}
	raw, err := json.Marshal(summary)
	if err != nil {
		t.Fatal(err)
	}
	var body map[string]any
	if err := json.Unmarshal(raw, &body); err != nil {
		t.Fatal(err)
	}
	availability, ok := body["data_availability"].(map[string]any)
	if !ok {
		t.Fatalf("data_availability has unexpected type: %T", body["data_availability"])
	}
	wantAvailability := []string{"platform_operating_expense", "actual_platform_revenue", "payment_processing_expense", "owner_payable"}
	if len(availability) != len(wantAvailability) {
		t.Fatalf("data_availability keys = %#v, want exactly %#v", availability, wantAvailability)
	}
	for _, key := range wantAvailability {
		if _, exists := availability[key]; !exists {
			t.Fatalf("data_availability missing %q", key)
		}
	}

	breakdown, err := svc.GetBreakdown(context.Background(), FinanceBreakdownQuery{
		FinanceQuery: FinanceQuery{StartDate: "2026-06-01", EndDate: "2026-06-01"},
		Dimension:    "owner",
	})
	if err != nil {
		t.Fatal(err)
	}
	if breakdown.Mode != "SIMULATION" {
		t.Fatalf("breakdown mode = %q, want SIMULATION", breakdown.Mode)
	}
	if breakdown.PlatformOperatingExpense == nil || *breakdown.PlatformOperatingExpense != "123" {
		t.Fatalf("breakdown platform_operating_expense = %v, want 123", breakdown.PlatformOperatingExpense)
	}
	if breakdown.DataAvailability.PlatformOperatingExpense != "AVAILABLE" {
		t.Fatalf("breakdown OPEX availability = %q, want AVAILABLE", breakdown.DataAvailability.PlatformOperatingExpense)
	}
}

func TestServiceScopedBreakdownDoesNotExposeGlobalOPEX(t *testing.T) {
	svc := NewService(contractResponseRepo{})
	for name, query := range map[string]FinanceBreakdownQuery{
		"owner": {
			FinanceQuery: FinanceQuery{StartDate: "2026-06-01", EndDate: "2026-06-01", OwnerProfileID: "00000000-0000-0000-0000-000000000001"},
			Dimension:    "owner",
		},
		"venue": {
			FinanceQuery: FinanceQuery{StartDate: "2026-06-01", EndDate: "2026-06-01", VenueID: "00000000-0000-0000-0000-000000000002"},
			Dimension:    "venue",
		},
	} {
		t.Run(name, func(t *testing.T) {
			breakdown, err := svc.GetBreakdown(context.Background(), query)
			if err != nil {
				t.Fatal(err)
			}
			if breakdown.PlatformOperatingExpense != nil {
				t.Fatalf("scoped breakdown OPEX = %v, want nil without allocation", *breakdown.PlatformOperatingExpense)
			}
			if breakdown.DataAvailability.PlatformOperatingExpense != "UNAVAILABLE_UNTIL_SCOPE_ALLOCATION" {
				t.Fatalf("scoped breakdown OPEX availability = %q, want UNAVAILABLE_UNTIL_SCOPE_ALLOCATION", breakdown.DataAvailability.PlatformOperatingExpense)
			}
		})
	}
}

func TestServiceScopedSummaryDoesNotSubtractGlobalOPEX(t *testing.T) {
	svc := NewService(contractResponseRepo{})
	for name, query := range map[string]FinanceQuery{
		"owner": {StartDate: "2026-06-01", EndDate: "2026-06-01", OwnerProfileID: "00000000-0000-0000-0000-000000000001"},
		"venue": {StartDate: "2026-06-01", EndDate: "2026-06-01", VenueID: "00000000-0000-0000-0000-000000000002"},
	} {
		t.Run(name, func(t *testing.T) {
			res, err := svc.GetSummary(context.Background(), query)
			if err != nil {
				t.Fatal(err)
			}
			if res.Metrics.ProjectedOperatingResultBeforeTransactionCosts != nil {
				t.Fatalf("scoped projected operating result = %v, want nil without OPEX allocation", *res.Metrics.ProjectedOperatingResultBeforeTransactionCosts)
			}
			if res.Metrics.PlatformOperatingExpense == nil || *res.Metrics.PlatformOperatingExpense != "123" {
				t.Fatalf("scoped OPEX = %v, want exact global 123", res.Metrics.PlatformOperatingExpense)
			}
		})
	}
}

func TestCanonicalPredicateRequiresBookingCourtVenueAndOwner(t *testing.T) {
	for _, fragment := range []string{"FROM bookings", "JOIN courts", "JOIN venues", "JOIN owner_profiles", "b.id = t.booking_id", "v.id = t.venue_id", "op.user_id = t.owner_id"} {
		if !strings.Contains(canonicalLedgerBookingPredicate, fragment) {
			t.Fatalf("canonical predicate missing %q: %s", fragment, canonicalLedgerBookingPredicate)
		}
	}
}

func TestMapRepositoryErrorUsesNumericOverflowSQLState(t *testing.T) {
	err := mapRepositoryError(&pgconn.PgError{Code: "22003"})
	if !errors.Is(err, ErrOverflowDetected) {
		t.Fatalf("error = %v, want ErrOverflowDetected", err)
	}
}

func TestServiceRejectsOwnerVenueMismatch(t *testing.T) {
	svc := NewService(mismatchedFilterRepo{})
	_, err := svc.GetSummary(context.Background(), FinanceQuery{
		StartDate:      "2026-06-01",
		EndDate:        "2026-06-01",
		OwnerProfileID: "00000000-0000-0000-0000-000000000001",
		VenueID:        "00000000-0000-0000-0000-000000000002",
	})
	if !errors.Is(err, ErrOwnerVenueMismatch) {
		t.Fatalf("error = %v, want ErrOwnerVenueMismatch", err)
	}
}

func TestBuildContinuousBucketsKeepsCalendarWeekAndMonthBoundaries(t *testing.T) {
	start := time.Date(2026, time.June, 3, 0, 0, 0, 0, jakartaLocation).UTC()
	end := time.Date(2026, time.June, 9, 0, 0, 0, 0, jakartaLocation).UTC()
	weeks := buildContinuousBuckets(start, end, "week", nil, nil)
	if len(weeks) != 2 || weeks[0].PeriodStart != "2026-06-01" || weeks[0].PeriodEnd != "2026-06-07" || weeks[1].PeriodStart != "2026-06-08" || weeks[1].PeriodEnd != "2026-06-14" {
		t.Fatalf("unexpected calendar-week buckets: %#v", weeks)
	}

	monthStart := time.Date(2026, time.June, 15, 0, 0, 0, 0, jakartaLocation).UTC()
	monthEnd := time.Date(2026, time.July, 3, 0, 0, 0, 0, jakartaLocation).UTC()
	months := buildContinuousBuckets(monthStart, monthEnd, "month", nil, nil)
	if len(months) != 2 || months[0].PeriodStart != "2026-06-01" || months[0].PeriodEnd != "2026-06-30" || months[1].PeriodStart != "2026-07-01" || months[1].PeriodEnd != "2026-07-31" {
		t.Fatalf("unexpected calendar-month buckets: %#v", months)
	}
}
