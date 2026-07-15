package platformfinance

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestHistoricalCommissionUsesExactHalfUp(t *testing.T) {
	tests := []struct{ amount, want int64 }{
		{100_000, 7_000},
		{10_000, 700},
		{155_000, 10_850},
		{5, 0},
		{8, 1},  // 8 * 700 / 10000 = 0.56
		{15, 1}, // 1.05 rounds to 1
	}
	for _, tc := range tests {
		got, err := calculateHistoricalCommission(tc.amount)
		if err != nil || got != tc.want {
			t.Fatalf("amount %d: got %d/%v, want %d", tc.amount, got, err, tc.want)
		}
	}
}

func TestProjectionBasisForEmptyRange(t *testing.T) {
	cutover := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	if got := projectionBasisForEmptyRange(cutover.AddDate(0, 0, -2), cutover, cutover); got != ProjectionBasisHistorical {
		t.Fatalf("pre-cutover basis=%q", got)
	}
	if got := projectionBasisForEmptyRange(cutover, cutover.AddDate(0, 0, 2), cutover); got != ProjectionBasisSnapshot {
		t.Fatalf("post-cutover basis=%q", got)
	}
	if got := projectionBasisForEmptyRange(cutover.Add(-time.Hour), cutover.Add(time.Hour), cutover); got != ProjectionBasisMixed {
		t.Fatalf("cross-cutover basis=%q", got)
	}
}

func TestProjectionBasisForClippedCalendarBucket(t *testing.T) {
	cutover := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	// The calendar week is Dec 29-Jan 4, but the requested report starts
	// after cutover. The effective range must therefore be snapshot-only.
	weekStart := time.Date(2025, 12, 29, 0, 0, 0, 0, time.UTC)
	weekEndExclusive := time.Date(2026, 1, 5, 0, 0, 0, 0, time.UTC)
	requestStart := time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC)
	requestEndExclusive := time.Date(2026, 1, 4, 0, 0, 0, 0, time.UTC)
	if requestStart.Before(weekStart) {
		requestStart = weekStart
	}
	if weekEndExclusive.After(requestEndExclusive) {
		weekEndExclusive = requestEndExclusive
	}
	if got := projectionBasisForEmptyRange(requestStart, weekEndExclusive, cutover); got != ProjectionBasisSnapshot {
		t.Fatalf("clipped post-cutover bucket basis=%q, want %q", got, ProjectionBasisSnapshot)
	}
}

func TestBuildContinuousBucketsClipsCalendarBasisToRequest(t *testing.T) {
	cutover := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	start := time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC)
	end := time.Date(2026, 1, 4, 0, 0, 0, 0, time.UTC)
	for _, granularity := range []string{"week", "month"} {
		t.Run(granularity, func(t *testing.T) {
			trend := buildContinuousBucketsAt(start, end, granularity, nil, nil, cutover)
			if len(trend) != 1 {
				t.Fatalf("trend length=%d, want 1", len(trend))
			}
			if trend[0].ProjectionBasis != ProjectionBasisSnapshot {
				t.Fatalf("basis=%q, want %q", trend[0].ProjectionBasis, ProjectionBasisSnapshot)
			}
		})
	}
}

func TestClassifyProjectionFailsClosedForSnapshotIntegrity(t *testing.T) {
	cutover := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	post := cutover.Add(time.Hour)
	tests := []struct {
		name                      string
		source                    string
		mode                      string
		channel                   string
		term, bpsValid            bool
		bps                       int32
		commission, final, ledger int64
		want                      error
	}{
		{"missing post-cutover snapshot", "", "", "", false, false, 0, 0, 0, 0, ErrMissingProjectionSnapshot},
		{"live policy", "POLICY", "LIVE", "MARKETPLACE_ONLINE", true, true, 700, 14000, 200000, 200000, ErrInvalidProjectionSource},
		{"policy without term", "POLICY", "SIMULATION", "MARKETPLACE_ONLINE", false, true, 700, 14000, 200000, 200000, ErrInvalidProjectionSource},
		{"policy arithmetic mismatch", "POLICY", "SIMULATION", "MARKETPLACE_ONLINE", true, true, 700, 13000, 200000, 200000, ErrProjectionSnapshotMismatch},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, _, err := classifyProjection(post, cutover, tc.source, tc.mode, tc.channel, tc.term, tc.bpsValid, tc.bps, tc.commission, tc.final, tc.ledger)
			if !errors.Is(err, tc.want) {
				t.Fatalf("error=%v want=%v", err, tc.want)
			}
		})
	}
}

