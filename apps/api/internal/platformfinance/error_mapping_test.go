package platformfinance_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"lapangango-api/internal/platformfinance"
)

type errorMockRepo struct {
	detailedMockRepo
	errToReturn error
}

func (m *errorMockRepo) OwnerMatchesVenue(ctx context.Context, ownerProfileID, venueID string) (bool, error) {
	return true, m.err
}

func (m *errorMockRepo) GetSummaryData(ctx context.Context, start, end time.Time, ownerID, venueID string) (*platformfinance.SummaryDataResult, error) {
	return nil, m.errToReturn
}

func TestService_ErrorMappings(t *testing.T) {
	testCases := []struct {
		name        string
		repoErr     error
		expectedErr error
	}{
		{
			name:        "fractional_ledger",
			repoErr:     platformfinance.ErrFractionalLedgerDetected,
			expectedErr: platformfinance.ErrFractionalLedgerDetected,
		},
		{
			name:        "duplicate_ledger",
			repoErr:     platformfinance.ErrDuplicateLedgerDetected,
			expectedErr: platformfinance.ErrDuplicateLedgerDetected,
		},
		{
			name:        "overflow",
			repoErr:     platformfinance.ErrOverflowDetected,
			expectedErr: platformfinance.ErrOverflowDetected,
		},
		{
			name:        "refund_amount_mismatch",
			repoErr:     platformfinance.ErrRefundAmountMismatch,
			expectedErr: platformfinance.ErrRefundAmountMismatch,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			repo := &errorMockRepo{errToReturn: tc.repoErr}
			svc := platformfinance.NewService(repo)

			_, err := svc.GetSummary(context.Background(), platformfinance.FinanceQuery{
				StartDate: "2026-06-01", EndDate: "2026-06-30",
			})
			assert.Error(t, err)
			assert.Equal(t, tc.expectedErr, err)
		})
	}
}
