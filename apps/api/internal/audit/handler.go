package audit

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"lapangango-api/internal/httputil"
)

type Handler struct {
	service Service
}

func NewHandler(service Service) *Handler {
	return &Handler{service: service}
}

func (h *Handler) RegisterRoutes(router *gin.Engine, authMiddleware gin.HandlerFunc, ownerWorkspaceMiddleware gin.HandlerFunc) {
	// Only actual owner can access this endpoint
	ownerGroup := router.Group("/owner", authMiddleware, ownerWorkspaceMiddleware, h.RequireActualOwner())
	ownerGroup.GET("/audit-logs", h.ListAuditLogs)
}

func (h *Handler) RequireActualOwner() gin.HandlerFunc {
	return func(c *gin.Context) {
		ownerCtx, ok := httputil.GetOwnerContext(c)
		if !ok {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
			c.Abort()
			return
		}

		if !ownerCtx.IsOwner {
			c.JSON(http.StatusForbidden, gin.H{"error": "Forbidden: only actual owner can view audit logs"})
			c.Abort()
			return
		}
		c.Next()
	}
}

func (h *Handler) ListAuditLogs(c *gin.Context) {
	ownerCtx, ok := httputil.GetOwnerContext(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{
			"message": "Unauthorized",
		})
		return
	}

	var req AuditLogQuery
	if err := c.ShouldBindQuery(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"message": "Invalid query parameters",
			"error":   err.Error(),
		})
		return
	}

	pageParams := httputil.GetPaginationParams(c)
	req.Page = pageParams.Page
	req.Limit = pageParams.Limit

	logs, total, err := h.service.ListOwnerLogs(c.Request.Context(), ownerCtx.OwnerProfileID, req)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"message": "Failed to list audit logs",
			"error":   err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, httputil.NewPaginatedResponse(logs, total, req.Page, req.Limit))
}
