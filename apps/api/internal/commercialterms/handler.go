package commercialterms

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
)

type Handler struct {
	service Service
}

func NewHandler(service Service) *Handler {
	return &Handler{service: service}
}

func (h *Handler) GetTerms(c *gin.Context) {
	var query GetTermsQuery
	if err := c.ShouldBindQuery(&query); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": "INVALID_QUERY", "message": "Invalid query parameters"})
		return
	}

	query.Scope = strings.ToUpper(query.Scope)
	if query.Scope != "GLOBAL" && query.Scope != "OWNER" && query.Scope != "ALL" {
		if query.Scope == "" {
			query.Scope = "ALL" // Default
		} else {
			c.JSON(http.StatusBadRequest, gin.H{"code": "INVALID_SCOPE", "message": "Scope must be GLOBAL, OWNER, or ALL"})
			return
		}
	}

	if query.Scope == "GLOBAL" && query.OwnerProfileID != "" {
		c.JSON(http.StatusBadRequest, gin.H{"code": "INVALID_FILTER", "message": "Cannot provide owner_profile_id when scope is GLOBAL"})
		return
	}

	if query.Scope == "OWNER" && query.OwnerProfileID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"code": "INVALID_FILTER", "message": "owner_profile_id is required when scope is OWNER"})
		return
	}

	if query.Status != "" {
		query.Status = strings.ToUpper(query.Status)
		if query.Status != "CURRENT" && query.Status != "SCHEDULED" && query.Status != "HISTORICAL" {
			c.JSON(http.StatusBadRequest, gin.H{"code": "INVALID_STATUS", "message": "Status must be CURRENT, SCHEDULED, or HISTORICAL"})
			return
		}
	}

	res, err := h.service.GetTerms(c.Request.Context(), query)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": "INTERNAL_ERROR", "message": "Failed to fetch commercial terms"})
		return
	}

	c.JSON(http.StatusOK, res)
}

func (h *Handler) Preview(c *gin.Context) {
	var req PreviewRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": "INVALID_BODY", "message": "Invalid request body"})
		return
	}

	res, err := h.service.Preview(c.Request.Context(), req)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": "VALIDATION_FAILED", "message": err.Error()})
		return
	}

	c.JSON(http.StatusOK, res)
}

func RegisterRoutes(router *gin.Engine, authMiddleware gin.HandlerFunc, requireActiveUser gin.HandlerFunc, requireRole func(...string) gin.HandlerFunc, service Service) {
	h := NewHandler(service)
	ctGroup := router.Group("/admin/commercial-terms")
	ctGroup.Use(authMiddleware, requireActiveUser, requireRole("SUPER_ADMIN"))

	ctGroup.GET("", h.GetTerms)
	ctGroup.POST("/preview", h.Preview)
}
