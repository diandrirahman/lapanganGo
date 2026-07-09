package auth

import "time"

type RegisterRequest struct {
	Name     string `json:"name" binding:"required,min=2,max=120"`
	Email    string `json:"email" binding:"required,email,max=191"`
	Phone    string `json:"phone" binding:"omitempty,numeric,min=10,max=15"`
	Password string `json:"password" binding:"required,min=8"`
	Role     string `json:"role" binding:"omitempty,oneof=CUSTOMER"`
}

type LoginRequest struct {
	Email    string `json:"email" binding:"required,email,max=191"`
	Password string `json:"password" binding:"required"`
}

type OwnerProfileResponse struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type StaffMembershipResponse struct {
	ID             string   `json:"id"`
	OwnerProfileID string   `json:"owner_profile_id"`
	OwnerName      string   `json:"owner_name"`
	Role           string   `json:"role"`
	Permissions    []string `json:"permissions"`
}

type UserResponse struct {
	ID               string                    `json:"id"`
	Name             string                    `json:"name"`
	Email            string                    `json:"email"`
	Phone            *string                   `json:"phone,omitempty"`
	Role             string                    `json:"role"`
	Status           string                    `json:"status"`
	CreatedAt        time.Time                 `json:"created_at"`
	OwnerProfile     *OwnerProfileResponse     `json:"owner_profile,omitempty"`
	StaffMemberships []StaffMembershipResponse `json:"staff_memberships,omitempty"`
}

type AuthResponse struct {
	Message string       `json:"message"`
	User    UserResponse `json:"user"`
}

type LoginResponse struct {
	Message string       `json:"message"`
	Token   string       `json:"token"`
	User    UserResponse `json:"user"`
}
