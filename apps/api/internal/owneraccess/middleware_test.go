package owneraccess_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"lapangango-api/internal/middleware"
	"lapangango-api/internal/owneraccess"

	"github.com/gin-gonic/gin"
)

type mockOwnerRepo struct{}

func (m *mockOwnerRepo) GetOwnerContextByUserID(ctx context.Context, userID string) (owneraccess.OwnerContextInfo, error) {
	return owneraccess.OwnerContextInfo{
		OwnerUserStatus: "ACTIVE",
	}, nil
}
func (m *mockOwnerRepo) GetStaffContextByUserID(ctx context.Context, userID string) (owneraccess.StaffContextInfo, error) {
	return owneraccess.StaffContextInfo{}, nil
}

func TestRequireOwnerWorkspace(t *testing.T) {
	gin.SetMode(gin.TestMode)

	tests := []struct {
		name           string
		userRole       string
		userID         string
		expectedStatus int
	}{
		{
			name:           "Super Admin Rejected",
			userRole:       "SUPER_ADMIN",
			userID:         "admin-1",
			expectedStatus: http.StatusForbidden,
		},
		{
			name:           "Customer Rejected",
			userRole:       "CUSTOMER",
			userID:         "cust-1",
			expectedStatus: http.StatusForbidden,
		},
		{
			name:           "Owner Accepted",
			userRole:       "OWNER",
			userID:         "owner-1",
			expectedStatus: http.StatusOK,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := gin.New()

			r.Use(func(c *gin.Context) {
				if tt.userID != "" {
					c.Set("auth_user_id", tt.userID)
					c.Set("auth_role", tt.userRole)
				}
				c.Next()
			})
			r.Use(middleware.OwnerWorkspaceAccess(&mockOwnerRepo{}))
			r.GET("/owner-test", func(c *gin.Context) {
				c.Status(http.StatusOK)
			})

			req, _ := http.NewRequest(http.MethodGet, "/owner-test", nil)
			w := httptest.NewRecorder()
			r.ServeHTTP(w, req)

			if w.Code != tt.expectedStatus {
				t.Errorf("expected status %d, got %d", tt.expectedStatus, w.Code)
			}
		})
	}
}
