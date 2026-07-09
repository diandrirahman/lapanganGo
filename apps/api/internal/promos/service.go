package promos

import (
	"context"
	"errors"
	"math"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"lapangango-api/internal/httputil"
)

var (
	ErrPromoNotFound       = errors.New("promo not found")
	ErrCodeExists          = errors.New("promo code already exists")
	ErrInvalidDiscount     = errors.New("invalid discount value")
	ErrInvalidPeriod       = errors.New("end time must be after start time")
	ErrPromoNotActive      = errors.New("promo is not active")
	ErrPromoExpired        = errors.New("promo has expired")
	ErrPromoNotStarted     = errors.New("promo has not started yet")
	ErrPromoVenueMismatch  = errors.New("promo is not valid for this venue")
	ErrPromoVenueForbidden = errors.New("venue does not belong to owner")
	ErrInvalidPrice        = errors.New("final price cannot be less than or equal to 0")
	ErrInvalidBookingDate  = errors.New("invalid booking date")
	ErrPromoAlreadyUsed    = errors.New("promo has already been used and cannot be deleted")
	ErrInvalidPromoCode    = errors.New("promo code must use letters/numbers without spaces")
)

var promoBusinessLocation = func() *time.Location {
	loc, err := time.LoadLocation("Asia/Jakarta")
	if err != nil {
		return time.FixedZone("Asia/Jakarta", 7*60*60)
	}
	return loc
}()

func dateOnly(t time.Time) time.Time {
	t = t.In(promoBusinessLocation)
	y, m, d := t.Date()
	return time.Date(y, m, d, 0, 0, 0, 0, promoBusinessLocation)
}

type Service struct {
	repo Repository
}

func NewService(repo Repository) *Service {
	return &Service{repo: repo}
}

func (s *Service) CreatePromo(ctx context.Context, ownerCtx httputil.OwnerContext, req CreatePromoRequest) (PromoResponse, error) {
	code := strings.ToUpper(strings.TrimSpace(req.Code))
	if strings.ContainsAny(code, " \t\n\r") {
		return PromoResponse{}, ErrInvalidPromoCode
	}
	startsAt, err := time.Parse(time.RFC3339, req.StartsAt)
	if err != nil {
		return PromoResponse{}, err
	}
	endsAt, err := time.Parse(time.RFC3339, req.EndsAt)
	if err != nil {
		return PromoResponse{}, err
	}
	if !endsAt.After(startsAt) {
		return PromoResponse{}, ErrInvalidPeriod
	}

	if req.DiscountType == "PERCENTAGE" && (req.DiscountValue <= 0 || req.DiscountValue >= 100) {
		return PromoResponse{}, ErrInvalidDiscount
	}
	if req.DiscountType == "FIXED_AMOUNT" && req.DiscountValue <= 0 {
		return PromoResponse{}, ErrInvalidDiscount
	}

	if req.VenueID != nil {
		isOwned, err := s.repo.IsVenueOwnedByOwner(ctx, ownerCtx.EffectiveOwnerUserID, *req.VenueID)
		if err != nil {
			return PromoResponse{}, err
		}
		if !isOwned {
			return PromoResponse{}, ErrPromoVenueForbidden
		}
		if !ownerCtx.IsOwner && !containsID(ownerCtx.AllowedVenueIDs, *req.VenueID) {
			return PromoResponse{}, ErrPromoVenueForbidden
		}
	} else if !ownerCtx.IsOwner {
		return PromoResponse{}, ErrPromoVenueForbidden
	}

	// Check existing code
	_, err = s.repo.FindActivePromoByCode(ctx, ownerCtx.EffectiveOwnerUserID, code)
	if err == nil {
		// Actually we should check if any promo exists with this code, but repo doesn't have FindPromoByCode for all status.
		// Wait, unique index will enforce it anyway. Let's just rely on DB unique constraint.
	} else if !errors.Is(err, pgx.ErrNoRows) {
		return PromoResponse{}, err
	}

	status := req.Status
	if status == "" {
		status = "ACTIVE"
	}

	var desc *string
	if req.Description != "" {
		desc = &req.Description
	}

	p := Promo{
		OwnerID:       ownerCtx.EffectiveOwnerUserID,
		VenueID:       req.VenueID,
		Code:          code,
		Name:          req.Name,
		Description:   desc,
		DiscountType:  req.DiscountType,
		DiscountValue: req.DiscountValue,
		StartsAt:      startsAt,
		EndsAt:        endsAt,
		Status:        status,
	}

	created, err := s.repo.CreatePromo(ctx, p)
	if err != nil {
		if strings.Contains(err.Error(), "idx_owner_promos_owner_code") {
			return PromoResponse{}, ErrCodeExists
		}
		return PromoResponse{}, err
	}
	return toPromoResponse(created), nil
}

