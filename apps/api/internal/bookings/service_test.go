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
	UpdatedBooking  Booking
	UpdateErr       error
	FindBooking     Booking
	FindErr         error
	FindFallback    *Booking
	findCallCount   int

	OwnerProfile     OwnerProfile
	OwnerProfileErr  error
	OwnerVenue       OwnerVenue
	OwnerVenueErr    error
	OwnerBookings    []OwnerBooking
	OwnerBookingsErr error
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
	m.findCallCount++
	if m.findCallCount > 1 && m.FindFallback != nil {
		return *m.FindFallback, m.FindErr
	}
	return m.FindBooking, m.FindErr
}

func (m *mockRepo) FindOwnerProfileByUserID(ctx context.Context, userID string) (OwnerProfile, error) {
	return m.OwnerProfile, m.OwnerProfileErr
}

func (m *mockRepo) FindVenueByIDAndOwnerProfileID(ctx context.Context, venueID, ownerProfileID string) (OwnerVenue, error) {
	return m.OwnerVenue, m.OwnerVenueErr
}

func (m *mockRepo) ListOwnerVenueBookings(ctx context.Context, ownerProfileID, venueID, date, status string, limit, offset int) ([]OwnerBooking, error) {
	return m.OwnerBookings, m.OwnerBookingsErr
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

func (m *mockRepo) CancelPendingByIDAndCustomerID(ctx context.Context, bookingID, customerID string) (Booking, error) {
	if m.UpdateErr != nil {
		return Booking{}, m.UpdateErr
	}
	b := m.UpdatedBooking
	b.Status = "CANCELLED"
	return b, nil
}

func (m *mockRepo) ConfirmPendingByIDAndCustomerID(ctx context.Context, bookingID, customerID string) (Booking, error) {
	if m.UpdateErr != nil {
		return Booking{}, m.UpdateErr
	}
	b := m.UpdatedBooking
	b.Status = "CONFIRMED"
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

func TestCreateBooking_Success_PgxBaseYear2000(t *testing.T) {
	loc, _ := time.LoadLocation("Asia/Jakarta")
	tomorrow := time.Now().In(loc).AddDate(0, 0, 1).Format("2006-01-02")

	// Simulate pgx returning base year 2000 for TIME types
	openTime := time.Date(2000, 1, 1, 8, 0, 0, 0, time.UTC)
	closeTime := time.Date(2000, 1, 1, 22, 0, 0, 0, time.UTC)

	repo := &mockRepo{
		CourtValidationInfo: CourtValidationInfo{PricePerHour: 100000, CourtStatus: "ACTIVE", VenueStatus: "ACTIVE"},
		OperatingHour:       OperatingHour{IsClosed: false, OpenTime: &openTime, CloseTime: &closeTime},
		IsBlocked:           false,
		IsOverlap:           false,
		InsertedBooking:     Booking{ID: "booking-regression"},
	}
	svc := NewService(repo)

	// Booking from 10:00 to 11:00 should pass despite the base year difference
	req := CreateBookingRequest{
		CourtID:     "court-1",
		BookingDate: tomorrow,
		StartTime:   "10:00",
		EndTime:     "11:00",
	}

	resp, err := svc.CreateBooking(context.Background(), "user-1", req)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if resp.ID != "booking-regression" {
		t.Fatalf("expected booking-regression, got %s", resp.ID)
	}
}

func TestCreateBooking_Success_BoundaryOperatingHours(t *testing.T) {
	loc, _ := time.LoadLocation("Asia/Jakarta")
	tomorrow := time.Now().In(loc).AddDate(0, 0, 1).Format("2006-01-02")

	openTime := time.Date(2000, 1, 1, 8, 0, 0, 0, time.UTC)
	closeTime := time.Date(2000, 1, 1, 22, 0, 0, 0, time.UTC)

	repo := &mockRepo{
		CourtValidationInfo: CourtValidationInfo{PricePerHour: 100000, CourtStatus: "ACTIVE", VenueStatus: "ACTIVE"},
		OperatingHour:       OperatingHour{IsClosed: false, OpenTime: &openTime, CloseTime: &closeTime},
		IsBlocked:           false,
		IsOverlap:           false,
		InsertedBooking:     Booking{ID: "booking-boundary"},
	}
	svc := NewService(repo)

	// Boundary 1: 08:00 to 09:00
	req1 := CreateBookingRequest{
		CourtID:     "court-1",
		BookingDate: tomorrow,
		StartTime:   "08:00",
		EndTime:     "09:00",
	}
	if _, err := svc.CreateBooking(context.Background(), "user-1", req1); err != nil {
		t.Fatalf("boundary 08:00-09:00 failed: %v", err)
	}

	// Boundary 2: 21:00 to 22:00
	req2 := CreateBookingRequest{
		CourtID:     "court-1",
		BookingDate: tomorrow,
		StartTime:   "21:00",
		EndTime:     "22:00",
	}
	if _, err := svc.CreateBooking(context.Background(), "user-1", req2); err != nil {
		t.Fatalf("boundary 21:00-22:00 failed: %v", err)
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

func TestListOwnerVenueBookings_Success(t *testing.T) {
	date := time.Date(2026, time.June, 25, 0, 0, 0, 0, time.UTC)
	start, _ := time.Parse("15:04", "10:00")
	end, _ := time.Parse("15:04", "12:00")
	phone := "081234567890"
	repo := &mockRepo{
		OwnerProfile: OwnerProfile{ID: "owner-profile-1", UserID: "owner-user-1"},
		OwnerVenue:   OwnerVenue{ID: "venue-1", Name: "Arena A"},
		OwnerBookings: []OwnerBooking{
			{
				ID:            "booking-1",
				CustomerID:    "customer-1",
				CustomerName:  "Customer One",
				CustomerEmail: "customer@example.com",
				CustomerPhone: &phone,
				VenueID:       "venue-1",
				VenueName:     "Arena A",
				CourtID:       "court-1",
				CourtName:     "Court 1",
				Date:          date,
				StartTime:     start,
				EndTime:       end,
				TotalPrice:    200000,
				Status:        "PENDING_PAYMENT",
			},
		},
	}
	svc := NewService(repo)

	result, err := svc.ListOwnerVenueBookings(context.Background(), "owner-user-1", "venue-1", OwnerVenueBookingsQuery{
		Date:   "2026-06-25",
		Status: "PENDING_PAYMENT",
		Limit:  20,
		Page:   2,
	})
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if result.Date != "2026-06-25" || result.Status != "PENDING_PAYMENT" || result.Limit != 20 || result.Page != 2 {
		t.Fatalf("unexpected pagination/query result: %+v", result)
	}
	if len(result.Bookings) != 1 {
		t.Fatalf("expected 1 booking, got %d", len(result.Bookings))
	}
	booking := result.Bookings[0]
	if booking.Customer.Name != "Customer One" || booking.Venue.Name != "Arena A" || booking.Court.Name != "Court 1" {
		t.Fatalf("unexpected owner booking response: %+v", booking)
	}
}

func TestListOwnerVenueBookings_Fail_OwnerProfileNotFound(t *testing.T) {
	repo := &mockRepo{OwnerProfileErr: pgx.ErrNoRows}
	svc := NewService(repo)

	_, err := svc.ListOwnerVenueBookings(context.Background(), "owner-user-1", "venue-1", OwnerVenueBookingsQuery{Date: "2026-06-25"})
	if err != ErrOwnerProfileNotFound {
		t.Fatalf("expected ErrOwnerProfileNotFound, got %v", err)
	}
}

func TestListOwnerVenueBookings_Fail_VenueNotFound(t *testing.T) {
	repo := &mockRepo{
		OwnerProfile:  OwnerProfile{ID: "owner-profile-1", UserID: "owner-user-1"},
		OwnerVenueErr: pgx.ErrNoRows,
	}
	svc := NewService(repo)

	_, err := svc.ListOwnerVenueBookings(context.Background(), "owner-user-1", "venue-1", OwnerVenueBookingsQuery{Date: "2026-06-25"})
	if err != ErrVenueNotFound {
		t.Fatalf("expected ErrVenueNotFound, got %v", err)
	}
}

func TestCancelBooking_Success(t *testing.T) {
	repo := &mockRepo{
		FindBooking:    Booking{ID: "booking-1", CustomerID: "user-1", Status: "PENDING_PAYMENT"},
		UpdatedBooking: Booking{ID: "booking-1", CustomerID: "user-1", Status: "CANCELLED"},
	}
	svc := NewService(repo)

	resp, err := svc.CancelBooking(context.Background(), "user-1", "booking-1")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if resp.Status != "CANCELLED" {
		t.Fatalf("expected status CANCELLED, got %s", resp.Status)
	}
}

func TestCancelBooking_Fail_AlreadyCancelled(t *testing.T) {
	repo := &mockRepo{
		FindBooking: Booking{ID: "booking-1", CustomerID: "user-1", Status: "CANCELLED"},
	}
	svc := NewService(repo)

	_, err := svc.CancelBooking(context.Background(), "user-1", "booking-1")
	if err != ErrBookingAlreadyCancelled {
		t.Fatalf("expected ErrBookingAlreadyCancelled, got %v", err)
	}
}

func TestCancelBooking_Fail_CannotCancelPaid(t *testing.T) {
	repo := &mockRepo{
		FindBooking: Booking{ID: "booking-1", CustomerID: "user-1", Status: "PAID"},
	}
	svc := NewService(repo)

	_, err := svc.CancelBooking(context.Background(), "user-1", "booking-1")
	if err != ErrBookingCannotBeCancelled {
		t.Fatalf("expected ErrBookingCannotBeCancelled, got %v", err)
	}
}

func TestCancelBooking_Fail_NotFound(t *testing.T) {
	repo := &mockRepo{
		FindErr: pgx.ErrNoRows,
	}
	svc := NewService(repo)

	_, err := svc.CancelBooking(context.Background(), "user-1", "booking-1")
	if err != ErrBookingNotFound {
		t.Fatalf("expected ErrBookingNotFound, got %v", err)
	}
}

func TestCancelBooking_Fail_CannotCancelConfirmed(t *testing.T) {
	repo := &mockRepo{
		FindBooking: Booking{ID: "booking-1", CustomerID: "user-1", Status: "CONFIRMED"},
	}
	svc := NewService(repo)

	_, err := svc.CancelBooking(context.Background(), "user-1", "booking-1")
	if err != ErrBookingCannotBeCancelled {
		t.Fatalf("expected ErrBookingCannotBeCancelled, got %v", err)
	}
}

func TestCancelBooking_Fail_StatusChangedDuringCancel(t *testing.T) {
	repo := &mockRepo{
		FindBooking:  Booking{ID: "booking-1", CustomerID: "user-1", Status: "PENDING_PAYMENT"},
		FindFallback: &Booking{ID: "booking-1", CustomerID: "user-1", Status: "PAID"},
		UpdateErr:    pgx.ErrNoRows,
	}
	svc := NewService(repo)

	_, err := svc.CancelBooking(context.Background(), "user-1", "booking-1")
	if err != ErrBookingCannotBeCancelled {
		t.Fatalf("expected ErrBookingCannotBeCancelled, got %v", err)
	}
}

func TestCancelBooking_Fail_BecameCancelledDuringCancel(t *testing.T) {
	repo := &mockRepo{
		FindBooking:  Booking{ID: "booking-1", CustomerID: "user-1", Status: "PENDING_PAYMENT"},
		FindFallback: &Booking{ID: "booking-1", CustomerID: "user-1", Status: "CANCELLED"},
		UpdateErr:    pgx.ErrNoRows,
	}
	svc := NewService(repo)

	_, err := svc.CancelBooking(context.Background(), "user-1", "booking-1")
	if err != ErrBookingAlreadyCancelled {
		t.Fatalf("expected ErrBookingAlreadyCancelled, got %v", err)
	}
}

func TestConfirmBookingPayment_Success(t *testing.T) {
	repo := &mockRepo{
		FindBooking:    Booking{ID: "booking-1", CustomerID: "user-1", Status: "PENDING_PAYMENT"},
		UpdatedBooking: Booking{ID: "booking-1", CustomerID: "user-1", Status: "CONFIRMED"},
	}
	svc := NewService(repo)

	resp, err := svc.ConfirmBookingPayment(context.Background(), "user-1", "booking-1")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if resp.Status != "CONFIRMED" {
		t.Fatalf("expected status CONFIRMED, got %s", resp.Status)
	}
}

func TestConfirmBookingPayment_Fail_NotFound(t *testing.T) {
	repo := &mockRepo{
		FindErr: pgx.ErrNoRows,
	}
	svc := NewService(repo)

	_, err := svc.ConfirmBookingPayment(context.Background(), "user-1", "booking-1")
	if err != ErrBookingNotFound {
		t.Fatalf("expected ErrBookingNotFound, got %v", err)
	}
}

func TestConfirmBookingPayment_Fail_AlreadyCancelled(t *testing.T) {
	repo := &mockRepo{
		FindBooking: Booking{ID: "booking-1", CustomerID: "user-1", Status: "CANCELLED"},
	}
	svc := NewService(repo)

	_, err := svc.ConfirmBookingPayment(context.Background(), "user-1", "booking-1")
	if err != ErrBookingAlreadyCancelled {
		t.Fatalf("expected ErrBookingAlreadyCancelled, got %v", err)
	}
}

func TestConfirmBookingPayment_Fail_AlreadyConfirmed(t *testing.T) {
	repo := &mockRepo{
		FindBooking: Booking{ID: "booking-1", CustomerID: "user-1", Status: "CONFIRMED"},
	}
	svc := NewService(repo)

	_, err := svc.ConfirmBookingPayment(context.Background(), "user-1", "booking-1")
	if err != ErrBookingAlreadyConfirmed {
		t.Fatalf("expected ErrBookingAlreadyConfirmed, got %v", err)
	}
}

func TestConfirmBookingPayment_Fail_PaidCannotConfirm(t *testing.T) {
	repo := &mockRepo{
		FindBooking: Booking{ID: "booking-1", CustomerID: "user-1", Status: "PAID"},
	}
	svc := NewService(repo)

	_, err := svc.ConfirmBookingPayment(context.Background(), "user-1", "booking-1")
	if err != ErrBookingCannotBeConfirmed {
		t.Fatalf("expected ErrBookingCannotBeConfirmed, got %v", err)
	}
}

func TestConfirmBookingPayment_Fail_StatusChangedToCancelledDuringConfirm(t *testing.T) {
	repo := &mockRepo{
		FindBooking:  Booking{ID: "booking-1", CustomerID: "user-1", Status: "PENDING_PAYMENT"},
		FindFallback: &Booking{ID: "booking-1", CustomerID: "user-1", Status: "CANCELLED"},
		UpdateErr:    pgx.ErrNoRows,
	}
	svc := NewService(repo)

	_, err := svc.ConfirmBookingPayment(context.Background(), "user-1", "booking-1")
	if err != ErrBookingAlreadyCancelled {
		t.Fatalf("expected ErrBookingAlreadyCancelled, got %v", err)
	}
}

func TestConfirmBookingPayment_Fail_StatusChangedToConfirmedDuringConfirm(t *testing.T) {
	repo := &mockRepo{
		FindBooking:  Booking{ID: "booking-1", CustomerID: "user-1", Status: "PENDING_PAYMENT"},
		FindFallback: &Booking{ID: "booking-1", CustomerID: "user-1", Status: "CONFIRMED"},
		UpdateErr:    pgx.ErrNoRows,
	}
	svc := NewService(repo)

	_, err := svc.ConfirmBookingPayment(context.Background(), "user-1", "booking-1")
	if err != ErrBookingAlreadyConfirmed {
		t.Fatalf("expected ErrBookingAlreadyConfirmed, got %v", err)
	}
}
