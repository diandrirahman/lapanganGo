package platformfinance_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"lapangango-api/internal/platformfinance"
)

func TestService_Summary_CalculationRules(t *testing.T) {
	repo := &detailedMockRepo{summary: &platformfinance.SummaryDataResult{
		AsOf:                   time.Now(),
		Gross:                  1_000_000,
		RefundPrincipal:        200_000,
		ProjectedCommGross:     70_000,
		ProjectedCommRefunded:  14_000,
		RealizedBookingCount:   8,
		RefundedBookingCount:   1,
		ProjectionBasis:        platformfinance.ProjectionBasisHistorical,
		LegacyScenarioCount:    8,
		LegacyProjectionAmount: 56_000,
		LegacyGross:            1_000_000,
	}}
	res, err := platformfinance.NewService(repo).GetSummary(context.Background(), platformfinance.FinanceQuery{
		StartDate: "2026-06-01",
		EndDate:   "2026-06-01",
	})
	assert.NoError(t, err)
	assert.Equal(t, "1000000", res.Metrics.OnlineGMVGross)
	assert.Equal(t, "200000", res.Metrics.RefundPrincipal)
	assert.Equal(t, "800000", res.Metrics.OnlineGMVNet)
	assert.Equal(t, "56000", res.Metrics.ProjectedCommission)
	assert.Equal(t, "744000", res.Metrics.ProjectedOwnerNetAfterHypotheticalCommission)
	assert.Equal(t, 700, *res.Metrics.ProjectedTakeRateBps)
	assert.Equal(t, 8, res.DataQuality.LegacyScenarioCount)
	assert.Equal(t, "56000", res.DataQuality.NonBillableProjectionAmount)
}

func TestService_Summary_RejectsMissingProjectionBasis(t *testing.T) {
	repo := &detailedMockRepo{summary: &platformfinance.SummaryDataResult{
		AsOf:                 time.Now(),
		Gross:                100_000,
		ProjectedCommGross:   7_000,
		RealizedBookingCount: 1,
	}}
	_, err := platformfinance.NewService(repo).GetSummary(context.Background(), platformfinance.FinanceQuery{
		StartDate: "2026-06-01",
		EndDate:   "2026-06-01",
	})
	assert.ErrorIs(t, err, platformfinance.ErrProjectionIntegrity)
}

func TestService_Summary_RejectsMissingProjectionBasisForRefundOnly(t *testing.T) {
	repo := &detailedMockRepo{summary: &platformfinance.SummaryDataResult{
		AsOf:                  time.Now(),
		RefundPrincipal:       100_000,
		ProjectedCommRefunded: 7_000,
		RefundedBookingCount:  1,
	}}
	_, err := platformfinance.NewService(repo).GetSummary(context.Background(), platformfinance.FinanceQuery{
		StartDate: "2026-06-01",
		EndDate:   "2026-06-01",
	})
	assert.ErrorIs(t, err, platformfinance.ErrProjectionIntegrity)
}

type detailedMockRepo struct {
	summary *platformfinance.SummaryDataResult
	err     error
}

func (m *detailedMockRepo) OwnerMatchesVenue(ctx context.Context, ownerProfileID, venueID string) (bool, error) {
	return true, m.err
}

func (m *detailedMockRepo) GetSummaryData(ctx context.Context, start, end time.Time, ownerID, venueID string) (*platformfinance.SummaryDataResult, error) {
	return m.summary, m.err
}

func (m *detailedMockRepo) GetPaginatedBreakdown(ctx context.Context, start, end time.Time, ownerID, venueID, dimension string, page, limit int) (*platformfinance.BreakdownResult, error) {
	return nil, m.err
}

func TestService_Summary_NegativeGMV_And_Reversal(t *testing.T) {
	bucketTime, _ := time.Parse("2006-01-02", "2026-06-01")
	summaryData := &platformfinance.SummaryDataResult{
		AsOf:                  time.Now(),
		Gross:                 0,
		RefundPrincipal:       100000,
		ProjectedCommGross:    0,
		ProjectedCommRefunded: 7000,
		IncomeBuckets: []platformfinance.BucketResult{
			{Bucket: bucketTime, Amount: 0, Comm: 0},
		},
		RefundBuckets: []platformfinance.BucketResult{
			{Bucket: bucketTime, Amount: 100000, Comm: 7000},
		},
	}

	repo := &detailedMockRepo{summary: summaryData, err: nil}
	svc := platformfinance.NewService(repo)

	query := platformfinance.FinanceQuery{
		StartDate: "2026-06-01",
		EndDate:   "2026-06-07",
	}

	res, err := svc.GetSummary(context.Background(), query)
	assert.NoError(t, err)

	assert.Equal(t, "-100000", res.Metrics.OnlineGMVNet)
	assert.Equal(t, "-7000", res.Metrics.ProjectedCommission)

	// Test available zero vs unavailable null
	assert.Nil(t, res.Metrics.PlatformOperatingExpense)
	assert.Nil(t, res.Metrics.ActualCommissionRevenue)
	assert.Nil(t, res.Metrics.PaymentProcessingExpense)
	assert.Nil(t, res.Metrics.PlatformRevenue)

	// Trend array contains the bucket with negative numbers (first day)
	assert.Len(t, res.Trend, 7)
	assert.Equal(t, "-100000", res.Trend[0].OnlineGMVNet)
	assert.Equal(t, "-7000", res.Trend[0].ProjectedCommission)

	// Check if take rate is nil since net gmv <= 0
	assert.Nil(t, res.Metrics.ProjectedTakeRateBps)
}

func TestService_Summary_EmptyResponse(t *testing.T) {
	summaryData := &platformfinance.SummaryDataResult{
		AsOf:          time.Now(),
		IncomeBuckets: []platformfinance.BucketResult{},
		RefundBuckets: []platformfinance.BucketResult{},
	}

	repo := &detailedMockRepo{summary: summaryData, err: nil}
	svc := platformfinance.NewService(repo)

	query := platformfinance.FinanceQuery{
		StartDate:   "2026-06-01", // Monday
		EndDate:     "2026-06-14",
		Granularity: "week",
	}

	res, err := svc.GetSummary(context.Background(), query)
	assert.NoError(t, err)

	assert.Equal(t, "0", res.Metrics.OnlineGMVNet)
	assert.Len(t, res.Trend, 2)
	assert.Equal(t, "2026-06-01", res.Trend[0].PeriodStart)
	assert.Equal(t, "2026-06-07", res.Trend[0].PeriodEnd)
	assert.Equal(t, "2026-06-08", res.Trend[1].PeriodStart)
	assert.Equal(t, "2026-06-14", res.Trend[1].PeriodEnd)

	assert.Equal(t, "0", res.Trend[0].OnlineGMVNet)
	assert.Equal(t, "0", res.Trend[1].OnlineGMVNet)
}
