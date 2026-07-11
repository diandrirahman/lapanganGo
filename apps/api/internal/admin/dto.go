package admin

import "time"

type PaginationQuery struct {
	Page  int `form:"page" binding:"omitempty,min=1"`
	Limit int `form:"limit" binding:"omitempty,min=1,max=100"`
}

type UserQuery struct {
	PaginationQuery
	Search string `form:"search"`
	Role   string `form:"role"`
	Status string `form:"status"`
}

type UserResponse struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	Email     string    `json:"email"`
	Phone     *string   `json:"phone"`
	Role      string    `json:"role"`
	Status    string    `json:"status"`
	CreatedAt time.Time `json:"created_at"`
}

type OwnerQuery struct {
	PaginationQuery
	Search string `form:"search"`
	Status string `form:"status"`
}

type OwnerResponse struct {
	ID           string    `json:"id"`
	UserID       string    `json:"user_id"`
	BusinessName string    `json:"business_name"`
	Status       string    `json:"status"`
	CreatedAt    time.Time `json:"created_at"`
}

type VenueQuery struct {
	PaginationQuery
	Search string `form:"search"`
	Status string `form:"status"`
}

type VenueResponse struct {
	ID        string    `json:"id"`
	OwnerID   string    `json:"owner_profile_id"`
	Name      string    `json:"name"`
	City      string    `json:"city"`
	Status    string    `json:"status"`
	CreatedAt time.Time `json:"created_at"`
}

type AuditLogQuery struct {
	PaginationQuery
	Action     string `form:"action"`
	EntityType string `form:"entity_type"`
}

type AuditLogResponse struct {
	ID             string    `json:"id"`
	OwnerProfileID *string   `json:"owner_profile_id,omitempty"`
	ActorUserID    *string   `json:"actor_user_id,omitempty"`
	ActorRole      string    `json:"actor_role"`
	Action         string    `json:"action"`
	EntityType     string    `json:"entity_type"`
	EntityID       *string   `json:"entity_id,omitempty"`
	Metadata       any       `json:"metadata"`
	IPAddress      *string   `json:"ip_address,omitempty"`
	UserAgent      *string   `json:"user_agent,omitempty"`
	CreatedAt      time.Time `json:"created_at"`
}

type UpdateStatusRequest struct {
	Status string `json:"status" binding:"required,oneof=ACTIVE SUSPENDED"`
}

type PaginatedResponse struct {
	Data       any `json:"data"`
	TotalItems int `json:"total_items"`
	TotalPages int `json:"total_pages"`
	Page       int `json:"page"`
	Limit      int `json:"limit"`
}

type DashboardStatsResponse struct {
	TotalUsers    int `json:"total_users"`
	TotalOwners   int `json:"total_owners"`
	TotalVenues   int `json:"total_venues"`
	TotalBookings int `json:"total_bookings"`
}
