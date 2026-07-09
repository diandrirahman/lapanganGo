package audit

import "time"

const (
	ActionStaffCreated                = "STAFF_CREATED"
	ActionStaffUpdated                = "STAFF_UPDATED"
	ActionStaffStatusUpdated          = "STAFF_STATUS_UPDATED"
	ActionStaffVenuesUpdated          = "STAFF_VENUES_UPDATED"
	ActionStaffInviteCreated          = "STAFF_INVITE_CREATED"
	ActionStaffInviteRegenerated      = "STAFF_INVITE_REGENERATED"
	ActionStaffPasswordResetRequested = "STAFF_PASSWORD_RESET_REQUESTED"
	ActionStaffPasswordResetCompleted = "STAFF_PASSWORD_RESET_COMPLETED"
	ActionStaffPasswordSetupCompleted = "STAFF_PASSWORD_SETUP_COMPLETED"

	ActionBookingPaymentVerified = "BOOKING_PAYMENT_VERIFIED"
	ActionBookingPaymentRejected = "BOOKING_PAYMENT_REJECTED"
	ActionBookingMarkedPaid      = "BOOKING_MARKED_PAID"
	ActionBookingCompleted       = "BOOKING_COMPLETED"
	ActionBookingCancelRefund    = "BOOKING_CANCEL_REFUND"

	ActionRefundApproved = "REFUND_APPROVED"
	ActionRefundRejected = "REFUND_REJECTED"

	ActionFinanceCreated = "FINANCE_CREATED"
	ActionFinanceUpdated = "FINANCE_UPDATED"
	ActionFinanceDeleted = "FINANCE_DELETED"
)

const (
	EntityStaff              = "STAFF"
	EntityBooking            = "BOOKING"
	EntityRefund             = "REFUND"
	EntityFinanceTransaction = "FINANCE_TRANSACTION"
	EntityVenue              = "VENUE"
)

type CreateAuditLogParams struct {
	OwnerProfileID string
	ActorUserID    string
	ActorRole      string
	Action         string
	EntityType     string
	EntityID       *string
	Metadata       map[string]any
	IPAddress      *string
	UserAgent      *string
}

type AuditActorResponse struct {
	ID    *string `json:"id,omitempty"`
	Name  *string `json:"name,omitempty"`
	Email *string `json:"email,omitempty"`
	Role  string  `json:"role"`
}

type AuditLogResponse struct {
	ID         string             `json:"id"`
	Actor      AuditActorResponse `json:"actor"`
	Action     string             `json:"action"`
	EntityType string             `json:"entity_type"`
	EntityID   *string            `json:"entity_id,omitempty"`
	Metadata   map[string]any     `json:"metadata"`
	IPAddress  *string            `json:"ip_address,omitempty"`
	UserAgent  *string            `json:"user_agent,omitempty"`
	CreatedAt  time.Time          `json:"created_at"`
}

type AuditLogQuery struct {
	Action      string `form:"action"`
	EntityType  string `form:"entity_type"`
	ActorUserID string `form:"actor_user_id" binding:"omitempty,uuid"`
	StartDate   string `form:"start_date" binding:"omitempty,datetime=2006-01-02"`
	EndDate     string `form:"end_date" binding:"omitempty,datetime=2006-01-02"`
	Page        int    `form:"page" binding:"omitempty,min=1"`
	Limit       int    `form:"limit" binding:"omitempty,min=1,max=100"`
}
