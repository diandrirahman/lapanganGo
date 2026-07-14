package bookings

import (
	"context"
	"errors"
	"log"
	"math"
	"strings"
	"time"

	"lapangango-api/internal/httputil"
	"lapangango-api/internal/notifications"
	"lapangango-api/internal/platformfinance"
	"lapangango-api/internal/promos"

	"github.com/jackc/pgx/v5"
)

var (
	ErrPastDate                        = errors.New("booking date cannot be in the past")
	ErrInvalidTimeRange                = errors.New("start time must be before end time")
	ErrCourtInactive                   = errors.New("court is not active")
	ErrVenueInactive                   = errors.New("venue is not active")
	ErrOutsideOpHours                  = errors.New("booking time is outside court operating hours")
	ErrOverlapBlockedSlot              = errors.New("court is blocked/maintenance during the requested time")
	ErrOverlapBooking                  = errors.New("court is already booked for the requested time")
	ErrBookingNotFound                 = errors.New("booking not found")
	ErrBookingAlreadyCancelled         = errors.New("booking already cancelled")
	ErrBookingCannotBeCancelled        = errors.New("booking cannot be cancelled in current status")
	ErrBookingAlreadyConfirmed         = errors.New("booking already confirmed")
	ErrBookingCannotBeConfirmed        = errors.New("booking cannot be confirmed in current status")
	ErrOwnerProfileNotFound            = errors.New("owner profile not found")
	ErrVenueNotFound                   = errors.New("venue not found")
	ErrForbidden                       = errors.New("forbidden: you do not own this booking's venue")
	ErrBookingCannotBeMarkedPaid       = errors.New("Booking tidak dapat ditandai lunas pada status ini")
	ErrBookingCannotBeCompleted        = errors.New("Gagal menyelesaikan booking. Pastikan jadwal main telah terlewati dan status sudah Lunas.")
	ErrBookingCannotBeRefunded         = errors.New("booking cannot be cancelled/refunded in current status")
	ErrBookingRefundAlreadyExists      = errors.New("refund already recorded for this booking")
	ErrBookingIncomeLedgerNotFound     = errors.New("booking income ledger not found; backfill ledger before refund")
	ErrPromoNotActive                  = errors.New("promo is not active")
	ErrPromoExpired                    = errors.New("promo has expired")
	ErrPromoNotStarted                 = errors.New("promo has not started yet")
	ErrPromoVenueMismatch              = errors.New("promo is not valid for this venue")
	ErrPromoNotFound                   = errors.New("promo not found")
	ErrInvalidPromoCode                = errors.New("promo code is invalid")
	ErrInvalidPromoPrice               = errors.New("final price after promo cannot be less than or equal to 0")
	ErrSnapshotOrchestratorUnavailable = errors.New("booking finance snapshot service is unavailable")
	ErrInvalidPrice                    = errors.New("final price must be greater than zero")
	ErrPriceOverrideReasonRequired     = errors.New("price override reason is required when final price differs from system price")
	ErrPriceOverrideReasonTooLong      = errors.New("price override reason must be at most 500 characters")
)

