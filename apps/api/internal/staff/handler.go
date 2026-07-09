package staff

import (
	"errors"
	"net/http"

	"lapangango-api/internal/audit"
	"lapangango-api/internal/httputil"

	"github.com/gin-gonic/gin"
)

type Handler struct {
	service      *Service
	auditService audit.Service
}

func NewHandler(service *Service, auditService audit.Service) *Handler {
	return &Handler{service: service, auditService: auditService}
}

func (h *Handler) RegisterRoutes(router *gin.Engine, authMiddleware gin.HandlerFunc, ownerWorkspaceMiddleware gin.HandlerFunc, requireActualOwner gin.HandlerFunc) {
	staffGroup := router.Group("/owner/staff", authMiddleware, ownerWorkspaceMiddleware, requireActualOwner)
	staffGroup.POST("", h.CreateStaff)
	staffGroup.GET("", h.ListStaff)
	staffGroup.GET("/:id", h.GetStaff)
	staffGroup.PUT("/:id", h.UpdateStaff)
	staffGroup.PATCH("/:id/status", h.UpdateStatus)
	staffGroup.PUT("/:id/venues", h.UpdateVenues)
}

func (h *Handler) CreateStaff(c *gin.Context) {
	var req CreateStaffRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	ownerProfileID := httputil.GetOwnerProfileID(c)
	actorUserID := httputil.GetActorUserID(c)

	staff, err := h.service.CreateStaff(c.Request.Context(), ownerProfileID, actorUserID, req)
	if err != nil {
		if errors.Is(err, ErrEmailAlreadyUsed) || errors.Is(err, ErrPhoneAlreadyUsed) {
			c.JSON(http.StatusConflict, gin.H{"message": err.Error()})
			return
		}
		if errors.Is(err, ErrWeakPassword) {
			c.JSON(http.StatusBadRequest, gin.H{"message": err.Error()})
			return
		}
		if errors.Is(err, ErrInvalidVenueAccess) {
			c.JSON(http.StatusBadRequest, gin.H{"message": err.Error()})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"message": "failed to create staff", "error": err.Error()})
		return
	}

	ip := c.ClientIP()
	ua := c.Request.UserAgent()
	h.auditService.Record(c.Request.Context(), audit.CreateAuditLogParams{
		OwnerProfileID: ownerProfileID,
		ActorUserID:    actorUserID,
		ActorRole:      httputil.GetActorRole(c),
		Action:         audit.ActionStaffCreated,
		EntityType:     audit.EntityStaff,
		EntityID:       &staff.ID,
		Metadata: map[string]any{
			"email":       req.Email,
			"role":        req.Role,
			"permissions": req.Permissions,
			"venue_ids":   req.VenueIDs,
		},
		IPAddress: &ip,
		UserAgent: &ua,
	})

	c.JSON(http.StatusCreated, staff)
}

func (h *Handler) ListStaff(c *gin.Context) {
	ownerProfileID := httputil.GetOwnerProfileID(c)

	staffList, err := h.service.ListStaff(c.Request.Context(), ownerProfileID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"message": "failed to get staff list", "error": err.Error()})
		return
	}

	if staffList == nil {
		staffList = []StaffResponse{}
	}

	c.JSON(http.StatusOK, gin.H{"staff": staffList})
}

func (h *Handler) GetStaff(c *gin.Context) {
	ownerProfileID := httputil.GetOwnerProfileID(c)
	staffID := c.Param("id")

	staff, err := h.service.GetStaff(c.Request.Context(), ownerProfileID, staffID)
	if err != nil {
		if errors.Is(err, ErrStaffNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"message": err.Error()})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"message": "failed to get staff", "error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, staff)
}

