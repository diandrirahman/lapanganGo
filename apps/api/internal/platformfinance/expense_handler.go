package platformfinance

import (
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"lapangango-api/internal/httputil"
)

type ExpenseHandler struct {
	expenseService ExpenseService
	journalService JournalReadService
}

func NewExpenseHandler(expenseService ExpenseService, journalService JournalReadService) *ExpenseHandler {
	return &ExpenseHandler{expenseService: expenseService, journalService: journalService}
}

func (h *ExpenseHandler) ListExpenses(c *gin.Context) {
	var query ExpenseListQuery
	if err := c.ShouldBindQuery(&query); err != nil {
		writeExpenseHTTPError(c, ErrExpenseValidation)
		return
	}
	page, err := h.expenseService.ListExpenses(c.Request.Context(), query)
	if err != nil {
		writeExpenseHTTPError(c, err)
		return
	}
	c.JSON(http.StatusOK, page)
}

func (h *ExpenseHandler) CreateExpense(c *gin.Context) {
	key := strings.TrimSpace(c.GetHeader("Idempotency-Key"))
	if key == "" {
		writeExpenseHTTPError(c, ErrExpenseMissingKey)
		return
	}
	if len(key) > 255 {
		writeExpenseHTTPError(c, ErrExpenseInvalidKey)
		return
	}
	var req CreateExpenseRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": "INVALID_BODY", "message": "Invalid request body", "field_errors": gin.H{}})
		return
	}
	actorID, ok := httputil.GetAuthenticatedUserID(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"code": "UNAUTHORIZED", "message": "Missing or invalid authentication"})
		return
	}
	item, replayed, err := h.expenseService.CreateDraft(c.Request.Context(), req, key, actorID, c.GetString("auth_role"), c.ClientIP(), c.GetHeader("User-Agent"))
	if err != nil {
		writeExpenseHTTPError(c, err)
		return
	}
	if replayed {
		c.Header("Idempotent-Replay", "true")
		c.JSON(http.StatusOK, item)
		return
	}
	c.JSON(http.StatusCreated, item)
}

func (h *ExpenseHandler) ListJournals(c *gin.Context) {
	var raw struct {
		StartDate   string `form:"start_date"`
		EndDate     string `form:"end_date"`
		EventType   string `form:"event_type"`
		AccountCode string `form:"account_code"`
		Page        int    `form:"page"`
		Limit       int    `form:"limit"`
	}
	if err := c.ShouldBindQuery(&raw); err != nil {
		writeExpenseHTTPError(c, ErrInvalidJournalReadQuery)
		return
	}
	query := JournalListQuery{EventType: strings.ToUpper(strings.TrimSpace(raw.EventType)), AccountCode: strings.ToUpper(strings.TrimSpace(raw.AccountCode)), Page: raw.Page, Limit: raw.Limit}
	if raw.StartDate != "" {
		value, err := time.Parse("2006-01-02", raw.StartDate)
		if err != nil {
			writeExpenseHTTPError(c, ErrInvalidJournalReadQuery)
			return
		}
		value = value.Add(-7 * time.Hour)
		query.EffectiveFrom = &value
	}
	if raw.EndDate != "" {
		value, err := time.Parse("2006-01-02", raw.EndDate)
		if err != nil {
			writeExpenseHTTPError(c, ErrInvalidJournalReadQuery)
			return
		}
		value = value.Add(17 * time.Hour)
		query.EffectiveTo = &value
	}
	page, err := h.journalService.ListJournals(c.Request.Context(), query)
	if err != nil {
		writeExpenseHTTPError(c, err)
		return
	}
	c.JSON(http.StatusOK, page)
}

func writeExpenseHTTPError(c *gin.Context, err error) {
	status := http.StatusInternalServerError
	code, message := "EXPENSE_UNAVAILABLE", "Expense data is unavailable"
	fields := gin.H{}
	var validationErr *ExpenseValidationError
	switch {
	case errors.Is(err, ErrExpenseMissingKey):
		status, code, message = http.StatusBadRequest, "MISSING_IDEMPOTENCY_KEY", "Idempotency-Key header is required"
	case errors.Is(err, ErrExpenseInvalidKey):
		status, code, message = http.StatusBadRequest, "INVALID_IDEMPOTENCY_KEY", "Idempotency-Key cannot exceed 255 characters"
	case errors.As(err, &validationErr):
		status, code, message, fields = http.StatusBadRequest, "VALIDATION_ERROR", "Expense request is invalid", gin.H{}
		for key, value := range validationErr.Fields {
			fields[key] = value
		}
	case errors.Is(err, ErrExpenseValidation), errors.Is(err, ErrInvalidJournalReadQuery):
		status, code, message = http.StatusBadRequest, "INVALID_QUERY", "Invalid finance query"
	case errors.Is(err, ErrExpenseConflict):
		status, code, message = http.StatusConflict, "CONFLICT", "Request conflicts with an existing finance operation"
	}
	c.JSON(status, gin.H{"code": code, "message": message, "field_errors": fields})
}

func RegisterExpenseRoutes(router *gin.Engine, authMiddleware gin.HandlerFunc, requireActiveUser gin.HandlerFunc, requireRole func(...string) gin.HandlerFunc, expenseService ExpenseService, journalService JournalReadService) {
	h := NewExpenseHandler(expenseService, journalService)
	group := router.Group("/admin/finance")
	group.Use(authMiddleware, requireActiveUser, requireRole("SUPER_ADMIN"))
	group.GET("/expenses", h.ListExpenses)
	group.POST("/expenses", h.CreateExpense)
	group.GET("/journals", h.ListJournals)
}
