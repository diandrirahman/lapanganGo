package refunds

import (
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"lapangango-api/internal/audit"
	"lapangango-api/internal/httputil"
	"lapangango-api/internal/middleware"
)

type Handler struct {
	service      Service
	auditService audit.Service
}

func NewHandler(service Service, auditService audit.Service) *Handler {
	return &Handler{service: service, auditService: auditService}
}

func (h *Handler) RegisterRoutes(router *gin.Engine, authMiddleware gin.HandlerFunc, requireActiveUser gin.HandlerFunc, customerMiddleware gin.HandlerFunc, ownerWorkspaceMiddleware gin.HandlerFunc) {
	customerGroup := router.Group("/bookings")
	customerGroup.Use(authMiddleware, requireActiveUser, customerMiddleware)
	customerGroup.POST("/:id/refund-request", h.RequestBookingRefund)
	customerGroup.GET("/:id/refund-request", h.GetRefundRequestByBooking)

	ownerGroup := router.Group("/owner/refund-requests")
	ownerGroup.Use(authMiddleware, requireActiveUser, ownerWorkspaceMiddleware)
	ownerGroup.GET("", middleware.RequireOwnerPermission("REFUNDS_READ"), h.ListOwnerRefundRequests)
	ownerGroup.PATCH("/:id/approve", middleware.RequireOwnerPermission("REFUNDS_WRITE"), h.ApproveRefundRequest)
	ownerGroup.PATCH("/:id/reject", middleware.RequireOwnerPermission("REFUNDS_WRITE"), h.RejectRefundRequest)
}

func (h *Handler) RequestBookingRefund(c *gin.Context) {
	customerID, exists := httputil.GetAuthenticatedUserID(c)
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"message": "unauthorized"})
		return
	}

	bookingID := c.Param("id")
	if bookingID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"message": "booking ID required"})
		return
	}

	var req CreateRefundRequestRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"message": "invalid request payload", "error": err.Error()})
		return
	}

	res, err := h.service.RequestBookingRefund(c.Request.Context(), customerID, bookingID, req.Reason)
	if err != nil {
		switch {
		case errors.Is(err, ErrInvalidReason):
			c.JSON(http.StatusBadRequest, gin.H{"message": err.Error()})
		case errors.Is(err, ErrForbidden):
			c.JSON(http.StatusForbidden, gin.H{"message": err.Error()})
		case errors.Is(err, ErrRefundRequestNotAllowed), errors.Is(err, ErrBookingRefundCutoffExceeded), errors.Is(err, ErrRefundRequestAlreadyExists):
			c.JSON(http.StatusConflict, gin.H{"message": err.Error()})
		case strings.Contains(err.Error(), "not found"):
			c.JSON(http.StatusNotFound, gin.H{"message": err.Error()})
		default:
			c.JSON(http.StatusInternalServerError, gin.H{"message": err.Error()})
		}
		return
	}

	c.JSON(http.StatusCreated, gin.H{
		"message":        "Refund request submitted successfully",
		"refund_request": res,
	})
}

func (h *Handler) GetRefundRequestByBooking(c *gin.Context) {
	customerID, exists := httputil.GetAuthenticatedUserID(c)
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"message": "unauthorized"})
		return
	}

	bookingID := c.Param("id")
	if bookingID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"message": "booking ID required"})
		return
	}

	req, err := h.service.GetRefundRequestByBooking(c.Request.Context(), customerID, bookingID)
	if err != nil {
		if errors.Is(err, ErrForbidden) {
			c.JSON(http.StatusForbidden, gin.H{"message": err.Error()})
		} else if strings.Contains(err.Error(), "not found") {
			c.JSON(http.StatusNotFound, gin.H{"message": err.Error()})
		} else {
			c.JSON(http.StatusInternalServerError, gin.H{"message": err.Error()})
		}
		return
	}

	if req == nil {
		c.JSON(http.StatusOK, gin.H{"data": nil})
		return
	}

	c.JSON(http.StatusOK, gin.H{"data": req})
}

func (h *Handler) ListOwnerRefundRequests(c *gin.Context) {
	ownerCtx, exists := httputil.GetOwnerContext(c)
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"message": "unauthorized"})
		return
	}

	status := c.Query("status")
	venueID := c.Query("venue_id")
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "10"))

	res, err := h.service.ListOwnerRefundRequests(c.Request.Context(), ownerCtx, status, venueID, page, limit)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"message": err.Error()})
		return
	}

	c.JSON(http.StatusOK, res)
}

