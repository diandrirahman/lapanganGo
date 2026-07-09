package analytics

import (
	"context"
	"lapangango-api/internal/httputil"
	"time"
)

type Service interface {
	GetBookingsTrend(ctx context.Context, ownerCtx httputil.OwnerContext, venueID *string, startDate *time.Time, endDate *time.Time) (BookingsTrendResponse, error)
	GetRevenueTrend(ctx context.Context, ownerCtx httputil.OwnerContext, venueID *string, startDate *time.Time, endDate *time.Time) (RevenueResponse, error)
	GetStatusBreakdown(ctx context.Context, ownerCtx httputil.OwnerContext, venueID *string, startDate *time.Time, endDate *time.Time) (StatusResponse, error)
	GetExpensesBreakdown(ctx context.Context, ownerCtx httputil.OwnerContext, venueID *string, startDate *time.Time, endDate *time.Time) (ExpensesResponse, error)
}

type service struct {
	repo Repository
}

func NewService(repo Repository) Service {
	return &service{repo: repo}
}

func (s *service) GetBookingsTrend(ctx context.Context, ownerCtx httputil.OwnerContext, venueID *string, startDate *time.Time, endDate *time.Time) (BookingsTrendResponse, error) {
	if !ownerCtx.IsOwner {
		if len(ownerCtx.AllowedVenueIDs) == 0 {
			return BookingsTrendResponse{Trend: []BookingTrendItem{}}, nil
		}
		if venueID != nil && *venueID != "" {
			if !containsID(ownerCtx.AllowedVenueIDs, *venueID) {
				return BookingsTrendResponse{Trend: []BookingTrendItem{}}, nil
			}
		}
	}

	params := AnalyticsParams{
		OwnerID:         ownerCtx.EffectiveOwnerUserID,
		VenueID:         venueID,
		AllowedVenueIDs: ownerCtx.AllowedVenueIDs,
		StartDate:       startDate,
		EndDate:         endDate,
	}
	trend, err := s.repo.GetBookingsTrend(ctx, params)
	if err != nil {
		return BookingsTrendResponse{}, err
	}
	if trend == nil {
		trend = []BookingTrendItem{}
	}
	return BookingsTrendResponse{Trend: trend}, nil
}

func (s *service) GetRevenueTrend(ctx context.Context, ownerCtx httputil.OwnerContext, venueID *string, startDate *time.Time, endDate *time.Time) (RevenueResponse, error) {
	if !ownerCtx.IsOwner {
		if len(ownerCtx.AllowedVenueIDs) == 0 {
			return RevenueResponse{Trend: []RevenueTrendItem{}, VenueBreakdown: []RevenueVenueItem{}}, nil
		}
		if venueID != nil && *venueID != "" {
			if !containsID(ownerCtx.AllowedVenueIDs, *venueID) {
				return RevenueResponse{Trend: []RevenueTrendItem{}, VenueBreakdown: []RevenueVenueItem{}}, nil
			}
		}
	}

	params := AnalyticsParams{
		OwnerID:         ownerCtx.EffectiveOwnerUserID,
		VenueID:         venueID,
		AllowedVenueIDs: ownerCtx.AllowedVenueIDs,
		StartDate:       startDate,
		EndDate:         endDate,
	}
	trend, err := s.repo.GetRevenueTrend(ctx, params)
	if err != nil {
		return RevenueResponse{}, err
	}
	if trend == nil {
		trend = []RevenueTrendItem{}
	}

	venueBreakdown, err := s.repo.GetRevenueByVenue(ctx, params)
	if err != nil {
		return RevenueResponse{}, err
	}
	if venueBreakdown == nil {
		venueBreakdown = []RevenueVenueItem{}
	}

	return RevenueResponse{
		Trend:          trend,
		VenueBreakdown: venueBreakdown,
	}, nil
}

func (s *service) GetStatusBreakdown(ctx context.Context, ownerCtx httputil.OwnerContext, venueID *string, startDate *time.Time, endDate *time.Time) (StatusResponse, error) {
	if !ownerCtx.IsOwner {
		if len(ownerCtx.AllowedVenueIDs) == 0 {
			return StatusResponse{Breakdown: []StatusBreakdownItem{}}, nil
		}
		if venueID != nil && *venueID != "" {
			if !containsID(ownerCtx.AllowedVenueIDs, *venueID) {
				return StatusResponse{Breakdown: []StatusBreakdownItem{}}, nil
			}
		}
	}

	params := AnalyticsParams{
		OwnerID:         ownerCtx.EffectiveOwnerUserID,
		VenueID:         venueID,
		AllowedVenueIDs: ownerCtx.AllowedVenueIDs,
		StartDate:       startDate,
		EndDate:         endDate,
	}
	breakdown, err := s.repo.GetStatusBreakdown(ctx, params)
	if err != nil {
		return StatusResponse{}, err
	}
	if breakdown == nil {
		breakdown = []StatusBreakdownItem{}
	}
	return StatusResponse{Breakdown: breakdown}, nil
}

func (s *service) GetExpensesBreakdown(ctx context.Context, ownerCtx httputil.OwnerContext, venueID *string, startDate *time.Time, endDate *time.Time) (ExpensesResponse, error) {
	if !ownerCtx.IsOwner {
		if len(ownerCtx.AllowedVenueIDs) == 0 {
			return ExpensesResponse{Breakdown: []ExpenseCategoryItem{}}, nil
		}
		if venueID != nil && *venueID != "" {
			if !containsID(ownerCtx.AllowedVenueIDs, *venueID) {
				return ExpensesResponse{Breakdown: []ExpenseCategoryItem{}}, nil
			}
		}
	}

	params := AnalyticsParams{
		OwnerID:         ownerCtx.EffectiveOwnerUserID,
		VenueID:         venueID,
		AllowedVenueIDs: ownerCtx.AllowedVenueIDs,
		StartDate:       startDate,
		EndDate:         endDate,
	}
	breakdown, err := s.repo.GetExpenseByCategory(ctx, params)
	if err != nil {
		return ExpensesResponse{}, err
	}
	if breakdown == nil {
		breakdown = []ExpenseCategoryItem{}
	}
	return ExpensesResponse{Breakdown: breakdown}, nil
}

func containsID(ids []string, id string) bool {
	for _, val := range ids {
		if val == id {
			return true
		}
	}
	return false
}
