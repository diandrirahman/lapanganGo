package commercialterms

import (
	"context"
)

type Service interface {
	GetTerms(ctx context.Context, query GetTermsQuery) (PaginatedTermsResponse, error)
	Preview(ctx context.Context, req PreviewRequest) (PreviewResponse, error)
}

type service struct {
	repo Repository
}

func NewService(repo Repository) Service {
	return &service{repo: repo}
}

func (s *service) GetTerms(ctx context.Context, query GetTermsQuery) (PaginatedTermsResponse, error) {
	return s.repo.GetTerms(ctx, query)
}

func (s *service) Preview(ctx context.Context, req PreviewRequest) (PreviewResponse, error) {
	if err := req.Validate(); err != nil {
		return PreviewResponse{}, err
	}

	bookingAmounts := []int64{100_000, 200_000, 500_000}
	var scenarios []PreviewScenario

	for _, amount := range bookingAmounts {
		commission := (amount * int64(req.CommissionBps)) / 10000
		net := amount - commission

		scenarios = append(scenarios, PreviewScenario{
			BookingAmountInt64:        amount,
			CommissionBps:             req.CommissionBps,
			ProjectedCommissionRupiah: commission,
			ProjectedOwnerNetRupiah:   net,
		})
	}

	return PreviewResponse{
		FinanceMode:      req.FinanceMode,
		CollectionMethod: req.CollectionMethod,
		Scenarios:        scenarios,
	}, nil
}
