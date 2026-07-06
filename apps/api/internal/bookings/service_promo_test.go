package bookings

import (
	"context"
	"errors"
	"testing"
	"time"

	"lapangango-api/internal/promos"
)

type mockPromosRepo struct {
	Promo     promos.Promo
	PromoErr  error
	CallCount int
}

func (m *mockPromosRepo) CreatePromo(ctx context.Context, p promos.Promo) (promos.Promo, error) { return p, nil }
func (m *mockPromosRepo) ListOwnerPromos(ctx context.Context, ownerID string) ([]promos.Promo, error) { return nil, nil }
func (m *mockPromosRepo) GetPromoByIDAndOwner(ctx context.Context, id, ownerID string) (promos.Promo, error) { return m.Promo, nil }
func (m *mockPromosRepo) UpdatePromo(ctx context.Context, id, ownerID string, params promos.UpdatePromoParams) (promos.Promo, error) { return m.Promo, nil }
func (m *mockPromosRepo) FindActivePromoByCode(ctx context.Context, ownerID, code string) (promos.Promo, error) {
	m.CallCount++
	return m.Promo, m.PromoErr
}
func (m *mockPromosRepo) IsVenueOwnedByOwner(ctx context.Context, ownerUserID, venueID string) (bool, error) { return true, nil }
func (m *mockPromosRepo) GetCourtValidationInfo(ctx context.Context, courtID string) (promos.CourtValidationInfo, error) { return promos.CourtValidationInfo{}, nil }
func (m *mockPromosRepo) DeletePromo(ctx context.Context, id, ownerID string) error { return nil }

func ptrString(s string) *string {
	return &s
}

func getBaseTestSetup() (*mockRepo, *mockPromosRepo, *Service, string) {
	loc, _ := time.LoadLocation("Asia/Jakarta")
	tomorrow := time.Now().In(loc).AddDate(0, 0, 1).Format("2006-01-02")

	start, _ := time.Parse("15:04", "10:00")
	end, _ := time.Parse("15:04", "12:00")

	repo := &mockRepo{
		CourtValidationInfo: CourtValidationInfo{PricePerHour: 100000, CourtStatus: "ACTIVE", VenueStatus: "ACTIVE", OwnerUserID: "owner-1"},
		OperatingHour:       OperatingHour{IsClosed: false, OpenTime: ptrTime(start.Add(-time.Hour)), CloseTime: ptrTime(end.Add(time.Hour))},
		IsBlocked:           false,
		IsOverlap:           false,
		InsertedBooking:     Booking{ID: "booking-promo"},
	}

	promosRepo := &mockPromosRepo{}
	svc := NewService(repo, 30, nil, promosRepo)

	return repo, promosRepo, svc, tomorrow
}

func TestCreateBooking_Promo_WithoutPromo(t *testing.T) {
	repo, promosRepo, svc, tomorrow := getBaseTestSetup()

	req := CreateBookingRequest{
		CourtID:     "court-1",
		BookingDate: tomorrow,
		StartTime:   "10:00",
		EndTime:     "12:00",
		PromoCode:   nil,
	}

	resp, err := svc.CreateBooking(context.Background(), "user-1", req)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if promosRepo.CallCount != 0 {
		t.Fatalf("expected FindActivePromoByCode to not be called, got %d", promosRepo.CallCount)
	}
	if resp.TotalPrice != 200000 {
		t.Fatalf("expected total price 200000, got %f", resp.TotalPrice)
	}
	if repo.LastCreateParams.PromoID != nil || repo.LastCreateParams.PromoCode != nil {
		t.Fatalf("expected no promo to be inserted")
	}
	if repo.LastCreateParams.OriginalPrice == nil || *repo.LastCreateParams.OriginalPrice != 200000 {
		t.Fatalf("expected OriginalPrice 200000")
	}
	if repo.LastCreateParams.FinalPrice == nil || *repo.LastCreateParams.FinalPrice != 200000 {
		t.Fatalf("expected FinalPrice 200000")
	}
	if repo.LastCreateParams.DiscountAmount != 0 {
		t.Fatalf("expected DiscountAmount 0, got %f", repo.LastCreateParams.DiscountAmount)
	}
}

