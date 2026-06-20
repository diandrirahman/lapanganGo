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

type FacilityResponse struct {
	ID   string  `json:"id"`
	Name string  `json:"name"`
	Icon *string `json:"icon,omitempty"`
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
	Latitude       *float64           `json:"latitude,omitempty"`
	Longitude      *float64           `json:"longitude,omitempty"`
	Status         string             `json:"status"`
	Facilities     []FacilityResponse `json:"facilities"`
	CreatedAt      time.Time          `json:"created_at"`
	UpdatedAt      time.Time          `json:"updated_at"`
}
