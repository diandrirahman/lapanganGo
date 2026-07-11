package middleware

import (
	"net/http"
	"os"
	"strings"

	"github.com/gin-gonic/gin"
)

// CORS is a custom middleware to handle Cross-Origin Resource Sharing
func CORS() gin.HandlerFunc {
	allowedOriginsStr := os.Getenv("CORS_ALLOWED_ORIGINS")
	if allowedOriginsStr == "" {
		allowedOriginsStr = "http://localhost:5173,http://localhost:5174,http://localhost:3000"
	}

	originsList := strings.Split(allowedOriginsStr, ",")
	allowedOrigins := make(map[string]bool)
	for _, o := range originsList {
		allowedOrigins[strings.TrimSpace(o)] = true
	}

	return func(c *gin.Context) {
		origin := c.Request.Header.Get("Origin")
		if allowedOrigins[origin] {
			c.Writer.Header().Set("Access-Control-Allow-Origin", origin)
		} else if len(allowedOrigins) > 0 && origin == "" {
			// Some tools like curl don't send Origin
			c.Writer.Header().Set("Access-Control-Allow-Origin", originsList[0])
		}

		c.Writer.Header().Set("Access-Control-Allow-Credentials", "true")
		c.Writer.Header().Set("Access-Control-Allow-Headers", "Content-Type, Content-Length, Accept-Encoding, X-CSRF-Token, Authorization, X-Request-Deadline-Ms, accept, origin, Cache-Control, X-Requested-With")
		c.Writer.Header().Set("Access-Control-Allow-Methods", "POST, OPTIONS, GET, PUT, PATCH, DELETE")

		if c.Request.Method == "OPTIONS" {
			c.AbortWithStatus(http.StatusNoContent)
			return
		}

		c.Next()
	}
}