func TestCreateBooking_Promo_WithPercentagePromo(t *testing.T) {
	repo, promosRepo, svc, tomorrow := getBaseTestSetup()

	now := time.Now()
	promosRepo.Promo = promos.Promo{
		ID:            "promo-1",
		Code:          "TEST20",
		DiscountType:  "PERCENTAGE",
		DiscountValue: 20.0,
		StartsAt:      now.Add(-time.Hour),
		EndsAt:        now.Add(48 * time.Hour),
		Status:        "ACTIVE",
	}

	req := CreateBookingRequest{
		CourtID:     "court-1",
		BookingDate: tomorrow,
		StartTime:   "10:00",
		EndTime:     "12:00",
		PromoCode:   ptrString("TEST20"),
	}

	resp, err := svc.CreateBooking(context.Background(), "user-1", req)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if promosRepo.CallCount != 1 {
		t.Fatalf("expected FindActivePromoByCode to be called 1 time, got %d", promosRepo.CallCount)
	}
	// 2 hours * 100000 = 200000. 20% discount = 40000. Total = 160000.
	if resp.TotalPrice != 160000 {
		t.Fatalf("expected total price 160000, got %f", resp.TotalPrice)
	}
	if repo.LastCreateParams.OriginalPrice == nil || *repo.LastCreateParams.OriginalPrice != 200000 {
		t.Fatalf("expected OriginalPrice 200000")
	}
	if repo.LastCreateParams.DiscountAmount != 40000 {
		t.Fatalf("expected DiscountAmount 40000, got %f", repo.LastCreateParams.DiscountAmount)
	}
	if repo.LastCreateParams.PromoID == nil || *repo.LastCreateParams.PromoID != "promo-1" {
		t.Fatalf("expected PromoID promo-1")
	}
	if repo.LastCreateParams.PromoCode == nil || *repo.LastCreateParams.PromoCode != "TEST20" {
		t.Fatalf("expected PromoCode TEST20")
	}
}

func TestCreateBooking_Promo_WithFixedPromo(t *testing.T) {
	repo, promosRepo, svc, tomorrow := getBaseTestSetup()

	now := time.Now()
	promosRepo.Promo = promos.Promo{
		ID:            "promo-2",
		Code:          "FIXED50K",
		DiscountType:  "FIXED_AMOUNT",
		DiscountValue: 50000,
		StartsAt:      now.Add(-time.Hour),
		EndsAt:        now.Add(48 * time.Hour),
		Status:        "ACTIVE",
	}

	req := CreateBookingRequest{
		CourtID:     "court-1",
		BookingDate: tomorrow,
		StartTime:   "10:00",
		EndTime:     "12:00",
		PromoCode:   ptrString("FIXED50K"),
	}

	resp, err := svc.CreateBooking(context.Background(), "user-1", req)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if promosRepo.CallCount != 1 {
		t.Fatalf("expected FindActivePromoByCode to be called 1 time, got %d", promosRepo.CallCount)
	}
	// 2 hours * 100000 = 200000. 50000 discount. Total = 150000.
	if resp.TotalPrice != 150000 {
		t.Fatalf("expected total price 150000, got %f", resp.TotalPrice)
	}
	if repo.LastCreateParams.OriginalPrice == nil || *repo.LastCreateParams.OriginalPrice != 200000 {
		t.Fatalf("expected OriginalPrice 200000")
	}
	if repo.LastCreateParams.DiscountAmount != 50000 {
		t.Fatalf("expected DiscountAmount 50000, got %f", repo.LastCreateParams.DiscountAmount)
	}
}

func TestCreateBooking_Promo_InvalidPromo(t *testing.T) {
	_, promosRepo, svc, tomorrow := getBaseTestSetup()

	promosRepo.PromoErr = errors.New("promo not found")

	req := CreateBookingRequest{
		CourtID:     "court-1",
		BookingDate: tomorrow,
		StartTime:   "10:00",
		EndTime:     "12:00",
		PromoCode:   ptrString("INVALID"),
	}

	_, err := svc.CreateBooking(context.Background(), "user-1", req)
	if err == nil {
		t.Fatalf("expected error, got nil")
	}
	if err.Error() != "promo not found" {
		t.Fatalf("expected promo not found error, got %v", err)
	}
}

