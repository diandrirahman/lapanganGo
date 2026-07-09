package middleware

import (
	"context"
	"net/http"
	"strings"

	"lapangango-api/internal/auth"

	"github.com/gin-gonic/gin"
)

type UserStatusChecker interface {
	GetUserStatus(ctx context.Context, userID string) (string, error)
}

func Auth(tokenService *auth.TokenService) gin.HandlerFunc {
	return func(c *gin.Context) {
		authHeader := c.GetHeader("Authorization")
		if authHeader == "" {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
				"message": "Authorization header is required",
			})
			return
		}

		parts := strings.SplitN(authHeader, " ", 2)
		if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") || strings.TrimSpace(parts[1]) == "" {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
				"message": "Authorization header must use Bearer token",
			})
			return
		}

		claims, err := tokenService.Parse(strings.TrimSpace(parts[1]))
		if err != nil {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
				"message": "Invalid or expired token",
			})
			return
		}

		c.Set("auth_user_id", claims.UserID)
		c.Set("auth_email", claims.Email)
		c.Set("auth_role", claims.Role)

		c.Next()
	}
}

func RequireActiveUser(repo UserStatusChecker) gin.HandlerFunc {
	return func(c *gin.Context) {
		userID, exists := c.Get("auth_user_id")
		if !exists {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"message": "Unauthorized"})
			return
		}

		userIDStr, ok := userID.(string)
		if !ok || userIDStr == "" {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"message": "Unauthorized"})
			return
		}

		status, err := repo.GetUserStatus(c.Request.Context(), userIDStr)
		if err != nil {
			c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"message": "Failed to verify user status"})
			return
		}

		if status != "ACTIVE" {
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"message": "User account is suspended or inactive"})
			return
		}

		c.Next()
	}
}
