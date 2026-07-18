package platformfinance_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"lapangango-api/internal/auth"
	"lapangango-api/internal/middleware"
	"lapangango-api/internal/platformfinance"
)

type financeAuthStatusRepo struct {
	statuses map[string]string
	checks   int
}

func (r *financeAuthStatusRepo) GetUserStatus(_ context.Context, userID string) (string, error) {
	r.checks++
	return r.statuses[userID], nil
}

func TestHandler_Integration_UsesProductionAuthChain(t *testing.T) {
	gin.SetMode(gin.TestMode)
	const secret = "task-3b2-06-finance-auth-test-secret"

	tests := []struct {
		name          string
		withToken     bool
		validToken    bool
		role          string
		status        string
		wantStatus    int
		wantStatusHit int
	}{
		{name: "tanpa token", wantStatus: http.StatusUnauthorized},
		{name: "invalid jwt", withToken: true, wantStatus: http.StatusUnauthorized},
		{name: "customer role", withToken: true, validToken: true, role: "CUSTOMER", status: "ACTIVE", wantStatus: http.StatusForbidden, wantStatusHit: 2},
		{name: "owner role", withToken: true, validToken: true, role: "OWNER", status: "ACTIVE", wantStatus: http.StatusForbidden, wantStatusHit: 2},
		{name: "staff role", withToken: true, validToken: true, role: "STAFF", status: "ACTIVE", wantStatus: http.StatusForbidden, wantStatusHit: 2},
		{name: "suspended superadmin", withToken: true, validToken: true, role: "SUPER_ADMIN", status: "SUSPENDED", wantStatus: http.StatusForbidden, wantStatusHit: 2},
		{name: "active superadmin", withToken: true, validToken: true, role: "SUPER_ADMIN", status: "ACTIVE", wantStatus: http.StatusOK, wantStatusHit: 2},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := &mockRepo{err: nil}
			service := platformfinance.NewService(repo)
			statusRepo := &financeAuthStatusRepo{statuses: map[string]string{}}
			tokenService := auth.NewTokenService(secret, 1)
			router := gin.New()
			platformfinance.RegisterRoutes(
				router,
				middleware.Auth(tokenService),
				middleware.RequireActiveUser(statusRepo),
				middleware.RequireRole,
				service,
			)

			var authorization string
			if tt.withToken {
				if tt.validToken {
					userID := uuid.NewString()
					statusRepo.statuses[userID] = tt.status
					token, err := tokenService.Generate(auth.UserResponse{ID: userID, Email: "finance-auth@example.com", Role: tt.role, Status: tt.status})
					require.NoError(t, err)
					authorization = "Bearer " + token
				} else {
					authorization = "Bearer invalid"
				}
			}

			for _, path := range []string{
				"/admin/finance/summary?start_date=2026-06-01&end_date=2026-06-30",
				"/admin/finance/breakdown?start_date=2026-06-01&end_date=2026-06-30&dimension=owner",
			} {
				req := httptest.NewRequest(http.MethodGet, path, nil)
				if authorization != "" {
					req.Header.Set("Authorization", authorization)
				}
				response := httptest.NewRecorder()
				router.ServeHTTP(response, req)
				assert.Equal(t, tt.wantStatus, response.Code, path)
			}
			assert.Equal(t, tt.wantStatusHit, statusRepo.checks)
		})
	}
}
