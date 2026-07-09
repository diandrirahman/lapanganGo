package courts

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
	ownerGroup.POST("/venues/:id/courts", middleware.RequireOwnerPermission("COURTS_WRITE"), h.CreateCourt)
	ownerGroup.GET("/venues/:id/courts", middleware.RequireOwnerPermission("COURTS_READ"), h.ListCourts)
	ownerGroup.GET("/courts/:id", middleware.RequireOwnerPermission("COURTS_READ"), h.GetCourt)
	ownerGroup.PUT("/courts/:id", middleware.RequireOwnerPermission("COURTS_WRITE"), h.UpdateCourt)
	ownerGroup.PATCH("/courts/:id/status", middleware.RequireOwnerPermission("COURTS_WRITE"), h.UpdateCourtStatus)
}

func (h *Handler) CreateCourt(c *gin.Context) {
	ownerCtx, ok := httputil.GetOwnerContext(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{
			"message": "Unauthorized",
		})
		return
	}

	venueID, ok := httputil.GetUUIDParam(c, "id", "Venue ID must be a valid UUID")
	if !ok {
		return
	}

	var req CreateCourtRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"message": "Invalid request payload",
			"error":   err.Error(),
		})
		return
	}

	court, err := h.service.CreateCourt(c.Request.Context(), ownerCtx, venueID, req)
	if err != nil {
		respondCourtError(c, err, "Failed to create court")
		return
	}

	c.JSON(http.StatusCreated, gin.H{
		"message": "Court created successfully",
		"court":   court,
	})
}

func (h *Handler) ListCourts(c *gin.Context) {
	ownerCtx, ok := httputil.GetOwnerContext(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{
			"message": "Unauthorized",
		})
		return
	}

	venueID, ok := httputil.GetUUIDParam(c, "id", "Venue ID must be a valid UUID")
	if !ok {
		return
	}

	courts, err := h.service.ListCourts(c.Request.Context(), ownerCtx, venueID)
	if err != nil {
		respondCourtError(c, err, "Failed to list courts")
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"courts": courts,
	})
}

func (h *Handler) GetCourt(c *gin.Context) {
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

	court, err := h.service.GetCourt(c.Request.Context(), ownerCtx, courtID)
	if err != nil {
		respondCourtError(c, err, "Failed to get court")
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"court": court,
	})
}

func (h *Handler) UpdateCourt(c *gin.Context) {
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

	var req UpdateCourtRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"message": "Invalid request payload",
			"error":   err.Error(),
		})
		return
	}

	court, err := h.service.UpdateCourt(c.Request.Context(), ownerCtx, courtID, req)
	if err != nil {
		respondCourtError(c, err, "Failed to update court")
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "Court updated successfully",
		"court":   court,
	})
}

func (h *Handler) UpdateCourtStatus(c *gin.Context) {
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

	var req UpdateCourtStatusRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"message": "Invalid request payload",
			"error":   err.Error(),
		})
		return
	}

	court, err := h.service.UpdateCourtStatus(c.Request.Context(), ownerCtx, courtID, req.Status)
	if err != nil {
		respondCourtError(c, err, "Failed to update court status")
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "Court status updated successfully",
		"court":   court,
	})
}

func respondCourtError(c *gin.Context, err error, fallbackMessage string) {
	switch {
	case errors.Is(err, ErrOwnerProfileNotFound):
		c.JSON(http.StatusBadRequest, gin.H{
			"message": "Owner profile is required before managing courts",
		})
	case errors.Is(err, ErrVenueNotFound):
		c.JSON(http.StatusNotFound, gin.H{
			"message": "Venue not found",
		})
	case errors.Is(err, ErrCourtNotFound):
		c.JSON(http.StatusNotFound, gin.H{
			"message": "Court not found",
		})
	case errors.Is(err, ErrCourtAlreadyExists):
		c.JSON(http.StatusConflict, gin.H{
			"message": "Court name already exists in this venue",
		})
	case errors.Is(err, ErrSportNotFound):
		c.JSON(http.StatusBadRequest, gin.H{
			"message": "Sport not found",
		})
	case errors.Is(err, ErrInvalidCourtPayload):
		c.JSON(http.StatusBadRequest, gin.H{
			"message": "Invalid court payload",
		})
	case errors.Is(err, ErrInvalidCourtStatus):
		c.JSON(http.StatusBadRequest, gin.H{
			"message": "Invalid court status",
		})
	default:
		c.JSON(http.StatusInternalServerError, gin.H{
			"message": fallbackMessage,
		})
	}
}
