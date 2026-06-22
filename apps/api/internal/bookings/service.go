package bookings

import (
	"context"
	"errors"
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
)

type BookingRepository interface {
	LockCourtValidationInfo(ctx context.Context, tx pgx.Tx, courtID string) (CourtValidationInfo, error)
	FindOperatingHours(ctx context.Context, tx pgx.Tx, courtID string, dayOfWeek int) (OperatingHour, error)
	ListByCustomerID(ctx context.Context, customerID string) ([]Booking, error)
	FindByIDAndCustomerID(ctx context.Context, id, customerID string) (Booking, error)
	FindOwnerProfileByUserID(ctx context.Context, userID string) (OwnerProfile, error)
	FindVenueByIDAndOwnerProfileID(ctx context.Context, venueID, ownerProfileID string) (OwnerVenue, error)
	ListOwnerVenueBookings(ctx context.Context, ownerProfileID, venueID, date, status string, limit, offset int) ([]OwnerBooking, error)
	ExecuteBookingTx(ctx context.Context, fn func(tx pgx.Tx) error) error
	CheckBlockedSlots(ctx context.Context, tx pgx.Tx, courtID string, startTz, endTz time.Time) (bool, error)
	CheckExistingBookings(ctx context.Context, tx pgx.Tx, courtID, date, startTime, endTime string) (bool, error)
	InsertBooking(ctx context.Context, tx pgx.Tx, params CreateBookingParams) (Booking, error)
	CancelPendingByIDAndCustomerID(ctx context.Context, bookingID, customerID string) (Booking, error)
	ConfirmPendingByIDAndCustomerID(ctx context.Context, bookingID, customerID string) (Booking, error)
}

type Service struct {
	repository BookingRepository
}

func NewService(repository BookingRepository) *Service {
	return &Service{repository: repository}
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

		// Insert
		params := CreateBookingParams{
			CustomerID: customerID,
			CourtID:    req.CourtID,
			Date:       req.BookingDate,
			StartTime:  req.StartTime,
			EndTime:    req.EndTime,
			TotalPrice: totalPrice,
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

func (s *Service) ListBookings(ctx context.Context, customerID string) ([]BookingResponse, error) {
	bookings, err := s.repository.ListByCustomerID(ctx, customerID)
	if err != nil {
		return nil, err
	}

	responses := make([]BookingResponse, len(bookings))
	for i, b := range bookings {
		responses[i] = toBookingResponse(b)
	}

	return responses, nil
}

func (s *Service) GetBooking(ctx context.Context, customerID, bookingID string) (BookingResponse, error) {
	b, err := s.repository.FindByIDAndCustomerID(ctx, bookingID, customerID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return BookingResponse{}, ErrBookingNotFound
		}
		return BookingResponse{}, err
	}
	return toBookingResponse(b), nil
}

func (s *Service) ListOwnerVenueBookings(ctx context.Context, ownerUserID, venueID string, req OwnerVenueBookingsQuery) (OwnerVenueBookingsResult, error) {
	profile, err := s.repository.FindOwnerProfileByUserID(ctx, ownerUserID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return OwnerVenueBookingsResult{}, ErrOwnerProfileNotFound
		}
		return OwnerVenueBookingsResult{}, err
	}

	_, err = s.repository.FindVenueByIDAndOwnerProfileID(ctx, venueID, profile.ID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return OwnerVenueBookingsResult{}, ErrVenueNotFound
		}
		return OwnerVenueBookingsResult{}, err
	}

	query := normalizeOwnerVenueBookingsQuery(req)
	offset := (query.Page - 1) * query.Limit
	bookings, err := s.repository.ListOwnerVenueBookings(ctx, profile.ID, venueID, query.Date, query.Status, query.Limit, offset)
	if err != nil {
		return OwnerVenueBookingsResult{}, err
	}

	responses := make([]OwnerBookingResponse, len(bookings))
	for i, booking := range bookings {
		responses[i] = toOwnerBookingResponse(booking)
	}

	return OwnerVenueBookingsResult{
		Bookings: responses,
		Date:     query.Date,
		Status:   query.Status,
		Page:     query.Page,
		Limit:    query.Limit,
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
	if req.Date == "" {
		req.Date = todayJakarta().Format("2006-01-02")
	}
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
		ID:         b.ID,
		CustomerID: b.CustomerID,
		CourtID:    b.CourtID,
		Date:       b.Date.Format("2006-01-02"),
		StartTime:  b.StartTime.Format("15:04"),
		EndTime:    b.EndTime.Format("15:04"),
		TotalPrice: b.TotalPrice,
		Status:     b.Status,
		CreatedAt:  b.CreatedAt,
		UpdatedAt:  b.UpdatedAt,
	}
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
		Date:       b.Date.Format("2006-01-02"),
		StartTime:  b.StartTime.Format("15:04"),
		EndTime:    b.EndTime.Format("15:04"),
		TotalPrice: b.TotalPrice,
		Status:     b.Status,
		CreatedAt:  b.CreatedAt,
		UpdatedAt:  b.UpdatedAt,
	}
}
