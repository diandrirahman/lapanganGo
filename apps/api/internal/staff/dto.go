package staff

import "time"

type CreateStaffRequest struct {
	Name        string   `json:"name" binding:"required,min=2,max=120"`
	Email       string   `json:"email" binding:"required,email,max=191"`
	Phone       *string  `json:"phone" binding:"omitempty,min=10,max=15"`
	Role        string   `json:"role" binding:"required,oneof=MANAGER CASHIER OPERATIONS"`
	Permissions []string `json:"permissions" binding:"required"`
	VenueIDs    []string `json:"venue_ids" binding:"omitempty"` // empty means no venue-scoped data access
}

type UpdateStaffRequest struct {
	Name        string   `json:"name" binding:"required,min=2,max=120"`
	Phone       *string  `json:"phone" binding:"omitempty,min=10,max=15"`
	Role        string   `json:"role" binding:"required,oneof=MANAGER CASHIER OPERATIONS"`
	Permissions []string `json:"permissions" binding:"required"`
	VenueIDs    []string `json:"venue_ids" binding:"omitempty"`
}

type UpdateStaffStatusRequest struct {
	Status string `json:"status" binding:"required,oneof=ACTIVE INACTIVE"`
}

type UpdateStaffVenuesRequest struct {
	VenueIDs []string `json:"venue_ids" binding:"omitempty"`
}

type StaffResponse struct {
	ID               string     `json:"id"`
	OwnerProfileID   string     `json:"owner_profile_id"`
	UserID           string     `json:"user_id"`
	Name             string     `json:"name"`
	Email            string     `json:"email"`
	Phone            *string    `json:"phone,omitempty"`
	Role             string     `json:"role"`
	Permissions      []string   `json:"permissions"`
	Status           string     `json:"status"`
	InvitationStatus string     `json:"invitation_status"`
	VenueIDs         []string   `json:"venue_ids"`
	CreatedAt        time.Time  `json:"created_at"`
	UpdatedAt        time.Time  `json:"updated_at"`
	InvitedAt        *time.Time `json:"invited_at,omitempty"`
	ActivatedAt      *time.Time `json:"activated_at,omitempty"`
	InviteURL        *string    `json:"invite_url,omitempty"`
	EmailDelivery    *EmailDeliveryStatus `json:"email_delivery,omitempty"`
}

type EmailDeliveryStatus struct {
	Attempted bool   `json:"attempted"`
	Sent      bool   `json:"sent"`
	Message   string `json:"message"`
}

type SetupStaffPasswordRequest struct {
	Token    string `json:"token" binding:"required"`
	Password string `json:"password" binding:"required,min=8"`
}

type RegenerateInviteResponse struct {
	InviteURL     string              `json:"invite_url"`
	ExpiresAt     time.Time           `json:"expires_at"`
	EmailDelivery EmailDeliveryStatus `json:"email_delivery"`
}

type ResetStaffPasswordResponse struct {
	ResetURL      string              `json:"reset_url"`
	ExpiresAt     time.Time           `json:"expires_at"`
	EmailDelivery EmailDeliveryStatus `json:"email_delivery"`
}
