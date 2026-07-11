package admin_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"lapangango-api/internal/admin"

	"github.com/gin-gonic/gin"
)

type mockAdminService struct {
	updateErr   error
	updateCalls int
}

func (m *mockAdminService) GetUsers(ctx context.Context, query admin.UserQuery) (admin.PaginatedResponse, error) {
	return admin.PaginatedResponse{}, nil
}
func (m *mockAdminService) GetOwners(ctx context.Context, query admin.OwnerQuery) (admin.PaginatedResponse, error) {
	return admin.PaginatedResponse{}, nil
}
func (m *mockAdminService) UpdateOwnerStatus(ctx context.Context, ownerProfileID string, status string, actorID string) error {
	m.updateCalls++
	return m.updateErr
}
func (m *mockAdminService) GetVenues(ctx context.Context, query admin.VenueQuery) (admin.PaginatedResponse, error) {
	return admin.PaginatedResponse{}, nil
}
func (m *mockAdminService) UpdateVenueStatus(ctx context.Context, venueID string, status string, actorID string) error {
	m.updateCalls++
	return m.updateErr
}

func TestUpdateStatusRequestDeadline(t *testing.T) {
	gin.SetMode(gin.TestMode)

	tests := []struct {
		name           string
		path           string
		handler        func(*admin.Handler) gin.HandlerFunc
		deadline       string
		expectedStatus int
		expectedCalls  int
	}{
		{
			name:           "Owner expired request",
			path:           "/admin/owners/1/status",
			handler:        func(h *admin.Handler) gin.HandlerFunc { return h.UpdateOwnerStatus },
			deadline:       strconv.FormatInt(time.Now().Add(-time.Minute).UnixMilli(), 10),
			expectedStatus: http.StatusRequestTimeout,
			expectedCalls:  0,
		},
		{
			name:           "Venue expired request",
			path:           "/admin/venues/1/status",
			handler:        func(h *admin.Handler) gin.HandlerFunc { return h.UpdateVenueStatus },
			deadline:       strconv.FormatInt(time.Now().Add(-time.Minute).UnixMilli(), 10),
			expectedStatus: http.StatusRequestTimeout,
			expectedCalls:  0,
		},
		{
			name:           "Invalid deadline",
			path:           "/admin/owners/1/status",
			handler:        func(h *admin.Handler) gin.HandlerFunc { return h.UpdateOwnerStatus },
			deadline:       "not-a-timestamp",
			expectedStatus: http.StatusBadRequest,
			expectedCalls:  0,
		},
		{
			name:           "Future deadline",
			path:           "/admin/owners/1/status",
			handler:        func(h *admin.Handler) gin.HandlerFunc { return h.UpdateOwnerStatus },
			deadline:       strconv.FormatInt(time.Now().Add(time.Minute).UnixMilli(), 10),
			expectedStatus: http.StatusOK,
			expectedCalls:  1,
		},
		{
			name:           "Missing deadline remains backward compatible",
			path:           "/admin/owners/1/status",
			handler:        func(h *admin.Handler) gin.HandlerFunc { return h.UpdateOwnerStatus },
			expectedStatus: http.StatusOK,
			expectedCalls:  1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			service := &mockAdminService{}
			handler := admin.NewHandler(service)
			router := gin.New()
			router.PATCH(tt.path, tt.handler(handler))

			body, _ := json.Marshal(map[string]string{"status": "SUSPENDED"})
			req := httptest.NewRequest(http.MethodPatch, tt.path, bytes.NewReader(body))
			if tt.deadline != "" {
				req.Header.Set("X-Request-Deadline-Ms", tt.deadline)
			}
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)

			if w.Code != tt.expectedStatus {
				t.Fatalf("expected status %d, got %d: %s", tt.expectedStatus, w.Code, w.Body.String())
			}
			if service.updateCalls != tt.expectedCalls {
				t.Fatalf("expected %d update calls, got %d", tt.expectedCalls, service.updateCalls)
			}
		})
	}
}
func (m *mockAdminService) GetAuditLogs(ctx context.Context, query admin.AuditLogQuery) (admin.PaginatedResponse, error) {
	return admin.PaginatedResponse{}, nil
}
func (m *mockAdminService) GetDashboardStats(ctx context.Context) (admin.DashboardStatsResponse, error) {
	return admin.DashboardStatsResponse{TotalUsers: 10}, nil
}
func (m *mockAdminService) GetVenueOwnerProfileID(ctx context.Context, venueID string) (string, error) {
	return "", nil
}

