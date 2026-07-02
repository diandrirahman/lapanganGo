package refunds

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"
)

var (
	ErrRefundRequestNotFound        = errors.New("refund request not found")
	ErrRefundRequestAlreadyExists   = errors.New("active pending refund request already exists")
	ErrRefundRequestNotAllowed      = errors.New("refund request not allowed for this booking")
	ErrBookingRefundCutoffExceeded  = errors.New("refund hanya dapat diajukan paling lambat 1 jam sebelum jadwal mulai")
	ErrRefundRequestAlreadyReviewed = errors.New("refund request already reviewed")
	ErrBookingIncomeLedgerNotFound  = errors.New("booking income ledger not found")
	ErrBookingRefundAlreadyExists   = errors.New("booking refund ledger already exists")
	ErrForbidden                    = errors.New("forbidden")
	ErrInvalidReason                = errors.New("reason must be between 10 and 1000 characters")
)

var timeNow = time.Now

type Service interface {
	RequestBookingRefund(ctx context.Context, customerID, bookingID, reason string) (RefundRequestResponse, error)
	GetRefundRequestByBooking(ctx context.Context, customerID, bookingID string) (*RefundRequestResponse, error)
	ListOwnerRefundRequests(ctx context.Context, ownerID, status, venueID string, page, limit int) (PaginatedOwnerRefundRequests, error)
	ApproveRefundRequest(ctx context.Context, ownerID, requestID, ownerNote string) (RefundRequestResponse, error)
	RejectRefundRequest(ctx context.Context, ownerID, requestID, ownerNote string) (RefundRequestResponse, error)
}

type service struct {
	repo Repository
}

func NewService(repo Repository) Service {
	return &service{repo: repo}
}

func canRequestRefund(now time.Time, bookingDate time.Time, startTime time.Time, loc *time.Location) bool {
	bookingStartAt := time.Date(
		bookingDate.Year(), bookingDate.Month(), bookingDate.Day(),
		startTime.Hour(), startTime.Minute(), 0, 0,
		loc,
	)
	return now.In(loc).Before(bookingStartAt.Add(-1 * time.Hour))
}

func (s *service) RequestBookingRefund(ctx context.Context, customerID, bookingID, reason string) (RefundRequestResponse, error) {
	reason = strings.TrimSpace(reason)
	if len(reason) < 10 || len(reason) > 1000 {
		return RefundRequestResponse{}, ErrInvalidReason
	}

	b, err := s.repo.FindBookingForRefundRequest(ctx, bookingID)
	if err != nil {
		return RefundRequestResponse{}, fmt.Errorf("booking not found: %w", err)
	}

	if b.CustomerID != customerID {
		return RefundRequestResponse{}, ErrForbidden
	}

	if b.Status != "PAID" {
		return RefundRequestResponse{}, ErrRefundRequestNotAllowed
	}

	loc, err := time.LoadLocation("Asia/Jakarta")
	if err != nil {
		loc = time.FixedZone("WIB", 7*3600)
	}

	if !canRequestRefund(timeNow(), b.Date, b.StartTime, loc) {
		return RefundRequestResponse{}, ErrBookingRefundCutoffExceeded
	}

	pendingReq, err := s.repo.GetActiveRefundRequestByBookingID(ctx, bookingID)
	if err != nil {
		return RefundRequestResponse{}, err
	}
	if pendingReq != nil {
		return RefundRequestResponse{}, ErrRefundRequestAlreadyExists
	}

	req := RefundRequestResponse{
		BookingID:  bookingID,
		CustomerID: customerID,
		OwnerID:    b.OwnerID,
		VenueID:    b.VenueID,
		Reason:     reason,
		Status:     "PENDING",
	}

	return s.repo.CreateRefundRequest(ctx, req)
}

func (s *service) GetRefundRequestByBooking(ctx context.Context, customerID, bookingID string) (*RefundRequestResponse, error) {
	b, err := s.repo.FindBookingForRefundRequest(ctx, bookingID)
	if err != nil {
		return nil, fmt.Errorf("booking not found: %w", err)
	}

	if b.CustomerID != customerID {
		return nil, ErrForbidden
	}

	return s.repo.GetLatestRefundRequestByBookingID(ctx, bookingID)
}

