package admin

import (
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"lapangango-api/internal/audit"
	"lapangango-api/internal/middleware"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5"
)

const requestDeadlineHeader = "X-Request-Deadline-Ms"

type Handler struct {
	service Service
}

func NewHandler(service Service) *Handler {
	return &Handler{service: service}
}

func (h *Handler) RegisterRoutes(router *gin.Engine, authMiddleware gin.HandlerFunc, requireActiveUser gin.HandlerFunc) {
	adminGroup := router.Group("/admin")
	adminGroup.Use(authMiddleware, requireActiveUser, middleware.RequireRole("SUPER_ADMIN"))

	adminGroup.GET("/users", h.GetUsers)
	adminGroup.GET("/dashboard", h.GetDashboardStats)
	adminGroup.GET("/owners", h.GetOwners)
	adminGroup.PATCH("/owners/:id/status", h.UpdateOwnerStatus)
	adminGroup.GET("/venues", h.GetVenues)
	adminGroup.PATCH("/venues/:id/status", h.UpdateVenueStatus)
	adminGroup.GET("/audit-logs", h.GetAuditLogs)
}

func (h *Handler) GetUsers(c *gin.Context) {
	var query UserQuery
	if err := c.ShouldBindQuery(&query); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"message": "Invalid query parameters"})
		return
	}

	res, err := h.service.GetUsers(c.Request.Context(), query)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"message": "Failed to fetch users", "error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, res)
}

func (h *Handler) GetOwners(c *gin.Context) {
	var query OwnerQuery
	if err := c.ShouldBindQuery(&query); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"message": "Invalid query parameters"})
		return
	}

	res, err := h.service.GetOwners(c.Request.Context(), query)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"message": "Failed to fetch owners", "error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, res)
}

func (h *Handler) UpdateOwnerStatus(c *gin.Context) {
	if rejectExpiredMutation(c) {
		return
	}

	ownerProfileID := c.Param("id")
	var req UpdateStatusRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"message": "Invalid request body"})
		return
	}

	actorID := c.GetString("auth_user_id")

	err := h.service.UpdateOwnerStatus(c.Request.Context(), ownerProfileID, req.Status, actorID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			c.JSON(http.StatusNotFound, gin.H{"message": "Owner not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"message": "Failed to update owner status", "error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Owner status updated successfully"})
}

func (h *Handler) GetVenues(c *gin.Context) {
	var query VenueQuery
	if err := c.ShouldBindQuery(&query); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"message": "Invalid query parameters"})
		return
	}

	res, err := h.service.GetVenues(c.Request.Context(), query)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"message": "Failed to fetch venues", "error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, res)
}

func (h *Handler) UpdateVenueStatus(c *gin.Context) {
	if rejectExpiredMutation(c) {
		return
	}

	venueID := c.Param("id")
	var req UpdateStatusRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"message": "Invalid request body"})
		return
	}

	actorID := c.GetString("auth_user_id")

	err := h.service.UpdateVenueStatus(c.Request.Context(), venueID, req.Status, actorID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			c.JSON(http.StatusNotFound, gin.H{"message": "Venue not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"message": "Failed to update venue status", "error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Venue status updated successfully"})
}

func (h *Handler) GetAuditLogs(c *gin.Context) {
	var query AuditLogQuery
	if err := c.ShouldBindQuery(&query); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"message": "Invalid query parameters"})
		return
	}

	if query.EntityType != "" {
		query.EntityType = strings.ToUpper(strings.TrimSpace(query.EntityType))
		validEntities := map[string]bool{
			audit.EntityOwnerProfile:       true,
			audit.EntityVenue:              true,
			audit.EntityUser:               true,
			audit.EntityBooking:            true,
			audit.EntityStaff:              true,
			audit.EntityRefund:             true,
			audit.EntityFinanceTransaction: true,
		}
		if !validEntities[query.EntityType] {
			c.JSON(http.StatusBadRequest, gin.H{"message": "Invalid entity_type"})
			return
		}
	}

	res, err := h.service.GetAuditLogs(c.Request.Context(), query)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"message": "Failed to fetch audit logs", "error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, res)
}

func (h *Handler) GetDashboardStats(c *gin.Context) {
	res, err := h.service.GetDashboardStats(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"message": "Failed to fetch dashboard stats", "error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, res)
}

func rejectExpiredMutation(c *gin.Context) bool {
	rawDeadline := strings.TrimSpace(c.GetHeader(requestDeadlineHeader))
	if rawDeadline == "" {
		return false
	}

	deadlineMillis, err := strconv.ParseInt(rawDeadline, 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"message": "Invalid request deadline"})
		return true
	}

	if time.Now().UnixMilli() >= deadlineMillis {
		c.JSON(http.StatusRequestTimeout, gin.H{"message": "Request expired before processing"})
		return true
	}

	return false
}
