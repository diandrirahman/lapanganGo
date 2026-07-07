package promos

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
	"lapangango-api/internal/httputil"
)

type Handler struct {
	service *Service
}

func NewHandler(service *Service) *Handler {
	return &Handler{service: service}
}

func (h *Handler) RegisterOwnerRoutes(router *gin.Engine, authMiddleware gin.HandlerFunc, ownerRoleMiddleware gin.HandlerFunc) {
	group := router.Group("/owner/promos", authMiddleware, ownerRoleMiddleware)
	group.POST("", h.CreatePromo)
	group.GET("", h.ListPromos)
	group.GET("/:id", h.GetPromo)
	group.PUT("/:id", h.UpdatePromo)
	group.PATCH("/:id/toggle", h.TogglePromo)
	group.DELETE("/:id", h.DeletePromo)
}

func (h *Handler) CreatePromo(c *gin.Context) {
	ownerID, ok := httputil.GetAuthenticatedUserID(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"message": "Unauthorized"})
		return
	}

	var req CreatePromoRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"message": "Invalid request payload", "error": err.Error()})
		return
	}

	res, err := h.service.CreatePromo(c.Request.Context(), ownerID, req)
	if err != nil {
		respondError(c, err)
		return
	}
	c.JSON(http.StatusCreated, res)
}

func (h *Handler) ListPromos(c *gin.Context) {
	ownerID, ok := httputil.GetAuthenticatedUserID(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"message": "Unauthorized"})
		return
	}

	res, err := h.service.ListOwnerPromos(c.Request.Context(), ownerID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"message": "Internal server error"})
		return
	}
	c.JSON(http.StatusOK, res)
}

func (h *Handler) GetPromo(c *gin.Context) {
	ownerID, ok := httputil.GetAuthenticatedUserID(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"message": "Unauthorized"})
		return
	}

	id := c.Param("id")
	res, err := h.service.GetPromo(c.Request.Context(), id, ownerID)
	if err != nil {
		respondError(c, err)
		return
	}
	c.JSON(http.StatusOK, res)
}

func (h *Handler) UpdatePromo(c *gin.Context) {
	ownerID, ok := httputil.GetAuthenticatedUserID(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"message": "Unauthorized"})
		return
	}

	id := c.Param("id")
	var req CreatePromoRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"message": "Invalid request payload", "error": err.Error()})
		return
	}

	res, err := h.service.UpdatePromo(c.Request.Context(), id, ownerID, req)
	if err != nil {
		respondError(c, err)
		return
	}
	c.JSON(http.StatusOK, res)
}

func (h *Handler) TogglePromo(c *gin.Context) {
	ownerID, ok := httputil.GetAuthenticatedUserID(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"message": "Unauthorized"})
		return
	}

	id := c.Param("id")
	res, err := h.service.TogglePromoStatus(c.Request.Context(), id, ownerID)
	if err != nil {
		respondError(c, err)
		return
	}
	c.JSON(http.StatusOK, res)
}

func (h *Handler) RegisterCustomerRoutes(router *gin.Engine, authMiddleware gin.HandlerFunc, customerRoleMiddleware gin.HandlerFunc) {
	group := router.Group("/promos", authMiddleware, customerRoleMiddleware)
	group.POST("/validate", h.ValidatePromo)
}

func (h *Handler) ValidatePromo(c *gin.Context) {
	_, ok := httputil.GetAuthenticatedUserID(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"message": "Unauthorized"})
		return
	}

	var req ValidatePromoRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"message": "Invalid request payload", "error": err.Error()})
		return
	}

	res, err := h.service.ValidatePromo(c.Request.Context(), req)
	if err != nil {
		respondError(c, err)
		return
	}
	c.JSON(http.StatusOK, res)
}

func respondError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, ErrPromoNotFound):
		c.JSON(http.StatusNotFound, gin.H{"message": "Promo not found"})
	case errors.Is(err, ErrCodeExists):
		c.JSON(http.StatusConflict, gin.H{"message": "Promo code already exists"})
	case errors.Is(err, ErrPromoVenueForbidden):
		c.JSON(http.StatusForbidden, gin.H{"message": "Venue does not belong to owner"})
	case errors.Is(err, ErrInvalidBookingDate), errors.Is(err, ErrInvalidDiscount), errors.Is(err, ErrInvalidPeriod), errors.Is(err, ErrInvalidPromoCode):
		c.JSON(http.StatusBadRequest, gin.H{"message": err.Error()})
	case errors.Is(err, ErrPromoNotActive), errors.Is(err, ErrPromoExpired), errors.Is(err, ErrPromoNotStarted), errors.Is(err, ErrPromoVenueMismatch), errors.Is(err, ErrInvalidPrice):
		c.JSON(http.StatusConflict, gin.H{"message": err.Error()})
	case errors.Is(err, ErrPromoAlreadyUsed):
		c.JSON(http.StatusConflict, gin.H{"message": "Promo sudah pernah digunakan. Nonaktifkan promo jika tidak ingin dipakai lagi."})
	default:
		if err.Error() == "court not found" || err.Error() == "invalid time range" || err.Error() == "court does not belong to the requested venue" {
			c.JSON(http.StatusBadRequest, gin.H{"message": err.Error()})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"message": "Internal server error"})
	}
}

func (h *Handler) DeletePromo(c *gin.Context) {
	ownerID, ok := httputil.GetAuthenticatedUserID(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"message": "Unauthorized"})
		return
	}

	id := c.Param("id")
	if !httputil.IsUUID(id) {
		c.JSON(http.StatusBadRequest, gin.H{"message": "Invalid promo ID format"})
		return
	}

	err := h.service.DeletePromo(c.Request.Context(), id, ownerID)
	if err != nil {
		respondError(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Promo deleted successfully"})
}
