package middleware

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

func RequireRole(allowedRoles ...string) gin.HandlerFunc {
	allowedRoleMap := make(map[string]bool, len(allowedRoles))
	for _, role := range allowedRoles {
		allowedRoleMap[role] = true
	}

	return func(c *gin.Context) {
		roleValue, exists := c.Get("auth_role")
		if !exists {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
				"message": "Authenticated role is required",
			})
			return
		}

		role, ok := roleValue.(string)
		if !ok || role == "" {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
				"message": "Authenticated role is invalid",
			})
			return
		}

		if !allowedRoleMap[role] {
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{
				"message": "You do not have permission to access this resource",
			})
			return
		}

		c.Next()
	}
}
