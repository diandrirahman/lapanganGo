package owners

import "time"

type CreateProfileRequest struct {
	BusinessName      string `json:"business_name" binding:"required,min=2,max=150"`
	IdentityNumber    string `json:"identity_number" binding:"omitempty,max=80"`
	BankName          string `json:"bank_name" binding:"omitempty,max=100"`
	BankAccountNumber string `json:"bank_account_number" binding:"omitempty,max=100"`
	BankAccountName   string `json:"bank_account_name" binding:"omitempty,max=150"`
}

type UpdateProfileRequest struct {
	BusinessName      string `json:"business_name" binding:"required,min=2,max=150"`
	IdentityNumber    string `json:"identity_number" binding:"omitempty,max=80"`
	BankName          string `json:"bank_name" binding:"omitempty,max=100"`
	BankAccountNumber string `json:"bank_account_number" binding:"omitempty,max=100"`
	BankAccountName   string `json:"bank_account_name" binding:"omitempty,max=150"`
}

type ProfileResponse struct {
	ID                 string     `json:"id"`
	UserID             string     `json:"user_id"`
	BusinessName       string     `json:"business_name"`
	IdentityNumber     *string    `json:"identity_number,omitempty"`
	BankName           *string    `json:"bank_name,omitempty"`
	BankAccountNumber  *string    `json:"bank_account_number,omitempty"`
	BankAccountName    *string    `json:"bank_account_name,omitempty"`
	VerificationStatus string     `json:"verification_status"`
	VerifiedAt         *time.Time `json:"verified_at,omitempty"`
	CreatedAt          time.Time  `json:"created_at"`
	UpdatedAt          time.Time  `json:"updated_at"`
}

type OwnerMetricsResponse struct {
	TotalVenues    int     `json:"total_venues"`
	ActiveBookings int     `json:"active_bookings"`
	TotalRevenue   float64 `json:"total_revenue"`
}