func (s *service) ListOwnerRefundRequests(ctx context.Context, ownerID, status, venueID string, page, limit int) (PaginatedOwnerRefundRequests, error) {
	if page < 1 {
		page = 1
	}
	if limit < 1 || limit > 100 {
		limit = 10
	}

	items, total, err := s.repo.ListOwnerRefundRequests(ctx, ownerID, status, venueID, page, limit)
	if err != nil {
		return PaginatedOwnerRefundRequests{}, err
	}

	totalPages := total / limit
	if total%limit != 0 {
		totalPages++
	}

	if items == nil {
		items = make([]OwnerRefundRequestListItem, 0)
	}

	return PaginatedOwnerRefundRequests{
		Data:       items,
		Total:      total,
		Page:       page,
		Limit:      limit,
		TotalPages: totalPages,
	}, nil
}

func (s *service) ApproveRefundRequest(ctx context.Context, ownerID, requestID, ownerNote string) (RefundRequestResponse, error) {
	tx, err := s.repo.BeginTx(ctx)
	if err != nil {
		return RefundRequestResponse{}, err
	}
	defer tx.Rollback(ctx)

	req, err := s.repo.LockRefundRequest(ctx, tx, requestID)
	if err != nil {
		return RefundRequestResponse{}, fmt.Errorf("%w: %v", ErrRefundRequestNotFound, err)
	}

	if req.OwnerID != ownerID {
		return RefundRequestResponse{}, ErrForbidden
	}

	if req.Status != "PENDING" {
		return RefundRequestResponse{}, ErrRefundRequestAlreadyReviewed
	}

	b, err := s.repo.LockBooking(ctx, tx, req.BookingID)
	if err != nil {
		return RefundRequestResponse{}, fmt.Errorf("booking not found: %w", err)
	}

	if b.Status != "PAID" {
		return RefundRequestResponse{}, ErrRefundRequestNotAllowed
	}

	hasIncome, err := s.repo.HasBookingIncomeLedger(ctx, tx, req.BookingID)
	if err != nil {
		return RefundRequestResponse{}, err
	}
	if !hasIncome {
		return RefundRequestResponse{}, ErrBookingIncomeLedgerNotFound
	}

	hasRefund, err := s.repo.HasRefundLedger(ctx, tx, req.BookingID)
	if err != nil {
		return RefundRequestResponse{}, err
	}
	if hasRefund {
		return RefundRequestResponse{}, ErrBookingRefundAlreadyExists
	}

	if err := s.repo.UpdateBookingStatus(ctx, tx, req.BookingID, "CANCELLED"); err != nil {
		return RefundRequestResponse{}, err
	}

	venueID := ""
	if req.VenueID != nil {
		venueID = *req.VenueID
	}
	desc := fmt.Sprintf("Refund approved for booking %s: %s", req.BookingID, req.Reason)
	if ownerNote != "" {
		desc = fmt.Sprintf("Refund approved for booking %s: %s", req.BookingID, ownerNote)
	}

	if err := s.repo.InsertRefundLedger(ctx, tx, ownerID, venueID, req.BookingID, ownerID, b.TotalPrice, desc); err != nil {
		return RefundRequestResponse{}, err
	}

	if err := s.repo.UpdateRefundRequest(ctx, tx, requestID, "APPROVED", ownerNote, ownerID); err != nil {
		return RefundRequestResponse{}, err
	}

	if err := tx.Commit(ctx); err != nil {
		return RefundRequestResponse{}, err
	}

	req.Status = "APPROVED"
	if ownerNote != "" {
		req.OwnerNote = &ownerNote
	}
	now := timeNow()
	req.ReviewedAt = &now
	req.ReviewedByUserID = &ownerID

	return req, nil
}

func (s *service) RejectRefundRequest(ctx context.Context, ownerID, requestID, ownerNote string) (RefundRequestResponse, error) {
	tx, err := s.repo.BeginTx(ctx)
	if err != nil {
		return RefundRequestResponse{}, err
	}
	defer tx.Rollback(ctx)

	req, err := s.repo.LockRefundRequest(ctx, tx, requestID)
	if err != nil {
		return RefundRequestResponse{}, fmt.Errorf("%w: %v", ErrRefundRequestNotFound, err)
	}

	if req.OwnerID != ownerID {
		return RefundRequestResponse{}, ErrForbidden
	}

	if req.Status != "PENDING" {
		return RefundRequestResponse{}, ErrRefundRequestAlreadyReviewed
	}

	if err := s.repo.UpdateRefundRequest(ctx, tx, requestID, "REJECTED", ownerNote, ownerID); err != nil {
		return RefundRequestResponse{}, err
	}

	if err := tx.Commit(ctx); err != nil {
		return RefundRequestResponse{}, err
	}

	req.Status = "REJECTED"
	if ownerNote != "" {
		req.OwnerNote = &ownerNote
	}
	now := timeNow()
	req.ReviewedAt = &now
	req.ReviewedByUserID = &ownerID

	return req, nil
}
