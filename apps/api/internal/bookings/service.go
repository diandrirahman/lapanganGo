package bookings

import (
	"context"
	"errors"
	"log"
	"math"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

var (
	ErrPastDate                 = errors.New("booking date cannot be in the past")
	ErrInvalidTimeRange         = errors.New("start time must be before end time")
	ErrCourtInactive            = errors.New("court is not active")
	ErrVenueInactive            = errors.New("venue is not active")
	ErrOutsideOpHours           = errors.New("booking time is outside court operating hours")
	ErrOverlapBlockedSlot       = errors.New("court is blocked/maintenance during the requested time")
	ErrOverlapBooking           = errors.New("court is already booked for the requested time")
	ErrBookingNotFound          = errors.New("booking not found")
	ErrBookingAlreadyCancelled  = errors.New("booking already cancelled")
	ErrBookingCannotBeCancelled = errors.New("booking cannot be cancelled in current status")
	ErrBookingAlreadyConfirmed  = errors.New("booking already confirmed")
	ErrBookingCannotBeConfirmed = errors.New("booking cannot be confirmed in current status")
	ErrOwnerProfileNotFound     = errors.New("owner profile not found")
	ErrVenueNotFound            = errors.New("venue not found")
	ErrForbidden                = errors.New("forbidden: you do not own this booking's venue")
	ErrBookingCannotBeMarkedPaid = errors.New("Booking tidak dapat ditandai lunas pada status ini")
	ErrBookingCannotBeCompleted  = errors.New("Gagal menyelesaikan booking. Pastikan jadwal main telah terlewati dan status sudah Lunas.")
	ErrBookingCannotBeRefunded   = errors.New("booking cannot be cancelled/refunded in current status")
	ErrBookingRefundAlreadyExists = errors.New("refund already recorded for this booking")
	ErrBookingIncomeLedgerNotFound = errors.New("booking income ledger not found; backfill ledger before refund")
)

type BookingRepository interface {
	LockCourtValidationInfo(ctx context.Context, tx pgx.Tx, courtID string) (CourtValidationInfo, error)
	FindOperatingHours(ctx context.Context, tx pgx.Tx, courtID string, dayOfWeek int) (OperatingHour, error)
	ListByCustomerID(ctx context.Context, customerID string, limit, offset int) ([]CustomerBooking, int, error)
	FindByIDAndCustomerID(ctx context.Context, id, customerID string) (Booking, error)
	FindCustomerBookingByID(ctx context.Context, id, customerID string) (CustomerBooking, error)
	FindOwnerProfileByUserID(ctx context.Context, userID string) (OwnerProfile, error)
	FindVenueByIDAndOwnerProfileID(ctx context.Context, venueID, ownerProfileID string) (OwnerVenue, error)
	ListOwnerVenueBookings(ctx context.Context, ownerProfileID, venueID, date, status, scope string, limit, offset int) ([]OwnerBooking, int, error)
	ListOwnerBookings(ctx context.Context, ownerProfileID string, query OwnerBookingsQuery, limit, offset int) ([]OwnerBooking, int, error)
	ExecuteBookingTx(ctx context.Context, fn func(tx pgx.Tx) error) error
	CheckBlockedSlots(ctx context.Context, tx pgx.Tx, courtID string, startTz, endTz time.Time) (bool, error)
	CheckExistingBookings(ctx context.Context, tx pgx.Tx, courtID, date, startTime, endTime string) (bool, error)
	InsertBooking(ctx context.Context, tx pgx.Tx, params CreateBookingParams) (Booking, error)
	CancelPendingByIDAndCustomerID(ctx context.Context, bookingID, customerID string) (Booking, error)
	ConfirmPendingByIDAndCustomerID(ctx context.Context, bookingID, customerID string) (Booking, error)
	GetOwnerMetrics(ctx context.Context, ownerProfileID string, startDate string, endDate string) (OwnerMetrics, error)
	UpdatePaymentReference(ctx context.Context, bookingID, customerID, reference string) (Booking, error)
	VerifyPayment(ctx context.Context, ownerUserID string, bookingID string, isApproved bool) (Booking, error)
	MarkBookingPaid(ctx context.Context, ownerUserID string, bookingID string) (Booking, error)
	CompleteBooking(ctx context.Context, bookingID string) (Booking, error)
	GetBookingOwnerProfileID(ctx context.Context, bookingID string) (string, error)
	CancelExpiredPendingBookings(ctx context.Context) (int64, error)
	CancelPaidBookingWithRefund(ctx context.Context, ownerUserID string, bookingID string, reason string) (Booking, error)
}

type Service struct {
	repository BookingRepository
	ttlMinutes int
}

func NewService(repository BookingRepository, ttlMinutes int) *Service {
	return &Service{repository: repository, ttlMinutes: ttlMinutes}
}

func (s *Service) CreateBooking(ctx context.Context, customerID string, req CreateBookingRequest) (BookingResponse, error) {
	// 1. Parse and validate dates/times
	loc, err := time.LoadLocation("Asia/Jakarta")
	if err != nil {
		loc = time.FixedZone("WIB", 7*3600)
	}

	reqDate, err := time.Parse("2006-01-02", req.BookingDate)
	if err != nil {
		return BookingResponse{}, err
	}

	nowTz := time.Now().In(loc)
	today := time.Date(nowTz.Year(), nowTz.Month(), nowTz.Day(), 0, 0, 0, 0, time.UTC)
	if reqDate.Before(today) {
		return BookingResponse{}, ErrPastDate
	}

	startParsed, err := time.Parse("15:04", req.StartTime)
	if err != nil {
		return BookingResponse{}, err
	}
	endParsed, err := time.Parse("15:04", req.EndTime)
	if err != nil {
		return BookingResponse{}, err
	}

	if !startParsed.Before(endParsed) {
		return BookingResponse{}, ErrInvalidTimeRange
	}

	dayOfWeek := int(reqDate.Weekday())

	// Build TZ-aware times for blocked slots check
	startTz := time.Date(reqDate.Year(), reqDate.Month(), reqDate.Day(), startParsed.Hour(), startParsed.Minute(), 0, 0, loc)
	endTz := time.Date(reqDate.Year(), reqDate.Month(), reqDate.Day(), endParsed.Hour(), endParsed.Minute(), 0, 0, loc)

	var created Booking
	err = s.repository.ExecuteBookingTx(ctx, func(tx pgx.Tx) error {
		// 2. Fetch court & venue info with lock
		info, err := s.repository.LockCourtValidationInfo(ctx, tx, req.CourtID)
		if err != nil {
			return err
		}
		if info.CourtStatus != "ACTIVE" {
			return ErrCourtInactive
		}
		if info.VenueStatus != "ACTIVE" {
			return ErrVenueInactive
		}

		// 3. Operating hours validation
		oh, err := s.repository.FindOperatingHours(ctx, tx, req.CourtID, dayOfWeek)
		if err != nil {
			return err
		}
		if oh.IsClosed || oh.OpenTime == nil || oh.CloseTime == nil {
			return ErrOutsideOpHours
		}

		// Normalize base dates for time comparison to ensure they only compare hour and minute
		// pgx may use year 2000 for TIME types, while time.Parse("15:04", ...) uses year 0000.
		startNorm := time.Date(0, 1, 1, startParsed.Hour(), startParsed.Minute(), 0, 0, time.UTC)
		endNorm := time.Date(0, 1, 1, endParsed.Hour(), endParsed.Minute(), 0, 0, time.UTC)
		openNorm := time.Date(0, 1, 1, oh.OpenTime.Hour(), oh.OpenTime.Minute(), 0, 0, time.UTC)
		closeNorm := time.Date(0, 1, 1, oh.CloseTime.Hour(), oh.CloseTime.Minute(), 0, 0, time.UTC)

		if startNorm.Before(openNorm) || endNorm.After(closeNorm) {
			return ErrOutsideOpHours
		}

		// 4. Calculate total price
		durationHours := endParsed.Sub(startParsed).Hours()
		totalPrice := math.Round(durationHours*info.PricePerHour*100) / 100

		// Check blocked slots
		isBlocked, err := s.repository.CheckBlockedSlots(ctx, tx, req.CourtID, startTz, endTz)
		if err != nil {
			return err
		}
		if isBlocked {
			return ErrOverlapBlockedSlot
		}

		// Check existing bookings
		isOverlap, err := s.repository.CheckExistingBookings(ctx, tx, req.CourtID, req.BookingDate, req.StartTime, req.EndTime)
		if err != nil {
			return err
		}
		if isOverlap {
			return ErrOverlapBooking
		}

		expiresAt := nowTz.Add(time.Duration(s.ttlMinutes) * time.Minute)

		// Insert
		params := CreateBookingParams{
			CustomerID: customerID,
			CourtID:    req.CourtID,
			Date:       req.BookingDate,
			StartTime:  req.StartTime,
			EndTime:    req.EndTime,
			TotalPrice: totalPrice,
			ExpiresAt:  &expiresAt,
		}
		b, err := s.repository.InsertBooking(ctx, tx, params)
		if err != nil {
			return err
		}
		created = b
		return nil
	})

	if err != nil {
		return BookingResponse{}, err
	}

	return toBookingResponse(created), nil
}

func (s *Service) SubmitPaymentProof(ctx context.Context, customerID, bookingID, reference string) (BookingResponse, error) {
	b, err := s.repository.UpdatePaymentReference(ctx, bookingID, customerID, reference)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return BookingResponse{}, ErrBookingNotFound
		}
		return BookingResponse{}, err
	}
	return toBookingResponse(b), nil
}

