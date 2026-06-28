package availability

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
	"lapangango-api/internal/httputil"
)

type Handler struct {
	service *Service
}

func NewHandler(service *Service) *Handler {
	return &Handler{service: service}
}

func (h *Handler) RegisterRoutes(router *gin.Engine) {
	router.GET("/courts/:id/availability", h.GetAvailability)
}

func (h *Handler) GetAvailability(c *gin.Context) {
	courtID, ok := httputil.GetUUIDParam(c, "id", "Court ID must be a valid UUID")
	if !ok {
		return
	}

	availability, err := h.service.GetAvailability(c.Request.Context(), courtID, c.Query("date"))
	if err != nil {
		respondAvailabilityError(c, err)
		return
	}

	c.JSON(http.StatusOK, availability)
}

func respondAvailabilityError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, ErrCourtNotFound):
		c.JSON(http.StatusNotFound, gin.H{
			"message": "Court not found",
		})
	case errors.Is(err, ErrInvalidAvailabilityDate):
		c.JSON(http.StatusBadRequest, gin.H{
			"message": "Date must use YYYY-MM-DD format",
		})
	case errors.Is(err, ErrInvalidOperatingHours):
		c.JSON(http.StatusInternalServerError, gin.H{
			"message": "Court operating hours are invalid",
		})
	default:
		c.JSON(http.StatusInternalServerError, gin.H{
			"message": "Failed to get availability",
		})
	}
}
