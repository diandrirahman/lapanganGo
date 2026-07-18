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

const (
	AuditScopeOwner    = "OWNER"
	AuditScopePlatform = "PLATFORM"
	AuditScopeAll      = "ALL"
)

var ownerAuditEntities = map[string]struct{}{
	audit.EntityOwnerProfile:       {},
	audit.EntityVenue:              {},
	audit.EntityUser:               {},
	audit.EntityBooking:            {},
	audit.EntityStaff:              {},
	audit.EntityRefund:             {},
	audit.EntityFinanceTransaction: {},
}

var platformAuditEntities = map[string]struct{}{
	audit.EntityPlatformCommercialTerm: {},
	audit.EntityPlatformFinanceJournal: {},
	audit.EntityPlatformExpense:        {},
}

var ownerAuditActions = map[string]struct{}{
	audit.ActionStaffCreated:                {},
	audit.ActionStaffUpdated:                {},
	audit.ActionStaffStatusUpdated:          {},
	audit.ActionStaffVenuesUpdated:          {},
	audit.ActionStaffInviteCreated:          {},
	audit.ActionStaffInviteRegenerated:      {},
	audit.ActionStaffPasswordResetRequested: {},
	audit.ActionStaffPasswordResetCompleted: {},
	audit.ActionStaffPasswordSetupCompleted: {},
	audit.ActionBookingPaymentVerified:      {},
	audit.ActionBookingPaymentRejected:      {},
	audit.ActionBookingMarkedPaid:           {},
	audit.ActionBookingCompleted:            {},
	audit.ActionBookingCancelRefund:         {},
	audit.ActionRefundApproved:              {},
	audit.ActionRefundRejected:              {},
	audit.ActionFinanceCreated:              {},
	audit.ActionFinanceUpdated:              {},
	audit.ActionFinanceDeleted:              {},
	"UPDATE_OWNER_STATUS":                   {},
	"UPDATE_VENUE_STATUS":                   {},
}

var platformAuditActions = map[string]struct{}{
	audit.ActionPlatformCommercialTermCreated:      {},
	audit.ActionPlatformCommercialTermSuperseded:   {},
	audit.ActionPlatformCommercialTermLiveRejected: {},
	audit.ActionPlatformFinanceJournalReversed:     {},
	audit.ActionPlatformFinanceLiveWriteRejected:   {},
	audit.ActionPlatformExpenseCreated:             {},
	audit.ActionPlatformExpenseCancelled:           {},
	audit.ActionPlatformExpenseApproved:            {},
}

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
		writeAuditError(c, http.StatusBadRequest, "INVALID_QUERY", "Invalid query parameters")
		return
	}

	query.Scope = strings.ToUpper(strings.TrimSpace(query.Scope))
	if query.Scope == "" {
		query.Scope = AuditScopeOwner
	}
	if query.Scope != AuditScopeOwner && query.Scope != AuditScopePlatform && query.Scope != AuditScopeAll {
		writeAuditError(c, http.StatusBadRequest, "INVALID_SCOPE", "Scope must be OWNER, PLATFORM, or ALL")
		return
	}
	query.EntityType = strings.ToUpper(strings.TrimSpace(query.EntityType))
	query.Action = strings.ToUpper(strings.TrimSpace(query.Action))
	if query.EntityType != "" && !validAuditEntity(query.Scope, query.EntityType) {
		writeAuditError(c, http.StatusBadRequest, "INVALID_ENTITY_TYPE", "Invalid entity_type for the selected scope")
		return
	}
	if query.Action != "" && !validAuditAction(query.Scope, query.Action) {
		writeAuditError(c, http.StatusBadRequest, "INVALID_ACTION", "Invalid action for the selected scope")
		return
	}

	res, err := h.service.GetAuditLogs(c.Request.Context(), query)
	if err != nil {
		writeAuditError(c, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to fetch audit logs")
		return
	}

	c.JSON(http.StatusOK, res)
}

func validAuditEntity(scope, entity string) bool {
	switch scope {
	case AuditScopeOwner:
		_, ok := ownerAuditEntities[entity]
		return ok
	case AuditScopePlatform:
		_, ok := platformAuditEntities[entity]
		return ok
	case AuditScopeAll:
		if _, ok := ownerAuditEntities[entity]; ok {
			return true
		}
		_, ok := platformAuditEntities[entity]
		return ok
	default:
		return false
	}
}

func validAuditAction(scope, action string) bool {
	switch scope {
	case AuditScopeOwner:
		_, ok := ownerAuditActions[action]
		return ok || legacyAuditActionToken(action)
	case AuditScopePlatform:
		_, ok := platformAuditActions[action]
		return ok
	case AuditScopeAll:
		if _, ok := ownerAuditActions[action]; ok {
			return true
		}
		if _, ok := platformAuditActions[action]; ok {
			return true
		}
		return legacyAuditActionToken(action)
	default:
		return false
	}
}

func legacyAuditActionToken(action string) bool {
	if action == "" || len(action) > 100 {
		return false
	}
	for _, char := range action {
		if (char < 'A' || char > 'Z') && (char < '0' || char > '9') && char != '_' {
			return false
		}
	}
	return true
}

func writeAuditError(c *gin.Context, status int, code, message string) {
	c.JSON(status, gin.H{"code": code, "message": message})
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
