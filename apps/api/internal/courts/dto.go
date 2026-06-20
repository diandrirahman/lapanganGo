package courts

import "time"

type CreateCourtRequest struct {
	SportID      string   `json:"sport_id" binding:"required,uuid"`
	Name         string   `json:"name" binding:"required,min=2,max=120"`
	Description  string   `json:"description" binding:"omitempty"`
	LocationType string   `json:"location_type" binding:"required,oneof=INDOOR OUTDOOR"`
	SurfaceType  string   `json:"surface_type" binding:"omitempty,max=80"`
	PricePerHour *float64 `json:"price_per_hour" binding:"required,gte=0"`
}

type UpdateCourtRequest struct {
	SportID      string   `json:"sport_id" binding:"required,uuid"`
	Name         string   `json:"name" binding:"required,min=2,max=120"`
	Description  string   `json:"description" binding:"omitempty"`
	LocationType string   `json:"location_type" binding:"required,oneof=INDOOR OUTDOOR"`
	SurfaceType  string   `json:"surface_type" binding:"omitempty,max=80"`
	PricePerHour *float64 `json:"price_per_hour" binding:"required,gte=0"`
}

type UpdateCourtStatusRequest struct {
	Status string `json:"status" binding:"required,oneof=ACTIVE INACTIVE MAINTENANCE"`
}

type SportResponse struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type CourtResponse struct {
	ID           string        `json:"id"`
	VenueID      string        `json:"venue_id"`
	Sport        SportResponse `json:"sport"`
	Name         string        `json:"name"`
	Description  *string       `json:"description,omitempty"`
	LocationType string        `json:"location_type"`
	SurfaceType  *string       `json:"surface_type,omitempty"`
	PricePerHour float64       `json:"price_per_hour"`
	Status       string        `json:"status"`
	CreatedAt    time.Time     `json:"created_at"`
	UpdatedAt    time.Time     `json:"updated_at"`
}