func TestClassifyProjectionSourceAndRupiahMatrix(t *testing.T) {
	cutover := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	pre := cutover.Add(-time.Hour)
	tests := []struct {
		name       string
		source     string
		bps        int32
		commission int64
		final      int64
		wantBasis  string
		wantComm   int64
	}{
		{name: "policy zero bps", source: "POLICY", bps: 0, commission: 0, final: 100_000, wantBasis: ProjectionBasisSnapshot, wantComm: 0},
		{name: "policy five hundred bps", source: "POLICY", bps: 500, commission: 5_000, final: 100_000, wantBasis: ProjectionBasisSnapshot, wantComm: 5_000},
		{name: "policy seven hundred bps", source: "POLICY", bps: 700, commission: 10_850, final: 155_000, wantBasis: ProjectionBasisSnapshot, wantComm: 10_850},
		{name: "policy promo final basis", source: "POLICY", bps: 500, commission: 7_750, final: 155_000, wantBasis: ProjectionBasisSnapshot, wantComm: 7_750},
		{name: "legacy no commission historical", source: "LEGACY_NO_COMMISSION", bps: 0, commission: 0, final: 100_000, wantBasis: ProjectionBasisHistorical, wantComm: 0},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			basis, commission, err := classifyProjection(pre, cutover, tc.source, "SIMULATION", "MARKETPLACE_ONLINE", tc.source == "POLICY", true, tc.bps, tc.commission, tc.final, tc.final)
			if err != nil {
				t.Fatal(err)
			}
			if basis != tc.wantBasis || commission != tc.wantComm {
				t.Fatalf("basis/commission=%s/%d, want %s/%d", basis, commission, tc.wantBasis, tc.wantComm)
			}
		})
	}
}

func TestProjectionAggregateRefundOnlyKeepsSourcePresence(t *testing.T) {
	row := projectionRow{Event: projectionEvent{
		OwnerProfileID: "owner-1",
		VenueID:        "venue-1",
		Amount:         100_000,
		Commission:     7_000,
		Source:         ProjectionBasisSnapshot,
	}}
	total, _, _ := aggregateProjection(nil, []projectionRow{row})
	if total.SnapshotCount != 0 || !total.SnapshotPresent {
		t.Fatalf("refund-only aggregate=%#v", total)
	}
	if got := projectionBasisWithPresence(total.LegacyCount, total.SnapshotCount, total.LegacyPresent, total.SnapshotPresent, ProjectionBasisHistorical); got != ProjectionBasisSnapshot {
		t.Fatalf("refund-only basis=%q", got)
	}
}

type projectionResponseRepo struct{}

func (projectionResponseRepo) OwnerMatchesVenue(context.Context, string, string) (bool, error) {
	return true, nil
}
func (projectionResponseRepo) GetSummaryData(context.Context, time.Time, time.Time, string, string) (*SummaryDataResult, error) {
	return &SummaryDataResult{
		AsOf:  time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC),
		Gross: 300_000, ProjectedCommGross: 14_000, RealizedBookingCount: 2,
		ProjectionBasis: ProjectionBasisMixed, LegacyScenarioCount: 1, SnapshotProjectionCount: 1,
		LegacyProjectionAmount: 7_000, SnapshotProjectionAmount: 7_000,
		IncomeBuckets: []BucketResult{{Bucket: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), Amount: 100_000, Comm: 7_000, Source: ProjectionBasisHistorical}, {Bucket: time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC), Amount: 200_000, Comm: 7_000, Source: ProjectionBasisSnapshot}},
		RefundBuckets: []BucketResult{},
	}, nil
}
func (projectionResponseRepo) GetPaginatedBreakdown(context.Context, time.Time, time.Time, string, string, string, int, int) (*BreakdownResult, error) {
	return &BreakdownResult{AsOf: time.Now(), ProjectionBasis: ProjectionBasisMixed}, nil
}

