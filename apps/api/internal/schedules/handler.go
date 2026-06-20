package schedules

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
	ownerGroup := router.Group("/owner", authMiddleware, ownerRoleMiddleware)
	ownerGroup.GET("/courts/:id/operating-hours", h.GetOperatingHours)
	ownerGroup.PUT("/courts/:id/operating-hours", h.ReplaceOperatingHours)
}

func (h *Handler) GetOperatingHours(c *gin.Context) {
	userID, ok := getAuthenticatedUserID(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{
			"message": "Unauthorized",
		})
		return
	}

	courtID, ok := getUUIDParam(c, "id", "Court ID must be a valid UUID")
	if !ok {
		return
	}

	operatingHours, err := h.service.GetOperatingHours(c.Request.Context(), userID, courtID)
	if err != nil {
		respondScheduleError(c, err, "Failed to get operating hours")
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"operating_hours": operatingHours,
	})
}

func (h *Handler) ReplaceOperatingHours(c *gin.Context) {
	userID, ok := getAuthenticatedUserID(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{
			"message": "Unauthorized",
		})
		return
	}

	courtID, ok := getUUIDParam(c, "id", "Court ID must be a valid UUID")
	if !ok {
		return
	}

	var req ReplaceOperatingHoursRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"message": "Invalid request payload",
			"error":   err.Error(),
		})
		return
	}

	operatingHours, err := h.service.ReplaceOperatingHours(c.Request.Context(), userID, courtID, req)
	if err != nil {
		respondScheduleError(c, err, "Failed to update operating hours")
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message":         "Operating hours updated successfully",
		"operating_hours": operatingHours,
	})
}

func respondScheduleError(c *gin.Context, err error, fallbackMessage string) {
	switch {
	case errors.Is(err, ErrOwnerProfileNotFound):
		c.JSON(http.StatusBadRequest, gin.H{
			"message": "Owner profile is required before managing schedules",
		})
	case errors.Is(err, ErrCourtNotFound):
		c.JSON(http.StatusNotFound, gin.H{
			"message": "Court not found",
		})
	case errors.Is(err, ErrIncompleteOperatingSet):
		c.JSON(http.StatusBadRequest, gin.H{
			"message": "Operating hours must contain all 7 days",
		})
	case errors.Is(err, ErrDuplicateOperatingDay):
		c.JSON(http.StatusBadRequest, gin.H{
			"message": "Operating hours contain duplicate day_of_week values",
		})
	case errors.Is(err, ErrInvalidOperatingHours):
		c.JSON(http.StatusBadRequest, gin.H{
			"message": "Invalid operating hours",
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

func getUUIDParam(c *gin.Context, name, message string) (string, bool) {
	value := c.Param(name)
	if !isUUID(value) {
		c.JSON(http.StatusBadRequest, gin.H{
			"message": message,
		})
		return "", false
	}

	return value, true
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
