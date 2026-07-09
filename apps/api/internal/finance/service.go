package finance

import (
	"context"
	"fmt"
	"lapangango-api/internal/httputil"
)

type Service interface {
	CreateTransaction(ctx context.Context, ownerCtx httputil.OwnerContext, req CreateTransactionRequest) (FinanceTransaction, error)
	UpdateTransaction(ctx context.Context, id string, ownerCtx httputil.OwnerContext, req UpdateTransactionRequest) (FinanceTransaction, error)
	DeleteTransaction(ctx context.Context, id string, ownerCtx httputil.OwnerContext) error
	GetTransaction(ctx context.Context, id string, ownerID string) (FinanceTransaction, error)
	GetTransactions(ctx context.Context, ownerCtx httputil.OwnerContext, query TransactionQuery) (TransactionListResponse, error)
	GetFinanceSummary(ctx context.Context, ownerCtx httputil.OwnerContext, req FinanceSummaryQuery) (FinanceSummaryResult, error)
}

type service struct {
	repo Repository
}

func NewService(repo Repository) Service {
	return &service{repo: repo}
}

func emptySummary() FinanceSummaryResult {
	return FinanceSummaryResult{
		VenueBreakdown:    []VenueRevenueItem{},
		StatusBreakdown:   []StatusRevenueItem{},
		DailyCashflow:     []DailyCashflowItem{},
		ExpenseByCategory: []ExpenseCategoryItem{},
	}
}

func (s *service) CreateTransaction(ctx context.Context, ownerCtx httputil.OwnerContext, req CreateTransactionRequest) (FinanceTransaction, error) {
	if req.VenueID != nil && *req.VenueID != "" {
		if !ownerCtx.IsOwner && !containsID(ownerCtx.AllowedVenueIDs, *req.VenueID) {
			return FinanceTransaction{}, fmt.Errorf("forbidden: you do not have access to this venue")
		}
		if err := s.repo.VerifyVenueOwnership(ctx, *req.VenueID, ownerCtx.EffectiveOwnerUserID); err != nil {
			return FinanceTransaction{}, err
		}
	} else if !ownerCtx.IsOwner {
		return FinanceTransaction{}, fmt.Errorf("forbidden: staff must specify a venue_id")
	}

	tx := FinanceTransaction{
		OwnerID:         ownerCtx.EffectiveOwnerUserID,
		VenueID:         req.VenueID,
		CreatedByUserID: &ownerCtx.ActorUserID,
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

func (s *service) UpdateTransaction(ctx context.Context, id string, ownerCtx httputil.OwnerContext, req UpdateTransactionRequest) (FinanceTransaction, error) {
	if !ownerCtx.IsOwner {
		existing, err := s.repo.GetTransaction(ctx, id, ownerCtx.EffectiveOwnerUserID)
		if err != nil {
			return FinanceTransaction{}, err
		}
		if existing.VenueID == nil || !containsID(ownerCtx.AllowedVenueIDs, *existing.VenueID) {
			return FinanceTransaction{}, fmt.Errorf("forbidden: you do not have access to this venue")
		}
	}

	if req.VenueID != nil && *req.VenueID != "" {
		if !ownerCtx.IsOwner && !containsID(ownerCtx.AllowedVenueIDs, *req.VenueID) {
			return FinanceTransaction{}, fmt.Errorf("forbidden: you do not have access to this venue")
		}
		if err := s.repo.VerifyVenueOwnership(ctx, *req.VenueID, ownerCtx.EffectiveOwnerUserID); err != nil {
			return FinanceTransaction{}, err
		}
	}
	// For update, the repo also checks if the existing transaction belongs to ownerCtx.EffectiveOwnerUserID
	return s.repo.UpdateTransaction(ctx, id, ownerCtx.EffectiveOwnerUserID, req)
}

func (s *service) DeleteTransaction(ctx context.Context, id string, ownerCtx httputil.OwnerContext) error {
	if !ownerCtx.IsOwner {
		existing, err := s.repo.GetTransaction(ctx, id, ownerCtx.EffectiveOwnerUserID)
		if err != nil {
			return err
		}
		if existing.VenueID == nil || !containsID(ownerCtx.AllowedVenueIDs, *existing.VenueID) {
			return fmt.Errorf("forbidden: you do not have access to this venue")
		}
	}
	return s.repo.DeleteTransaction(ctx, id, ownerCtx.EffectiveOwnerUserID)
}

func (s *service) GetTransactions(ctx context.Context, ownerCtx httputil.OwnerContext, query TransactionQuery) (TransactionListResponse, error) {
	if !ownerCtx.IsOwner {
		if len(ownerCtx.AllowedVenueIDs) == 0 {
			page := query.Page
			if page < 1 {
				page = 1
			}
			limit := query.Limit
			if limit < 1 {
				limit = 10
			}
			return TransactionListResponse{Transactions: []FinanceTransaction{}, Page: page, Limit: limit}, nil
		}
		if query.VenueID != "" {
			if !containsID(ownerCtx.AllowedVenueIDs, query.VenueID) {
				return TransactionListResponse{Transactions: []FinanceTransaction{}}, nil
			}
		} else {
			query.AllowedVenueIDs = ownerCtx.AllowedVenueIDs
		}
	}

	txs, total, err := s.repo.GetTransactions(ctx, ownerCtx.EffectiveOwnerUserID, query)
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

func (s *service) GetTransaction(ctx context.Context, id string, ownerID string) (FinanceTransaction, error) {
	return s.repo.GetTransaction(ctx, id, ownerID)
}

func (s *service) GetFinanceSummary(ctx context.Context, ownerCtx httputil.OwnerContext, req FinanceSummaryQuery) (FinanceSummaryResult, error) {
	if !ownerCtx.IsOwner {
		if len(ownerCtx.AllowedVenueIDs) == 0 {
			return emptySummary(), nil
		}
		if req.VenueID != "" {
			if !containsID(ownerCtx.AllowedVenueIDs, req.VenueID) {
				return emptySummary(), nil
			}
		} else {
			req.AllowedVenueIDs = ownerCtx.AllowedVenueIDs
		}
	}
	return s.repo.GetFinanceSummary(ctx, ownerCtx.EffectiveOwnerUserID, req)
}

func containsID(ids []string, id string) bool {
	for _, val := range ids {
		if val == id {
			return true
		}
	}
	return false
}
