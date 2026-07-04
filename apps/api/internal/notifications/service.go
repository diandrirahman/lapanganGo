package notifications

import (
	"context"
	"math"
)

// Constants for notification types
const (
	TypeBookingCreated        = "BOOKING_CREATED"
	TypePaymentExpiringSoon   = "PAYMENT_EXPIRING_SOON"
	TypePaymentProofSubmitted = "PAYMENT_PROOF_SUBMITTED"
	TypePaymentApproved       = "PAYMENT_APPROVED"
	TypePaymentRejected       = "PAYMENT_REJECTED"
	TypeRefundRequested       = "REFUND_REQUESTED"
	TypeRefundApproved        = "REFUND_APPROVED"
	TypeRefundRejected        = "REFUND_REJECTED"
	TypeBookingCompleted      = "BOOKING_COMPLETED"
)

// Constants for entity types
const (
	EntityBooking = "BOOKING"
	EntityRefund  = "REFUND"
)

type Service interface {
	Create(ctx context.Context, params CreateNotificationParams) error
	ListByUser(ctx context.Context, userID string, page, limit int) (NotificationListResponse, error)
	UnreadCount(ctx context.Context, userID string) (UnreadCountResponse, error)
	MarkRead(ctx context.Context, userID string, notificationID string) error
	MarkAllRead(ctx context.Context, userID string) error
}

type service struct {
	repo Repository
}

func NewService(repo Repository) Service {
	return &service{repo: repo}
}

func (s *service) Create(ctx context.Context, params CreateNotificationParams) error {
	// Notifications are best-effort, do not panic if they fail, but we return error so caller can log it.
	return s.repo.Create(ctx, params)
}

func (s *service) ListByUser(ctx context.Context, userID string, page, limit int) (NotificationListResponse, error) {
	if page < 1 {
		page = 1
	}
	if limit < 1 || limit > 100 {
		limit = 10
	}
	offset := (page - 1) * limit

	notifications, total, err := s.repo.ListByUser(ctx, userID, limit, offset)
	if err != nil {
		return NotificationListResponse{}, err
	}

	data := make([]NotificationResponse, 0, len(notifications))
	for _, n := range notifications {
		data = append(data, NotificationResponse{
			ID:         n.ID,
			Type:       n.Type,
			Title:      n.Title,
			Message:    n.Message,
			EntityType: n.EntityType,
			EntityID:   n.EntityID,
			ReadAt:     n.ReadAt,
			CreatedAt:  n.CreatedAt,
		})
	}

	totalPages := int(math.Ceil(float64(total) / float64(limit)))

	return NotificationListResponse{
		Data:       data,
		Page:       page,
		Limit:      limit,
		Total:      total,
		TotalPages: totalPages,
	}, nil
}

func (s *service) UnreadCount(ctx context.Context, userID string) (UnreadCountResponse, error) {
	count, err := s.repo.UnreadCount(ctx, userID)
	if err != nil {
		return UnreadCountResponse{}, err
	}
	return UnreadCountResponse{Count: count}, nil
}

func (s *service) MarkRead(ctx context.Context, userID string, notificationID string) error {
	return s.repo.MarkRead(ctx, userID, notificationID)
}

func (s *service) MarkAllRead(ctx context.Context, userID string) error {
	return s.repo.MarkAllRead(ctx, userID)
}