type BookingRepository interface {
	LockCourtValidationInfo(ctx context.Context, tx pgx.Tx, courtID string) (CourtValidationInfo, error)
	LockOwnerCourtValidationInfo(ctx context.Context, tx pgx.Tx, courtID, venueID, ownerProfileID string) (CourtValidationInfo, error)
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
	InsertOfflineBookingTx(ctx context.Context, tx pgx.Tx, params CreateOfflineBookingParams) (Booking, error)
	CancelPendingByIDAndCustomerID(ctx context.Context, bookingID, customerID string) (Booking, error)
	ConfirmPendingByIDAndCustomerID(ctx context.Context, bookingID, customerID string) (Booking, error)
	GetOwnerMetrics(ctx context.Context, ownerProfileID string, startDate string, endDate string) (OwnerMetrics, error)
	UpdatePaymentReference(ctx context.Context, bookingID, customerID, reference string) (Booking, error)
	VerifyPayment(ctx context.Context, ownerUserID string, bookingID string, isApproved bool) (Booking, error)
	MarkBookingPaid(ctx context.Context, ownerUserID string, bookingID string) (Booking, error)
	CompleteBooking(ctx context.Context, bookingID string) (Booking, error)
	GetOwnerUserIDByCourtID(ctx context.Context, courtID string) (string, error)
	GetOwnerUserIDByBookingID(ctx context.Context, bookingID string) (string, error)
	GetNotifiableUserIDsByCourtID(ctx context.Context, courtID string) ([]string, error)
	GetNotifiableUserIDsByBookingID(ctx context.Context, bookingID string) ([]string, error)
	GetBookingOwnerProfileID(ctx context.Context, bookingID string) (string, error)
	GetBookingOwnerProfileAndVenueID(ctx context.Context, bookingID string) (string, string, error)
	CancelExpiredPendingBookings(ctx context.Context) (int64, error)
	CancelPaidBookingWithRefund(ctx context.Context, ownerUserID string, actorUserID string, bookingID string, reason string) (Booking, error)
	AutoCompleteFinishedBookings(ctx context.Context) ([]Booking, error)
	GetBookingsExpiringSoon(ctx context.Context, cutoff time.Time) ([]Booking, error)
}

type Service struct {
	repository   BookingRepository
	ttlMinutes   int
	notifService notifications.Service
	promosRepo   promos.Repository
	orchestrator SnapshotOrchestrator
}

func NewService(repository BookingRepository, ttlMinutes int, notifService notifications.Service, promosRepo promos.Repository, orchestrator SnapshotOrchestrator) *Service {
	return &Service{
		repository:   repository,
		ttlMinutes:   ttlMinutes,
		notifService: notifService,
		promosRepo:   promosRepo,
		orchestrator: orchestrator,
	}
}

