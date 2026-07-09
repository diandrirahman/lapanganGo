package mabar

import (
	"errors"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"lapangango-api/internal/httputil"
)

type Handler struct {
	service *Service
}

func NewHandler(service *Service) *Handler {
	return &Handler{service: service}
}

func (h *Handler) RegisterRoutes(router *gin.Engine, authMiddleware gin.HandlerFunc, requireActiveUser gin.HandlerFunc, customerRoleMiddleware gin.HandlerFunc) {
	// Public routes
	router.GET("/open-matches", h.ListOpenMatches)
	router.GET("/open-matches/:id", h.GetOpenMatchDetail)

	// Protected routes
	protected := router.Group("", authMiddleware, requireActiveUser, customerRoleMiddleware)
	protected.POST("/bookings/:id/open-matches", h.CreateOpenMatch)
	protected.POST("/open-matches/:id/join", h.JoinOpenMatch)
	protected.DELETE("/open-matches/:id/join", h.LeaveOpenMatch)
	protected.PATCH("/open-matches/:id/cancel", h.CancelOpenMatch)
}

func (h *Handler) CreateOpenMatch(c *gin.Context) {
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

	var req CreateOpenMatchRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"message": "Invalid request payload",
			"error":   err.Error(),
		})
		return
	}

	resp, err := h.service.CreateOpenMatch(c.Request.Context(), bookingID, userID, req)
	if err != nil {
		respondError(c, err, "Failed to create open match")
		return
	}

	c.JSON(http.StatusCreated, gin.H{
		"message":    "Open match created successfully",
		"open_match": resp,
	})
}

func (h *Handler) ListOpenMatches(c *gin.Context) {
	filter := ListOpenMatchesFilter{
		SportID: c.Query("sport_id"),
		City:    c.Query("city"),
		Date:    c.Query("date"),
		Level:   c.Query("level"),
	}

	if filter.Date != "" {
		if _, err := time.Parse("2006-01-02", filter.Date); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"message": "Date must use YYYY-MM-DD format"})
			return
		}
	}

	pageParams := httputil.GetPaginationParams(c)
	filter.Limit = pageParams.Limit
	filter.Offset = (pageParams.Page - 1) * pageParams.Limit

	matches, total, err := h.service.ListOpenMatches(c.Request.Context(), filter)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"message": "Failed to list open matches", "error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, httputil.NewPaginatedResponse(matches, total, pageParams.Page, pageParams.Limit))
}

func (h *Handler) GetOpenMatchDetail(c *gin.Context) {
	id := c.Param("id")
	if !httputil.IsUUID(id) {
		c.JSON(http.StatusBadRequest, gin.H{"message": "Invalid open match ID format"})
		return
	}

	detail, err := h.service.GetOpenMatchDetail(c.Request.Context(), id)
	if err != nil {
		respondError(c, err, "Failed to get open match detail")
		return
	}

	c.JSON(http.StatusOK, detail)
}

func (h *Handler) JoinOpenMatch(c *gin.Context) {
	userID, ok := httputil.GetAuthenticatedUserID(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"message": "Unauthorized"})
		return
	}

	id := c.Param("id")
	if !httputil.IsUUID(id) {
		c.JSON(http.StatusBadRequest, gin.H{"message": "Invalid open match ID format"})
		return
	}

	if err := h.service.JoinOpenMatch(c.Request.Context(), id, userID); err != nil {
		respondError(c, err, "Failed to join open match")
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Successfully joined open match"})
}

func (h *Handler) LeaveOpenMatch(c *gin.Context) {
	userID, ok := httputil.GetAuthenticatedUserID(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"message": "Unauthorized"})
		return
	}

	id := c.Param("id")
	if !httputil.IsUUID(id) {
		c.JSON(http.StatusBadRequest, gin.H{"message": "Invalid open match ID format"})
		return
	}

	if err := h.service.LeaveOpenMatch(c.Request.Context(), id, userID); err != nil {
		respondError(c, err, "Failed to leave open match")
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Successfully left open match"})
}

func (h *Handler) CancelOpenMatch(c *gin.Context) {
	userID, ok := httputil.GetAuthenticatedUserID(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"message": "Unauthorized"})
		return
	}

	id := c.Param("id")
	if !httputil.IsUUID(id) {
		c.JSON(http.StatusBadRequest, gin.H{"message": "Invalid open match ID format"})
		return
	}

	if err := h.service.CancelOpenMatch(c.Request.Context(), id, userID); err != nil {
		respondError(c, err, "Failed to cancel open match")
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Successfully cancelled open match"})
}

func respondError(c *gin.Context, err error, fallbackMessage string) {
	switch {
	case errors.Is(err, ErrUnauthorized):
		c.JSON(http.StatusForbidden, gin.H{"message": err.Error()})
	case errors.Is(err, ErrBookingNotFound), errors.Is(err, ErrOpenMatchNotFound):
		c.JSON(http.StatusNotFound, gin.H{"message": err.Error()})
	case errors.Is(err, ErrBookingInvalid), errors.Is(err, ErrInvalidLevel), errors.Is(err, ErrBookingPassed), errors.Is(err, ErrMatchPassed), errors.Is(err, ErrMatchNotOpen), errors.Is(err, ErrHostCannotJoin), errors.Is(err, ErrInvalidMaxPlayers), errors.Is(err, ErrInvalidPricePerPlayer), errors.Is(err, ErrCannotLeaveClosedMatch), errors.Is(err, ErrCannotCancelClosedMatch), errors.Is(err, ErrInvalidTitle):
		c.JSON(http.StatusBadRequest, gin.H{"message": err.Error()})
	case errors.Is(err, ErrMatchAlreadyExists), errors.Is(err, ErrAlreadyJoined), errors.Is(err, ErrMatchFull), errors.Is(err, ErrNotJoined), errors.Is(err, ErrBookingCancelled), errors.Is(err, ErrBookingNotConfirmed):
		c.JSON(http.StatusConflict, gin.H{"message": err.Error()})
	default:
		c.JSON(http.StatusInternalServerError, gin.H{"message": fallbackMessage})
	}
}