func (h *Handler) ApproveRefundRequest(c *gin.Context) {
	ownerCtx, exists := httputil.GetOwnerContext(c)
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"message": "unauthorized"})
		return
	}

	requestID := c.Param("id")
	if requestID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"message": "request ID required"})
		return
	}

	var req ApproveRefundRequestRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		// Allow empty body
	}

	res, err := h.service.ApproveRefundRequest(c.Request.Context(), ownerCtx, requestID, req.OwnerNote)
	if err != nil {
		switch {
		case errors.Is(err, ErrForbidden):
			c.JSON(http.StatusForbidden, gin.H{"message": err.Error()})
		case errors.Is(err, ErrRefundRequestNotFound):
			c.JSON(http.StatusNotFound, gin.H{"message": err.Error()})
		case errors.Is(err, ErrRefundRequestAlreadyReviewed), errors.Is(err, ErrRefundRequestNotAllowed), errors.Is(err, ErrBookingRefundAlreadyExists), errors.Is(err, ErrBookingIncomeLedgerNotFound):
			c.JSON(http.StatusConflict, gin.H{"message": err.Error()})
		default:
			c.JSON(http.StatusInternalServerError, gin.H{"message": err.Error()})
		}
		return
	}

	ip := c.ClientIP()
	ua := c.Request.UserAgent()
	h.auditService.Record(c.Request.Context(), audit.CreateAuditLogParams{
		OwnerProfileID: ownerCtx.OwnerProfileID,
		ActorUserID:    ownerCtx.ActorUserID,
		ActorRole:      ownerCtx.ActorRole,
		Action:         audit.ActionRefundApproved,
		EntityType:     audit.EntityRefund,
		EntityID:       &res.ID,
		Metadata: map[string]any{
			"refund_request_id": res.ID,
			"booking_id":        res.BookingID,
			"owner_note":        req.OwnerNote,
		},
		IPAddress: &ip,
		UserAgent: &ua,
	})

	c.JSON(http.StatusOK, gin.H{
		"message":        "Refund request approved successfully",
		"refund_request": res,
		"booking": gin.H{
			"id":     res.BookingID,
			"status": "CANCELLED",
		},
	})
}

func (h *Handler) RejectRefundRequest(c *gin.Context) {
	ownerCtx, exists := httputil.GetOwnerContext(c)
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"message": "unauthorized"})
		return
	}

	requestID := c.Param("id")
	if requestID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"message": "request ID required"})
		return
	}

	var req RejectRefundRequestRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		// Allow empty body
	}

	res, err := h.service.RejectRefundRequest(c.Request.Context(), ownerCtx, requestID, req.OwnerNote)
	if err != nil {
		switch {
		case errors.Is(err, ErrForbidden):
			c.JSON(http.StatusForbidden, gin.H{"message": err.Error()})
		case errors.Is(err, ErrRefundRequestNotFound):
			c.JSON(http.StatusNotFound, gin.H{"message": err.Error()})
		case errors.Is(err, ErrRefundRequestAlreadyReviewed):
			c.JSON(http.StatusConflict, gin.H{"message": err.Error()})
		default:
			c.JSON(http.StatusInternalServerError, gin.H{"message": err.Error()})
		}
		return
	}

	ip := c.ClientIP()
	ua := c.Request.UserAgent()
	h.auditService.Record(c.Request.Context(), audit.CreateAuditLogParams{
		OwnerProfileID: ownerCtx.OwnerProfileID,
		ActorUserID:    ownerCtx.ActorUserID,
		ActorRole:      ownerCtx.ActorRole,
		Action:         audit.ActionRefundRejected,
		EntityType:     audit.EntityRefund,
		EntityID:       &res.ID,
		Metadata: map[string]any{
			"refund_request_id": res.ID,
			"booking_id":        res.BookingID,
			"owner_note":        req.OwnerNote,
		},
		IPAddress: &ip,
		UserAgent: &ua,
	})

	c.JSON(http.StatusOK, gin.H{
		"message":        "Refund request rejected successfully",
		"refund_request": res,
	})
}
