package analytics

import (
	"context"
	"time"
)

type Service interface {
	GetBookingsTrend(ctx context.Context, ownerID string, venueID *string, startDate *time.Time, endDate *time.Time) (BookingsTrendResponse, error)
	GetRevenueTrend(ctx context.Context, ownerID string, venueID *string, startDate *time.Time, endDate *time.Time) (RevenueResponse, error)
	GetStatusBreakdown(ctx context.Context, ownerID string, venueID *string, startDate *time.Time, endDate *time.Time) (StatusResponse, error)
	GetExpensesBreakdown(ctx context.Context, ownerID string, venueID *string, startDate *time.Time, endDate *time.Time) (ExpensesResponse, error)
}

type service struct {
	repo Repository
}

func NewService(repo Repository) Service {
	return &service{repo: repo}
}

func (s *service) GetBookingsTrend(ctx context.Context, ownerID string, venueID *string, startDate *time.Time, endDate *time.Time) (BookingsTrendResponse, error) {
	params := AnalyticsParams{
		OwnerID:   ownerID,
		VenueID:   venueID,
		StartDate: startDate,
		EndDate:   endDate,
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

func (s *service) GetRevenueTrend(ctx context.Context, ownerID string, venueID *string, startDate *time.Time, endDate *time.Time) (RevenueResponse, error) {
	params := AnalyticsParams{
		OwnerID:   ownerID,
		VenueID:   venueID,
		StartDate: startDate,
		EndDate:   endDate,
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

func (s *service) GetStatusBreakdown(ctx context.Context, ownerID string, venueID *string, startDate *time.Time, endDate *time.Time) (StatusResponse, error) {
	params := AnalyticsParams{
		OwnerID:   ownerID,
		VenueID:   venueID,
		StartDate: startDate,
		EndDate:   endDate,
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

func (s *service) GetExpensesBreakdown(ctx context.Context, ownerID string, venueID *string, startDate *time.Time, endDate *time.Time) (ExpensesResponse, error) {
	params := AnalyticsParams{
		OwnerID:   ownerID,
		VenueID:   venueID,
		StartDate: startDate,
		EndDate:   endDate,
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
