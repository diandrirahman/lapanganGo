package bookings

import (
	"context"
	"errors"
	"math"
	"time"

	"github.com/jackc/pgx/v5"
)

var (
	ErrPastDate           = errors.New("booking date cannot be in the past")
	ErrInvalidTimeRange   = errors.New("start time must be before end time")
	ErrCourtInactive      = errors.New("court is not active")
	ErrVenueInactive      = errors.New("venue is not active")
	ErrOutsideOpHours     = errors.New("booking time is outside court operating hours")
	ErrOverlapBlockedSlot = errors.New("court is blocked/maintenance during the requested time")
	ErrOverlapBooking     = errors.New("court is already booked for the requested time")
	ErrBookingNotFound    = errors.New("booking not found")
)

type BookingRepository interface {
	LockCourtValidationInfo(ctx context.Context, tx pgx.Tx, courtID string) (CourtValidationInfo, error)
	FindOperatingHours(ctx context.Context, tx pgx.Tx, courtID string, dayOfWeek int) (OperatingHour, error)
	ListByCustomerID(ctx context.Context, customerID string) ([]Booking, error)
	FindByIDAndCustomerID(ctx context.Context, id, customerID string) (Booking, error)
	ExecuteBookingTx(ctx context.Context, fn func(tx pgx.Tx) error) error
	CheckBlockedSlots(ctx context.Context, tx pgx.Tx, courtID string, startTz, endTz time.Time) (bool, error)
	CheckExistingBookings(ctx context.Context, tx pgx.Tx, courtID, date, startTime, endTime string) (bool, error)
	InsertBooking(ctx context.Context, tx pgx.Tx, params CreateBookingParams) (Booking, error)
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

		if startParsed.Before(*oh.OpenTime) || endParsed.After(*oh.CloseTime) {
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

	responses := make([]BookingResponse, 0, len(bookings))
	for _, b := range bookings {
		responses = append(responses, toBookingResponse(b))
	}
	return responses, nil
}

func (s *Service) GetBooking(ctx context.Context, customerID, bookingID string) (BookingResponse, error) {
	b, err := s.repository.FindByIDAndCustomerID(ctx, bookingID, customerID)
	if err != nil {
		if err == pgx.ErrNoRows {
			return BookingResponse{}, ErrBookingNotFound
		}
		return BookingResponse{}, err
	}

	return toBookingResponse(b), nil
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
