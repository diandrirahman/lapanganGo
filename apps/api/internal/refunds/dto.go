package refunds

import (
	"time"
)

type CreateRefundRequestRequest struct {
	Reason string `json:"reason"`
}

type ApproveRefundRequestRequest struct {
	OwnerNote string `json:"owner_note"`
}

type RejectRefundRequestRequest struct {
	OwnerNote string `json:"owner_note"`
}

type RefundRequestResponse struct {
	ID                 string     `json:"id"`
	BookingID          string     `json:"booking_id"`
	CustomerID         string     `json:"customer_id"`
	OwnerID            string     `json:"owner_id"`
	VenueID            *string    `json:"venue_id,omitempty"`
	Reason             string     `json:"reason"`
	Status             string     `json:"status"`
	OwnerNote          *string    `json:"owner_note,omitempty"`
	RequestedAt        time.Time  `json:"requested_at"`
	ReviewedAt         *time.Time `json:"reviewed_at,omitempty"`
	ReviewedByUserID   *string    `json:"reviewed_by_user_id,omitempty"`
	CreatedAt          time.Time  `json:"created_at"`
	UpdatedAt          time.Time  `json:"updated_at"`
}

type OwnerRefundRequestListItem struct {
	ID            string    `json:"id"`
	BookingID     string    `json:"booking_id"`
	CustomerName  string    `json:"customer_name"`
	CustomerEmail string    `json:"customer_email"`
	VenueName     string    `json:"venue_name"`
	CourtName     string    `json:"court_name"`
	BookingDate   string    `json:"booking_date"`
	StartTime     string    `json:"start_time"`
	EndTime       string    `json:"end_time"`
	Amount        float64   `json:"amount"`
	Reason        string    `json:"reason"`
	Status        string    `json:"status"`
	RequestedAt   time.Time `json:"requested_at"`
}

type PaginatedOwnerRefundRequests struct {
	Data       []OwnerRefundRequestListItem `json:"data"`
	Total      int                          `json:"total"`
	Page       int                          `json:"page"`
	Limit      int                          `json:"limit"`
	TotalPages int                          `json:"total_pages"`
}