func containsID(ids []string, id string) bool {
	for _, v := range ids {
		if v == id {
			return true
		}
	}
	return false
}

func toPromoResponses(promos []Promo) []PromoResponse {
	res := make([]PromoResponse, len(promos))
	for i, p := range promos {
		res[i] = toPromoResponse(p)
	}
	return res
}

func (s *Service) ListOwnerPromos(ctx context.Context, ownerCtx httputil.OwnerContext) ([]PromoResponse, error) {
	promos, err := s.repo.ListOwnerPromos(ctx, ownerCtx.EffectiveOwnerUserID)
	if err != nil {
		return nil, err
	}

	var allowedPromos []Promo
	for _, p := range promos {
		if !ownerCtx.IsOwner && p.VenueID != nil {
			if !containsID(ownerCtx.AllowedVenueIDs, *p.VenueID) {
				continue
			}
		} else if !ownerCtx.IsOwner && p.VenueID == nil {
			continue // staff can't see global promos
		}
		allowedPromos = append(allowedPromos, p)
	}

	return toPromoResponses(allowedPromos), nil
}

func (s *Service) GetPromo(ctx context.Context, id string, ownerCtx httputil.OwnerContext) (PromoResponse, error) {
	promo, err := s.repo.GetPromoByIDAndOwner(ctx, id, ownerCtx.EffectiveOwnerUserID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return PromoResponse{}, ErrPromoNotFound
		}
		return PromoResponse{}, err
	}

	if !ownerCtx.IsOwner {
		if promo.VenueID == nil {
			return PromoResponse{}, ErrPromoNotFound
		}
		if !containsID(ownerCtx.AllowedVenueIDs, *promo.VenueID) {
			return PromoResponse{}, ErrPromoNotFound
		}
	}

	return toPromoResponse(promo), nil
}

func (s *Service) UpdatePromo(ctx context.Context, id string, ownerCtx httputil.OwnerContext, req CreatePromoRequest) (PromoResponse, error) {
	promo, err := s.repo.GetPromoByIDAndOwner(ctx, id, ownerCtx.EffectiveOwnerUserID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return PromoResponse{}, ErrPromoNotFound
		}
		return PromoResponse{}, err
	}

	if !ownerCtx.IsOwner {
		if promo.VenueID == nil {
			return PromoResponse{}, ErrPromoVenueForbidden
		}
		if !containsID(ownerCtx.AllowedVenueIDs, *promo.VenueID) {
			return PromoResponse{}, ErrPromoVenueForbidden
		}
	}

	startsAt, err := time.Parse(time.RFC3339, req.StartsAt)
	if err != nil {
		return PromoResponse{}, err
	}
	endsAt, err := time.Parse(time.RFC3339, req.EndsAt)
	if err != nil {
		return PromoResponse{}, err
	}
	if !endsAt.After(startsAt) {
		return PromoResponse{}, ErrInvalidPeriod
	}

	if req.DiscountType == "PERCENTAGE" && (req.DiscountValue <= 0 || req.DiscountValue >= 100) {
		return PromoResponse{}, ErrInvalidDiscount
	}
	if req.DiscountType == "FIXED_AMOUNT" && req.DiscountValue <= 0 {
		return PromoResponse{}, ErrInvalidDiscount
	}

	var desc *string
	if req.Description != "" {
		desc = &req.Description
	}

	params := UpdatePromoParams{
		Name:          &req.Name,
		Description:   desc,
		DiscountType:  &req.DiscountType,
		DiscountValue: &req.DiscountValue,
		StartsAt:      &startsAt,
		EndsAt:        &endsAt,
		Status:        &req.Status,
	}
	if req.Status == "" {
		params.Status = nil
	}

	_, err = s.repo.UpdatePromo(ctx, id, ownerCtx.EffectiveOwnerUserID, params)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return PromoResponse{}, ErrPromoNotFound
		}
		return PromoResponse{}, err
	}
	return s.GetPromo(ctx, id, ownerCtx)
}

