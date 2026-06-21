package venues

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
)

type Handler struct {
	service *Service
}

func NewHandler(service *Service) *Handler {
	return &Handler{service: service}
}

func (h *Handler) RegisterRoutes(router *gin.Engine, authMiddleware gin.HandlerFunc, ownerRoleMiddleware gin.HandlerFunc) {
	router.GET("/venues", h.GetPublicVenues)

	ownerGroup := router.Group("/owner", authMiddleware, ownerRoleMiddleware)
	ownerGroup.POST("/venues", h.CreateVenue)
	ownerGroup.GET("/venues", h.ListVenues)
	ownerGroup.GET("/venues/:id", h.GetVenue)
	ownerGroup.PUT("/venues/:id", h.UpdateVenue)
	ownerGroup.PATCH("/venues/:id/status", h.UpdateVenueStatus)
}

func (h *Handler) GetPublicVenues(c *gin.Context) {
	req := ListPublicVenuesQuery{
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

	venues, err := h.service.GetPublicVenues(c.Request.Context(), req)
	if err != nil {
		respondVenueError(c, err, "Failed to list public venues")
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"venues": venues,
		"page":   req.Page,
		"limit":  req.Limit,
	})
}

func (h *Handler) CreateVenue(c *gin.Context) {
	userID, ok := getAuthenticatedUserID(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{
			"message": "Unauthorized",
		})
		return
	}

	var req CreateVenueRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"message": "Invalid request payload",
			"error":   err.Error(),
		})
		return
	}

	venue, err := h.service.CreateVenue(c.Request.Context(), userID, req)
	if err != nil {
		respondVenueError(c, err, "Failed to create venue")
		return
	}

	c.JSON(http.StatusCreated, gin.H{
		"message": "Venue created successfully",
		"venue":   venue,
	})
}

func (h *Handler) ListVenues(c *gin.Context) {
	userID, ok := getAuthenticatedUserID(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{
			"message": "Unauthorized",
		})
		return
	}

	venues, err := h.service.ListVenues(c.Request.Context(), userID)
	if err != nil {
		respondVenueError(c, err, "Failed to list venues")
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"venues": venues,
	})
}

func (h *Handler) GetVenue(c *gin.Context) {
	userID, ok := getAuthenticatedUserID(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{
			"message": "Unauthorized",
		})
		return
	}

	venueID, ok := getVenueIDParam(c)
	if !ok {
		return
	}

	venue, err := h.service.GetVenue(c.Request.Context(), userID, venueID)
	if err != nil {
		respondVenueError(c, err, "Failed to get venue")
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"venue": venue,
	})
}

func (h *Handler) UpdateVenue(c *gin.Context) {
	userID, ok := getAuthenticatedUserID(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{
			"message": "Unauthorized",
		})
		return
	}

	venueID, ok := getVenueIDParam(c)
	if !ok {
		return
	}

	var req UpdateVenueRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"message": "Invalid request payload",
			"error":   err.Error(),
		})
		return
	}

	venue, err := h.service.UpdateVenue(c.Request.Context(), userID, venueID, req)
	if err != nil {
		respondVenueError(c, err, "Failed to update venue")
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "Venue updated successfully",
		"venue":   venue,
	})
}

func (h *Handler) UpdateVenueStatus(c *gin.Context) {
	userID, ok := getAuthenticatedUserID(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{
			"message": "Unauthorized",
		})
		return
	}

	venueID, ok := getVenueIDParam(c)
	if !ok {
		return
	}

	var req UpdateVenueStatusRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"message": "Invalid request payload",
			"error":   err.Error(),
		})
		return
	}

	venue, err := h.service.UpdateVenueStatus(c.Request.Context(), userID, venueID, req.Status)
	if err != nil {
		respondVenueError(c, err, "Failed to update venue status")
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "Venue status updated successfully",
		"venue":   venue,
	})
}

func respondVenueError(c *gin.Context, err error, fallbackMessage string) {
	switch {
	case errors.Is(err, ErrOwnerProfileNotFound):
		c.JSON(http.StatusBadRequest, gin.H{
			"message": "Owner profile is required before managing venues",
		})
	case errors.Is(err, ErrVenueNotFound):
		c.JSON(http.StatusNotFound, gin.H{
			"message": "Venue not found",
		})
	case errors.Is(err, ErrVenueAlreadyExists):
		c.JSON(http.StatusConflict, gin.H{
			"message": "Venue name already exists",
		})
	case errors.Is(err, ErrInvalidFacilities):
		c.JSON(http.StatusBadRequest, gin.H{
			"message": "One or more facility IDs are invalid",
		})
	case errors.Is(err, ErrInvalidVenuePayload):
		c.JSON(http.StatusBadRequest, gin.H{
			"message": "Name, address, and city are required",
		})
	case errors.Is(err, ErrInvalidVenueStatus):
		c.JSON(http.StatusBadRequest, gin.H{
			"message": "Invalid venue status",
		})
	default:
		c.JSON(http.StatusInternalServerError, gin.H{
			"message": fallbackMessage,
		})
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

func getVenueIDParam(c *gin.Context) (string, bool) {
	venueID := c.Param("id")
	if !isUUID(venueID) {
		c.JSON(http.StatusBadRequest, gin.H{
			"message": "Venue ID must be a valid UUID",
		})
		return "", false
	}

	return venueID, true
}

func isUUID(value string) bool {
	if len(value) != 36 {
		return false
	}

	for i, char := range value {
		switch i {
		case 8, 13, 18, 23:
			if char != '-' {
				return false
			}
		default:
			if !isHex(char) {
				return false
			}
		}
	}

	return true
}

func isHex(char rune) bool {
	return (char >= '0' && char <= '9') ||
		(char >= 'a' && char <= 'f') ||
		(char >= 'A' && char <= 'F')
}
