package bookings

import (
	"context"
	"errors"
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

	LastCreateParams CreateBookingParams
	InsertedBooking  Booking
	InsertErr        error
	UpdatedBooking   Booking
	UpdateErr        error
	FindBooking      Booking
	FindErr          error
	FindFallback     *Booking
	findCallCount    int

	OwnerProfile     OwnerProfile
	OwnerProfileErr  error
	OwnerVenue       OwnerVenue
	OwnerVenueErr    error
	OwnerBookings    []OwnerBooking
	OwnerBookingsErr error

	OwnerMetrics    OwnerMetrics
	OwnerMetricsErr error

	BookingOwnerProfileID    string
	BookingOwnerProfileIDErr error

	CancelExpiredCount int64
	CancelExpiredErr   error

	AutoCompleteCount int64
	AutoCompleteErr   error

	LastOperatingHourDayOfWeek int
}

func (m *mockRepo) CancelExpiredPendingBookings(ctx context.Context) (int64, error) {
	return m.CancelExpiredCount, m.CancelExpiredErr
}

func (m *mockRepo) GetOwnerUserIDByCourtID(ctx context.Context, courtID string) (string, error) {
	return "owner-user-123", nil
}

func (m *mockRepo) GetOwnerUserIDByBookingID(ctx context.Context, bookingID string) (string, error) {
	return "owner-user-123", nil
}

func (m *mockRepo) AutoCompleteFinishedBookings(ctx context.Context) ([]Booking, error) {
	if m.AutoCompleteErr != nil {
		return nil, m.AutoCompleteErr
	}
	var bookings []Booking
	for i := 0; i < int(m.AutoCompleteCount); i++ {
		bookings = append(bookings, Booking{ID: "dummy", CustomerID: "dummy"})
	}
	return bookings, nil
}

func (m *mockRepo) GetBookingsExpiringSoon(ctx context.Context, cutoff time.Time) ([]Booking, error) {
	return nil, nil
}

func (m *mockRepo) GetOwnerMetrics(ctx context.Context, ownerProfileID string, startDate string, endDate string) (OwnerMetrics, error) {
	return m.OwnerMetrics, m.OwnerMetricsErr
}

func (m *mockRepo) UpdatePaymentReference(ctx context.Context, bookingID, customerID, reference string) (Booking, error) {
	return m.UpdatedBooking, m.UpdateErr
}

func (m *mockRepo) VerifyPayment(ctx context.Context, ownerUserID string, bookingID string, isApproved bool) (Booking, error) {
	return m.UpdatedBooking, m.UpdateErr
}

func (m *mockRepo) MarkBookingPaid(ctx context.Context, ownerUserID string, bookingID string) (Booking, error) {
	return m.UpdatedBooking, m.UpdateErr
}

func (m *mockRepo) CompleteBooking(ctx context.Context, bookingID string) (Booking, error) {
	return m.UpdatedBooking, m.UpdateErr
}

func (m *mockRepo) CancelPaidBookingWithRefund(ctx context.Context, ownerUserID string, bookingID string, reason string) (Booking, error) {
	return m.UpdatedBooking, m.UpdateErr
}

func (m *mockRepo) GetBookingOwnerProfileID(ctx context.Context, bookingID string) (string, error) {
	if m.BookingOwnerProfileIDErr != nil {
		return "", m.BookingOwnerProfileIDErr
	}
	if m.BookingOwnerProfileID != "" {
		return m.BookingOwnerProfileID, nil
	}
	if m.OwnerProfileErr != nil {
		return "", m.OwnerProfileErr
	}
	if m.OwnerProfile.ID != "" {
		return m.OwnerProfile.ID, nil
	}
	return "mock-owner-profile-id", nil
}

func (m *mockRepo) LockCourtValidationInfo(ctx context.Context, tx pgx.Tx, courtID string) (CourtValidationInfo, error) {
	return m.CourtValidationInfo, m.CourtInfoErr
}

func (m *mockRepo) LockOwnerCourtValidationInfo(ctx context.Context, tx pgx.Tx, courtID, venueID, ownerProfileID string) (CourtValidationInfo, error) {
	return m.CourtValidationInfo, m.CourtInfoErr
}

func (m *mockRepo) InsertOfflineBookingTx(ctx context.Context, tx pgx.Tx, params CreateOfflineBookingParams) (Booking, error) {
	if m.InsertErr != nil {
		return Booking{}, m.InsertErr
	}
	return m.InsertedBooking, nil
}

