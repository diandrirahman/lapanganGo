package blockedslots

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
	ownerGroup.POST("/courts/:id/blocked-slots", h.CreateBlockedSlot)
	ownerGroup.GET("/courts/:id/blocked-slots", h.ListBlockedSlots)
	ownerGroup.DELETE("/blocked-slots/:id", h.DeleteBlockedSlot)
}

func (h *Handler) CreateBlockedSlot(c *gin.Context) {
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

	var req CreateBlockedSlotRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"message": "Invalid request payload",
			"error":   err.Error(),
		})
		return
	}

	blockedSlot, err := h.service.CreateBlockedSlot(c.Request.Context(), userID, courtID, req)
	if err != nil {
		respondBlockedSlotError(c, err, "Failed to create blocked slot")
		return
	}

	c.JSON(http.StatusCreated, gin.H{
		"message":      "Blocked slot created successfully",
		"blocked_slot": blockedSlot,
	})
}

func (h *Handler) ListBlockedSlots(c *gin.Context) {
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

	blockedSlots, err := h.service.ListBlockedSlots(
		c.Request.Context(),
		userID,
		courtID,
		c.Query("from"),
		c.Query("to"),
	)
	if err != nil {
		respondBlockedSlotError(c, err, "Failed to list blocked slots")
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"blocked_slots": blockedSlots,
	})
}

func (h *Handler) DeleteBlockedSlot(c *gin.Context) {
	userID, ok := getAuthenticatedUserID(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{
			"message": "Unauthorized",
		})
		return
	}

	blockedSlotID, ok := getUUIDParam(c, "id", "Blocked slot ID must be a valid UUID")
	if !ok {
		return
	}

	blockedSlot, err := h.service.DeleteBlockedSlot(c.Request.Context(), userID, blockedSlotID)
	if err != nil {
		respondBlockedSlotError(c, err, "Failed to delete blocked slot")
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message":      "Blocked slot deleted successfully",
		"blocked_slot": blockedSlot,
	})
}

func respondBlockedSlotError(c *gin.Context, err error, fallbackMessage string) {
	switch {
	case errors.Is(err, ErrOwnerProfileNotFound):
		c.JSON(http.StatusBadRequest, gin.H{
			"message": "Owner profile is required before managing blocked slots",
		})
	case errors.Is(err, ErrCourtNotFound):
		c.JSON(http.StatusNotFound, gin.H{
			"message": "Court not found",
		})
	case errors.Is(err, ErrBlockedSlotNotFound):
		c.JSON(http.StatusNotFound, gin.H{
			"message": "Blocked slot not found",
		})
	case errors.Is(err, ErrInvalidBlockedSlot):
		c.JSON(http.StatusBadRequest, gin.H{
			"message": "Invalid blocked slot datetime",
		})
	case errors.Is(err, ErrInvalidBlockedSlotRange):
		c.JSON(http.StatusBadRequest, gin.H{
			"message": "Blocked slot end time must be after start time",
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
