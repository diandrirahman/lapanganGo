package finance

import (
	"log"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"lapangango-api/internal/audit"
	"lapangango-api/internal/httputil"
	"lapangango-api/internal/middleware"
)

type Handler struct {
	service      Service
	auditService audit.Service
}

func NewHandler(service Service, auditService audit.Service) *Handler {
	return &Handler{service: service, auditService: auditService}
}

func (h *Handler) GetFinanceSummary(c *gin.Context) {
	ownerCtx, ok := httputil.GetOwnerContext(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return
	}

	var req FinanceSummaryQuery
	if err := c.ShouldBindQuery(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	result, err := h.service.GetFinanceSummary(c.Request.Context(), ownerCtx, req)
	if err != nil {
		log.Printf("error getting finance summary: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch finance summary"})
		return
	}

	c.JSON(http.StatusOK, result)
}

func (h *Handler) GetTransactions(c *gin.Context) {
	ownerCtx, ok := httputil.GetOwnerContext(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return
	}

	var req TransactionQuery
	if err := c.ShouldBindQuery(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	result, err := h.service.GetTransactions(c.Request.Context(), ownerCtx, req)
	if err != nil {
		log.Printf("error getting finance transactions: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch finance transactions"})
		return
	}

	c.JSON(http.StatusOK, result)
}

func (h *Handler) CreateTransaction(c *gin.Context) {
	ownerCtx, ok := httputil.GetOwnerContext(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return
	}

	var req CreateTransactionRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	tx, err := h.service.CreateTransaction(c.Request.Context(), ownerCtx, req)
	if err != nil {
		log.Printf("error creating finance transaction: %v", err)
		if strings.Contains(err.Error(), "forbidden") {
			c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create transaction"})
		return
	}

	ip := c.ClientIP()
	ua := c.Request.UserAgent()
	h.auditService.Record(c.Request.Context(), audit.CreateAuditLogParams{
		OwnerProfileID: ownerCtx.OwnerProfileID,
		ActorUserID:    ownerCtx.ActorUserID,
		ActorRole:      ownerCtx.ActorRole,
		Action:         audit.ActionFinanceCreated,
		EntityType:     audit.EntityFinanceTransaction,
		EntityID:       &tx.ID,
		Metadata: map[string]any{
			"venue_id":         req.VenueID,
			"type":             req.Type,
			"category":         req.Category,
			"amount":           req.Amount,
			"transaction_date": req.TransactionDate,
		},
		IPAddress: &ip,
		UserAgent: &ua,
	})

	c.JSON(http.StatusCreated, tx)
}

func (h *Handler) UpdateTransaction(c *gin.Context) {
	ownerCtx, ok := httputil.GetOwnerContext(c)
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

	beforeTx, err := h.service.GetTransaction(c.Request.Context(), id, ownerCtx.EffectiveOwnerUserID)
	if err != nil {
		// Just ignore if we can't find it for audit
	}

	tx, err := h.service.UpdateTransaction(c.Request.Context(), id, ownerCtx, req)
	if err != nil {
		log.Printf("error updating finance transaction: %v", err)
		if strings.Contains(err.Error(), "forbidden") {
			c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	ip := c.ClientIP()
	ua := c.Request.UserAgent()

	h.auditService.Record(c.Request.Context(), audit.CreateAuditLogParams{
		OwnerProfileID: ownerCtx.OwnerProfileID,
		ActorUserID:    ownerCtx.ActorUserID,
		ActorRole:      ownerCtx.ActorRole,
		Action:         audit.ActionFinanceUpdated,
		EntityType:     audit.EntityFinanceTransaction,
		EntityID:       &tx.ID,
		Metadata: map[string]any{
			"before": beforeTx,
			"after":  tx,
			"changed_fields": []string{},
		},
		IPAddress: &ip,
		UserAgent: &ua,
	})

	c.JSON(http.StatusOK, tx)
}

func (h *Handler) DeleteTransaction(c *gin.Context) {
	ownerCtx, ok := httputil.GetOwnerContext(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return
	}

	id := c.Param("id")

	beforeTx, err := h.service.GetTransaction(c.Request.Context(), id, ownerCtx.EffectiveOwnerUserID)
	if err != nil {
		// Ignore
	}

	err = h.service.DeleteTransaction(c.Request.Context(), id, ownerCtx)
	if err != nil {
		log.Printf("error deleting finance transaction: %v", err)
		if strings.Contains(err.Error(), "forbidden") {
			c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	ip := c.ClientIP()
	ua := c.Request.UserAgent()
	h.auditService.Record(c.Request.Context(), audit.CreateAuditLogParams{
		OwnerProfileID: ownerCtx.OwnerProfileID,
		ActorUserID:    ownerCtx.ActorUserID,
		ActorRole:      ownerCtx.ActorRole,
		Action:         audit.ActionFinanceDeleted,
		EntityType:     audit.EntityFinanceTransaction,
		EntityID:       &id,
		Metadata: map[string]any{
			"deleted_transaction": beforeTx,
		},
		IPAddress: &ip,
		UserAgent: &ua,
	})

	c.JSON(http.StatusOK, gin.H{"message": "Transaction deleted successfully"})
}

func (h *Handler) RegisterRoutes(r *gin.Engine, authMiddleware gin.HandlerFunc, requireActiveUser gin.HandlerFunc, ownerWorkspaceMiddleware gin.HandlerFunc) {
	ownerFinance := r.Group("/owner/finance")
	ownerFinance.Use(authMiddleware, requireActiveUser, ownerWorkspaceMiddleware)
	{
		ownerFinance.GET("/summary", middleware.RequireOwnerPermission("FINANCE_READ"), h.GetFinanceSummary)
		ownerFinance.GET("/transactions", middleware.RequireOwnerPermission("FINANCE_READ"), h.GetTransactions)
		ownerFinance.POST("/transactions", middleware.RequireOwnerPermission("FINANCE_WRITE"), h.CreateTransaction)
		ownerFinance.PATCH("/transactions/:id", middleware.RequireOwnerPermission("FINANCE_WRITE"), h.UpdateTransaction)
		ownerFinance.DELETE("/transactions/:id", middleware.RequireOwnerPermission("FINANCE_WRITE"), h.DeleteTransaction)
	}
}