func (s *Service) CreateBooking(ctx context.Context, customerID string, req CreateBookingRequest) (BookingResponse, error) {
	if s.orchestrator == nil {
		return BookingResponse{}, ErrSnapshotOrchestratorUnavailable
	}

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
		// Preserve the legacy price calculation boundary (rounded to cents) before
		// enforcing the new whole-rupiah contract. Without this normalization,
		// binary float noise can turn a mathematically whole price into a false
		// fractional amount (for example, an 11-minute booking at Rp150.000/hour).
		originalPrice := math.Round(durationHours*info.PricePerHour*100) / 100
		finalPrice := originalPrice

		var promoID *string
		var promoCode *string
		var reason string

		if req.PromoCode != nil {
			code := strings.ToUpper(strings.TrimSpace(*req.PromoCode))
			promo, err := s.promosRepo.FindActivePromoByCode(ctx, info.OwnerUserID, code)
			if err != nil {
				if errors.Is(err, pgx.ErrNoRows) {
					return ErrPromoNotFound
				}
				return err
			}

			if err := promos.ValidatePromoRules(promo, info.VenueID, originalPrice, reqDate); err != nil {
				// Translate promo errors to booking errors or use them directly
				return err
			}

			discountAmount := promos.CalculateDiscount(promo, originalPrice)
			finalPrice = originalPrice - discountAmount
			if finalPrice <= 0 {
				return ErrInvalidPromoPrice
			}

			promoID = &promo.ID
			promoCode = &promo.Code
			normalizedPromoCode := strings.ToUpper(strings.TrimSpace(promo.Code))
			if normalizedPromoCode == "" {
				return ErrInvalidPromoCode
			}
			reason = "PROMO:" + normalizedPromoCode
		}

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
		serverEffectiveAt := time.Now().UTC()

		originalPriceInt, err := exactFloat64ToRupiah(originalPrice)
		if err != nil {
			return err
		}

		finalPriceInt, err := exactFloat64ToRupiah(finalPrice)
		if err != nil {
			return err
		}
		adjustmentRupiah := finalPriceInt - originalPriceInt
		discountRupiah := originalPriceInt - finalPriceInt
		if promoID != nil && (adjustmentRupiah >= 0 || discountRupiah <= 0) {
			return ErrInvalidPromoPrice
		}

		reqOrch := SnapshotOrchestrationRequest{
			OwnerProfileID:             info.OwnerProfileID,
			VenueID:                    info.VenueID,
			EffectiveAt:                serverEffectiveAt,
			Channel:                    platformfinance.BookingChannelMarketplaceOnline,
			OriginalPriceRupiah:        originalPriceInt,
			OwnerPriceAdjustmentRupiah: adjustmentRupiah,
			PriceAdjustmentReason:      reason,
		}

		insertCallback := func(ctx context.Context, tx pgx.Tx, pricing CanonicalBookingPricing) (Booking, error) {
			op := float64(pricing.OriginalPriceRupiah)
			fp := float64(pricing.FinalBookingPriceRupiah)
			return s.repository.InsertBooking(ctx, tx, CreateBookingParams{
				CustomerID:     customerID,
				CourtID:        req.CourtID,
				Date:           req.BookingDate,
				StartTime:      req.StartTime,
				EndTime:        req.EndTime,
				OriginalPrice:  &op,
				DiscountAmount: float64(pricing.OriginalPriceRupiah - pricing.FinalBookingPriceRupiah),
				FinalPrice:     &fp,
				PromoID:        promoID,
				PromoCode:      promoCode,
				TotalPrice:     fp,
				ExpiresAt:      &expiresAt,
			})
		}

		b, _, err := s.orchestrator.CreateBookingWithSnapshot(ctx, tx, reqOrch, insertCallback)
		if err != nil {
			return err
		}
		created = b
		return nil
	})

	if err != nil {
		return BookingResponse{}, err
	}

	if s.notifService != nil {
		entityType := notifications.EntityBooking
		entityID := created.ID

		_ = s.notifService.Create(ctx, notifications.CreateNotificationParams{
			UserID:     customerID,
			Type:       notifications.TypeBookingCreated,
			Title:      "Pesanan Berhasil Dibuat",
			Message:    "Silakan unggah bukti pembayaran sebelum batas waktu.",
			EntityType: &entityType,
			EntityID:   &entityID,
		})

		if userIDs, err := s.repository.GetNotifiableUserIDsByCourtID(ctx, req.CourtID); err == nil {
			for _, uid := range userIDs {
				_ = s.notifService.Create(ctx, notifications.CreateNotificationParams{
					UserID:     uid,
					Type:       notifications.TypeBookingCreated,
					Title:      "Pesanan Baru Diterima",
					Message:    "Ada pesanan baru yang menunggu pembayaran.",
					EntityType: &entityType,
					EntityID:   &entityID,
				})
			}
		}
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

	if s.notifService != nil {
		entityType := notifications.EntityBooking
		entityID := b.ID

		_ = s.notifService.Create(ctx, notifications.CreateNotificationParams{
			UserID:     customerID,
			Type:       notifications.TypePaymentProofSubmitted,
			Title:      "Bukti Pembayaran Terkirim",
			Message:    "Bukti pembayaran Anda telah dikirim dan menunggu verifikasi pemilik.",
			EntityType: &entityType,
			EntityID:   &entityID,
		})

		if userIDs, err := s.repository.GetNotifiableUserIDsByBookingID(ctx, bookingID); err == nil {
			for _, uid := range userIDs {
				_ = s.notifService.Create(ctx, notifications.CreateNotificationParams{
					UserID:     uid,
					Type:       notifications.TypePaymentProofSubmitted,
					Title:      "Bukti Pembayaran Baru",
					Message:    "Customer telah mengunggah bukti pembayaran untuk diverifikasi.",
					EntityType: &entityType,
					EntityID:   &entityID,
				})
			}
		}
	}

	return toBookingResponse(b), nil
}