func (m *mockRepo) FindOperatingHours(ctx context.Context, tx pgx.Tx, courtID string, dayOfWeek int) (OperatingHour, error) {
	m.LastOperatingHourDayOfWeek = dayOfWeek
	return m.OperatingHour, m.OpHourErr
}

func (m *mockRepo) ListByCustomerID(ctx context.Context, customerID string, limit, offset int) ([]CustomerBooking, int, error) {
	return nil, 0, nil
}

func (m *mockRepo) FindCustomerBookingByID(ctx context.Context, id, customerID string) (CustomerBooking, error) {
	m.findCallCount++
	if m.findCallCount > 1 && m.FindFallback != nil {
		return CustomerBooking{Booking: *m.FindFallback}, m.FindErr
	}
	return CustomerBooking{Booking: m.FindBooking}, m.FindErr
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

func (m *mockRepo) ListOwnerVenueBookings(ctx context.Context, ownerProfileID, venueID, date, status, scope string, limit, offset int) ([]OwnerBooking, int, error) {
	return m.OwnerBookings, 0, m.OwnerBookingsErr
}

func (m *mockRepo) ListOwnerBookings(ctx context.Context, ownerProfileID string, query OwnerBookingsQuery, limit, offset int) ([]OwnerBooking, int, error) {
	return m.OwnerBookings, len(m.OwnerBookings), m.OwnerBookingsErr
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
	m.LastCreateParams = params
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
	svc := NewService(repo, 30, nil)

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
	svc := NewService(repo, 30, nil)

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
	svc := NewService(repo, 30, nil)

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

	svc := NewService(&mockRepo{}, 30, nil)
	req := CreateBookingRequest{BookingDate: yesterday, StartTime: "10:00", EndTime: "12:00"}

	_, err := svc.CreateBooking(context.Background(), "user-1", req)
	if err != ErrPastDate {
		t.Fatalf("expected ErrPastDate, got %v", err)
	}
}

func TestCreateBooking_Fail_InvalidTimeRange(t *testing.T) {
	loc, _ := time.LoadLocation("Asia/Jakarta")
	tomorrow := time.Now().In(loc).AddDate(0, 0, 1).Format("2006-01-02")

	svc := NewService(&mockRepo{}, 30, nil)
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
	svc := NewService(repo, 30, nil)
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
	svc := NewService(repo, 30, nil)

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
	svc := NewService(repo, 30, nil)

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
	svc := NewService(repo, 30, nil)

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
	svc := NewService(repo, 30, nil)

	result, _, err := svc.ListOwnerVenueBookings(context.Background(), "owner-user-1", "venue-1", OwnerVenueBookingsQuery{
		Date:   "2026-06-25",
		Status: "PENDING_PAYMENT",
		Limit:  20,
		Page:   2,
	})
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if len(result) != 1 {
		t.Fatalf("expected 1 booking, got %d", len(result))
	}
	booking := result[0]
	if booking.Customer.Name != "Customer One" || booking.Venue.Name != "Arena A" || booking.Court.Name != "Court 1" {
		t.Fatalf("unexpected owner booking response: %+v", booking)
	}
}

func TestListOwnerVenueBookings_Fail_OwnerProfileNotFound(t *testing.T) {
	repo := &mockRepo{OwnerProfileErr: pgx.ErrNoRows}
	svc := NewService(repo, 30, nil)

	_, _, err := svc.ListOwnerVenueBookings(context.Background(), "owner-user-1", "venue-1", OwnerVenueBookingsQuery{Date: "2026-06-25"})
	if err != ErrOwnerProfileNotFound {
		t.Fatalf("expected ErrOwnerProfileNotFound, got %v", err)
	}
}

func TestListOwnerVenueBookings_Fail_VenueNotFound(t *testing.T) {
	repo := &mockRepo{
		OwnerProfile:  OwnerProfile{ID: "owner-profile-1", UserID: "owner-user-1"},
		OwnerVenueErr: pgx.ErrNoRows,
	}
	svc := NewService(repo, 30, nil)

	_, _, err := svc.ListOwnerVenueBookings(context.Background(), "owner-user-1", "venue-1", OwnerVenueBookingsQuery{Date: "2026-06-25"})
	if err != ErrVenueNotFound {
		t.Fatalf("expected ErrVenueNotFound, got %v", err)
	}
}

func TestCancelBooking_Success(t *testing.T) {
	repo := &mockRepo{
		FindBooking:    Booking{ID: "booking-1", CustomerID: "user-1", Status: "PENDING_PAYMENT"},
		UpdatedBooking: Booking{ID: "booking-1", CustomerID: "user-1", Status: "CANCELLED"},
	}
	svc := NewService(repo, 30, nil)

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
	svc := NewService(repo, 30, nil)

	_, err := svc.CancelBooking(context.Background(), "user-1", "booking-1")
	if err != ErrBookingAlreadyCancelled {
		t.Fatalf("expected ErrBookingAlreadyCancelled, got %v", err)
	}
}

func TestCancelBooking_Fail_CannotCancelPaid(t *testing.T) {
	repo := &mockRepo{
		FindBooking: Booking{ID: "booking-1", CustomerID: "user-1", Status: "PAID"},
	}
	svc := NewService(repo, 30, nil)

	_, err := svc.CancelBooking(context.Background(), "user-1", "booking-1")
	if err != ErrBookingCannotBeCancelled {
		t.Fatalf("expected ErrBookingCannotBeCancelled, got %v", err)
	}
}

func TestCancelBooking_Fail_NotFound(t *testing.T) {
	repo := &mockRepo{
		FindErr: pgx.ErrNoRows,
	}
	svc := NewService(repo, 30, nil)

	_, err := svc.CancelBooking(context.Background(), "user-1", "booking-1")
	if err != ErrBookingNotFound {
		t.Fatalf("expected ErrBookingNotFound, got %v", err)
	}
}

func TestCancelBooking_Fail_CannotCancelConfirmed(t *testing.T) {
	repo := &mockRepo{
		FindBooking: Booking{ID: "booking-1", CustomerID: "user-1", Status: "CONFIRMED"},
	}
	svc := NewService(repo, 30, nil)

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
	svc := NewService(repo, 30, nil)

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
	svc := NewService(repo, 30, nil)

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
	svc := NewService(repo, 30, nil)

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
	svc := NewService(repo, 30, nil)

	_, err := svc.ConfirmBookingPayment(context.Background(), "user-1", "booking-1")
	if err != ErrBookingNotFound {
		t.Fatalf("expected ErrBookingNotFound, got %v", err)
	}
}

func TestConfirmBookingPayment_Fail_AlreadyCancelled(t *testing.T) {
	repo := &mockRepo{
		FindBooking: Booking{ID: "booking-1", CustomerID: "user-1", Status: "CANCELLED"},
	}
	svc := NewService(repo, 30, nil)

	_, err := svc.ConfirmBookingPayment(context.Background(), "user-1", "booking-1")
	if err != ErrBookingAlreadyCancelled {
		t.Fatalf("expected ErrBookingAlreadyCancelled, got %v", err)
	}
}

func TestConfirmBookingPayment_Fail_AlreadyConfirmed(t *testing.T) {
	repo := &mockRepo{
		FindBooking: Booking{ID: "booking-1", CustomerID: "user-1", Status: "CONFIRMED"},
	}
	svc := NewService(repo, 30, nil)

	_, err := svc.ConfirmBookingPayment(context.Background(), "user-1", "booking-1")
	if err != ErrBookingAlreadyConfirmed {
		t.Fatalf("expected ErrBookingAlreadyConfirmed, got %v", err)
	}
}

func TestConfirmBookingPayment_Fail_PaidCannotConfirm(t *testing.T) {
	repo := &mockRepo{
		FindBooking: Booking{ID: "booking-1", CustomerID: "user-1", Status: "PAID"},
	}
	svc := NewService(repo, 30, nil)

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
	svc := NewService(repo, 30, nil)

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
	svc := NewService(repo, 30, nil)

	_, err := svc.ConfirmBookingPayment(context.Background(), "user-1", "booking-1")
	if err != ErrBookingAlreadyConfirmed {
		t.Fatalf("expected ErrBookingAlreadyConfirmed, got %v", err)
	}
}

func TestVerifyPayment_Success(t *testing.T) {
	repo := &mockRepo{
		OwnerProfile:          OwnerProfile{ID: "owner-prof-1"},
		BookingOwnerProfileID: "owner-prof-1",
		UpdatedBooking:        Booking{ID: "booking-1", Status: "CONFIRMED"},
	}
	svc := NewService(repo, 30, nil)

	resp, err := svc.VerifyPayment(context.Background(), "owner-user-1", "booking-1", true)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if resp.Status != "CONFIRMED" {
		t.Fatalf("expected CONFIRMED status, got %s", resp.Status)
	}
}

func TestVerifyPayment_Fail_ErrForbidden(t *testing.T) {
	repo := &mockRepo{
		OwnerProfile:          OwnerProfile{ID: "owner-prof-1"},
		BookingOwnerProfileID: "owner-prof-2",
	}
	svc := NewService(repo, 30, nil)

	_, err := svc.VerifyPayment(context.Background(), "owner-user-1", "booking-1", true)
	if err != ErrForbidden {
		t.Fatalf("expected ErrForbidden, got %v", err)
	}
}

func TestVerifyPayment_Fail_ErrBookingNotFound(t *testing.T) {
	repo := &mockRepo{
		OwnerProfile:             OwnerProfile{ID: "owner-prof-1"},
		BookingOwnerProfileIDErr: pgx.ErrNoRows,
	}
	svc := NewService(repo, 30, nil)

	_, err := svc.VerifyPayment(context.Background(), "owner-user-1", "booking-1", true)
	if err != ErrBookingNotFound {
		t.Fatalf("expected ErrBookingNotFound, got %v", err)
	}
}

func TestVerifyPayment_Fail_ErrOwnerProfileNotFound(t *testing.T) {
	repo := &mockRepo{
		OwnerProfileErr: pgx.ErrNoRows,
	}
	svc := NewService(repo, 30, nil)

	_, err := svc.VerifyPayment(context.Background(), "owner-user-1", "booking-1", true)
	if err != ErrOwnerProfileNotFound {
		t.Fatalf("expected ErrOwnerProfileNotFound, got %v", err)
	}
}

func TestCreateBooking_Success_CustomTTL(t *testing.T) {
	loc, _ := time.LoadLocation("Asia/Jakarta")
	tomorrow := time.Now().In(loc).AddDate(0, 0, 1).Format("2006-01-02")
	start, _ := time.Parse("15:04", "10:00")
	end, _ := time.Parse("15:04", "12:00")

	repo := &mockRepo{
		CourtValidationInfo: CourtValidationInfo{PricePerHour: 150000, CourtStatus: "ACTIVE", VenueStatus: "ACTIVE"},
		OperatingHour:       OperatingHour{IsClosed: false, OpenTime: ptrTime(start.Add(-time.Hour)), CloseTime: ptrTime(end.Add(time.Hour))},
		IsBlocked:           false,
		IsOverlap:           false,
		InsertedBooking:     Booking{ID: "b1"},
	}

	customTTL := 45
	svc := NewService(repo, customTTL, nil)

	req := CreateBookingRequest{
		CourtID:     "court1",
		BookingDate: tomorrow,
		StartTime:   "10:00",
		EndTime:     "12:00",
	}

	_, err := svc.CreateBooking(context.Background(), "cust1", req)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if repo.LastCreateParams.ExpiresAt == nil {
		t.Fatalf("expected ExpiresAt to be set, got nil")
	}

	expectedExpires := time.Now().In(loc).Add(time.Duration(customTTL) * time.Minute)
	diff := repo.LastCreateParams.ExpiresAt.Sub(expectedExpires).Abs()

	if diff > 5*time.Second {
		t.Errorf("ExpiresAt is %v, expected %v (diff: %v)", repo.LastCreateParams.ExpiresAt, expectedExpires, diff)
	}
}

func TestSweepExpiredBookings_Success(t *testing.T) {
	repo := &mockRepo{
		CancelExpiredCount: 5,
	}
	svc := NewService(repo, 30, nil)

	count, err := svc.SweepExpiredBookings(context.Background())
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if count != 5 {
		t.Fatalf("expected 5 updated rows, got %d", count)
	}
}

func TestSweepExpiredBookings_Error(t *testing.T) {
	expectedErr := errors.New("database timeout")
	repo := &mockRepo{
		CancelExpiredErr: expectedErr,
	}
	svc := NewService(repo, 30, nil)

	_, err := svc.SweepExpiredBookings(context.Background())
	if err == nil {
		t.Fatalf("expected error, got nil")
	}
	if err.Error() != expectedErr.Error() {
		t.Fatalf("expected %v, got %v", expectedErr, err)
	}
}

func TestStartExpiryWorker(t *testing.T) {
	repo := &mockRepo{}
	svc := NewService(repo, 30, nil)

	ctx, cancel := context.WithCancel(context.Background())

	done := make(chan struct{})
	go func() {
		svc.StartExpiryWorker(ctx, 10*time.Millisecond)
		close(done)
	}()

	// Wait briefly to allow worker to start
	time.Sleep(50 * time.Millisecond)

	// Cancel context to stop the worker
	cancel()

	// Ensure the worker goroutine exits promptly
	select {
	case <-done:
		// Success
	case <-time.After(1 * time.Second):
		t.Fatalf("worker did not exit promptly after context cancellation")
	}
}

func TestCancelPaidBookingWithRefund_Success(t *testing.T) {
	repo := &mockRepo{
		OwnerProfile:          OwnerProfile{ID: "profile-1", UserID: "user-1"},
		BookingOwnerProfileID: "profile-1",
		UpdatedBooking:        Booking{ID: "booking-1", Status: "CANCELLED"},
	}
	svc := NewService(repo, 30, nil)

	resp, err := svc.CancelPaidBookingWithRefund(context.Background(), "user-1", "booking-1", "Customer requested")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if resp.Status != "CANCELLED" {
		t.Errorf("expected status CANCELLED, got %v", resp.Status)
	}
}

func TestCancelPaidBookingWithRefund_Forbidden(t *testing.T) {
	repo := &mockRepo{
		OwnerProfile:          OwnerProfile{ID: "profile-2", UserID: "user-2"},
		BookingOwnerProfileID: "profile-1",
	}
	svc := NewService(repo, 30, nil)

	_, err := svc.CancelPaidBookingWithRefund(context.Background(), "user-2", "booking-1", "Customer requested")
	if err != ErrForbidden {
		t.Fatalf("expected ErrForbidden, got %v", err)
	}
}

func TestCancelPaidBookingWithRefund_NoIncomeLedger(t *testing.T) {
	repo := &mockRepo{
		OwnerProfile:          OwnerProfile{ID: "profile-1", UserID: "user-1"},
		BookingOwnerProfileID: "profile-1",
		UpdateErr:             ErrBookingIncomeLedgerNotFound,
	}
	svc := NewService(repo, 30, nil)

	_, err := svc.CancelPaidBookingWithRefund(context.Background(), "user-1", "booking-1", "Customer requested")
	if err != ErrBookingIncomeLedgerNotFound {
		t.Fatalf("expected ErrBookingIncomeLedgerNotFound, got %v", err)
	}
}

func TestOwnerCreateOfflineBooking_Success(t *testing.T) {
	tm, _ := time.Parse("15:04", "22:00")
	repo := &mockRepo{
		OwnerProfile: OwnerProfile{ID: "owner-prof-1"},
		OwnerVenue:   OwnerVenue{ID: "venue-1"},
		CourtValidationInfo: CourtValidationInfo{
			CourtStatus: "ACTIVE",
			VenueStatus: "ACTIVE",
		},
		OperatingHour: OperatingHour{
			IsClosed:  false,
			CloseTime: &tm,
		},
		IsBlocked:       false,
		IsOverlap:       false,
		InsertedBooking: Booking{ID: "booking-1", Status: "PAID"},
	}
	svc := NewService(repo, 30, nil)

	req := OwnerCreateOfflineBookingRequest{
		VenueID:      "venue-1",
		CourtID:      "court-1",
		BookingDate:  "2026-10-10",
		StartTime:    "10:00",
		EndTime:      "12:00",
		CustomerName: "Budi",
		TotalPrice:   150000,
		Status:       "PAID",
	}
	resp, err := svc.OwnerCreateOfflineBooking(context.Background(), "owner-user-1", req)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if resp.ID != "booking-1" {
		t.Fatalf("expected booking-1, got %s", resp.ID)
	}
}

func TestOwnerCreateOfflineBooking_Fail_Forbidden(t *testing.T) {
	repo := &mockRepo{
		OwnerProfile:  OwnerProfile{ID: "owner-prof-1"},
		OwnerVenueErr: pgx.ErrNoRows,
	}
	svc := NewService(repo, 30, nil)

	req := OwnerCreateOfflineBookingRequest{
		VenueID:     "venue-2", // not owned
		BookingDate: "2026-10-10",
		StartTime:   "10:00",
		EndTime:     "12:00",
	}
	_, err := svc.OwnerCreateOfflineBooking(context.Background(), "owner-user-1", req)
	if err != ErrForbidden {
		t.Fatalf("expected ErrForbidden, got %v", err)
	}
}

func TestOwnerCreateOfflineBooking_Fail_Overlap(t *testing.T) {
	tm, _ := time.Parse("15:04", "22:00")
	repo := &mockRepo{
		OwnerProfile: OwnerProfile{ID: "owner-prof-1"},
		OwnerVenue:   OwnerVenue{ID: "venue-1"},
		CourtValidationInfo: CourtValidationInfo{
			CourtStatus: "ACTIVE",
			VenueStatus: "ACTIVE",
		},
		OperatingHour: OperatingHour{
			IsClosed:  false,
			CloseTime: &tm,
		},
		IsBlocked: false,
		IsOverlap: true, // Overlap!
	}
	svc := NewService(repo, 30, nil)

	req := OwnerCreateOfflineBookingRequest{
		BookingDate: "2026-10-10",
		StartTime:   "10:00",
		EndTime:     "12:00",
	}
	_, err := svc.OwnerCreateOfflineBooking(context.Background(), "owner-user-1", req)
	if err != ErrOverlapBooking {
		t.Fatalf("expected ErrOverlapBooking, got %v", err)
	}

	// Partial overlap start
	reqPartialStart := OwnerCreateOfflineBookingRequest{
		BookingDate: "2026-10-10",
		StartTime:   "09:00",
		EndTime:     "11:00",
	}
	_, err = svc.OwnerCreateOfflineBooking(context.Background(), "owner-user-1", reqPartialStart)
	if err != ErrOverlapBooking {
		t.Fatalf("expected ErrOverlapBooking for partial overlap start, got %v", err)
	}

	// Partial overlap end
	reqPartialEnd := OwnerCreateOfflineBookingRequest{
		BookingDate: "2026-10-10",
		StartTime:   "11:00",
		EndTime:     "13:00",
	}
	_, err = svc.OwnerCreateOfflineBooking(context.Background(), "owner-user-1", reqPartialEnd)
	if err != ErrOverlapBooking {
		t.Fatalf("expected ErrOverlapBooking for partial overlap end, got %v", err)
	}
}

func TestOwnerCreateOfflineBooking_Fail_OutsideOpHours(t *testing.T) {
	tm, _ := time.Parse("15:04", "22:00")
	repo := &mockRepo{
		OwnerProfile: OwnerProfile{ID: "owner-prof-1"},
		OwnerVenue:   OwnerVenue{ID: "venue-1"},
		CourtValidationInfo: CourtValidationInfo{
			CourtStatus: "ACTIVE",
			VenueStatus: "ACTIVE",
		},
		OperatingHour: OperatingHour{
			IsClosed:  true, // Closed!
			CloseTime: &tm,
		},
	}
	svc := NewService(repo, 30, nil)

	req := OwnerCreateOfflineBookingRequest{
		BookingDate: "2026-10-10",
		StartTime:   "10:00",
		EndTime:     "12:00",
	}
	_, err := svc.OwnerCreateOfflineBooking(context.Background(), "owner-user-1", req)
	if err != ErrOutsideOpHours {
		t.Fatalf("expected ErrOutsideOpHours, got %v", err)
	}
}

func TestOwnerCreateOfflineBooking_SundayUsesDayOfWeekZero(t *testing.T) {
	closeTime, _ := time.Parse("15:04", "22:00")
	openTime, _ := time.Parse("15:04", "08:00")

	repo := &mockRepo{
		OwnerProfile: OwnerProfile{ID: "owner-prof-1"},
		OwnerVenue:   OwnerVenue{ID: "venue-1"},
		CourtValidationInfo: CourtValidationInfo{
			CourtStatus: "ACTIVE",
			VenueStatus: "ACTIVE",
		},
		OperatingHour: OperatingHour{
			IsClosed:  false,
			OpenTime:  &openTime,
			CloseTime: &closeTime,
		},
		IsBlocked:       false,
		IsOverlap:       false,
		InsertedBooking: Booking{ID: "booking-1", Status: "PAID"},
	}
	svc := NewService(repo, 30, nil)

	// 2026-07-05 is a Sunday
	req := OwnerCreateOfflineBookingRequest{
		VenueID:      "venue-1",
		CourtID:      "court-1",
		BookingDate:  "2026-07-05",
		StartTime:    "10:00",
		EndTime:      "12:00",
		CustomerName: "Budi",
		TotalPrice:   150000,
		Status:       "PAID",
	}

	_, err := svc.OwnerCreateOfflineBooking(context.Background(), "owner-user-1", req)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if repo.LastOperatingHourDayOfWeek != 0 {
		t.Fatalf("expected Sunday day_of_week 0, got %d", repo.LastOperatingHourDayOfWeek)
	}
}