func TestCreateBooking_Promo_ExpiredPromo(t *testing.T) {
	_, promosRepo, svc, tomorrow := getBaseTestSetup()

	now := time.Now()
	promosRepo.Promo = promos.Promo{
		ID:            "promo-3",
		Code:          "EXPIRED",
		DiscountType:  "FIXED_AMOUNT",
		DiscountValue: 10000,
		StartsAt:      now.Add(-48 * time.Hour),
		EndsAt:        now.Add(-24 * time.Hour), // Expired
		Status:        "ACTIVE",
	}

	req := CreateBookingRequest{
		CourtID:     "court-1",
		BookingDate: tomorrow, // Booking date is tomorrow, definitely expired
		StartTime:   "10:00",
		EndTime:     "12:00",
		PromoCode:   ptrString("EXPIRED"),
	}

	_, err := svc.CreateBooking(context.Background(), "user-1", req)
	if err == nil {
		t.Fatalf("expected error, got nil")
	}
	if err.Error() != "promo has expired" {
		t.Fatalf("expected promo has expired error, got %v", err)
	}
}

func TestCreateBooking_Promo_NotStartedPromo(t *testing.T) {
	_, promosRepo, svc, tomorrow := getBaseTestSetup()

	loc, _ := time.LoadLocation("Asia/Jakarta")
	tomorrowTime, _ := time.ParseInLocation("2006-01-02", tomorrow, loc)

	promosRepo.Promo = promos.Promo{
		ID:            "promo-4",
		Code:          "FUTURE",
		DiscountType:  "FIXED_AMOUNT",
		DiscountValue: 10000,
		StartsAt:      tomorrowTime.Add(48 * time.Hour), // Starts day after tomorrow
		EndsAt:        tomorrowTime.Add(72 * time.Hour), 
		Status:        "ACTIVE",
	}

	req := CreateBookingRequest{
		CourtID:     "court-1",
		BookingDate: tomorrow, 
		StartTime:   "10:00",
		EndTime:     "12:00",
		PromoCode:   ptrString("FUTURE"),
	}

	_, err := svc.CreateBooking(context.Background(), "user-1", req)
	if err == nil {
		t.Fatalf("expected error, got nil")
	}
	if err.Error() != "promo has not started yet" {
		t.Fatalf("expected promo has not started yet error, got %v", err)
	}
}

func TestCreateBooking_Promo_VenueMismatch(t *testing.T) {
	repo, promosRepo, svc, tomorrow := getBaseTestSetup()

	// Court info returns venue "ACTIVE", but let's say venueID is "venue-1"
	repo.CourtValidationInfo.VenueID = "venue-1"

	now := time.Now()
	promosRepo.Promo = promos.Promo{
		ID:            "promo-5",
		Code:          "WRONGVENUE",
		DiscountType:  "FIXED_AMOUNT",
		DiscountValue: 10000,
		StartsAt:      now.Add(-time.Hour),
		EndsAt:        now.Add(48 * time.Hour),
		Status:        "ACTIVE",
		VenueID:       ptrString("venue-2"), // different venue
	}

	req := CreateBookingRequest{
		CourtID:     "court-1",
		BookingDate: tomorrow, 
		StartTime:   "10:00",
		EndTime:     "12:00",
		PromoCode:   ptrString("WRONGVENUE"),
	}

	_, err := svc.CreateBooking(context.Background(), "user-1", req)
	if err == nil {
		t.Fatalf("expected error, got nil")
	}
	if err.Error() != "promo is not valid for this venue" {
		t.Fatalf("expected promo is not valid for this venue error, got %v", err)
	}
}

func TestCreateBooking_Promo_Inactive(t *testing.T) {
	_, promosRepo, svc, tomorrow := getBaseTestSetup()

	now := time.Now()
	promosRepo.Promo = promos.Promo{
		ID:            "promo-6",
		Code:          "INACTIVE",
		DiscountType:  "FIXED_AMOUNT",
		DiscountValue: 10000,
		StartsAt:      now.Add(-time.Hour),
		EndsAt:        now.Add(48 * time.Hour),
		Status:        "INACTIVE", // not active
	}

	req := CreateBookingRequest{
		CourtID:     "court-1",
		BookingDate: tomorrow, 
		StartTime:   "10:00",
		EndTime:     "12:00",
		PromoCode:   ptrString("INACTIVE"),
	}

	_, err := svc.CreateBooking(context.Background(), "user-1", req)
	if err == nil {
		t.Fatalf("expected error, got nil")
	}
	if err.Error() != "promo is not active" {
		t.Fatalf("expected promo is not active error, got %v", err)
	}
}
