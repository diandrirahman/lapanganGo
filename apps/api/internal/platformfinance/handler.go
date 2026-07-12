package platformfinance

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
)

type Handler struct {
	service Service
}

func NewHandler(service Service) *Handler {
	return &Handler{service: service}
}

func (h *Handler) GetSummary(c *gin.Context) {
	var query FinanceQuery
	if err := c.ShouldBindQuery(&query); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": "INVALID_QUERY", "message": "Invalid query parameters", "field_errors": gin.H{}})
		return
	}

	res, err := h.service.GetSummary(c.Request.Context(), query)
	if err != nil {
		writeServiceError(c, err)
		return
	}

	c.JSON(http.StatusOK, res)
}

func (h *Handler) GetBreakdown(c *gin.Context) {
	var query FinanceBreakdownQuery
	if err := c.ShouldBindQuery(&query); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": "INVALID_QUERY", "message": "Invalid query parameters", "field_errors": gin.H{}})
		return
	}

	res, err := h.service.GetBreakdown(c.Request.Context(), query)
	if err != nil {
		writeServiceError(c, err)
		return
	}

	c.JSON(http.StatusOK, res)
}

func writeServiceError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, ErrInvalidDateRange):
		c.JSON(http.StatusBadRequest, gin.H{"code": "INVALID_DATE_RANGE", "message": "Invalid finance query", "field_errors": gin.H{"start_date": "must not be after end_date"}})
	case errors.Is(err, ErrDateRangeTooLarge):
		c.JSON(http.StatusBadRequest, gin.H{"code": "DATE_RANGE_TOO_LARGE", "message": "Invalid finance query", "field_errors": gin.H{"end_date": "date range cannot exceed 366 days"}})
	case errors.Is(err, ErrOneSidedDate):
		c.JSON(http.StatusBadRequest, gin.H{"code": "DATE_RANGE_INCOMPLETE", "message": "Invalid finance query", "field_errors": gin.H{"start_date": "start_date and end_date must be provided together", "end_date": "start_date and end_date must be provided together"}})
	case errors.Is(err, ErrInvalidDateFormat):
		c.JSON(http.StatusBadRequest, gin.H{"code": "INVALID_DATE_FORMAT", "message": "Invalid finance query", "field_errors": gin.H{"start_date": "must use YYYY-MM-DD", "end_date": "must use YYYY-MM-DD"}})
	case errors.Is(err, ErrOwnerVenueMismatch):
		c.JSON(http.StatusBadRequest, gin.H{"code": "OWNER_VENUE_MISMATCH", "message": "Invalid finance query", "field_errors": gin.H{"venue_id": "venue does not belong to owner_profile_id"}})
	case errors.Is(err, ErrDuplicateLedgerDetected):
		c.JSON(http.StatusInternalServerError, gin.H{"code": "DUPLICATE_LEDGER_DETECTED", "message": "Finance integrity check failed", "field_errors": gin.H{}})
	case errors.Is(err, ErrFractionalLedgerDetected):
		c.JSON(http.StatusInternalServerError, gin.H{"code": "FRACTIONAL_LEDGER_DETECTED", "message": "Finance integrity check failed", "field_errors": gin.H{}})
	case errors.Is(err, ErrOrphanRefundDetected):
		c.JSON(http.StatusInternalServerError, gin.H{"code": "ORPHAN_REFUND_DETECTED", "message": "Finance integrity check failed", "field_errors": gin.H{}})
	case errors.Is(err, ErrRefundAmountMismatch):
		c.JSON(http.StatusInternalServerError, gin.H{"code": "REFUND_AMOUNT_MISMATCH", "message": "Finance integrity check failed", "field_errors": gin.H{}})
	case errors.Is(err, ErrOverflowDetected):
		c.JSON(http.StatusInternalServerError, gin.H{"code": "MONEY_OVERFLOW", "message": "Finance calculation could not be completed", "field_errors": gin.H{}})
	default:
		// Deliberately avoid serializing SQL, provider, or internal error text.
		c.JSON(http.StatusInternalServerError, gin.H{"code": "FINANCE_UNAVAILABLE", "message": "Failed to fetch finance data", "field_errors": gin.H{}})
	}
}

func RegisterRoutes(router *gin.Engine, authMiddleware gin.HandlerFunc, requireActiveUser gin.HandlerFunc, requireRole func(...string) gin.HandlerFunc, service Service) {
	h := NewHandler(service)
	adminFinance := router.Group("/admin/finance")
	adminFinance.Use(authMiddleware, requireActiveUser, requireRole("SUPER_ADMIN"))

	adminFinance.GET("/summary", h.GetSummary)
	adminFinance.GET("/breakdown", h.GetBreakdown)
}
