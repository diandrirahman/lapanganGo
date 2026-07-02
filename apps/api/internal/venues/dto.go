package venues

import "time"

type CreateVenueRequest struct {
	Name        string   `json:"name" binding:"required,min=2,max=150"`
	Description string   `json:"description" binding:"omitempty"`
	Address     string   `json:"address" binding:"required"`
	District    string   `json:"district" binding:"omitempty,max=100"`
	City        string   `json:"city" binding:"required,max=100"`
	Province    string   `json:"province" binding:"omitempty,max=100"`
	PostalCode  string   `json:"postal_code" binding:"omitempty,max=20"`
	Latitude    *float64 `json:"latitude" binding:"omitempty,gte=-90,lte=90"`
	Longitude   *float64 `json:"longitude" binding:"omitempty,gte=-180,lte=180"`
	FacilityIDs []string `json:"facility_ids" binding:"omitempty,dive,uuid"`
}

type UpdateVenueRequest struct {
	Name        string   `json:"name" binding:"required,min=2,max=150"`
	Description string   `json:"description" binding:"omitempty"`
	Address     string   `json:"address" binding:"required"`
	District    string   `json:"district" binding:"omitempty,max=100"`
	City        string   `json:"city" binding:"required,max=100"`
	Province    string   `json:"province" binding:"omitempty,max=100"`
	PostalCode  string   `json:"postal_code" binding:"omitempty,max=20"`
	Latitude    *float64 `json:"latitude" binding:"omitempty,gte=-90,lte=90"`
	Longitude   *float64 `json:"longitude" binding:"omitempty,gte=-180,lte=180"`
	FacilityIDs []string `json:"facility_ids" binding:"omitempty,dive,uuid"`
}

type UpdateVenueStatusRequest struct {
	Status string `json:"status" binding:"required,oneof=DRAFT ACTIVE INACTIVE"`
}

type ListPublicVenuesQuery struct {
	Limit       int      `form:"limit" binding:"omitempty,min=1,max=50"`
	Page        int      `form:"page" binding:"omitempty,min=1"`
	Q           string   `form:"q" binding:"omitempty,max=100"`
	City        string   `form:"city" binding:"omitempty,max=100"`
	SportID     string   `form:"sport_id" binding:"omitempty,uuid"`
	FacilityIDs []string `form:"facility_ids" binding:"omitempty,dive,uuid"`
	MinPrice    float64  `form:"min_price" binding:"omitempty,min=0"`
	MaxPrice    float64  `form:"max_price" binding:"omitempty,min=0"`
}

type FacilityResponse struct {
	ID   string  `json:"id"`
	Name string  `json:"name"`
	Icon *string `json:"icon,omitempty"`
}

type PublicVenueResponse struct {
	ID          string             `json:"id"`
	Name        string             `json:"name"`
	Description *string            `json:"description,omitempty"`
	Address     string             `json:"address"`
	District    *string            `json:"district,omitempty"`
	City        string             `json:"city"`
	Province    *string            `json:"province,omitempty"`
	PostalCode  *string            `json:"postal_code,omitempty"`
	Latitude     *float64           `json:"latitude,omitempty"`
	Longitude    *float64           `json:"longitude,omitempty"`
	PrimaryPhoto *string            `json:"primary_photo,omitempty"`
	Facilities   []FacilityResponse `json:"facilities"`
	CreatedAt    time.Time          `json:"created_at"`
	UpdatedAt    time.Time          `json:"updated_at"`
}

type PublicSportResponse struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type SportResponse struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type PublicCourtResponse struct {
	ID           string              `json:"id"`
	Sport        PublicSportResponse `json:"sport"`
	Name         string              `json:"name"`
	Description  *string             `json:"description,omitempty"`
	LocationType string              `json:"location_type"`
	SurfaceType  *string             `json:"surface_type,omitempty"`
	PricePerHour float64             `json:"price_per_hour"`
	CreatedAt    time.Time           `json:"created_at"`
	UpdatedAt    time.Time           `json:"updated_at"`
}

type PublicVenueDetailResponse struct {
	PublicVenueResponse
	Photos []VenuePhotoResponse  `json:"photos"`
	Courts []PublicCourtResponse `json:"courts"`
}

type VenueResponse struct {
	ID             string             `json:"id"`
	OwnerProfileID string             `json:"owner_profile_id"`
	Name           string             `json:"name"`
	Description    *string            `json:"description,omitempty"`
	Address        string             `json:"address"`
	District       *string            `json:"district,omitempty"`
	City           string             `json:"city"`
	Province       *string            `json:"province,omitempty"`
	PostalCode     *string            `json:"postal_code,omitempty"`
	Latitude       *float64             `json:"latitude,omitempty"`
	Longitude      *float64             `json:"longitude,omitempty"`
	Status         string               `json:"status"`
	PrimaryPhoto   *string              `json:"primary_photo,omitempty"`
	Photos         []VenuePhotoResponse `json:"photos"`
	Facilities     []FacilityResponse   `json:"facilities"`
	CreatedAt      time.Time            `json:"created_at"`
	UpdatedAt      time.Time            `json:"updated_at"`
}

type VenuePhotoResponse struct {
	ID        string    `json:"id"`
	VenueID   string    `json:"venue_id"`
	ImageURL  string    `json:"image_url"`
	AltText   *string   `json:"alt_text,omitempty"`
	SortOrder int       `json:"sort_order"`
	IsPrimary bool      `json:"is_primary"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

type CreateVenuePhotoRequest struct {
	ImageURL  string  `json:"image_url" binding:"required,url,max=2000"`
	AltText   *string `json:"alt_text" binding:"omitempty,max=255"`
	SortOrder *int    `json:"sort_order" binding:"omitempty,min=0"`
	IsPrimary *bool   `json:"is_primary" binding:"omitempty"`
}

type UpdateVenuePhotoRequest struct {
	AltText   *string `json:"alt_text" binding:"omitempty,max=255"`
	SortOrder *int    `json:"sort_order" binding:"omitempty,min=0"`
	IsPrimary *bool   `json:"is_primary" binding:"omitempty"`
}