func (s *Service) VerifyPayment(ctx context.Context, ownerUserID, bookingID string, isApproved bool) (BookingResponse, error) {
	// 1. Ensure the owner actually owns this booking's venue
	profile, err := s.repository.FindOwnerProfileByUserID(ctx, ownerUserID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return BookingResponse{}, ErrOwnerProfileNotFound
		}
		return BookingResponse{}, err
	}

	ownerProfileID, err := s.repository.GetBookingOwnerProfileID(ctx, bookingID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return BookingResponse{}, ErrBookingNotFound
		}
		return BookingResponse{}, err
	}

	if ownerProfileID != profile.ID {
		return BookingResponse{}, ErrForbidden
	}

	b, err := s.repository.VerifyPayment(ctx, ownerUserID, bookingID, isApproved)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return BookingResponse{}, ErrBookingNotFound
		}
		return BookingResponse{}, err
	}
	return toBookingResponse(b), nil
}

func (s *Service) MarkBookingPaid(ctx context.Context, ownerUserID, bookingID string) (BookingResponse, error) {
	// 1. Ensure the owner actually owns this booking's venue
	profile, err := s.repository.FindOwnerProfileByUserID(ctx, ownerUserID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return BookingResponse{}, ErrOwnerProfileNotFound
		}
		return BookingResponse{}, err
	}

	ownerProfileID, err := s.repository.GetBookingOwnerProfileID(ctx, bookingID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return BookingResponse{}, ErrBookingNotFound
		}
		return BookingResponse{}, err
	}

	if ownerProfileID != profile.ID {
		return BookingResponse{}, ErrForbidden
	}

	b, err := s.repository.MarkBookingPaid(ctx, ownerUserID, bookingID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return BookingResponse{}, ErrBookingCannotBeMarkedPaid
		}
		return BookingResponse{}, err
	}
	return toBookingResponse(b), nil
}

