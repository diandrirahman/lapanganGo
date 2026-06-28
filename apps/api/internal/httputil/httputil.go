package httputil

import (
	"net/http"
	"regexp"

	"github.com/gin-gonic/gin"
)

var uuidRegex = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)

// GetAuthenticatedUserID retrieves the authenticated user ID from the context.
func GetAuthenticatedUserID(c *gin.Context) (string, bool) {
	userIDValue, exists := c.Get("auth_user_id")
	if !exists {
		return "", false
	}

	userID, ok := userIDValue.(string)
	if !ok || userID == "" {
		return "", false
	}

	return userID, true
}

// IsUUID checks if a string is a valid UUID.
func IsUUID(value string) bool {
	return uuidRegex.MatchString(value)
}

// GetUUIDParam retrieves a UUID parameter from the request URL and validates it.
func GetUUIDParam(c *gin.Context, name, message string) (string, bool) {
	value := c.Param(name)
	if !IsUUID(value) {
		c.JSON(http.StatusBadRequest, gin.H{
			"message": message,
		})
		return "", false
	}

	return value, true
}

// PaginationParams holds the page and limit query parameters.
type PaginationParams struct {
	Page  int `form:"page"`
	Limit int `form:"limit"`
}

// PaginatedResponse is the standard JSON structure for paginated lists.
type PaginatedResponse struct {
	Data       any `json:"data"`
	Page       int `json:"page"`
	Limit      int `json:"limit"`
	Total      int `json:"total"`
	TotalPages int `json:"total_pages"`
}

// GetPaginationParams extracts page and limit from the query string with sensible defaults.
func GetPaginationParams(c *gin.Context) PaginationParams {
	var params PaginationParams
	if err := c.ShouldBindQuery(&params); err != nil {
		// Ignore binding errors and use defaults
	}

	if params.Page <= 0 {
		params.Page = 1
	}

	if params.Limit <= 0 {
		params.Limit = 10
	} else if params.Limit > 100 {
		params.Limit = 100
	}

	return params
}

// NewPaginatedResponse creates a new PaginatedResponse.
func NewPaginatedResponse(data any, total, page, limit int) PaginatedResponse {
	totalPages := 0
	if limit > 0 {
		totalPages = total / limit
		if total%limit != 0 {
			totalPages++
		}
	}

	if data == nil {
		data = []any{}
	}

	return PaginatedResponse{
		Data:       data,
		Page:       page,
		Limit:      limit,
		Total:      total,
		TotalPages: totalPages,
	}
}
