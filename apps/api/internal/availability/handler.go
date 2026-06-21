package availability

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
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
	courtID, ok := getUUIDParam(c, "id", "Court ID must be a valid UUID")
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

func getUUIDParam(c *gin.Context, name, message string) (string, bool) {
	value := c.Param(name)
	if !isUUID(value) {
		c.JSON(http.StatusBadRequest, gin.H{
			"message": message,
		})
		return "", false
	}

	return value, true
}

func isUUID(value string) bool {
	if len(value) != 36 {
		return false
	}

	for i, char := range value {
		switch i {
		case 8, 13, 18, 23:
			if char != '-' {
				return false
			}
		default:
			if !isHex(char) {
				return false
			}
		}
	}

	return true
}

func isHex(char rune) bool {
	return (char >= '0' && char <= '9') ||
		(char >= 'a' && char <= 'f') ||
		(char >= 'A' && char <= 'F')
}