func TestServiceProjectionSourceContract(t *testing.T) {
	res, err := NewService(projectionResponseRepo{}).GetSummary(context.Background(), FinanceQuery{StartDate: "2026-01-01", EndDate: "2026-01-02", Granularity: "day"})
	if err != nil {
		t.Fatal(err)
	}
	if res.ProjectionBasis != ProjectionBasisMixed || res.MetricSourceVersion != ProjectionMetricVersion {
		t.Fatalf("source contract = %#v", res)
	}
	if res.DataQuality.LegacyScenarioCount != 1 || res.DataQuality.SnapshotProjectionCount != 1 || res.DataQuality.NonBillableProjectionAmount != "7000" || res.DataQuality.SnapshotProjectionAmount != "7000" {
		t.Fatalf("quality = %#v", res.DataQuality)
	}
	if len(res.Trend) != 2 || res.Trend[0].ProjectionBasis != ProjectionBasisHistorical || res.Trend[1].ProjectionBasis != ProjectionBasisSnapshot {
		t.Fatalf("trend source metadata = %#v", res.Trend)
	}
}

type projectionBreakdownResponseRepo struct {
	page, limit int
}

func (r *projectionBreakdownResponseRepo) OwnerMatchesVenue(context.Context, string, string) (bool, error) {
	return true, nil
}

func (r *projectionBreakdownResponseRepo) GetSummaryData(context.Context, time.Time, time.Time, string, string) (*SummaryDataResult, error) {
	return &SummaryDataResult{AsOf: time.Date(2026, 1, 3, 0, 0, 0, 0, time.UTC)}, nil
}

func (r *projectionBreakdownResponseRepo) GetPaginatedBreakdown(_ context.Context, _ time.Time, _ time.Time, _ string, _ string, _ string, page, limit int) (*BreakdownResult, error) {
	r.page, r.limit = page, limit
	return &BreakdownResult{
		AsOf:                        time.Date(2026, 1, 3, 0, 0, 0, 0, time.UTC),
		TotalItems:                  3,
		Rows:                        []BreakdownRow{{ID: "owner-1", Name: "Owner", Net: 100_000, BookingCount: 1, NetComm: 7_000, ProjectionBasis: ProjectionBasisSnapshot, SnapshotProjectionCount: 1, SnapshotProjectionAmount: 7_000}},
		ProjectionBasis:             ProjectionBasisSnapshot,
		SnapshotProjectionCount:     1,
		SnapshotProjectionAmount:    7_000,
		SnapshotProjectionPresent:   true,
		LegacyProjectionPresent:     false,
		LegacyScenarioCount:         0,
		NonBillableProjectionAmount: 0,
	}, nil
}

func TestServiceBreakdownSourceContractAndPagination(t *testing.T) {
	repo := &projectionBreakdownResponseRepo{}
	res, err := NewService(repo).GetBreakdown(context.Background(), FinanceBreakdownQuery{
		FinanceQuery: FinanceQuery{StartDate: "2026-01-01", EndDate: "2026-01-31"},
		Dimension:    "owner", Page: 2, Limit: 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	if repo.page != 2 || repo.limit != 1 || res.TotalItems != 3 || res.TotalPages != 3 {
		t.Fatalf("pagination response=%#v page=%d limit=%d", res, repo.page, repo.limit)
	}
	if res.ProjectionBasis != ProjectionBasisSnapshot || res.SnapshotProjectionCount != 1 || res.SnapshotProjectionAmount != "7000" || res.MetricSourceVersion != ProjectionMetricVersion {
		t.Fatalf("breakdown source response=%#v", res)
	}
	items, ok := res.Data.([]any)
	if !ok || len(items) != 1 {
		t.Fatalf("breakdown items=%#v", res.Data)
	}
	item, ok := items[0].(TopOwnerItem)
	if !ok || item.ProjectionBasis != ProjectionBasisSnapshot || item.SnapshotProjectionAmount != "7000" {
		t.Fatalf("breakdown item=%#v", items[0])
	}
}