func (s *Service) CompleteBooking(ctx context.Context, ownerUserID, bookingID string) (BookingResponse, error) {
	profile, err := s.repository.FindOwnerProfileByUserID(ctx, ownerUserID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return BookingResponse{}, ErrOwnerProfileNotFound
		}
		return BookingResponse{}, err
	}

	ownerProfileID, err := s.repository.GetBookingOwnerProfileID(ctx, bookingID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return BookingResponse{}, ErrBookingNotFound
		}
		return BookingResponse{}, err
	}

	if ownerProfileID != profile.ID {
		return BookingResponse{}, ErrForbidden
	}

	b, err := s.repository.CompleteBooking(ctx, bookingID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return BookingResponse{}, ErrBookingCannotBeCompleted
		}
		return BookingResponse{}, err
	}
	return toBookingResponse(b), nil
}

func (s *Service) CancelPaidBookingWithRefund(ctx context.Context, ownerUserID, bookingID string, reason string) (BookingResponse, error) {
	profile, err := s.repository.FindOwnerProfileByUserID(ctx, ownerUserID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return BookingResponse{}, ErrOwnerProfileNotFound
		}
		return BookingResponse{}, err
	}

	ownerProfileID, err := s.repository.GetBookingOwnerProfileID(ctx, bookingID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return BookingResponse{}, ErrBookingNotFound
		}
		return BookingResponse{}, err
	}

	if ownerProfileID != profile.ID {
		return BookingResponse{}, ErrForbidden
	}

	trimmedReason := strings.TrimSpace(reason)

	b, err := s.repository.CancelPaidBookingWithRefund(ctx, profile.UserID, bookingID, trimmedReason)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return BookingResponse{}, ErrBookingNotFound
		}
		return BookingResponse{}, err
	}

	return toBookingResponse(b), nil
}

