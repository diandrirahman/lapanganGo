package bookings

import "time"

type CreateBookingRequest struct {
	CourtID     string `json:"court_id" binding:"required,uuid"`
	BookingDate string `json:"booking_date" binding:"required,datetime=2006-01-02"`
	StartTime   string `json:"start_time" binding:"required,datetime=15:04"`
	EndTime     string `json:"end_time" binding:"required,datetime=15:04"`
}

type BookingResponse struct {
	ID         string    `json:"id"`
	CustomerID string    `json:"customer_id"`
	CourtID    string    `json:"court_id"`
	Date       string    `json:"booking_date"`
	StartTime  string    `json:"start_time"`
	EndTime    string    `json:"end_time"`
	TotalPrice float64   `json:"total_price"`
	Status     string    `json:"status"`
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
}

type OwnerVenueBookingsQuery struct {
	Date   string `form:"date" binding:"omitempty,datetime=2006-01-02"`
	Status string `form:"status" binding:"omitempty,oneof=PENDING_PAYMENT PAID CONFIRMED CANCELLED"`
	Limit  int    `form:"limit" binding:"omitempty,min=1,max=100"`
	Page   int    `form:"page" binding:"omitempty,min=1"`
}

type OwnerVenueBookingsResult struct {
	Bookings []OwnerBookingResponse `json:"bookings"`
	Date     string                 `json:"date"`
	Status   string                 `json:"status,omitempty"`
	Page     int                    `json:"page"`
	Limit    int                    `json:"limit"`
}

type OwnerBookingResponse struct {
	ID         string                 `json:"id"`
	Customer   BookingCustomerSummary `json:"customer"`
	Venue      BookingVenueSummary    `json:"venue"`
	Court      BookingCourtSummary    `json:"court"`
	Date       string                 `json:"booking_date"`
	StartTime  string                 `json:"start_time"`
	EndTime    string                 `json:"end_time"`
	TotalPrice float64                `json:"total_price"`
	Status     string                 `json:"status"`
	CreatedAt  time.Time              `json:"created_at"`
	UpdatedAt  time.Time              `json:"updated_at"`
}

type BookingCustomerSummary struct {
	ID    string  `json:"id"`
	Name  string  `json:"name"`
	Email string  `json:"email"`
	Phone *string `json:"phone,omitempty"`
}

type BookingVenueSummary struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type BookingCourtSummary struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}