func (s *Service) TogglePromoStatus(ctx context.Context, id string, ownerCtx httputil.OwnerContext) (PromoResponse, error) {
	promo, err := s.repo.GetPromoByIDAndOwner(ctx, id, ownerCtx.EffectiveOwnerUserID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return PromoResponse{}, ErrPromoNotFound
		}
		return PromoResponse{}, err
	}

	if !ownerCtx.IsOwner {
		if promo.VenueID == nil {
			return PromoResponse{}, ErrPromoNotFound
		}
		if !containsID(ownerCtx.AllowedVenueIDs, *promo.VenueID) {
			return PromoResponse{}, ErrPromoNotFound
		}
	}

	newStatus := "ACTIVE"
	if promo.Status == "ACTIVE" {
		newStatus = "INACTIVE"
	}

	_, err = s.repo.UpdatePromo(ctx, id, ownerCtx.EffectiveOwnerUserID, UpdatePromoParams{
		Status: &newStatus,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return PromoResponse{}, ErrPromoNotFound
		}
		return PromoResponse{}, err
	}
	return s.GetPromo(ctx, id, ownerCtx)
}

func toPromoResponse(p Promo) PromoResponse {
	return PromoResponse{
		ID:                  p.ID,
		OwnerID:             p.OwnerID,
		VenueID:             p.VenueID,
		Code:                p.Code,
		Name:                p.Name,
		Description:         p.Description,
		DiscountType:        p.DiscountType,
		DiscountValue:       p.DiscountValue,
		StartsAt:            p.StartsAt,
		EndsAt:              p.EndsAt,
		Status:              p.Status,
		CreatedAt:           p.CreatedAt,
		UpdatedAt:           p.UpdatedAt,
		UsageCount:          p.UsageCount,
		TotalDiscountAmount: p.TotalDiscountAmount,
		TotalFinalRevenue:   p.TotalFinalRevenue,
		CanDelete:           p.BookingReferenceCount == 0,
	}
}

func roundMoney(v float64) float64 {
	return math.Round(v*100) / 100
}

func CalculateDiscount(promo Promo, originalPrice float64) float64 {
	if promo.DiscountType == "PERCENTAGE" {
		return roundMoney(originalPrice * promo.DiscountValue / 100)
	}
	if promo.DiscountType == "FIXED_AMOUNT" {
		return roundMoney(promo.DiscountValue)
	}
	return 0
}

func ValidatePromoRules(promo Promo, venueID string, originalPrice float64, bookingDate time.Time) error {
	if promo.Status != "ACTIVE" {
		return ErrPromoNotActive
	}
	if promo.VenueID != nil && *promo.VenueID != venueID {
		return ErrPromoVenueMismatch
	}

	promoStartDate := dateOnly(promo.StartsAt)
	promoEndDate := dateOnly(promo.EndsAt)
	bookingDateOnly := dateOnly(bookingDate)

	if bookingDateOnly.Before(promoStartDate) {
		return ErrPromoNotStarted
	}
	if bookingDateOnly.After(promoEndDate) {
		return ErrPromoExpired
	}

	discountAmount := CalculateDiscount(promo, originalPrice)
	finalPrice := originalPrice - discountAmount

	if finalPrice <= 0 {
		return ErrInvalidPrice
	}

	return nil
}