func (s *Service) ListBookings(ctx context.Context, customerID string, page, limit int) ([]BookingResponse, int, error) {
	if page <= 0 {
		page = 1
	}
	if limit <= 0 {
		limit = 10
	}
	offset := (page - 1) * limit

	bookings, total, err := s.repository.ListByCustomerID(ctx, customerID, limit, offset)
	if err != nil {
		return nil, 0, err
	}

	responses := make([]BookingResponse, 0, len(bookings))
	for _, b := range bookings {
		responses = append(responses, toCustomerBookingResponse(b))
	}

	return responses, total, nil
}

func (s *Service) GetBooking(ctx context.Context, customerID, bookingID string) (BookingResponse, error) {
	b, err := s.repository.FindCustomerBookingByID(ctx, bookingID, customerID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return BookingResponse{}, ErrBookingNotFound
		}
		return BookingResponse{}, err
	}
	return toCustomerBookingResponse(b), nil
}

func (s *Service) ListOwnerVenueBookings(ctx context.Context, ownerUserID, venueID string, req OwnerVenueBookingsQuery) ([]OwnerBookingResponse, int, error) {
	profile, err := s.repository.FindOwnerProfileByUserID(ctx, ownerUserID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, 0, ErrOwnerProfileNotFound
		}
		return nil, 0, err
	}

	_, err = s.repository.FindVenueByIDAndOwnerProfileID(ctx, venueID, profile.ID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, 0, ErrVenueNotFound
		}
		return nil, 0, err
	}

	query := normalizeOwnerVenueBookingsQuery(req)
	offset := (query.Page - 1) * query.Limit
	bookings, total, err := s.repository.ListOwnerVenueBookings(ctx, profile.ID, venueID, query.Date, query.Status, query.Scope, query.Limit, offset)
	if err != nil {
		return nil, 0, err
	}

	responses := make([]OwnerBookingResponse, len(bookings))
	for i, booking := range bookings {
		responses[i] = toOwnerBookingResponse(booking)
	}

	return responses, total, nil
}

func normalizeOwnerBookingsQuery(req OwnerBookingsQuery) OwnerBookingsQuery {
	if req.Limit <= 0 {
		req.Limit = 10
	}
	if req.Page <= 0 {
		req.Page = 1
	}
	return req
}

func (s *Service) ListOwnerBookings(ctx context.Context, ownerUserID string, req OwnerBookingsQuery) (OwnerBookingsResult, error) {
	profile, err := s.repository.FindOwnerProfileByUserID(ctx, ownerUserID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return OwnerBookingsResult{}, ErrOwnerProfileNotFound
		}
		return OwnerBookingsResult{}, err
	}

	query := normalizeOwnerBookingsQuery(req)
	
	// Apply search trim and length logic
	if query.Q != "" {
		trimmedQ := strings.TrimSpace(query.Q)
		if len(trimmedQ) >= 2 {
			query.Q = trimmedQ
		} else {
			query.Q = ""
		}
	}

	offset := (query.Page - 1) * query.Limit
	bookings, total, err := s.repository.ListOwnerBookings(ctx, profile.ID, query, query.Limit, offset)
	if err != nil {
		return OwnerBookingsResult{}, err
	}

	responses := make([]OwnerBookingResponse, len(bookings))
	for i, booking := range bookings {
		responses[i] = toOwnerBookingResponse(booking)
	}

	totalPages := (total + query.Limit - 1) / query.Limit

	return OwnerBookingsResult{
		Data:       responses,
		Page:       query.Page,
		Limit:      query.Limit,
		Total:      total,
		TotalPages: totalPages,
	}, nil
}

