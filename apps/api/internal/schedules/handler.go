package schedules

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
	"lapangango-api/internal/httputil"
	"lapangango-api/internal/middleware"
)

type Handler struct {
	service *Service
}

func NewHandler(service *Service) *Handler {
	return &Handler{service: service}
}

func (h *Handler) RegisterRoutes(router *gin.Engine, authMiddleware gin.HandlerFunc, requireActiveUser gin.HandlerFunc, ownerWorkspaceMiddleware gin.HandlerFunc) {
	ownerGroup := router.Group("/owner", authMiddleware, requireActiveUser, ownerWorkspaceMiddleware)
	ownerGroup.GET("/courts/:id/operating-hours", middleware.RequireOwnerPermission("SCHEDULE_READ"), h.GetOperatingHours)
	ownerGroup.PUT("/courts/:id/operating-hours", middleware.RequireOwnerPermission("SCHEDULE_WRITE"), h.ReplaceOperatingHours)
}

func (h *Handler) GetOperatingHours(c *gin.Context) {
	ownerCtx, ok := httputil.GetOwnerContext(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{
			"message": "Unauthorized",
		})
		return
	}

	courtID, ok := httputil.GetUUIDParam(c, "id", "Court ID must be a valid UUID")
	if !ok {
		return
	}

	operatingHours, err := h.service.GetOperatingHours(c.Request.Context(), ownerCtx, courtID)
	if err != nil {
		respondScheduleError(c, err, "Failed to get operating hours")
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"operating_hours": operatingHours,
	})
}

func (h *Handler) ReplaceOperatingHours(c *gin.Context) {
	ownerCtx, ok := httputil.GetOwnerContext(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{
			"message": "Unauthorized",
		})
		return
	}

	courtID, ok := httputil.GetUUIDParam(c, "id", "Court ID must be a valid UUID")
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

	operatingHours, err := h.service.ReplaceOperatingHours(c.Request.Context(), ownerCtx, courtID, req)
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
