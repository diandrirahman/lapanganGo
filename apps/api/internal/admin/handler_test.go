package admin_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/jackc/pgx/v5"
	"lapangango-api/internal/admin"

	"github.com/gin-gonic/gin"
)

type mockAdminService struct {
	updateErr error
}

func (m *mockAdminService) GetUsers(ctx context.Context, query admin.UserQuery) (admin.PaginatedResponse, error) {
	return admin.PaginatedResponse{}, nil
}
func (m *mockAdminService) GetOwners(ctx context.Context, query admin.OwnerQuery) (admin.PaginatedResponse, error) {
	return admin.PaginatedResponse{}, nil
}
func (m *mockAdminService) UpdateOwnerStatus(ctx context.Context, ownerProfileID string, status string, actorID string) error {
	return m.updateErr
}
func (m *mockAdminService) GetVenues(ctx context.Context, query admin.VenueQuery) (admin.PaginatedResponse, error) {
	return admin.PaginatedResponse{}, nil
}
func (m *mockAdminService) UpdateVenueStatus(ctx context.Context, venueID string, status string, actorID string) error {
	return m.updateErr
}
func (m *mockAdminService) GetAuditLogs(ctx context.Context, query admin.AuditLogQuery) (admin.PaginatedResponse, error) {
	return admin.PaginatedResponse{}, nil
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