func (s *Service) VerifyPayment(ctx context.Context, ownerCtx httputil.OwnerContext, bookingID string, isApproved bool) (BookingResponse, error) {
	// 1. Ensure the owner actually owns this booking's venue
	ownerProfileID, venueID, err := s.repository.GetBookingOwnerProfileAndVenueID(ctx, bookingID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return BookingResponse{}, ErrBookingNotFound
		}
		return BookingResponse{}, err
	}

	if ownerProfileID != ownerCtx.OwnerProfileID {
		return BookingResponse{}, ErrForbidden
	}

	if !ownerCtx.IsOwner && !containsID(ownerCtx.AllowedVenueIDs, venueID) {
		return BookingResponse{}, ErrForbidden
	}

	b, err := s.repository.VerifyPayment(ctx, ownerCtx.ActorUserID, bookingID, isApproved)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return BookingResponse{}, ErrBookingNotFound
		}
		return BookingResponse{}, err
	}

	if s.notifService != nil {
		entityType := notifications.EntityBooking
		entityID := b.ID
		if isApproved {
			_ = s.notifService.Create(ctx, notifications.CreateNotificationParams{
				UserID:     b.CustomerID,
				Type:       notifications.TypePaymentApproved,
				Title:      "Pembayaran Diterima",
				Message:    "Pembayaran pesanan Anda telah diverifikasi dan pesanan Anda kini aktif.",
				EntityType: &entityType,
				EntityID:   &entityID,
			})
		} else {
			_ = s.notifService.Create(ctx, notifications.CreateNotificationParams{
				UserID:     b.CustomerID,
				Type:       notifications.TypePaymentRejected,
				Title:      "Pembayaran Ditolak",
				Message:    "Pembayaran pesanan Anda ditolak. Silakan periksa kembali atau hubungi pemilik.",
				EntityType: &entityType,
				EntityID:   &entityID,
			})
		}
	}

	return toBookingResponse(b), nil
}

func (s *Service) MarkBookingPaid(ctx context.Context, ownerCtx httputil.OwnerContext, bookingID string) (BookingResponse, error) {
	// 1. Ensure the owner actually owns this booking's venue
	ownerProfileID, venueID, err := s.repository.GetBookingOwnerProfileAndVenueID(ctx, bookingID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return BookingResponse{}, ErrBookingNotFound
		}
		return BookingResponse{}, err
	}

	if ownerProfileID != ownerCtx.OwnerProfileID {
		return BookingResponse{}, ErrForbidden
	}

	if !ownerCtx.IsOwner && !containsID(ownerCtx.AllowedVenueIDs, venueID) {
		return BookingResponse{}, ErrForbidden
	}

	b, err := s.repository.MarkBookingPaid(ctx, ownerCtx.ActorUserID, bookingID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return BookingResponse{}, ErrBookingCannotBeMarkedPaid
		}
		return BookingResponse{}, err
	}
	return toBookingResponse(b), nil
}

