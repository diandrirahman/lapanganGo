package platformfinance_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"

	"lapangango-api/internal/platformfinance"
)

func mockAuth(t *testing.T, expectedToken string) gin.HandlerFunc {
	return func(c *gin.Context) {
		reqToken := c.GetHeader("Authorization")
		if reqToken == "" {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"message": "missing token"})
			return
		}
		if reqToken != expectedToken {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"message": "invalid token"})
			return
		}
		c.Next()
	}
}

func mockRequireActiveUser(t *testing.T, active bool) gin.HandlerFunc {
	return func(c *gin.Context) {
		if !active {
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"message": "suspended"})
			return
		}
		c.Next()
	}
}

func mockRequireRole(t *testing.T, expectedRole, actualRole string) func(...string) gin.HandlerFunc {
	return func(roles ...string) gin.HandlerFunc {
		return func(c *gin.Context) {
			expectedRoleRequested := false
			for _, role := range roles {
				if role == expectedRole {
					expectedRoleRequested = true
					break
				}
			}
			if !expectedRoleRequested || actualRole != expectedRole {
				c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"message": "forbidden"})
				return
			}
			c.Next()
		}
	}
}

func TestHandler_Integration_Auth(t *testing.T) {
	gin.SetMode(gin.TestMode)
	repo := &mockRepo{err: nil}
	svc := platformfinance.NewService(repo)

	tests := []struct {
		name           string
		token          string
		active         bool
		actualRole     string
		expectedStatus int
	}{
		{"tanpa token", "", true, "SUPER_ADMIN", http.StatusUnauthorized},
		{"invalid token", "Bearer invalid", true, "SUPER_ADMIN", http.StatusUnauthorized},
		{"customer", "Bearer valid", true, "CUSTOMER", http.StatusForbidden},
		{"owner", "Bearer valid", true, "OWNER", http.StatusForbidden},
		{"staff", "Bearer valid", true, "STAFF", http.StatusForbidden},
		{"suspended superadmin", "Bearer valid", false, "SUPER_ADMIN", http.StatusForbidden},
		{"active superadmin", "Bearer valid", true, "SUPER_ADMIN", http.StatusOK},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := gin.Default()

			authMW := mockAuth(t, "Bearer valid")
			if tt.name == "tanpa token" {
				authMW = func(c *gin.Context) {
					if c.GetHeader("Authorization") == "" {
						c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"message": "missing token"})
						return
					}
					c.Next()
				}
			}

			platformfinance.RegisterRoutes(r, authMW, mockRequireActiveUser(t, tt.active), mockRequireRole(t, "SUPER_ADMIN", tt.actualRole), svc)

			req, _ := http.NewRequest(http.MethodGet, "/admin/finance/summary?start_date=2026-06-01&end_date=2026-06-30", nil)
			if tt.token != "" {
				req.Header.Set("Authorization", tt.token)
			}
			w := httptest.NewRecorder()
			r.ServeHTTP(w, req)

			assert.Equal(t, tt.expectedStatus, w.Code)
		})
	}
}
