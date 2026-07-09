package middleware_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"lapangango-api/internal/middleware"

	"github.com/gin-gonic/gin"
)

type mockAuthRepo struct {
	status string
	err    error
}

func (m *mockAuthRepo) GetUserStatus(ctx context.Context, userID string) (string, error) {
	return m.status, m.err
}

func TestRequireActiveUser(t *testing.T) {
	gin.SetMode(gin.TestMode)

	tests := []struct {
		name           string
		userID         string
		repoStatus     string
		repoErr        error
		expectedStatus int
	}{
		{
			name:           "Active User",
			userID:         "user-1",
			repoStatus:     "ACTIVE",
			expectedStatus: http.StatusOK,
		},
		{
			name:           "Suspended User",
			userID:         "user-2",
			repoStatus:     "SUSPENDED",
			expectedStatus: http.StatusForbidden,
		},
		{
			name:           "Missing Auth User ID",
			userID:         "",
			expectedStatus: http.StatusUnauthorized,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := gin.New()
			mockRepo := &mockAuthRepo{status: tt.repoStatus, err: tt.repoErr}

			r.Use(func(c *gin.Context) {
				if tt.userID != "" {
					c.Set("auth_user_id", tt.userID)
				}
				c.Next()
			})
			r.Use(middleware.RequireActiveUser(mockRepo))
			r.GET("/test", func(c *gin.Context) {
				c.Status(http.StatusOK)
			})

			req, _ := http.NewRequest(http.MethodGet, "/test", nil)
			w := httptest.NewRecorder()
			r.ServeHTTP(w, req)

			if w.Code != tt.expectedStatus {
				t.Errorf("expected status %d, got %d", tt.expectedStatus, w.Code)
			}
		})
	}
}

func TestRequireRole(t *testing.T) {
	gin.SetMode(gin.TestMode)

	tests := []struct {
		name           string
		userRole       string
		requiredRole   string
		expectedStatus int
	}{
		{
			name:           "Matching Role",
			userRole:       "SUPER_ADMIN",
			requiredRole:   "SUPER_ADMIN",
			expectedStatus: http.StatusOK,
		},
		{
			name:           "Different Role",
			userRole:       "OWNER",
			requiredRole:   "SUPER_ADMIN",
			expectedStatus: http.StatusForbidden,
		},
		{
			name:           "Missing Role",
			userRole:       "",
			requiredRole:   "SUPER_ADMIN",
			expectedStatus: http.StatusUnauthorized,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := gin.New()
			r.Use(func(c *gin.Context) {
				if tt.userRole != "" {
					c.Set("auth_role", tt.userRole)
				}
				c.Next()
			})
			r.Use(middleware.RequireRole(tt.requiredRole))
			r.GET("/test", func(c *gin.Context) {
				c.Status(http.StatusOK)
			})

			req, _ := http.NewRequest(http.MethodGet, "/test", nil)
			w := httptest.NewRecorder()
			r.ServeHTTP(w, req)

			if w.Code != tt.expectedStatus {
				t.Errorf("expected status %d, got %d", tt.expectedStatus, w.Code)
			}
		})
	}
}
