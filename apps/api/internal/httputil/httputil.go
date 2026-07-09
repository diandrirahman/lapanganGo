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

// GetActorUserID gets the user ID of the person making the request.
func GetActorUserID(c *gin.Context) string {
	return c.GetString("auth_actor_user_id")
}

// GetEffectiveOwnerUserID gets the owner user ID. For OWNER this is their own user ID, for STAFF this is their boss's user ID.
func GetEffectiveOwnerUserID(c *gin.Context) string {
	return c.GetString("auth_effective_owner_user_id")
}

// GetOwnerProfileID gets the owner profile ID context.
func GetOwnerProfileID(c *gin.Context) string {
	return c.GetString("auth_owner_profile_id")
}

// GetStaffVenueIDs gets the venue IDs that the staff is allowed to access.
func GetStaffVenueIDs(c *gin.Context) []string {
	if val, ok := c.Get("auth_staff_venue_ids"); ok {
		if ids, ok := val.([]string); ok {
			return ids
		}
	}
	return nil
}

// IsWorkspaceOwner returns true if the actor is the OWNER.
func IsWorkspaceOwner(c *gin.Context) bool {
	return c.GetBool("auth_is_owner")
}

// GetActorRole gets the actor role (OWNER or STAFF).
func GetActorRole(c *gin.Context) string {
	if IsWorkspaceOwner(c) {
		return "OWNER"
	}
	return "STAFF"
}

// GetStaffPermissions gets the permissions the staff has.
func GetStaffPermissions(c *gin.Context) []string {
	if val, ok := c.Get("auth_staff_permissions"); ok {
		if perms, ok := val.([]string); ok {
			return perms
		}
	}
	return nil
}

// OwnerContext groups all owner workspace related context variables.
type OwnerContext struct {
	ActorUserID          string
	ActorRole            string
	EffectiveOwnerUserID string
	OwnerProfileID       string
	IsOwner              bool
	AllowedVenueIDs      []string
}

// GetOwnerContext retrieves the unified owner workspace context.
func GetOwnerContext(c *gin.Context) (OwnerContext, bool) {
	actorID := GetActorUserID(c)
	if actorID == "" {
		return OwnerContext{}, false
	}

	return OwnerContext{
		ActorUserID:          actorID,
		ActorRole:            GetActorRole(c),
		EffectiveOwnerUserID: GetEffectiveOwnerUserID(c),
		OwnerProfileID:       GetOwnerProfileID(c),
		IsOwner:              IsWorkspaceOwner(c),
		AllowedVenueIDs:      GetStaffVenueIDs(c),
	}, true
}