func (s *Service) ValidatePromo(ctx context.Context, req ValidatePromoRequest) (ValidatePromoResponse, error) {
	info, err := s.repo.GetCourtValidationInfo(ctx, req.CourtID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ValidatePromoResponse{}, errors.New("court not found")
		}
		return ValidatePromoResponse{}, err
	}
	if info.VenueID != req.VenueID {
		return ValidatePromoResponse{}, errors.New("court does not belong to the requested venue")
	}

	start, err := time.Parse("15:04", req.StartTime)
	if err != nil {
		return ValidatePromoResponse{}, err
	}
	end, err := time.Parse("15:04", req.EndTime)
	if err != nil {
		return ValidatePromoResponse{}, err
	}
	hours := end.Sub(start).Hours()
	if hours <= 0 {
		return ValidatePromoResponse{}, errors.New("invalid time range")
	}

	originalPrice := info.PricePerHour * hours

	promo, err := s.repo.FindActivePromoByCode(ctx, info.OwnerUserID, strings.ToUpper(strings.TrimSpace(req.PromoCode)))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ValidatePromoResponse{}, ErrPromoNotFound
		}
		return ValidatePromoResponse{}, err
	}

	if promo.Status != "ACTIVE" {
		return ValidatePromoResponse{}, ErrPromoNotActive
	}

	bookingDate, err := time.Parse("2006-01-02", req.BookingDate)
	if err != nil {
		return ValidatePromoResponse{}, ErrInvalidBookingDate
	}

	promoStartDate := dateOnly(promo.StartsAt)
	promoEndDate := dateOnly(promo.EndsAt)
	bookingDateOnly := dateOnly(bookingDate)

	if bookingDateOnly.Before(promoStartDate) {
		return ValidatePromoResponse{}, ErrPromoNotStarted
	}
	if bookingDateOnly.After(promoEndDate) {
		return ValidatePromoResponse{}, ErrPromoExpired
	}

	if promo.VenueID != nil && *promo.VenueID != req.VenueID {
		return ValidatePromoResponse{}, ErrPromoVenueMismatch
	}

	var discountAmount float64
	if promo.DiscountType == "PERCENTAGE" {
		discountAmount = math.Round(originalPrice * promo.DiscountValue / 100)
	} else if promo.DiscountType == "FIXED_AMOUNT" {
		discountAmount = promo.DiscountValue
	}

	if discountAmount > originalPrice {
		discountAmount = originalPrice
	}

	finalPrice := originalPrice - discountAmount
	if finalPrice <= 0 {
		return ValidatePromoResponse{}, ErrInvalidPrice
	}

	return ValidatePromoResponse{
		PromoID:        promo.ID,
		PromoCode:      promo.Code,
		PromoName:      promo.Name,
		OriginalPrice:  originalPrice,
		DiscountAmount: discountAmount,
		FinalPrice:     finalPrice,
	}, nil
}

func (s *Service) DeletePromo(ctx context.Context, id string, ownerCtx httputil.OwnerContext) error {
	promo, err := s.repo.GetPromoByIDAndOwner(ctx, id, ownerCtx.EffectiveOwnerUserID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrPromoNotFound
		}
		return err
	}

	if !ownerCtx.IsOwner {
		if promo.VenueID == nil {
			return ErrPromoNotFound
		}
		if !containsID(ownerCtx.AllowedVenueIDs, *promo.VenueID) {
			return ErrPromoNotFound
		}
	}

	if promo.BookingReferenceCount > 0 {
		return ErrPromoAlreadyUsed
	}

	err = s.repo.DeletePromo(ctx, id, ownerCtx.EffectiveOwnerUserID)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrPromoNotFound
	}
	return err
}
