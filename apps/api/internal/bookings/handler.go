package bookings

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
	"lapangango-api/internal/audit"
	"lapangango-api/internal/httputil"
	"lapangango-api/internal/middleware"
)

type Handler struct {
	service      *Service
	auditService audit.Service
}

func NewHandler(service *Service, auditService audit.Service) *Handler {
	return &Handler{service: service, auditService: auditService}
}

func (h *Handler) RegisterRoutes(router *gin.Engine, authMiddleware gin.HandlerFunc, requireActiveUser gin.HandlerFunc, customerRoleMiddleware gin.HandlerFunc) {
	group := router.Group("/bookings", authMiddleware, requireActiveUser, customerRoleMiddleware)
	group.POST("", h.CreateBooking)
	group.GET("", h.ListBookings)
	group.GET("/:id", h.GetBooking)
	group.PATCH("/:id/cancel", h.CancelBooking)
	group.POST("/:id/pay", h.ConfirmBookingPayment)
	group.POST("/:id/payment-proof", h.SubmitPaymentProof)
}

func (h *Handler) RegisterOwnerRoutes(router *gin.Engine, authMiddleware gin.HandlerFunc, requireActiveUser gin.HandlerFunc, ownerWorkspaceMiddleware gin.HandlerFunc) {
	ownerGroup := router.Group("/owner", authMiddleware, requireActiveUser, ownerWorkspaceMiddleware)
	ownerGroup.GET("/bookings", middleware.RequireOwnerPermission("BOOKINGS_READ"), h.ListOwnerBookings)
	ownerGroup.GET("/venues/:id/bookings", middleware.RequireOwnerPermission("BOOKINGS_READ"), h.ListOwnerVenueBookings)
	ownerGroup.PATCH("/bookings/:id/verify-payment", middleware.RequireOwnerPermission("PAYMENT_VERIFY"), h.VerifyPayment)
	ownerGroup.PATCH("/bookings/:id/mark-paid", middleware.RequireOwnerPermission("PAYMENT_VERIFY"), h.MarkBookingPaid)
	ownerGroup.PATCH("/bookings/:id/complete", middleware.RequireOwnerPermission("BOOKINGS_WRITE"), h.CompleteBooking)
	ownerGroup.PATCH("/bookings/:id/cancel-refund", middleware.RequireOwnerPermission("BOOKINGS_WRITE"), h.CancelPaidBookingWithRefund)
	ownerGroup.POST("/bookings/offline", middleware.RequireOwnerPermission("OFFLINE_BOOKINGS_CREATE"), h.OwnerCreateOfflineBooking)
	ownerGroup.GET("/metrics", middleware.RequireOwnerPermission("BOOKINGS_READ"), h.GetOwnerMetrics)
}

func (h *Handler) CreateBooking(c *gin.Context) {
	userID, ok := httputil.GetAuthenticatedUserID(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"message": "Unauthorized"})
		return
	}

	var req CreateBookingRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"message": "Invalid request payload",
			"error":   err.Error(),
		})
		return
	}

	booking, err := h.service.CreateBooking(c.Request.Context(), userID, req)
	if err != nil {
		respondBookingError(c, err, "Failed to create booking")
		return
	}

	c.JSON(http.StatusCreated, gin.H{
		"message": "Booking created successfully",
		"booking": booking,
	})
}

func (h *Handler) ListBookings(c *gin.Context) {
	userID, ok := httputil.GetAuthenticatedUserID(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"message": "Unauthorized"})
		return
	}

	pageParams := httputil.GetPaginationParams(c)

	bookings, total, err := h.service.ListBookings(c.Request.Context(), userID, pageParams.Page, pageParams.Limit)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"message": "Failed to list bookings"})
		return
	}

	c.JSON(http.StatusOK, httputil.NewPaginatedResponse(bookings, total, pageParams.Page, pageParams.Limit))
}

func (h *Handler) GetBooking(c *gin.Context) {
	userID, ok := httputil.GetAuthenticatedUserID(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"message": "Unauthorized"})
		return
	}

	bookingID := c.Param("id")
	if !httputil.IsUUID(bookingID) {
		c.JSON(http.StatusBadRequest, gin.H{"message": "Invalid booking ID format"})
		return
	}

	booking, err := h.service.GetBooking(c.Request.Context(), userID, bookingID)
	if err != nil {
		if errors.Is(err, ErrBookingNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"message": "Booking not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"message": "Failed to get booking"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"booking": booking,
	})
}

