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
