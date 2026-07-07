package promos

import (
	"context"
	"errors"
	"testing"
	"time"
)

type mockRepo struct {
	CreatedPromo Promo
	CreateErr    error

	Promos  []Promo
	ListErr error

	Promo  Promo
	GetErr error

	UpdatedPromo Promo
	UpdateErr    error

	ActivePromo Promo
	ActiveErr   error

	IsVenueOwned bool
	VenueErr     error

	CourtInfo CourtValidationInfo
	CourtErr  error

	DeleteErr    error
	DeleteCalled bool

	GetCalls    int
	UpdateCalls int
}

func (m *mockRepo) CreatePromo(ctx context.Context, p Promo) (Promo, error) {
	return m.CreatedPromo, m.CreateErr
}

func (m *mockRepo) ListOwnerPromos(ctx context.Context, ownerID string) ([]Promo, error) {
	return m.Promos, m.ListErr
}

func (m *mockRepo) GetPromoByIDAndOwner(ctx context.Context, id, ownerID string) (Promo, error) {
	m.GetCalls++
	return m.Promo, m.GetErr
}

func (m *mockRepo) UpdatePromo(ctx context.Context, id, ownerID string, params UpdatePromoParams) (Promo, error) {
	m.UpdateCalls++
	return m.UpdatedPromo, m.UpdateErr
}

func (m *mockRepo) FindActivePromoByCode(ctx context.Context, ownerID, code string) (Promo, error) {
	return m.ActivePromo, m.ActiveErr
}

func (m *mockRepo) IsVenueOwnedByOwner(ctx context.Context, ownerUserID, venueID string) (bool, error) {
	return m.IsVenueOwned, m.VenueErr
}

func (m *mockRepo) GetCourtValidationInfo(ctx context.Context, courtID string) (CourtValidationInfo, error) {
	return m.CourtInfo, m.CourtErr
}

func (m *mockRepo) DeletePromo(ctx context.Context, id, ownerID string) error {
	m.DeleteCalled = true
	return m.DeleteErr
}

func TestCalculateDiscount(t *testing.T) {
	tests := []struct {
		name          string
		promo         Promo
		originalPrice float64
		expected      float64
	}{
		{
			name:          "Percentage",
			promo:         Promo{DiscountType: "PERCENTAGE", DiscountValue: 10},
			originalPrice: 150000,
			expected:      15000,
		},
		{
			name:          "Percentage with rounding",
			promo:         Promo{DiscountType: "PERCENTAGE", DiscountValue: 12.5},
			originalPrice: 155000,
			expected:      19375, // 155000 * 0.125
		},
		{
			name:          "Fixed Amount",
			promo:         Promo{DiscountType: "FIXED_AMOUNT", DiscountValue: 25000},
			originalPrice: 150000,
			expected:      25000,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := CalculateDiscount(tt.promo, tt.originalPrice)
			if result != tt.expected {
				t.Errorf("expected %f, got %f", tt.expected, result)
			}
		})
	}
}

func TestValidatePromoRules(t *testing.T) {
	now := time.Now()
	venue1 := "venue-1"
	venue2 := "venue-2"
	tests := []struct {
		name          string
		promo         Promo
		venueID       string
		originalPrice float64
		expectedErr   error
	}{
		{
			name: "Valid Promo",
			promo: Promo{
				Status:        "ACTIVE",
				StartsAt:      now.Add(-1 * time.Hour),
				EndsAt:        now.Add(1 * time.Hour),
				DiscountType:  "FIXED_AMOUNT",
				DiscountValue: 10000,
			},
			venueID:       venue1,
			originalPrice: 50000,
			expectedErr:   nil,
		},
		{
			name: "Inactive Promo",
			promo: Promo{
				Status: "INACTIVE",
			},
			expectedErr: ErrPromoNotActive,
		},
		{
			name: "Venue Mismatch",
			promo: Promo{
				Status:   "ACTIVE",
				VenueID:  &venue1,
				StartsAt: now.Add(-1 * time.Hour),
				EndsAt:   now.Add(1 * time.Hour),
			},
			venueID:     venue2,
			expectedErr: ErrPromoVenueMismatch,
		},
		{
			name: "Not Started",
			promo: Promo{
				Status:   "ACTIVE",
				StartsAt: now.Add(24 * time.Hour),
				EndsAt:   now.Add(48 * time.Hour),
			},
			expectedErr: ErrPromoNotStarted,
		},
		{
			name: "Expired",
			promo: Promo{
				Status:   "ACTIVE",
				StartsAt: now.Add(-48 * time.Hour),
				EndsAt:   now.Add(-24 * time.Hour),
			},
			expectedErr: ErrPromoExpired,
		},
		{
			name: "Invalid Price - Free",
			promo: Promo{
				Status:        "ACTIVE",
				StartsAt:      now.Add(-1 * time.Hour),
				EndsAt:        now.Add(1 * time.Hour),
				DiscountType:  "FIXED_AMOUNT",
				DiscountValue: 50000,
			},
			originalPrice: 50000,
			expectedErr:   ErrInvalidPrice,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidatePromoRules(tt.promo, tt.venueID, tt.originalPrice, now)
			if !errors.Is(err, tt.expectedErr) {
				t.Errorf("expected %v, got %v", tt.expectedErr, err)
			}
		})
	}
}

