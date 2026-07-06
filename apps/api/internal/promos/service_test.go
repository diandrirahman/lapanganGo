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
}

func (m *mockRepo) CreatePromo(ctx context.Context, p Promo) (Promo, error) {
	return m.CreatedPromo, m.CreateErr
}

func (m *mockRepo) ListOwnerPromos(ctx context.Context, ownerID string) ([]Promo, error) {
	return m.Promos, m.ListErr
}

func (m *mockRepo) GetPromoByIDAndOwner(ctx context.Context, id, ownerID string) (Promo, error) {
	return m.Promo, m.GetErr
}

func (m *mockRepo) UpdatePromo(ctx context.Context, id, ownerID string, params UpdatePromoParams) (Promo, error) {
	return m.UpdatedPromo, m.UpdateErr
}

func (m *mockRepo) FindActivePromoByCode(ctx context.Context, ownerID, code string) (Promo, error) {
	return m.ActivePromo, m.ActiveErr
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
