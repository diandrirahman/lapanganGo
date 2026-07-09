package finance

import (
	"context"
	"lapangango-api/internal/httputil"
	"testing"
)

type mockRepository struct{}

func (m *mockRepository) VerifyVenueOwnership(ctx context.Context, venueID string, ownerID string) error {
	return nil
}

func (m *mockRepository) CreateTransaction(ctx context.Context, tx FinanceTransaction) (FinanceTransaction, error) {
	return tx, nil
}

func (m *mockRepository) GetTransaction(ctx context.Context, id string, ownerID string) (FinanceTransaction, error) {
	return FinanceTransaction{ID: id, OwnerID: ownerID, Source: "MANUAL"}, nil
}

func (m *mockRepository) UpdateTransaction(ctx context.Context, id string, ownerID string, req UpdateTransactionRequest) (FinanceTransaction, error) {
	return FinanceTransaction{}, nil
}

func (m *mockRepository) DeleteTransaction(ctx context.Context, id string, ownerID string) error {
	return nil
}

func (m *mockRepository) GetTransactions(ctx context.Context, ownerID string, query TransactionQuery) ([]FinanceTransaction, int, error) {
	return []FinanceTransaction{}, 0, nil
}

func (m *mockRepository) GetFinanceSummary(ctx context.Context, ownerID string, req FinanceSummaryQuery) (FinanceSummaryResult, error) {
	return FinanceSummaryResult{
		TotalIncome: 1000,
	}, nil
}

func TestGetOwnerFinanceSummary(t *testing.T) {
	svc := NewService(&mockRepository{})
	ownerCtx := httputil.OwnerContext{
		ActorUserID:          "user1",
		EffectiveOwnerUserID: "user1",
		IsOwner:              true,
	}
	res, err := svc.GetFinanceSummary(context.Background(), ownerCtx, FinanceSummaryQuery{})
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if res.TotalIncome != 1000 {
		t.Errorf("expected 1000, got %f", res.TotalIncome)
	}
}