func TestUpdateOwnerStatus(t *testing.T) {
	gin.SetMode(gin.TestMode)

	tests := []struct {
		name           string
		status         string
		serviceErr     error
		expectedStatus int
	}{
		{
			name:           "Success",
			status:         "SUSPENDED",
			serviceErr:     nil,
			expectedStatus: http.StatusOK,
		},
		{
			name:           "Not Found",
			status:         "SUSPENDED",
			serviceErr:     pgx.ErrNoRows,
			expectedStatus: http.StatusNotFound,
		},
		{
			name:           "Invalid Status Binding",
			status:         "INVALID_STATUS",
			serviceErr:     nil, // Wont reach service
			expectedStatus: http.StatusBadRequest,
		},
		{
			name:           "Internal Error",
			status:         "ACTIVE",
			serviceErr:     errors.New("db error"),
			expectedStatus: http.StatusInternalServerError,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := gin.New()
			mockService := &mockAdminService{updateErr: tt.serviceErr}
			handler := admin.NewHandler(mockService)
			r.PATCH("/admin/owners/:id/status", handler.UpdateOwnerStatus)

			body, _ := json.Marshal(map[string]string{"status": tt.status})
			req, _ := http.NewRequest(http.MethodPatch, "/admin/owners/1/status", bytes.NewBuffer(body))
			w := httptest.NewRecorder()
			r.ServeHTTP(w, req)

			if w.Code != tt.expectedStatus {
				t.Errorf("expected status %d, got %d", tt.expectedStatus, w.Code)
			}
		})
	}
}

func TestGetAuditLogsEntityValidation(t *testing.T) {
	gin.SetMode(gin.TestMode)

	tests := []struct {
		name           string
		entityType     string
		expectedStatus int
	}{
		{"Valid Entity USER Upper", "USER", http.StatusOK},
		{"Valid Entity USER Lower", "user", http.StatusOK},
		{"Valid Entity STAFF", "STAFF", http.StatusOK},
		{"Valid Entity OWNER_PROFILE Lower", "owner_profile", http.StatusOK},
		{"Valid Entity REFUND Lower", "refund", http.StatusOK},
		{"Valid Entity FINANCE_TRANSACTION", "FINANCE_TRANSACTION", http.StatusOK},
		{"Invalid Entity", "INVALID_ENTITY", http.StatusBadRequest},
		{"Empty Entity", "", http.StatusOK},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := gin.New()
			mockService := &mockAdminService{}
			handler := admin.NewHandler(mockService)
			r.GET("/admin/audit-logs", handler.GetAuditLogs)

			req, _ := http.NewRequest(http.MethodGet, "/admin/audit-logs?entity_type="+tt.entityType, nil)
			w := httptest.NewRecorder()
			r.ServeHTTP(w, req)

			if w.Code != tt.expectedStatus {
				t.Errorf("expected status %d, got %d", tt.expectedStatus, w.Code)
			}
		})
	}
}

func TestGetDashboardStats(t *testing.T) {
	gin.SetMode(gin.TestMode)

	r := gin.New()
	mockService := &mockAdminService{}
	handler := admin.NewHandler(mockService)
	r.GET("/admin/dashboard", handler.GetDashboardStats)

	req, _ := http.NewRequest(http.MethodGet, "/admin/dashboard", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", w.Code)
	}

	var res admin.DashboardStatsResponse
	if err := json.Unmarshal(w.Body.Bytes(), &res); err != nil {
		t.Fatalf("failed to unmarshal response: %v", err)
	}

	if res.TotalUsers != 10 {
		t.Errorf("expected 10 total users, got %d", res.TotalUsers)
	}
}
