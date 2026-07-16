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
	"lapangango-api/internal/auth"
	"lapangango-api/internal/middleware"
)

type auditReadService struct {
	query admin.AuditLogQuery
	res   admin.PaginatedResponse
	err   error
	calls int
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
	s.calls++
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
			wantCalls := 0
			if tt.wantStatus == http.StatusOK {
				wantCalls = 1
			}
			if service.calls != wantCalls {
				t.Fatalf("expected service calls %d, got %d", wantCalls, service.calls)
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
		name                 string
		userID               string
		role                 string
		status               string
		expectedStatus       int
		expectedStatusChecks int
	}{
		{name: "anonymous", expectedStatus: http.StatusUnauthorized},
		{name: "active customer", userID: "customer-1", role: "CUSTOMER", status: "ACTIVE", expectedStatus: http.StatusForbidden, expectedStatusChecks: 1},
		{name: "active owner", userID: "owner-1", role: "OWNER", status: "ACTIVE", expectedStatus: http.StatusForbidden, expectedStatusChecks: 1},
		{name: "active staff", userID: "staff-1", role: "STAFF", status: "ACTIVE", expectedStatus: http.StatusForbidden, expectedStatusChecks: 1},
		{name: "suspended super admin", userID: "super-admin-suspended", role: "SUPER_ADMIN", status: "SUSPENDED", expectedStatus: http.StatusForbidden, expectedStatusChecks: 1},
		{name: "active super admin", userID: "super-admin-active", role: "SUPER_ADMIN", status: "ACTIVE", expectedStatus: http.StatusOK, expectedStatusChecks: 1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			service := &auditReadService{res: admin.PaginatedResponse{Data: []admin.AuditLogResponse{}}}
			handler := admin.NewHandler(service)
			router := gin.New()
			statusRepo := &auditStatusRepo{statuses: map[string]string{}}
			tokenService := auth.NewTokenService("task-2c-04-test-secret", 1)
			handler.RegisterRoutes(router, middleware.Auth(tokenService), middleware.RequireActiveUser(statusRepo))

			req := httptest.NewRequest(http.MethodGet, "/admin/audit-logs?scope=PLATFORM", nil)
			if tt.role != "" {
				statusRepo.statuses[tt.userID] = tt.status
				token, err := tokenService.Generate(auth.UserResponse{
					ID:     tt.userID,
					Email:  tt.userID + "@example.test",
					Role:   tt.role,
					Status: tt.status,
				})
				if err != nil {
					t.Fatalf("generate test token: %v", err)
				}
				req.Header.Set("Authorization", "Bearer "+token)
			}
			resp := httptest.NewRecorder()
			router.ServeHTTP(resp, req)
			if resp.Code != tt.expectedStatus {
				t.Fatalf("expected status %d, got %d: %s", tt.expectedStatus, resp.Code, resp.Body.String())
			}
			wantCalls := 0
			if tt.expectedStatus == http.StatusOK {
				wantCalls = 1
			}
			if service.calls != wantCalls {
				t.Fatalf("expected service calls %d, got %d", wantCalls, service.calls)
			}
			if statusRepo.calls != tt.expectedStatusChecks {
				t.Fatalf("expected active-user status checks %d, got %d", tt.expectedStatusChecks, statusRepo.calls)
			}
		})
	}
}

type auditStatusRepo struct {
	statuses map[string]string
	calls    int
}

func (r *auditStatusRepo) GetUserStatus(_ context.Context, userID string) (string, error) {
	r.calls++
	return r.statuses[userID], nil
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
