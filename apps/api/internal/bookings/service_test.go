package bookings

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
)

type mockRepo struct {
	CourtValidationInfo CourtValidationInfo
	CourtInfoErr        error

	OperatingHour OperatingHour
	OpHourErr     error

	IsBlocked  bool
	BlockedErr error

	IsOverlap  bool
	OverlapErr error

	InsertedBooking Booking
	InsertErr       error
}

func (m *mockRepo) LockCourtValidationInfo(ctx context.Context, tx pgx.Tx, courtID string) (CourtValidationInfo, error) {
	return m.CourtValidationInfo, m.CourtInfoErr
}

func (m *mockRepo) FindOperatingHours(ctx context.Context, tx pgx.Tx, courtID string, dayOfWeek int) (OperatingHour, error) {
	return m.OperatingHour, m.OpHourErr
}

func (m *mockRepo) ListByCustomerID(ctx context.Context, customerID string) ([]Booking, error) {
	return nil, nil
}

func (m *mockRepo) FindByIDAndCustomerID(ctx context.Context, id, customerID string) (Booking, error) {
	return Booking{}, nil
}

func (m *mockRepo) ExecuteBookingTx(ctx context.Context, fn func(tx pgx.Tx) error) error {
	// mock transaction execution directly calling the function without a real tx
	return fn(nil)
}

func (m *mockRepo) CheckBlockedSlots(ctx context.Context, tx pgx.Tx, courtID string, startTz, endTz time.Time) (bool, error) {
	return m.IsBlocked, m.BlockedErr
}

func (m *mockRepo) CheckExistingBookings(ctx context.Context, tx pgx.Tx, courtID, date, startTime, endTime string) (bool, error) {
	return m.IsOverlap, m.OverlapErr
}

func (m *mockRepo) InsertBooking(ctx context.Context, tx pgx.Tx, params CreateBookingParams) (Booking, error) {
	if m.InsertErr != nil {
		return Booking{}, m.InsertErr
	}
	b := m.InsertedBooking
	b.TotalPrice = params.TotalPrice
	return b, nil
}

func ptrTime(t time.Time) *time.Time {
	return &t
}

func TestCreateBooking_Success(t *testing.T) {
	loc, _ := time.LoadLocation("Asia/Jakarta")
	tomorrow := time.Now().In(loc).AddDate(0, 0, 1).Format("2006-01-02")

	start, _ := time.Parse("15:04", "10:00")
	end, _ := time.Parse("15:04", "12:00")

	repo := &mockRepo{
		CourtValidationInfo: CourtValidationInfo{PricePerHour: 100000, CourtStatus: "ACTIVE", VenueStatus: "ACTIVE"},
		OperatingHour:       OperatingHour{IsClosed: false, OpenTime: ptrTime(start.Add(-time.Hour)), CloseTime: ptrTime(end.Add(time.Hour))},
		IsBlocked:           false,
		IsOverlap:           false,
		InsertedBooking:     Booking{ID: "booking-1"},
	}
	svc := NewService(repo)

	req := CreateBookingRequest{
		CourtID:     "court-1",
		BookingDate: tomorrow,
		StartTime:   "10:00",
		EndTime:     "12:00",
	}

	resp, err := svc.CreateBooking(context.Background(), "user-1", req)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if resp.ID != "booking-1" {
		t.Fatalf("expected booking-1, got %s", resp.ID)
	}
	if resp.TotalPrice != 200000 {
		t.Fatalf("expected total price 200000, got %f", resp.TotalPrice)
	}
}

func TestCreateBooking_Fail_PastDate(t *testing.T) {
	loc, _ := time.LoadLocation("Asia/Jakarta")
	yesterday := time.Now().In(loc).AddDate(0, 0, -1).Format("2006-01-02")

	svc := NewService(&mockRepo{})
	req := CreateBookingRequest{BookingDate: yesterday, StartTime: "10:00", EndTime: "12:00"}

	_, err := svc.CreateBooking(context.Background(), "user-1", req)
	if err != ErrPastDate {
		t.Fatalf("expected ErrPastDate, got %v", err)
	}
}

func TestCreateBooking_Fail_InvalidTimeRange(t *testing.T) {
	loc, _ := time.LoadLocation("Asia/Jakarta")
	tomorrow := time.Now().In(loc).AddDate(0, 0, 1).Format("2006-01-02")

	svc := NewService(&mockRepo{})
	req := CreateBookingRequest{BookingDate: tomorrow, StartTime: "12:00", EndTime: "10:00"}

	_, err := svc.CreateBooking(context.Background(), "user-1", req)
	if err != ErrInvalidTimeRange {
		t.Fatalf("expected ErrInvalidTimeRange, got %v", err)
	}
}

