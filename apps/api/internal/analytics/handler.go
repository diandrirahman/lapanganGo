package analytics

import (
	"log"
	"net/http"
	"time"

	"lapangango-api/internal/httputil"

	"github.com/gin-gonic/gin"
)

type Handler struct {
	service Service
}

func NewHandler(service Service) *Handler {
	return &Handler{service: service}
}

func parseDateQuery(c *gin.Context, paramName string) *time.Time {
	val := c.Query(paramName)
	if val == "" {
		return nil
	}
	t, err := time.Parse("2006-01-02", val)
	if err != nil {
		log.Printf("invalid date format for %s: %v", paramName, err)
		return nil
	}
	return &t
}

func parseVenueQuery(c *gin.Context) *string {
	val := c.Query("venue_id")
	if val == "" {
		return nil
	}
	return &val
}

func (h *Handler) GetBookingsTrend(c *gin.Context) {
	ownerCtx, ok := httputil.GetOwnerContext(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return
	}

	venueID := parseVenueQuery(c)
	startDate := parseDateQuery(c, "start_date")
	endDate := parseDateQuery(c, "end_date")

	resp, err := h.service.GetBookingsTrend(c.Request.Context(), ownerCtx, venueID, startDate, endDate)
	if err != nil {
		log.Printf("error getting bookings trend: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch bookings trend"})
		return
	}
	c.JSON(http.StatusOK, resp)
}

func (h *Handler) GetRevenueTrend(c *gin.Context) {
	ownerCtx, ok := httputil.GetOwnerContext(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return
	}

	venueID := parseVenueQuery(c)
	startDate := parseDateQuery(c, "start_date")
	endDate := parseDateQuery(c, "end_date")

	resp, err := h.service.GetRevenueTrend(c.Request.Context(), ownerCtx, venueID, startDate, endDate)
	if err != nil {
		log.Printf("error getting revenue trend: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch revenue trend"})
		return
	}
	c.JSON(http.StatusOK, resp)
}

func (h *Handler) GetStatusBreakdown(c *gin.Context) {
	ownerCtx, ok := httputil.GetOwnerContext(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return
	}

	venueID := parseVenueQuery(c)
	startDate := parseDateQuery(c, "start_date")
	endDate := parseDateQuery(c, "end_date")

	resp, err := h.service.GetStatusBreakdown(c.Request.Context(), ownerCtx, venueID, startDate, endDate)
	if err != nil {
		log.Printf("error getting status breakdown: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch status breakdown"})
		return
	}
	c.JSON(http.StatusOK, resp)
}

func (h *Handler) GetExpensesBreakdown(c *gin.Context) {
	ownerCtx, ok := httputil.GetOwnerContext(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return
	}

	venueID := parseVenueQuery(c)
	startDate := parseDateQuery(c, "start_date")
	endDate := parseDateQuery(c, "end_date")

	resp, err := h.service.GetExpensesBreakdown(c.Request.Context(), ownerCtx, venueID, startDate, endDate)
	if err != nil {
		log.Printf("error getting expenses breakdown: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch expenses breakdown"})
		return
	}
	c.JSON(http.StatusOK, resp)
}
