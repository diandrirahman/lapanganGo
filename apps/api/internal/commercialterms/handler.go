package commercialterms

import (
	"errors"
	"log"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"lapangango-api/internal/httputil"
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

func (h *Handler) CreateTerm(c *gin.Context) {
	idempotencyKey := c.GetHeader("Idempotency-Key")
	idempotencyKey = strings.TrimSpace(idempotencyKey)
	if idempotencyKey == "" {
		c.JSON(http.StatusBadRequest, gin.H{"code": "MISSING_IDEMPOTENCY_KEY", "message": "Idempotency-Key header is required"})
		return
	}
	if len(idempotencyKey) > 255 {
		c.JSON(http.StatusBadRequest, gin.H{"code": "INVALID_IDEMPOTENCY_KEY", "message": "Idempotency-Key cannot exceed 255 characters"})
		return
	}

	var req CreateTermRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": "INVALID_BODY", "message": "Invalid request body"})
		return
	}

	adminID, ok := httputil.GetAuthenticatedUserID(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"code": "UNAUTHORIZED", "message": "Missing or invalid authentication"})
		return
	}

	ipAddress := c.ClientIP()
	userAgent := c.GetHeader("User-Agent")
	term, err := h.service.CreateTerm(c.Request.Context(), req, idempotencyKey, adminID, ipAddress, userAgent)
	if err != nil {
		var validationErr ErrValidationError
		if errors.As(err, &validationErr) {
			c.JSON(http.StatusBadRequest, gin.H{"code": "VALIDATION_ERROR", "message": validationErr.Error()})
			return
		}
		var notFoundErr ErrNotFound
		if errors.As(err, &notFoundErr) {
			c.JSON(http.StatusNotFound, gin.H{"code": "NOT_FOUND", "message": notFoundErr.Error()})
			return
		}
		var forbiddenErr ErrForbidden
		if errors.As(err, &forbiddenErr) {
			c.JSON(http.StatusForbidden, gin.H{"code": "FORBIDDEN", "message": forbiddenErr.Error()})
			return
		}
		var conflictErr ErrConflict
		if errors.As(err, &conflictErr) {
			c.JSON(http.StatusConflict, gin.H{"code": "CONFLICT", "message": conflictErr.Error()})
			return
		}
		log.Printf("Internal error creating term: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"code": "INTERNAL_ERROR", "message": "An internal server error occurred"})
		return
	}

	c.JSON(http.StatusCreated, term)
}

func RegisterRoutes(router *gin.Engine, authMiddleware gin.HandlerFunc, requireActiveUser gin.HandlerFunc, requireRole func(...string) gin.HandlerFunc, service Service) {
	h := NewHandler(service)
	ctGroup := router.Group("/admin/commercial-terms")
	ctGroup.Use(authMiddleware, requireActiveUser, requireRole("SUPER_ADMIN"))

	ctGroup.GET("", h.GetTerms)
	ctGroup.POST("/preview", h.Preview)
	ctGroup.POST("", h.CreateTerm)
}
