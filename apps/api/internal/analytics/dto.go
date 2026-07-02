package analytics

import (
	"time"
)

type BookingTrendItem struct {
	Date         string `json:"date"`
	BookingCount int    `json:"booking_count"`
}

type BookingsTrendResponse struct {
	Trend []BookingTrendItem `json:"trend"`
}

type RevenueTrendItem struct {
	Date    string  `json:"date"`
	Revenue float64 `json:"revenue"`
}

type RevenueVenueItem struct {
	VenueID   string  `json:"venue_id"`
	VenueName string  `json:"venue_name"`
	Revenue   float64 `json:"revenue"`
}

type RevenueResponse struct {
	Trend         []RevenueTrendItem `json:"trend"`
	VenueBreakdown []RevenueVenueItem `json:"venue_breakdown"`
}

type StatusBreakdownItem struct {
	Status       string  `json:"status"`
	BookingCount int     `json:"booking_count"`
	Amount       float64 `json:"amount"`
}

type StatusResponse struct {
	Breakdown []StatusBreakdownItem `json:"breakdown"`
}

type ExpenseCategoryItem struct {
	Category string  `json:"category"`
	Amount   float64 `json:"amount"`
}

type ExpensesResponse struct {
	Breakdown []ExpenseCategoryItem `json:"breakdown"`
}

type AnalyticsParams struct {
	OwnerID   string
	VenueID   *string
	StartDate *time.Time
	EndDate   *time.Time
}
