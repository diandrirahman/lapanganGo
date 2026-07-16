package admin_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"lapangango-api/internal/admin"
)

type auditReadService struct {
	query admin.AuditLogQuery
	res   admin.PaginatedResponse
	err   error
}

func (s *auditReadService) GetUsers(context.Context, admin.UserQuery) (admin.PaginatedResponse, error) {
	return admin.PaginatedResponse{}, nil
}
func (s *auditReadService) GetOwners(context.Context, admin.OwnerQuery) (admin.PaginatedResponse, error) {
	return admin.PaginatedResponse{}, nil
}
func (s *auditReadService) UpdateOwnerStatus(context.Context, string, string, string) error {
	return nil
}
func (s *auditReadService) GetVenues(context.Context, admin.VenueQuery) (admin.PaginatedResponse, error) {
	return admin.PaginatedResponse{}, nil
}
func (s *auditReadService) UpdateVenueStatus(context.Context, string, string, string) error {
	return nil
}
func (s *auditReadService) GetVenueOwnerProfileID(context.Context, string) (string, error) {
	return "", nil
}
func (s *auditReadService) GetAuditLogs(_ context.Context, query admin.AuditLogQuery) (admin.PaginatedResponse, error) {
	s.query = query
	return s.res, s.err
}
func (s *auditReadService) GetDashboardStats(context.Context) (admin.DashboardStatsResponse, error) {
	return admin.DashboardStatsResponse{}, nil
}

func TestGetAuditLogsReadContract(t *testing.T) {
	tests := []struct {
		name       string
		query      string
		wantStatus int
		wantScope  string
	}{
		{name: "default owner", query: "", wantStatus: http.StatusOK, wantScope: "OWNER"},
		{name: "platform scope", query: "scope=platform&entity_type=platform_commercial_term&action=platform_commercial_term_created&limit=100", wantStatus: http.StatusOK, wantScope: "PLATFORM"},
		{name: "all scope", query: "scope=ALL", wantStatus: http.StatusOK, wantScope: "ALL"},
		{name: "invalid scope", query: "scope=INVALID", wantStatus: http.StatusBadRequest},
		{name: "invalid platform entity", query: "scope=PLATFORM&entity_type=BOOKING", wantStatus: http.StatusBadRequest},
		{name: "invalid platform action", query: "scope=PLATFORM&action=BOOKING_COMPLETED", wantStatus: http.StatusBadRequest},
		{name: "invalid limit", query: "limit=101", wantStatus: http.StatusBadRequest},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			service := &auditReadService{res: admin.PaginatedResponse{Data: []admin.AuditLogResponse{}}}
			handler := admin.NewHandler(service)
			router := gin.New()
			router.GET("/admin/audit-logs", handler.GetAuditLogs)

			req := httptest.NewRequest(http.MethodGet, "/admin/audit-logs"+querySuffix(tt.query), nil)
			resp := httptest.NewRecorder()
			router.ServeHTTP(resp, req)
			if resp.Code != tt.wantStatus {
				t.Fatalf("expected status %d, got %d: %s", tt.wantStatus, resp.Code, resp.Body.String())
			}
			if tt.wantScope != "" && service.query.Scope != tt.wantScope {
				t.Fatalf("expected normalized scope %q, got %q", tt.wantScope, service.query.Scope)
			}
		})
	}
}

func TestGetAuditLogsSanitizedInternalError(t *testing.T) {
	service := &auditReadService{err: errors.New("postgres://secret-host/raw error")}
	handler := admin.NewHandler(service)
	router := gin.New()
	router.GET("/admin/audit-logs", handler.GetAuditLogs)

	req := httptest.NewRequest(http.MethodGet, "/admin/audit-logs?scope=PLATFORM", nil)
	resp := httptest.NewRecorder()
	router.ServeHTTP(resp, req)
	if resp.Code != http.StatusInternalServerError {
		t.Fatalf("expected status 500, got %d", resp.Code)
	}
	var body map[string]any
	if err := json.Unmarshal(resp.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if body["code"] != "INTERNAL_ERROR" || body["message"] != "Failed to fetch audit logs" {
		t.Fatalf("unexpected sanitized error body: %#v", body)
	}
	if string(resp.Body.Bytes()) == "" || containsText(resp.Body.String(), "postgres://") {
		t.Fatal("internal database details leaked")
	}
}

func TestAdminAuditLogsAuthMatrix(t *testing.T) {
	tests := []struct {
		name           string
		role           string
		status         string
		expectedStatus int
	}{
		{name: "anonymous", expectedStatus: http.StatusUnauthorized},
		{name: "active customer", role: "CUSTOMER", status: "ACTIVE", expectedStatus: http.StatusForbidden},
		{name: "active owner", role: "OWNER", status: "ACTIVE", expectedStatus: http.StatusForbidden},
		{name: "active staff", role: "STAFF", status: "ACTIVE", expectedStatus: http.StatusForbidden},
		{name: "suspended super admin", role: "SUPER_ADMIN", status: "SUSPENDED", expectedStatus: http.StatusForbidden},
		{name: "active super admin", role: "SUPER_ADMIN", status: "ACTIVE", expectedStatus: http.StatusOK},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			service := &auditReadService{res: admin.PaginatedResponse{Data: []admin.AuditLogResponse{}}}
			handler := admin.NewHandler(service)
			router := gin.New()
			auth := func(c *gin.Context) {
				if tt.role == "" {
					c.AbortWithStatus(http.StatusUnauthorized)
					return
				}
				c.Set("auth_role", tt.role)
				c.Next()
			}
			requireActive := func(c *gin.Context) {
				if tt.status != "ACTIVE" {
					c.AbortWithStatus(http.StatusForbidden)
					return
				}
				c.Next()
			}
			handler.RegisterRoutes(router, auth, requireActive)

			req := httptest.NewRequest(http.MethodGet, "/admin/audit-logs?scope=PLATFORM", nil)
			resp := httptest.NewRecorder()
			router.ServeHTTP(resp, req)
			if resp.Code != tt.expectedStatus {
				t.Fatalf("expected status %d, got %d: %s", tt.expectedStatus, resp.Code, resp.Body.String())
			}
		})
	}
}

func querySuffix(query string) string {
	if query == "" {
		return ""
	}
	return "?" + query
}

func containsText(value, needle string) bool {
	for i := 0; i+len(needle) <= len(value); i++ {
		if value[i:i+len(needle)] == needle {
			return true
		}
	}
	return false
}