func (s *Service) CompleteBooking(ctx context.Context, ownerCtx httputil.OwnerContext, bookingID string) (BookingResponse, error) {
	// 1. Ensure the owner actually owns this booking's venue
	ownerProfileID, venueID, err := s.repository.GetBookingOwnerProfileAndVenueID(ctx, bookingID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return BookingResponse{}, ErrBookingNotFound
		}
		return BookingResponse{}, err
	}

	if ownerProfileID != ownerCtx.OwnerProfileID {
		return BookingResponse{}, ErrForbidden
	}

	if !ownerCtx.IsOwner && !containsID(ownerCtx.AllowedVenueIDs, venueID) {
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

func (s *Service) CancelPaidBookingWithRefund(ctx context.Context, ownerCtx httputil.OwnerContext, bookingID string, reason string) (BookingResponse, error) {
	// 1. Ensure the owner actually owns this booking's venue
	ownerProfileID, venueID, err := s.repository.GetBookingOwnerProfileAndVenueID(ctx, bookingID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return BookingResponse{}, ErrBookingNotFound
		}
		return BookingResponse{}, err
	}

	if ownerProfileID != ownerCtx.OwnerProfileID {
		return BookingResponse{}, ErrForbidden
	}

	if !ownerCtx.IsOwner && !containsID(ownerCtx.AllowedVenueIDs, venueID) {
		return BookingResponse{}, ErrForbidden
	}

	trimmedReason := strings.TrimSpace(reason)

	b, err := s.repository.CancelPaidBookingWithRefund(ctx, ownerCtx.EffectiveOwnerUserID, ownerCtx.ActorUserID, bookingID, trimmedReason)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return BookingResponse{}, ErrBookingNotFound
		}
		return BookingResponse{}, err
	}

	if s.notifService != nil {
		entityType := notifications.EntityBooking
		entityID := b.ID
		if err := s.notifService.Create(ctx, notifications.CreateNotificationParams{
			UserID:     b.CustomerID,
			Type:       notifications.TypeRefundApproved,
			Title:      "Booking Dibatalkan & Refund Diproses",
			Message:    "Booking Anda telah dibatalkan oleh owner dan refund telah dicatat.",
			EntityType: &entityType,
			EntityID:   &entityID,
		}); err != nil {
			log.Printf("Failed to create owner cancel refund notification: %v", err)
		}
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

func (s *Service) ListOwnerVenueBookings(ctx context.Context, ownerCtx httputil.OwnerContext, venueID string, req OwnerVenueBookingsQuery) ([]OwnerBookingResponse, int, error) {
	if !ownerCtx.IsOwner && !containsID(ownerCtx.AllowedVenueIDs, venueID) {
		return []OwnerBookingResponse{}, 0, nil
	}

	query := normalizeOwnerVenueBookingsQuery(req)
	offset := (query.Page - 1) * query.Limit
	bookings, total, err := s.repository.ListOwnerVenueBookings(ctx, ownerCtx.OwnerProfileID, venueID, query.Date, query.Status, query.Scope, query.Limit, offset)
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

func (s *Service) ListOwnerBookings(ctx context.Context, ownerCtx httputil.OwnerContext, req OwnerBookingsQuery) (OwnerBookingsResult, error) {
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

	if !ownerCtx.IsOwner {
		query.AllowedVenueIDs = ownerCtx.AllowedVenueIDs
	}

	offset := (query.Page - 1) * query.Limit
	bookings, total, err := s.repository.ListOwnerBookings(ctx, ownerCtx.OwnerProfileID, query, query.Limit, offset)
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
		OriginalPrice:    b.OriginalPrice,
		DiscountAmount:   b.DiscountAmount,
		FinalPrice:       b.FinalPrice,
		PromoID:          b.PromoID,
		PromoCode:        b.PromoCode,
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
		OriginalPrice:    b.OriginalPrice,
		DiscountAmount:   b.DiscountAmount,
		TotalPrice:       b.TotalPrice,
		PromoID:          b.PromoID,
		PromoCode:        b.PromoCode,
		Status:           b.Status,
		PaymentReference: b.PaymentReference,
		ExpiresAt:        b.ExpiresAt,
		CreatedAt:        b.CreatedAt,
		UpdatedAt:        b.UpdatedAt,
	}
}
func (s *Service) GetOwnerMetrics(ctx context.Context, ownerCtx httputil.OwnerContext, query OwnerMetricsQuery) (OwnerMetricsResponse, error) {
	if !ownerCtx.IsOwner {
		return OwnerMetricsResponse{}, ErrForbidden
	}

	metrics, err := s.repository.GetOwnerMetrics(ctx, ownerCtx.OwnerProfileID, query.StartDate, query.EndDate)
	if err != nil {
		return OwnerMetricsResponse{}, err
	}

	return OwnerMetricsResponse{
		TotalVenues:           metrics.TotalVenues,
		UpcomingBookings:      metrics.UpcomingBookings,
		PendingVerifications:  metrics.PendingVerifications,
		RevenueCurrent:        metrics.RevenueCurrent,
		BookingRevenueCurrent: metrics.BookingRevenueCurrent,
		RefundCurrent:         metrics.RefundCurrent,
		NetRevenueCurrent:     metrics.NetRevenueCurrent,
		RevenueAllTime:        metrics.RevenueAllTime,
		OccupancyRate:         metrics.OccupancyRate,
	}, nil
}

func (s *Service) SweepExpiredBookings(ctx context.Context) (int64, error) {
	return s.repository.CancelExpiredPendingBookings(ctx)
}

func (s *Service) SweepPaymentExpiringSoonNotifications(ctx context.Context) (int64, error) {
	// cutoff is next 5 minutes
	cutoff := time.Now().In(time.UTC).Add(5 * time.Minute)
	bookings, err := s.repository.GetBookingsExpiringSoon(ctx, cutoff)
	if err != nil {
		return 0, err
	}

	if s.notifService != nil {
		entityType := notifications.EntityBooking
		for _, b := range bookings {
			entityID := b.ID
			_ = s.notifService.Create(ctx, notifications.CreateNotificationParams{
				UserID:     b.CustomerID,
				Type:       notifications.TypePaymentExpiringSoon,
				Title:      "Pembayaran hampir habis",
				Message:    "Selesaikan pembayaran sebelum batas waktu agar pesanan tidak dibatalkan.",
				EntityType: &entityType,
				EntityID:   &entityID,
			})
		}
	}

	return int64(len(bookings)), nil
}

func (s *Service) StartExpiryWorker(ctx context.Context, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			// Sweep notifications for expiring soon
			_, err := s.SweepPaymentExpiringSoonNotifications(ctx)
			if err != nil {
				log.Printf("Error sweeping expiring soon notifications: %v", err)
			}
			// Cancel expired bookings
			_, err = s.SweepExpiredBookings(ctx)
			if err != nil {
				log.Printf("Error sweeping expired bookings: %v", err)
			}
		}
	}
}