func TestCreateBooking_Fail_InactiveCourtOrVenue(t *testing.T) {
	loc, _ := time.LoadLocation("Asia/Jakarta")
	tomorrow := time.Now().In(loc).AddDate(0, 0, 1).Format("2006-01-02")

	repo := &mockRepo{CourtValidationInfo: CourtValidationInfo{CourtStatus: "INACTIVE", VenueStatus: "ACTIVE"}}
	svc := NewService(repo)
	req := CreateBookingRequest{BookingDate: tomorrow, StartTime: "10:00", EndTime: "12:00"}

	_, err := svc.CreateBooking(context.Background(), "user-1", req)
	if err != ErrCourtInactive {
		t.Fatalf("expected ErrCourtInactive, got %v", err)
	}

	repo.CourtValidationInfo = CourtValidationInfo{CourtStatus: "ACTIVE", VenueStatus: "INACTIVE"}
	_, err = svc.CreateBooking(context.Background(), "user-1", req)
	if err != ErrVenueInactive {
		t.Fatalf("expected ErrVenueInactive, got %v", err)
	}
}

func TestCreateBooking_Fail_OutsideOperatingHours(t *testing.T) {
	loc, _ := time.LoadLocation("Asia/Jakarta")
	tomorrow := time.Now().In(loc).AddDate(0, 0, 1).Format("2006-01-02")

	start, _ := time.Parse("15:04", "08:00")
	end, _ := time.Parse("15:04", "10:00") // Court opens 08:00 to 10:00

	repo := &mockRepo{
		CourtValidationInfo: CourtValidationInfo{CourtStatus: "ACTIVE", VenueStatus: "ACTIVE"},
		OperatingHour:       OperatingHour{IsClosed: false, OpenTime: ptrTime(start), CloseTime: ptrTime(end)},
	}
	svc := NewService(repo)

	// Requesting 10:00 to 12:00
	req := CreateBookingRequest{BookingDate: tomorrow, StartTime: "10:00", EndTime: "12:00"}
	_, err := svc.CreateBooking(context.Background(), "user-1", req)
	if err != ErrOutsideOpHours {
		t.Fatalf("expected ErrOutsideOpHours, got %v", err)
	}
}

func TestCreateBooking_Fail_OverlapBlockedSlots(t *testing.T) {
	loc, _ := time.LoadLocation("Asia/Jakarta")
	tomorrow := time.Now().In(loc).AddDate(0, 0, 1).Format("2006-01-02")

	start, _ := time.Parse("15:04", "08:00")
	end, _ := time.Parse("15:04", "20:00")

	repo := &mockRepo{
		CourtValidationInfo: CourtValidationInfo{CourtStatus: "ACTIVE", VenueStatus: "ACTIVE"},
		OperatingHour:       OperatingHour{IsClosed: false, OpenTime: ptrTime(start), CloseTime: ptrTime(end)},
		IsBlocked:           true,
	}
	svc := NewService(repo)

	req := CreateBookingRequest{BookingDate: tomorrow, StartTime: "10:00", EndTime: "12:00"}
	_, err := svc.CreateBooking(context.Background(), "user-1", req)
	if err != ErrOverlapBlockedSlot {
		t.Fatalf("expected ErrOverlapBlockedSlot, got %v", err)
	}
}

func TestCreateBooking_Fail_OverlapExistingBooking(t *testing.T) {
	loc, _ := time.LoadLocation("Asia/Jakarta")
	tomorrow := time.Now().In(loc).AddDate(0, 0, 1).Format("2006-01-02")

	start, _ := time.Parse("15:04", "08:00")
	end, _ := time.Parse("15:04", "20:00")

	repo := &mockRepo{
		CourtValidationInfo: CourtValidationInfo{CourtStatus: "ACTIVE", VenueStatus: "ACTIVE"},
		OperatingHour:       OperatingHour{IsClosed: false, OpenTime: ptrTime(start), CloseTime: ptrTime(end)},
		IsBlocked:           false,
		IsOverlap:           true,
	}
	svc := NewService(repo)

	req := CreateBookingRequest{BookingDate: tomorrow, StartTime: "10:00", EndTime: "12:00"}
	_, err := svc.CreateBooking(context.Background(), "user-1", req)
	if err != ErrOverlapBooking {
		t.Fatalf("expected ErrOverlapBooking, got %v", err)
	}
}