func (h *Handler) GetOwnerMetrics(c *gin.Context) {
	ownerCtx, ok := httputil.GetOwnerContext(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"message": "Unauthorized"})
		return
	}

	var req OwnerMetricsQuery
	if err := c.ShouldBindQuery(&req); err != nil {
		respondBookingError(c, err, "Invalid query parameters")
		return
	}

	metrics, err := h.service.GetOwnerMetrics(c.Request.Context(), ownerCtx, req)
	if err != nil {
		respondBookingError(c, err, "Failed to get owner metrics")
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"metrics": metrics,
	})
}

func (h *Handler) ConfirmBookingPayment(c *gin.Context) {
	customerID, ok := httputil.GetAuthenticatedUserID(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"message": "Unauthorized"})
		return
	}

	bookingID := c.Param("id")
	if !httputil.IsUUID(bookingID) {
		c.JSON(http.StatusBadRequest, gin.H{"message": "Invalid booking ID format"})
		return
	}

	resp, err := h.service.ConfirmBookingPayment(c.Request.Context(), customerID, bookingID)
	if err != nil {
		respondBookingError(c, err, "Failed to confirm booking payment")
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "Booking payment confirmed successfully",
		"booking": resp,
	})
}

func (h *Handler) ListOwnerVenueBookings(c *gin.Context) {
	ownerCtx, ok := httputil.GetOwnerContext(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"message": "Unauthorized"})
		return
	}

	venueID := c.Param("id")
	if !httputil.IsUUID(venueID) {
		c.JSON(http.StatusBadRequest, gin.H{"message": "Invalid venue ID format"})
		return
	}

	req := OwnerVenueBookingsQuery{}
	if err := c.ShouldBindQuery(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"message": "Invalid query parameters",
			"error":   err.Error(),
		})
		return
	}

	pageParams := httputil.GetPaginationParams(c)
	req.Page = pageParams.Page
	req.Limit = pageParams.Limit

	bookings, total, err := h.service.ListOwnerVenueBookings(c.Request.Context(), ownerCtx, venueID, req)
	if err != nil {
		respondBookingError(c, err, "Failed to list owner venue bookings")
		return
	}

	c.JSON(http.StatusOK, httputil.NewPaginatedResponse(bookings, total, req.Page, req.Limit))
}

func (h *Handler) CancelBooking(c *gin.Context) {
	userID, ok := httputil.GetAuthenticatedUserID(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"message": "Unauthorized"})
		return
	}

	bookingID := c.Param("id")
	if !httputil.IsUUID(bookingID) {
		c.JSON(http.StatusBadRequest, gin.H{"message": "Invalid booking ID format"})
		return
	}

	booking, err := h.service.CancelBooking(c.Request.Context(), userID, bookingID)
	if err != nil {
		respondBookingError(c, err, "Failed to cancel booking")
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "Booking cancelled successfully",
		"booking": booking,
	})
}

func (h *Handler) SubmitPaymentProof(c *gin.Context) {
	userID, ok := httputil.GetAuthenticatedUserID(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"message": "Unauthorized"})
		return
	}

	bookingID := c.Param("id")
	if !httputil.IsUUID(bookingID) {
		c.JSON(http.StatusBadRequest, gin.H{"message": "Invalid booking ID format"})
		return
	}

	var req SubmitPaymentProofRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"message": "Invalid request payload",
			"error":   err.Error(),
		})
		return
	}

	booking, err := h.service.SubmitPaymentProof(c.Request.Context(), userID, bookingID, req.PaymentReference)
	if err != nil {
		respondBookingError(c, err, "Failed to submit payment proof")
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "Payment proof submitted successfully",
		"booking": booking,
	})
}

func (h *Handler) VerifyPayment(c *gin.Context) {
	ownerCtx, ok := httputil.GetOwnerContext(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"message": "Unauthorized"})
		return
	}

	bookingID := c.Param("id")
	if !httputil.IsUUID(bookingID) {
		c.JSON(http.StatusBadRequest, gin.H{"message": "Invalid booking ID format"})
		return
	}

	var req VerifyPaymentRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"message": "Invalid request payload",
			"error":   err.Error(),
		})
		return
	}

	booking, err := h.service.VerifyPayment(c.Request.Context(), ownerCtx, bookingID, req.IsApproved)
	if err != nil {
		respondBookingError(c, err, "Failed to verify payment")
		return
	}

	action := audit.ActionBookingPaymentRejected
	if req.IsApproved {
		action = audit.ActionBookingPaymentVerified
	}

	ip := c.ClientIP()
	ua := c.Request.UserAgent()
	h.auditService.Record(c.Request.Context(), audit.CreateAuditLogParams{
		OwnerProfileID: ownerCtx.OwnerProfileID,
		ActorUserID:    ownerCtx.ActorUserID,
		ActorRole:      ownerCtx.ActorRole,
		Action:         action,
		EntityType:     audit.EntityBooking,
		EntityID:       &booking.ID,
		Metadata: map[string]any{
			"booking_id":  booking.ID,
			"is_approved": req.IsApproved,
			"new_status":  booking.Status,
		},
		IPAddress: &ip,
		UserAgent: &ua,
	})

	c.JSON(http.StatusOK, gin.H{
		"message": "Payment verified successfully",
		"booking": booking,
	})
}

