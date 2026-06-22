package bookings

import (
	"errors"
	"net/http"

	"regexp"

	"github.com/gin-gonic/gin"
)

var uuidRegex = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)

func isValidUUID(u string) bool {
	return uuidRegex.MatchString(u)
}

type Handler struct {
	service *Service
}

func NewHandler(service *Service) *Handler {
	return &Handler{service: service}
}

func (h *Handler) RegisterRoutes(router *gin.Engine, authMiddleware gin.HandlerFunc, customerRoleMiddleware gin.HandlerFunc) {
	group := router.Group("/bookings", authMiddleware, customerRoleMiddleware)
	group.POST("", h.CreateBooking)
	group.GET("", h.ListBookings)
	group.GET("/:id", h.GetBooking)
	group.PATCH("/:id/cancel", h.CancelBooking)
	group.POST("/:id/pay", h.ConfirmBookingPayment)
}

func (h *Handler) RegisterOwnerRoutes(router *gin.Engine, authMiddleware gin.HandlerFunc, ownerRoleMiddleware gin.HandlerFunc) {
	group := router.Group("/owner", authMiddleware, ownerRoleMiddleware)
	group.GET("/venues/:id/bookings", h.ListOwnerVenueBookings)
}

func (h *Handler) CreateBooking(c *gin.Context) {
	userID, ok := getAuthenticatedUserID(c)
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
	userID, ok := getAuthenticatedUserID(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"message": "Unauthorized"})
		return
	}

	bookings, err := h.service.ListBookings(c.Request.Context(), userID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"message": "Failed to list bookings"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"bookings": bookings})
}

func (h *Handler) GetBooking(c *gin.Context) {
	userID, ok := getAuthenticatedUserID(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"message": "Unauthorized"})
		return
	}

	bookingID := c.Param("id")
	if !isValidUUID(bookingID) {
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

	c.JSON(http.StatusOK, gin.H{"booking": booking})
}

func (h *Handler) ConfirmBookingPayment(c *gin.Context) {
	customerID, ok := getAuthenticatedUserID(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"message": "Unauthorized"})
		return
	}

	bookingID := c.Param("id")
	if !isValidUUID(bookingID) {
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
	userID, ok := getAuthenticatedUserID(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"message": "Unauthorized"})
		return
	}

	venueID := c.Param("id")
	if !isValidUUID(venueID) {
		c.JSON(http.StatusBadRequest, gin.H{"message": "Invalid venue ID format"})
		return
	}

	req := OwnerVenueBookingsQuery{
		Limit: 10,
		Page:  1,
	}
	if err := c.ShouldBindQuery(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"message": "Invalid query parameters",
			"error":   err.Error(),
		})
		return
	}

	result, err := h.service.ListOwnerVenueBookings(c.Request.Context(), userID, venueID, req)
	if err != nil {
		respondBookingError(c, err, "Failed to list owner venue bookings")
		return
	}

	c.JSON(http.StatusOK, result)
}

func (h *Handler) CancelBooking(c *gin.Context) {
	userID, ok := getAuthenticatedUserID(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"message": "Unauthorized"})
		return
	}

	bookingID := c.Param("id")
	if !isValidUUID(bookingID) {
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

func respondBookingError(c *gin.Context, err error, fallbackMessage string) {
	switch {
	case errors.Is(err, ErrPastDate), errors.Is(err, ErrInvalidTimeRange):
		c.JSON(http.StatusBadRequest, gin.H{"message": err.Error()})
	case errors.Is(err, ErrCourtInactive), errors.Is(err, ErrVenueInactive):
		c.JSON(http.StatusBadRequest, gin.H{"message": err.Error()})
	case errors.Is(err, ErrOutsideOpHours):
		c.JSON(http.StatusBadRequest, gin.H{"message": err.Error()})
	case errors.Is(err, ErrOverlapBlockedSlot), errors.Is(err, ErrOverlapBooking), errors.Is(err, ErrBookingAlreadyCancelled), errors.Is(err, ErrBookingCannotBeCancelled), errors.Is(err, ErrBookingAlreadyConfirmed), errors.Is(err, ErrBookingCannotBeConfirmed):
		c.JSON(http.StatusConflict, gin.H{"message": err.Error()})
	case errors.Is(err, ErrBookingNotFound):
		c.JSON(http.StatusNotFound, gin.H{"message": "Booking not found"})
	case errors.Is(err, ErrCourtNotFound):
		c.JSON(http.StatusNotFound, gin.H{"message": "Court not found"})
	case errors.Is(err, ErrOwnerProfileNotFound):
		c.JSON(http.StatusBadRequest, gin.H{"message": "Owner profile is required before viewing bookings"})
	case errors.Is(err, ErrVenueNotFound):
		c.JSON(http.StatusNotFound, gin.H{"message": "Venue not found"})
	default:
		c.JSON(http.StatusInternalServerError, gin.H{"message": fallbackMessage})
	}
}

func getAuthenticatedUserID(c *gin.Context) (string, bool) {
	userIDValue, exists := c.Get("auth_user_id")
	if !exists {
		return "", false
	}
	userID, ok := userIDValue.(string)
	return userID, ok && userID != ""
}
