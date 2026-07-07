package promos

import (
	"time"
)

type CreatePromoRequest struct {
	VenueID       *string `json:"venue_id" binding:"omitempty,uuid"`
	Code          string  `json:"code" binding:"required,min=3,max=50"`
	Name          string  `json:"name" binding:"required,min=3,max=120"`
	Description   string  `json:"description" binding:"omitempty,max=500"`
	DiscountType  string  `json:"discount_type" binding:"required,oneof=PERCENTAGE FIXED_AMOUNT"`
	DiscountValue float64 `json:"discount_value" binding:"required,gt=0"`
	StartsAt      string  `json:"starts_at" binding:"required"`
	EndsAt        string  `json:"ends_at" binding:"required"`
	Status        string  `json:"status" binding:"omitempty,oneof=ACTIVE INACTIVE"`
}

type PromoResponse struct {
	ID            string    `json:"id"`
	OwnerID       string    `json:"owner_id"`
	VenueID       *string   `json:"venue_id"`
	Code          string    `json:"code"`
	Name          string    `json:"name"`
	Description   *string   `json:"description"`
	DiscountType  string    `json:"discount_type"`
	DiscountValue float64   `json:"discount_value"`
	StartsAt      time.Time `json:"starts_at"`
	EndsAt        time.Time `json:"ends_at"`
	Status              string    `json:"status"`
	CreatedAt           time.Time `json:"created_at"`
	UpdatedAt           time.Time `json:"updated_at"`
	UsageCount          int       `json:"usage_count"`
	TotalDiscountAmount float64   `json:"total_discount_amount"`
	TotalFinalRevenue   float64   `json:"total_final_revenue"`
	CanDelete           bool      `json:"can_delete"`
}

type ValidatePromoRequest struct {
	VenueID     string `json:"venue_id" binding:"required,uuid"`
	CourtID     string `json:"court_id" binding:"required,uuid"`
	BookingDate string `json:"booking_date" binding:"required,datetime=2006-01-02"`
	StartTime   string `json:"start_time" binding:"required,datetime=15:04"`
	EndTime     string `json:"end_time" binding:"required,datetime=15:04"`
	PromoCode   string `json:"promo_code" binding:"required,min=3,max=50"`
}

type ValidatePromoResponse struct {
	PromoID        string  `json:"promo_id"`
	PromoCode      string  `json:"promo_code"`
	PromoName      string  `json:"promo_name"`
	OriginalPrice  float64 `json:"original_price"`
	DiscountAmount float64 `json:"discount_amount"`
	FinalPrice     float64 `json:"final_price"`
}