func (h *Handler) MarkBookingPaid(c *gin.Context) {
	ownerCtx, ok := httputil.GetOwnerContext(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"message": "Unauthorized"})
		return
	}

	bookingID := c.Param("id")
	if !httputil.IsUUID(bookingID) {
		c.JSON(http.StatusBadRequest, gin.H{"message": "Invalid booking ID format"})
		return
	}

	booking, err := h.service.MarkBookingPaid(c.Request.Context(), ownerCtx, bookingID)
	if err != nil {
		respondBookingError(c, err, "Failed to mark booking as paid")
		return
	}

	ip := c.ClientIP()
	ua := c.Request.UserAgent()
	h.auditService.Record(c.Request.Context(), audit.CreateAuditLogParams{
		OwnerProfileID: ownerCtx.OwnerProfileID,
		ActorUserID:    ownerCtx.ActorUserID,
		ActorRole:      ownerCtx.ActorRole,
		Action:         audit.ActionBookingMarkedPaid,
		EntityType:     audit.EntityBooking,
		EntityID:       &booking.ID,
		Metadata: map[string]any{
			"booking_id": booking.ID,
			"new_status": booking.Status,
		},
		IPAddress: &ip,
		UserAgent: &ua,
	})

	c.JSON(http.StatusOK, gin.H{
		"message": "Booking marked as paid successfully",
		"booking": booking,
	})
}

func (h *Handler) CompleteBooking(c *gin.Context) {
	ownerCtx, ok := httputil.GetOwnerContext(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"message": "Unauthorized"})
		return
	}

	bookingID := c.Param("id")
	if !httputil.IsUUID(bookingID) {
		c.JSON(http.StatusBadRequest, gin.H{"message": "Invalid booking ID format"})
		return
	}

	booking, err := h.service.CompleteBooking(c.Request.Context(), ownerCtx, bookingID)
	if err != nil {
		respondBookingError(c, err, "Failed to complete booking")
		return
	}

	ip := c.ClientIP()
	ua := c.Request.UserAgent()
	h.auditService.Record(c.Request.Context(), audit.CreateAuditLogParams{
		OwnerProfileID: ownerCtx.OwnerProfileID,
		ActorUserID:    ownerCtx.ActorUserID,
		ActorRole:      ownerCtx.ActorRole,
		Action:         audit.ActionBookingCompleted,
		EntityType:     audit.EntityBooking,
		EntityID:       &booking.ID,
		Metadata: map[string]any{
			"booking_id": booking.ID,
			"new_status": booking.Status,
		},
		IPAddress: &ip,
		UserAgent: &ua,
	})

	c.JSON(http.StatusOK, gin.H{
		"message": "Booking completed successfully",
		"booking": booking,
	})
}

