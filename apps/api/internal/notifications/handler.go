package notifications

import (
	"net/http"
	"strconv"

	"lapangango-api/internal/httputil"

	"github.com/gin-gonic/gin"
)

type Handler struct {
	service Service
}

func NewHandler(service Service) *Handler {
	return &Handler{service: service}
}

func (h *Handler) RegisterRoutes(r *gin.RouterGroup) {
	r.GET("", h.ListNotifications)
	r.GET("/", h.ListNotifications)
	r.GET("/unread-count", h.GetUnreadCount)
	r.PATCH("/:id/read", h.MarkRead)
	r.PATCH("/read-all", h.MarkAllRead)
}

func (h *Handler) ListNotifications(c *gin.Context) {
	userID, exists := httputil.GetAuthenticatedUserID(c)
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return
	}

	page, _ := strconv.Atoi(c.Query("page"))
	limit, _ := strconv.Atoi(c.Query("limit"))

	res, err := h.service.ListByUser(c.Request.Context(), userID, page, limit)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to list notifications"})
		return
	}

	c.JSON(http.StatusOK, res)
}

func (h *Handler) GetUnreadCount(c *gin.Context) {
	userID, exists := httputil.GetAuthenticatedUserID(c)
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return
	}

	res, err := h.service.UnreadCount(c.Request.Context(), userID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to get unread count"})
		return
	}

	c.JSON(http.StatusOK, res)
}

func (h *Handler) MarkRead(c *gin.Context) {
	userID, exists := httputil.GetAuthenticatedUserID(c)
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return
	}

	notificationID := c.Param("id")
	if notificationID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Notification ID is required"})
		return
	}

	err := h.service.MarkRead(c.Request.Context(), userID, notificationID)
	if err != nil {
		if err.Error() == "no rows in result set" { // pgx.ErrNoRows.Error()
			c.JSON(http.StatusNotFound, gin.H{"error": "Notification not found or forbidden"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to mark notification as read"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Notification marked as read"})
}

func (h *Handler) MarkAllRead(c *gin.Context) {
	userID, exists := httputil.GetAuthenticatedUserID(c)
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return
	}

	err := h.service.MarkAllRead(c.Request.Context(), userID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to mark all notifications as read"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "All notifications marked as read"})
}
