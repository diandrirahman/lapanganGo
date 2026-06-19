package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestRequireRole(t *testing.T) {
	gin.SetMode(gin.TestMode)

	tests := []struct {
		name           string
		contextRole    any
		contextRoleSet bool
		allowedRoles   []string
		wantStatus     int
	}{
		{
			name:           "allows matching role",
			contextRole:    "OWNER",
			contextRoleSet: true,
			allowedRoles:   []string{"OWNER"},
			wantStatus:     http.StatusOK,
		},
		{
			name:           "allows one of multiple roles",
			contextRole:    "SUPER_ADMIN",
			contextRoleSet: true,
			allowedRoles:   []string{"OWNER", "SUPER_ADMIN"},
			wantStatus:     http.StatusOK,
		},
		{
			name:           "forbids non matching role",
			contextRole:    "CUSTOMER",
			contextRoleSet: true,
			allowedRoles:   []string{"OWNER"},
			wantStatus:     http.StatusForbidden,
		},
		{
			name:           "rejects missing authenticated role",
			contextRoleSet: false,
			allowedRoles:   []string{"CUSTOMER"},
			wantStatus:     http.StatusUnauthorized,
		},
		{
			name:           "rejects invalid authenticated role value",
			contextRole:    123,
			contextRoleSet: true,
			allowedRoles:   []string{"CUSTOMER"},
			wantStatus:     http.StatusUnauthorized,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			router := gin.New()
			router.GET(
				"/protected",
				func(c *gin.Context) {
					if tt.contextRoleSet {
						c.Set("auth_role", tt.contextRole)
					}
				},
				RequireRole(tt.allowedRoles...),
				func(c *gin.Context) {
					c.JSON(http.StatusOK, gin.H{"message": "access granted"})
				},
			)

			recorder := httptest.NewRecorder()
			request := httptest.NewRequest(http.MethodGet, "/protected", nil)

			router.ServeHTTP(recorder, request)

			if recorder.Code != tt.wantStatus {
				t.Fatalf("expected status %d, got %d", tt.wantStatus, recorder.Code)
			}
		})
	}
}