func respondBookingError(c *gin.Context, err error, fallbackMessage string) {
	switch {
	case errors.Is(err, ErrPastDate),
		errors.Is(err, ErrInvalidTimeRange),
		errors.Is(err, ErrInvalidPrice),
		errors.Is(err, ErrPriceOverrideReasonRequired),
		errors.Is(err, ErrPriceOverrideReasonTooLong),
		errors.Is(err, ErrPromoNotActive),
		errors.Is(err, ErrPromoExpired),
		errors.Is(err, ErrPromoNotStarted),
		errors.Is(err, ErrPromoVenueMismatch):
		c.JSON(http.StatusBadRequest, gin.H{"message": err.Error()})
	case errors.Is(err, ErrCourtInactive), errors.Is(err, ErrVenueInactive), errors.Is(err, ErrOutsideOpHours):
		c.JSON(http.StatusBadRequest, gin.H{"message": err.Error()})
	case errors.Is(err, ErrOverlapBlockedSlot), errors.Is(err, ErrOverlapBooking):
		c.JSON(http.StatusConflict, gin.H{"message": err.Error()})
	case errors.Is(err, ErrBookingCannotBeMarkedPaid), errors.Is(err, ErrBookingCannotBeCompleted):
		c.JSON(http.StatusConflict, gin.H{"message": err.Error()})
	case errors.Is(err, ErrBookingCannotBeRefunded),
		errors.Is(err, ErrBookingRefundAlreadyExists),
		errors.Is(err, ErrBookingIncomeLedgerNotFound):
		c.JSON(http.StatusConflict, gin.H{"message": err.Error()})
	case errors.Is(err, ErrBookingAlreadyCancelled), errors.Is(err, ErrBookingCannotBeCancelled), errors.Is(err, ErrBookingAlreadyConfirmed), errors.Is(err, ErrBookingCannotBeConfirmed):
		c.JSON(http.StatusConflict, gin.H{"message": err.Error()})
	case errors.Is(err, ErrForbidden):
		c.JSON(http.StatusForbidden, gin.H{"message": err.Error()})
	case errors.Is(err, ErrBookingNotFound):
		c.JSON(http.StatusNotFound, gin.H{"message": "Booking not found"})
	case errors.Is(err, ErrCourtNotFound):
		c.JSON(http.StatusNotFound, gin.H{"message": "Court not found"})
	case errors.Is(err, ErrOwnerProfileNotFound):
		c.JSON(http.StatusBadRequest, gin.H{"message": "Owner profile is required before viewing bookings"})
	case errors.Is(err, ErrVenueNotFound):
		c.JSON(http.StatusNotFound, gin.H{"message": "Venue not found"})
	case errors.Is(err, ErrPromoNotFound):
		c.JSON(http.StatusNotFound, gin.H{"message": "Promo not found"})
	default:
		c.JSON(http.StatusInternalServerError, gin.H{"message": "Internal server error"})
	}
}

func (h *Handler) OwnerCreateOfflineBooking(c *gin.Context) {
	ownerCtx, ok := httputil.GetOwnerContext(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"message": "Unauthorized"})
		return
	}

	var req OwnerCreateOfflineBookingRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"message": "Invalid request payload", "error": err.Error()})
		return
	}

	res, err := h.service.OwnerCreateOfflineBooking(c.Request.Context(), ownerCtx, req)
	if err != nil {
		respondBookingError(c, err, "Failed to create offline booking")
		return
	}

	c.JSON(http.StatusCreated, res)
}

func (h *Handler) ListOwnerBookings(c *gin.Context) {
	ownerCtx, ok := httputil.GetOwnerContext(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"message": "Unauthorized"})
		return
	}

	var req OwnerBookingsQuery
	if err := c.ShouldBindQuery(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"message": err.Error()})
		return
	}

	result, err := h.service.ListOwnerBookings(c.Request.Context(), ownerCtx, req)
	if err != nil {
		if errors.Is(err, ErrOwnerProfileNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"message": err.Error()})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"message": err.Error()})
		return
	}

	c.JSON(http.StatusOK, result)
}

func (h *Handler) CancelPaidBookingWithRefund(c *gin.Context) {
	ownerCtx, ok := httputil.GetOwnerContext(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"message": "Unauthorized"})
		return
	}

	bookingID := c.Param("id")
	if !httputil.IsUUID(bookingID) {
		c.JSON(http.StatusBadRequest, gin.H{"message": "Invalid booking ID format"})
		return
	}

	var req OwnerCancelRefundRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"message": "Invalid request payload",
			"error":   err.Error(),
		})
		return
	}

	booking, err := h.service.CancelPaidBookingWithRefund(c.Request.Context(), ownerCtx, bookingID, req.Reason)
	if err != nil {
		respondBookingError(c, err, "Failed to cancel and refund booking")
		return
	}

	ip := c.ClientIP()
	ua := c.Request.UserAgent()
	h.auditService.Record(c.Request.Context(), audit.CreateAuditLogParams{
		OwnerProfileID: ownerCtx.OwnerProfileID,
		ActorUserID:    ownerCtx.ActorUserID,
		ActorRole:      ownerCtx.ActorRole,
		Action:         audit.ActionBookingCancelRefund,
		EntityType:     audit.EntityBooking,
		EntityID:       &booking.ID,
		Metadata: map[string]any{
			"booking_id": booking.ID,
			"reason":     req.Reason,
			"amount":     booking.TotalPrice,
			"venue_id":   booking.Venue.ID,
		},
		IPAddress: &ip,
		UserAgent: &ua,
	})

	c.JSON(http.StatusOK, gin.H{
		"message": "Booking cancelled and refund recorded successfully",
		"booking": booking,
	})
}