func (h *Handler) UpdateStaff(c *gin.Context) {
	var req UpdateStaffRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	ownerProfileID := httputil.GetOwnerProfileID(c)
	staffID := c.Param("id")

	staff, err := h.service.UpdateStaff(c.Request.Context(), ownerProfileID, staffID, req)
	if err != nil {
		if errors.Is(err, ErrStaffNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"message": err.Error()})
			return
		}
		if errors.Is(err, ErrPhoneAlreadyUsed) {
			c.JSON(http.StatusConflict, gin.H{"message": err.Error()})
			return
		}
		if errors.Is(err, ErrInvalidVenueAccess) {
			c.JSON(http.StatusBadRequest, gin.H{"message": err.Error()})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"message": "failed to update staff", "error": err.Error()})
		return
	}

	ip := c.ClientIP()
	ua := c.Request.UserAgent()
	h.auditService.Record(c.Request.Context(), audit.CreateAuditLogParams{
		OwnerProfileID: ownerProfileID,
		ActorUserID:    httputil.GetActorUserID(c),
		ActorRole:      httputil.GetActorRole(c),
		Action:         audit.ActionStaffUpdated,
		EntityType:     audit.EntityStaff,
		EntityID:       &staff.ID,
		Metadata: map[string]any{
			"role":        req.Role,
			"permissions": req.Permissions,
			"venue_ids":   req.VenueIDs,
		},
		IPAddress: &ip,
		UserAgent: &ua,
	})

	c.JSON(http.StatusOK, staff)
}

func (h *Handler) UpdateStatus(c *gin.Context) {
	var req UpdateStaffStatusRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	ownerProfileID := httputil.GetOwnerProfileID(c)
	staffID := c.Param("id")

	oldStaff, err := h.service.GetStaff(c.Request.Context(), ownerProfileID, staffID)
	if err != nil {
		if errors.Is(err, ErrStaffNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"message": err.Error()})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"message": "failed to get staff", "error": err.Error()})
		return
	}

	staff, err := h.service.UpdateStatus(c.Request.Context(), ownerProfileID, staffID, req)
	if err != nil {
		if errors.Is(err, ErrStaffNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"message": err.Error()})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"message": "failed to update status", "error": err.Error()})
		return
	}

	ip := c.ClientIP()
	ua := c.Request.UserAgent()
	h.auditService.Record(c.Request.Context(), audit.CreateAuditLogParams{
		OwnerProfileID: ownerProfileID,
		ActorUserID:    httputil.GetActorUserID(c),
		ActorRole:      httputil.GetActorRole(c),
		Action:         audit.ActionStaffStatusUpdated,
		EntityType:     audit.EntityStaff,
		EntityID:       &staff.ID,
		Metadata: map[string]any{
			"old_status": oldStaff.Status,
			"new_status": staff.Status,
		},
		IPAddress: &ip,
		UserAgent: &ua,
	})

	c.JSON(http.StatusOK, staff)
}

func (h *Handler) UpdateVenues(c *gin.Context) {
	var req UpdateStaffVenuesRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	ownerProfileID := httputil.GetOwnerProfileID(c)
	staffID := c.Param("id")

	oldStaff, err := h.service.GetStaff(c.Request.Context(), ownerProfileID, staffID)
	if err != nil {
		if errors.Is(err, ErrStaffNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"message": err.Error()})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"message": "failed to get staff", "error": err.Error()})
		return
	}



	staff, err := h.service.UpdateVenues(c.Request.Context(), ownerProfileID, staffID, req)
	if err != nil {
		if errors.Is(err, ErrStaffNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"message": err.Error()})
			return
		}
		if errors.Is(err, ErrInvalidVenueAccess) {
			c.JSON(http.StatusBadRequest, gin.H{"message": err.Error()})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"message": "failed to update venues", "error": err.Error()})
		return
	}

	ip := c.ClientIP()
	ua := c.Request.UserAgent()
	
	h.auditService.Record(c.Request.Context(), audit.CreateAuditLogParams{
		OwnerProfileID: ownerProfileID,
		ActorUserID:    httputil.GetActorUserID(c),
		ActorRole:      httputil.GetActorRole(c),
		Action:         audit.ActionStaffVenuesUpdated,
		EntityType:     audit.EntityStaff,
		EntityID:       &staff.ID,
		Metadata: map[string]any{
			"old_venue_ids": oldStaff.VenueIDs,
			"new_venue_ids": staff.VenueIDs,
		},
		IPAddress: &ip,
		UserAgent: &ua,
	})

	c.JSON(http.StatusOK, staff)
}
