package mabar

import (
	"errors"
	"net/http"
	"regexp"
	"strconv"
	"time"

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
	// Public routes
	router.GET("/open-matches", h.ListOpenMatches)
	router.GET("/open-matches/:id", h.GetOpenMatchDetail)

	// Protected routes
	protected := router.Group("", authMiddleware, customerRoleMiddleware)
	protected.POST("/bookings/:id/open-matches", h.CreateOpenMatch)
	protected.POST("/open-matches/:id/join", h.JoinOpenMatch)
	protected.DELETE("/open-matches/:id/join", h.LeaveOpenMatch)
	protected.PATCH("/open-matches/:id/cancel", h.CancelOpenMatch)
}

func (h *Handler) CreateOpenMatch(c *gin.Context) {
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

	if limitStr := c.Query("limit"); limitStr != "" {
		limit, _ := strconv.Atoi(limitStr)
		filter.Limit = limit
	} else {
		filter.Limit = 10
	}

	if pageStr := c.Query("page"); pageStr != "" {
		page, _ := strconv.Atoi(pageStr)
		if page > 0 {
			filter.Offset = (page - 1) * filter.Limit
		}
	}

	matches, err := h.service.ListOpenMatches(c.Request.Context(), filter)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"message": "Failed to list open matches", "error": err.Error()})
		return
	}

	if matches == nil {
		matches = []OpenMatchResponse{}
	}

	c.JSON(http.StatusOK, gin.H{"open_matches": matches})
}

func (h *Handler) GetOpenMatchDetail(c *gin.Context) {
	id := c.Param("id")
	if !isValidUUID(id) {
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
	userID, ok := getAuthenticatedUserID(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"message": "Unauthorized"})
		return
	}

	id := c.Param("id")
	if !isValidUUID(id) {
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
	userID, ok := getAuthenticatedUserID(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"message": "Unauthorized"})
		return
	}

	id := c.Param("id")
	if !isValidUUID(id) {
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
	userID, ok := getAuthenticatedUserID(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"message": "Unauthorized"})
		return
	}

	id := c.Param("id")
	if !isValidUUID(id) {
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
		c.JSON(http.StatusInternalServerError, gin.H{"message": fallbackMessage, "error": err.Error()})
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
