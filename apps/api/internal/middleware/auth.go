package middleware

import (
	"net/http"
	"strings"

	"lapangango-api/internal/auth"

	"github.com/gin-gonic/gin"
)

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