func TestValidatePromoRulesUsesJakartaBookingDate(t *testing.T) {
	loc, err := time.LoadLocation("Asia/Jakarta")
	if err != nil {
		t.Fatalf("failed to load Asia/Jakarta timezone: %v", err)
	}

	promo := Promo{
		Status:        "ACTIVE",
		StartsAt:      time.Date(2026, time.July, 6, 0, 0, 0, 0, loc).UTC(),
		EndsAt:        time.Date(2026, time.July, 13, 23, 59, 0, 0, loc).UTC(),
		DiscountType:  "PERCENTAGE",
		DiscountValue: 10,
	}

	beforeStartDate := time.Date(2026, time.July, 5, 0, 0, 0, 0, time.UTC)
	if err := ValidatePromoRules(promo, "venue-1", 100000, beforeStartDate); !errors.Is(err, ErrPromoNotStarted) {
		t.Fatalf("expected ErrPromoNotStarted for booking before Jakarta start date, got %v", err)
	}

	startDate := time.Date(2026, time.July, 6, 0, 0, 0, 0, time.UTC)
	if err := ValidatePromoRules(promo, "venue-1", 100000, startDate); err != nil {
		t.Fatalf("expected promo to be valid on Jakarta start date, got %v", err)
	}

	midDate := time.Date(2026, time.July, 7, 14, 0, 0, 0, time.UTC)
	if err := ValidatePromoRules(promo, "venue-1", 100000, midDate); err != nil {
		t.Fatalf("expected promo to be valid within the period, got %v", err)
	}

	endDate := time.Date(2026, time.July, 13, 0, 0, 0, 0, time.UTC)
	if err := ValidatePromoRules(promo, "venue-1", 100000, endDate); err != nil {
		t.Fatalf("expected promo to be valid on Jakarta end date (inclusive), got %v", err)
	}

	afterEndDate := time.Date(2026, time.July, 14, 0, 0, 0, 0, time.UTC)
	if err := ValidatePromoRules(promo, "venue-1", 100000, afterEndDate); !errors.Is(err, ErrPromoExpired) {
		t.Fatalf("expected ErrPromoExpired for booking after Jakarta end date, got %v", err)
	}
}

func TestPromoResponseCanDeleteUsesBookingReferenceCount(t *testing.T) {
	res := toPromoResponse(Promo{
		UsageCount:            0,
		BookingReferenceCount: 1,
	})

	if res.CanDelete {
		t.Fatal("expected promo with any booking reference to be non-deletable")
	}
}

func TestDeletePromoRejectsCancelledBookingReference(t *testing.T) {
	repo := &mockRepo{
		Promo: Promo{
			UsageCount:            0,
			BookingReferenceCount: 1,
		},
	}
	service := NewService(repo)

	err := service.DeletePromo(context.Background(), "promo-1", "owner-1")
	if !errors.Is(err, ErrPromoAlreadyUsed) {
		t.Fatalf("expected ErrPromoAlreadyUsed, got %v", err)
	}
	if repo.DeleteCalled {
		t.Fatal("expected repository delete not to be called")
	}
}

func TestDeletePromoAllowsUnusedPromo(t *testing.T) {
	repo := &mockRepo{
		Promo: Promo{
			UsageCount:            0,
			BookingReferenceCount: 0,
		},
	}
	service := NewService(repo)

	err := service.DeletePromo(context.Background(), "promo-1", "owner-1")
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if !repo.DeleteCalled {
		t.Fatal("expected repository delete to be called")
	}
}

func TestUpdatePromoReturnsRefetchedUsageSummary(t *testing.T) {
	now := time.Now()
	repo := &mockRepo{
		Promo: Promo{
			ID:                    "promo-1",
			Status:                "ACTIVE",
			UsageCount:            2,
			BookingReferenceCount: 2,
			TotalDiscountAmount:   50000,
			TotalFinalRevenue:     250000,
		},
		UpdatedPromo: Promo{ID: "promo-1", Status: "INACTIVE"},
	}
	service := NewService(repo)

	res, err := service.UpdatePromo(context.Background(), "promo-1", "owner-1", CreatePromoRequest{
		Code:          "PROMO10",
		Name:          "Promo 10",
		DiscountType:  "PERCENTAGE",
		DiscountValue: 10,
		StartsAt:      now.Add(-time.Hour).Format(time.RFC3339),
		EndsAt:        now.Add(time.Hour).Format(time.RFC3339),
		Status:        "ACTIVE",
	})
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if repo.UpdateCalls != 1 {
		t.Fatalf("expected one update call, got %d", repo.UpdateCalls)
	}
	if repo.GetCalls != 2 {
		t.Fatalf("expected initial ownership get and refetch, got %d get calls", repo.GetCalls)
	}
	if res.UsageCount != 2 || res.TotalDiscountAmount != 50000 || res.TotalFinalRevenue != 250000 || res.CanDelete {
		t.Fatalf("expected refetched usage summary, got %+v", res)
	}
}

func TestTogglePromoStatusReturnsRefetchedUsageSummary(t *testing.T) {
	repo := &mockRepo{
		Promo: Promo{
			ID:                    "promo-1",
			Status:                "ACTIVE",
			UsageCount:            3,
			BookingReferenceCount: 3,
			TotalDiscountAmount:   75000,
			TotalFinalRevenue:     300000,
		},
		UpdatedPromo: Promo{ID: "promo-1", Status: "INACTIVE"},
	}
	service := NewService(repo)

	res, err := service.TogglePromoStatus(context.Background(), "promo-1", "owner-1")
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if repo.UpdateCalls != 1 {
		t.Fatalf("expected one update call, got %d", repo.UpdateCalls)
	}
	if repo.GetCalls != 2 {
		t.Fatalf("expected initial get and refetch, got %d get calls", repo.GetCalls)
	}
	if res.UsageCount != 3 || res.TotalDiscountAmount != 75000 || res.TotalFinalRevenue != 300000 || res.CanDelete {
		t.Fatalf("expected refetched usage summary, got %+v", res)
	}
}