func (s *Service) SweepCompletedBookings(ctx context.Context) (int64, error) {
	bookings, err := s.repository.AutoCompleteFinishedBookings(ctx)
	if err != nil {
		return 0, err
	}

	if s.notifService != nil {
		entityType := notifications.EntityBooking
		for _, b := range bookings {
			entityID := b.ID
			_ = s.notifService.Create(ctx, notifications.CreateNotificationParams{
				UserID:     b.CustomerID,
				Type:       notifications.TypeBookingCompleted,
				Title:      "Booking Selesai",
				Message:    "Jadwal booking Anda telah selesai. Terima kasih telah menggunakan LapanganGo.",
				EntityType: &entityType,
				EntityID:   &entityID,
			})
		}
	}

	return int64(len(bookings)), nil
}

func (s *Service) StartAutoCompleteWorker(ctx context.Context, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			_, err := s.SweepCompletedBookings(ctx)
			if err != nil {
				log.Printf("Error sweeping completed bookings: %v", err)
			}
		}
	}
}

func (s *Service) OwnerCreateOfflineBooking(ctx context.Context, ownerCtx httputil.OwnerContext, req OwnerCreateOfflineBookingRequest) (BookingResponse, error) {
	if s.orchestrator == nil {
		return BookingResponse{}, ErrSnapshotOrchestratorUnavailable
	}

	// 1. Fetch court & venue info
	_, err := s.repository.FindVenueByIDAndOwnerProfileID(ctx, req.VenueID, ownerCtx.OwnerProfileID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return BookingResponse{}, ErrForbidden
		}
		return BookingResponse{}, err
	}

	if !ownerCtx.IsOwner && !containsID(ownerCtx.AllowedVenueIDs, req.VenueID) {
		return BookingResponse{}, ErrForbidden
	}

	// 3. Time validation
	reqDate, err := time.Parse("2006-01-02", req.BookingDate)
	if err != nil {
		return BookingResponse{}, err
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

	loc, _ := time.LoadLocation("Asia/Jakarta")
	startTz := time.Date(reqDate.Year(), reqDate.Month(), reqDate.Day(), startParsed.Hour(), startParsed.Minute(), 0, 0, loc)
	endTz := time.Date(reqDate.Year(), reqDate.Month(), reqDate.Day(), endParsed.Hour(), endParsed.Minute(), 0, 0, loc)

	var created Booking
	err = s.repository.ExecuteBookingTx(ctx, func(tx pgx.Tx) error {
		info, err := s.repository.LockOwnerCourtValidationInfo(ctx, tx, req.CourtID, req.VenueID, ownerCtx.OwnerProfileID)
		if err != nil {
			return err
		}
		if info.CourtStatus != "ACTIVE" {
			return ErrCourtInactive
		}
		if info.VenueStatus != "ACTIVE" {
			return ErrVenueInactive
		}

		dayOfWeek := int(reqDate.Weekday()) // Sunday = 0
		opHour, err := s.repository.FindOperatingHours(ctx, tx, req.CourtID, dayOfWeek)
		if err != nil {
			return err
		}
		if opHour.IsClosed {
			return ErrOutsideOpHours
		}

		// Check operating hours
		if opHour.OpenTime != nil && opHour.CloseTime != nil {
			reqStartMin := startParsed.Hour()*60 + startParsed.Minute()
			reqEndMin := endParsed.Hour()*60 + endParsed.Minute()

			openMin := opHour.OpenTime.Hour()*60 + opHour.OpenTime.Minute()
			closeMin := opHour.CloseTime.Hour()*60 + opHour.CloseTime.Minute()

			if reqStartMin < openMin || reqEndMin > closeMin {
				return ErrOutsideOpHours
			}
		}

		isBlocked, err := s.repository.CheckBlockedSlots(ctx, tx, req.CourtID, startTz, endTz)
		if err != nil {
			return err
		}
		if isBlocked {
			return ErrOverlapBlockedSlot
		}

		isOverlap, err := s.repository.CheckExistingBookings(ctx, tx, req.CourtID, req.BookingDate, req.StartTime, req.EndTime)
		if err != nil {
			return err
		}
		if isOverlap {
			return ErrOverlapBooking
		}

		durationHours := endTz.Sub(startTz).Hours()
		systemPrice := math.Round(durationHours*info.PricePerHour*100) / 100

		systemPriceRupiah, err := exactFloat64ToRupiah(systemPrice)
		if err != nil {
			return err
		}
		// Owner-supplied money must already be a whole rupiah amount. Do not
		// normalize it first: rounding here would silently accept fractional
		// request values and change the amount selected by the owner/staff actor.
		finalPriceRupiah, err := exactFloat64ToRupiah(req.TotalPrice)
		if err != nil {
			return err
		}

		if finalPriceRupiah <= 0 {
			return ErrInvalidPrice
		}

		adjustmentRupiah := finalPriceRupiah - systemPriceRupiah
		isOverride := adjustmentRupiah != 0
		reason := strings.TrimSpace(req.PriceOverrideReason)
		if isOverride && reason == "" {
			return ErrPriceOverrideReasonRequired
		}
		if isOverride && len([]rune(reason)) > 500 {
			return ErrPriceOverrideReasonTooLong
		}

		reqOrch := SnapshotOrchestrationRequest{
			OwnerProfileID:             info.OwnerProfileID,
			VenueID:                    info.VenueID,
			EffectiveAt:                time.Now().UTC(),
			Channel:                    platformfinance.BookingChannelOwnerWalkIn,
			OriginalPriceRupiah:        systemPriceRupiah,
			OwnerPriceAdjustmentRupiah: adjustmentRupiah,
		}
		if isOverride {
			reqOrch.PriceAdjustmentReason = reason
		}

		b, _, err := s.orchestrator.CreateBookingWithSnapshot(ctx, tx, reqOrch, func(ctx context.Context, tx pgx.Tx, pricing CanonicalBookingPricing) (Booking, error) {
			params := CreateOfflineBookingParams{
				VenueID:         info.VenueID,
				CourtID:         req.CourtID,
				Date:            req.BookingDate,
				StartTime:       req.StartTime,
				EndTime:         req.EndTime,
				SystemPrice:     float64(pricing.OriginalPriceRupiah),
				FinalPrice:      float64(pricing.FinalBookingPriceRupiah),
				Status:          req.Status,
				OwnerUserID:     ownerCtx.EffectiveOwnerUserID,
				CreatedByUserID: ownerCtx.ActorUserID,
				CustomerName:    req.CustomerName,
			}
			if isOverride {
				params.PriceOverrideReason = &reason
			}
			if req.CustomerPhone != "" {
				params.CustomerPhone = &req.CustomerPhone
			}
			if req.CustomerEmail != "" {
				params.CustomerEmail = &req.CustomerEmail
			}
			if req.Note != "" {
				params.Note = &req.Note
			}

			return s.repository.InsertOfflineBookingTx(ctx, tx, params)
		})

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

func containsID(ids []string, id string) bool {
	for _, val := range ids {
		if val == id {
			return true
		}
	}
	return false
}
