package bookings

import "time"

type CreateBookingRequest struct {
	CourtID     string `json:"court_id" binding:"required,uuid"`
	BookingDate string `json:"booking_date" binding:"required,datetime=2006-01-02"`
	StartTime   string `json:"start_time" binding:"required,datetime=15:04"`
	EndTime     string `json:"end_time" binding:"required,datetime=15:04"`
}

type SubmitPaymentProofRequest struct {
	PaymentReference string `json:"payment_reference" binding:"required,max=255"`
}

type OwnerCancelRefundRequest struct {
	Reason string `json:"reason" binding:"omitempty,max=500"`
}

type VerifyPaymentRequest struct {
	IsApproved bool `json:"is_approved"`
}

type BookingResponse struct {
	ID               string              `json:"id"`
	CustomerID       string              `json:"customer_id"`
	Venue            BookingVenueSummary `json:"venue,omitempty"`
	Court            BookingCourtSummary `json:"court,omitempty"`
	CourtID          string              `json:"court_id"`
	Date             string              `json:"booking_date"`
	StartTime        string              `json:"start_time"`
	EndTime          string              `json:"end_time"`
	TotalPrice       float64             `json:"total_price"`
	Status           string              `json:"status"`
	PaymentReference *string             `json:"payment_reference,omitempty"`
	ExpiresAt        *time.Time          `json:"expires_at,omitempty"`
	CreatedAt        time.Time           `json:"created_at"`
	UpdatedAt        time.Time           `json:"updated_at"`
}

type OwnerVenueBookingsQuery struct {
	Date   string `form:"date" binding:"omitempty,datetime=2006-01-02"`
	Status string `form:"status" binding:"omitempty,oneof=PENDING_PAYMENT WAITING_VERIFICATION PAID CONFIRMED COMPLETED CANCELLED"`
	Limit  int    `form:"limit" binding:"omitempty,min=1,max=100"`
	Page   int    `form:"page" binding:"omitempty,min=1"`
	Scope  string `form:"scope" binding:"omitempty,oneof=upcoming"`
}

type OwnerVenueBookingsResult struct {
	Bookings []OwnerBookingResponse `json:"bookings"`
	Date     string                 `json:"date"`
	Status   string                 `json:"status,omitempty"`
	Page     int                    `json:"page"`
	Limit    int                    `json:"limit"`
}

type OwnerBookingsQuery struct {
	VenueID   string `form:"venue_id" binding:"omitempty,uuid"`
	Status    string `form:"status" binding:"omitempty,oneof=PENDING_PAYMENT WAITING_VERIFICATION PAID CONFIRMED COMPLETED CANCELLED"`
	Scope     string `form:"scope" binding:"omitempty,oneof=upcoming"`
	StartDate string `form:"start_date" binding:"omitempty,datetime=2006-01-02"`
	EndDate   string `form:"end_date" binding:"omitempty,datetime=2006-01-02"`
	Q         string `form:"q" binding:"omitempty"`
	Sort      string `form:"sort" binding:"omitempty,oneof=newest oldest date_asc date_desc"`
	Limit     int    `form:"limit" binding:"omitempty,min=1,max=100"`
	Page      int    `form:"page" binding:"omitempty,min=1"`
}

type OwnerBookingsResult struct {
	Data       []OwnerBookingResponse `json:"data"`
	Page       int                    `json:"page"`
	Limit      int                    `json:"limit"`
	Total      int                    `json:"total"`
	TotalPages int                    `json:"total_pages"`
}

type OwnerBookingResponse struct {
	ID               string                 `json:"id"`
	Customer         BookingCustomerSummary `json:"customer"`
	Venue            BookingVenueSummary    `json:"venue"`
	Court            BookingCourtSummary    `json:"court"`
	Date             string                 `json:"booking_date"`
	StartTime        string                 `json:"start_time"`
	EndTime          string                 `json:"end_time"`
	TotalPrice       float64                `json:"total_price"`
	Status           string                 `json:"status"`
	PaymentReference *string                `json:"payment_reference,omitempty"`
	ExpiresAt        *time.Time             `json:"expires_at,omitempty"`
	CreatedAt        time.Time              `json:"created_at"`
	UpdatedAt        time.Time              `json:"updated_at"`
}

type BookingCustomerSummary struct {
	ID    string  `json:"id"`
	Name  string  `json:"name"`
	Email string  `json:"email"`
	Phone *string `json:"phone,omitempty"`
}

type BookingVenueSummary struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Address string `json:"address,omitempty"`
	City    string `json:"city,omitempty"`
}

type BookingCourtSummary struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	SportName string `json:"sport_name,omitempty"`
}

type OwnerMetricsQuery struct {
	StartDate string `form:"start_date" binding:"omitempty,datetime=2006-01-02"`
	EndDate   string `form:"end_date" binding:"omitempty,datetime=2006-01-02"`
}

type OwnerMetricsResponse struct {
	TotalVenues           int     `json:"total_venues"`
	UpcomingBookings      int     `json:"upcoming_bookings"`
	PendingVerifications  int     `json:"pending_verifications"`
	RevenueCurrent        float64 `json:"revenue_current"`
	BookingRevenueCurrent float64 `json:"booking_revenue_current"`
	RefundCurrent         float64 `json:"refund_current"`
	NetRevenueCurrent     float64 `json:"net_revenue_current"`
	RevenueAllTime        float64 `json:"revenue_all_time"`
	OccupancyRate         float64 `json:"occupancy_rate"`
}
