package finance

import (
	"log"
	"net/http"

	"lapangango-api/internal/httputil"

	"github.com/gin-gonic/gin"
)

type Handler struct {
	service Service
}

func NewHandler(service Service) *Handler {
	return &Handler{service: service}
}

func (h *Handler) GetFinanceSummary(c *gin.Context) {
	ownerID, ok := httputil.GetAuthenticatedUserID(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return
	}

	var req FinanceSummaryQuery
	if err := c.ShouldBindQuery(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	result, err := h.service.GetFinanceSummary(c.Request.Context(), ownerID, req)
	if err != nil {
		log.Printf("error getting finance summary: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch finance summary"})
		return
	}

	c.JSON(http.StatusOK, result)
}

func (h *Handler) GetTransactions(c *gin.Context) {
	ownerID, ok := httputil.GetAuthenticatedUserID(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return
	}

	var req TransactionQuery
	if err := c.ShouldBindQuery(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	result, err := h.service.GetTransactions(c.Request.Context(), ownerID, req)
	if err != nil {
		log.Printf("error getting finance transactions: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch finance transactions"})
		return
	}

	c.JSON(http.StatusOK, result)
}

func (h *Handler) CreateTransaction(c *gin.Context) {
	ownerID, ok := httputil.GetAuthenticatedUserID(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return
	}

	var req CreateTransactionRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	tx, err := h.service.CreateTransaction(c.Request.Context(), ownerID, req)
	if err != nil {
		log.Printf("error creating finance transaction: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create transaction"})
		return
	}

	c.JSON(http.StatusCreated, tx)
}

func (h *Handler) UpdateTransaction(c *gin.Context) {
	ownerID, ok := httputil.GetAuthenticatedUserID(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return
	}

	id := c.Param("id")

	var req UpdateTransactionRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	tx, err := h.service.UpdateTransaction(c.Request.Context(), id, ownerID, req)
	if err != nil {
		log.Printf("error updating finance transaction: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, tx)
}

func (h *Handler) DeleteTransaction(c *gin.Context) {
	ownerID, ok := httputil.GetAuthenticatedUserID(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return
	}

	id := c.Param("id")

	err := h.service.DeleteTransaction(c.Request.Context(), id, ownerID)
	if err != nil {
		log.Printf("error deleting finance transaction: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Transaction deleted successfully"})
}

func (h *Handler) RegisterRoutes(r *gin.Engine, authMiddleware gin.HandlerFunc, roleMiddleware gin.HandlerFunc) {
	ownerFinance := r.Group("/owner/finance")
	ownerFinance.Use(authMiddleware, roleMiddleware)
	{
		ownerFinance.GET("/summary", h.GetFinanceSummary)
		ownerFinance.GET("/transactions", h.GetTransactions)
		ownerFinance.POST("/transactions", h.CreateTransaction)
		ownerFinance.PATCH("/transactions/:id", h.UpdateTransaction)
		ownerFinance.DELETE("/transactions/:id", h.DeleteTransaction)
	}
}
