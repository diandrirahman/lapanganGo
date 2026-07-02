package finance

import (
	"context"
)

type Service interface {
	CreateTransaction(ctx context.Context, ownerID string, req CreateTransactionRequest) (FinanceTransaction, error)
	UpdateTransaction(ctx context.Context, id string, ownerID string, req UpdateTransactionRequest) (FinanceTransaction, error)
	DeleteTransaction(ctx context.Context, id string, ownerID string) error
	GetTransactions(ctx context.Context, ownerID string, query TransactionQuery) (TransactionListResponse, error)
	GetFinanceSummary(ctx context.Context, ownerID string, req FinanceSummaryQuery) (FinanceSummaryResult, error)
}

type service struct {
	repo Repository
}

func NewService(repo Repository) Service {
	return &service{repo: repo}
}

func (s *service) CreateTransaction(ctx context.Context, ownerID string, req CreateTransactionRequest) (FinanceTransaction, error) {
	if req.VenueID != nil && *req.VenueID != "" {
		if err := s.repo.VerifyVenueOwnership(ctx, *req.VenueID, ownerID); err != nil {
			return FinanceTransaction{}, err
		}
	}

	tx := FinanceTransaction{
		OwnerID:         ownerID,
		VenueID:         req.VenueID,
		CreatedByUserID: &ownerID,
		Type:            req.Type,
		Source:          "MANUAL",
		Category:        req.Category,
		Amount:          req.Amount,
		TransactionDate: req.TransactionDate,
		PaymentMethod:   req.PaymentMethod,
		Description:     req.Description,
	}
	return s.repo.CreateTransaction(ctx, tx)
}

func (s *service) UpdateTransaction(ctx context.Context, id string, ownerID string, req UpdateTransactionRequest) (FinanceTransaction, error) {
	if req.VenueID != nil && *req.VenueID != "" {
		if err := s.repo.VerifyVenueOwnership(ctx, *req.VenueID, ownerID); err != nil {
			return FinanceTransaction{}, err
		}
	}
	return s.repo.UpdateTransaction(ctx, id, ownerID, req)
}

func (s *service) DeleteTransaction(ctx context.Context, id string, ownerID string) error {
	return s.repo.DeleteTransaction(ctx, id, ownerID)
}

func (s *service) GetTransactions(ctx context.Context, ownerID string, query TransactionQuery) (TransactionListResponse, error) {
	txs, total, err := s.repo.GetTransactions(ctx, ownerID, query)
	if err != nil {
		return TransactionListResponse{}, err
	}
	page := query.Page
	if page < 1 {
		page = 1
	}
	limit := query.Limit
	if limit < 1 {
		limit = 10
	}
	return TransactionListResponse{
		Transactions: txs,
		Total:        total,
		Page:         page,
		Limit:        limit,
	}, nil
}

func (s *service) GetFinanceSummary(ctx context.Context, ownerID string, req FinanceSummaryQuery) (FinanceSummaryResult, error) {
	return s.repo.GetFinanceSummary(ctx, ownerID, req)
}