func (s *Service) CancelBooking(ctx context.Context, customerID, bookingID string) (BookingResponse, error) {
	b, err := s.repository.FindByIDAndCustomerID(ctx, bookingID, customerID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return BookingResponse{}, ErrBookingNotFound
		}
		return BookingResponse{}, err
	}

	if b.Status == "CANCELLED" {
		return BookingResponse{}, ErrBookingAlreadyCancelled
	}

	if b.Status != "PENDING_PAYMENT" {
		return BookingResponse{}, ErrBookingCannotBeCancelled
	}

	updated, err := s.repository.CancelPendingByIDAndCustomerID(ctx, bookingID, customerID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			// Race fallback / refetch
			latest, findErr := s.repository.FindByIDAndCustomerID(ctx, bookingID, customerID)
			if findErr != nil {
				if errors.Is(findErr, pgx.ErrNoRows) {
					return BookingResponse{}, ErrBookingNotFound
				}
				return BookingResponse{}, findErr
			}
			if latest.Status == "CANCELLED" {
				return BookingResponse{}, ErrBookingAlreadyCancelled
			}
			if latest.Status != "PENDING_PAYMENT" {
				return BookingResponse{}, ErrBookingCannotBeCancelled
			}
			// Status still PENDING_PAYMENT but update failed for unknown reason
			return BookingResponse{}, ErrBookingCannotBeCancelled
		}
		return BookingResponse{}, err
	}

	return toBookingResponse(updated), nil
}

func (s *Service) ConfirmBookingPayment(ctx context.Context, customerID, bookingID string) (BookingResponse, error) {
	b, err := s.repository.FindByIDAndCustomerID(ctx, bookingID, customerID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return BookingResponse{}, ErrBookingNotFound
		}
		return BookingResponse{}, err
	}

	if b.Status == "CANCELLED" {
		return BookingResponse{}, ErrBookingAlreadyCancelled
	}
	if b.Status == "CONFIRMED" {
		return BookingResponse{}, ErrBookingAlreadyConfirmed
	}
	if b.Status != "PENDING_PAYMENT" {
		return BookingResponse{}, ErrBookingCannotBeConfirmed
	}

	updated, err := s.repository.ConfirmPendingByIDAndCustomerID(ctx, bookingID, customerID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			latest, findErr := s.repository.FindByIDAndCustomerID(ctx, bookingID, customerID)
			if findErr != nil {
				if errors.Is(findErr, pgx.ErrNoRows) {
					return BookingResponse{}, ErrBookingNotFound
				}
				return BookingResponse{}, findErr
			}
			if latest.Status == "CANCELLED" {
				return BookingResponse{}, ErrBookingAlreadyCancelled
			}
			if latest.Status == "CONFIRMED" {
				return BookingResponse{}, ErrBookingAlreadyConfirmed
			}
			if latest.Status != "PENDING_PAYMENT" {
				return BookingResponse{}, ErrBookingCannotBeConfirmed
			}
			return BookingResponse{}, ErrBookingCannotBeConfirmed
		}
		return BookingResponse{}, err
	}

	return toBookingResponse(updated), nil
}

func normalizeOwnerVenueBookingsQuery(req OwnerVenueBookingsQuery) OwnerVenueBookingsQuery {
	if req.Limit <= 0 {
		req.Limit = 10
	}
	if req.Page <= 0 {
		req.Page = 1
	}
	req.Status = strings.TrimSpace(req.Status)
	req.Date = strings.TrimSpace(req.Date)
	// Removed: defaulting to today if empty to allow viewing all bookings
	return req
}

func todayJakarta() time.Time {
	location, err := time.LoadLocation("Asia/Jakarta")
	if err != nil {
		location = time.FixedZone("Asia/Jakarta", 7*60*60)
	}
	now := time.Now().In(location)
	return time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, location)
}

func toBookingResponse(b Booking) BookingResponse {
	return BookingResponse{
		ID:               b.ID,
		CustomerID:       b.CustomerID,
		CourtID:          b.CourtID,
		Date:             b.Date.Format("2006-01-02"),
		StartTime:        b.StartTime.Format("15:04"),
		EndTime:          b.EndTime.Format("15:04"),
		TotalPrice:       b.TotalPrice,
		Status:           b.Status,
		PaymentReference: b.PaymentReference,
		ExpiresAt:        b.ExpiresAt,
		CreatedAt:        b.CreatedAt,
		UpdatedAt:        b.UpdatedAt,
	}
}

func toCustomerBookingResponse(cb CustomerBooking) BookingResponse {
	resp := toBookingResponse(cb.Booking)
	resp.Venue = BookingVenueSummary{ID: cb.VenueID, Name: cb.VenueName, Address: cb.VenueAddress, City: cb.VenueCity}
	resp.Court = BookingCourtSummary{ID: cb.CourtID, Name: cb.CourtName, SportName: cb.CourtSportName}
	return resp
}

func toOwnerBookingResponse(b OwnerBooking) OwnerBookingResponse {
	return OwnerBookingResponse{
		ID: b.ID,
		Customer: BookingCustomerSummary{
			ID:    b.CustomerID,
			Name:  b.CustomerName,
			Email: b.CustomerEmail,
			Phone: b.CustomerPhone,
		},
		Venue: BookingVenueSummary{
			ID:   b.VenueID,
			Name: b.VenueName,
		},
		Court: BookingCourtSummary{
			ID:   b.CourtID,
			Name: b.CourtName,
		},
		Date:             b.Date.Format("2006-01-02"),
		StartTime:        b.StartTime.Format("15:04"),
		EndTime:          b.EndTime.Format("15:04"),
		TotalPrice:       b.TotalPrice,
		Status:           b.Status,
		PaymentReference: b.PaymentReference,
		ExpiresAt:        b.ExpiresAt,
		CreatedAt:        b.CreatedAt,
		UpdatedAt:        b.UpdatedAt,
	}
}
func (s *Service) GetOwnerMetrics(ctx context.Context, ownerUserID string, query OwnerMetricsQuery) (OwnerMetricsResponse, error) {
	profile, err := s.repository.FindOwnerProfileByUserID(ctx, ownerUserID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return OwnerMetricsResponse{}, ErrOwnerProfileNotFound
		}
		return OwnerMetricsResponse{}, err
	}

	metrics, err := s.repository.GetOwnerMetrics(ctx, profile.ID, query.StartDate, query.EndDate)
	if err != nil {
		return OwnerMetricsResponse{}, err
	}

	return OwnerMetricsResponse{
		TotalVenues:          metrics.TotalVenues,
		UpcomingBookings:     metrics.UpcomingBookings,
		PendingVerifications: metrics.PendingVerifications,
		RevenueCurrent:        metrics.RevenueCurrent,
		BookingRevenueCurrent: metrics.BookingRevenueCurrent,
		RefundCurrent:         metrics.RefundCurrent,
		NetRevenueCurrent:     metrics.NetRevenueCurrent,
		RevenueAllTime:        metrics.RevenueAllTime,
		OccupancyRate:        metrics.OccupancyRate,
	}, nil
}

func (s *Service) SweepExpiredBookings(ctx context.Context) (int64, error) {
	return s.repository.CancelExpiredPendingBookings(ctx)
}

func (s *Service) StartExpiryWorker(ctx context.Context, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			_, err := s.SweepExpiredBookings(ctx)
			if err != nil {
				log.Printf("Error sweeping expired bookings: %v", err)
			}
		}
	}
}
