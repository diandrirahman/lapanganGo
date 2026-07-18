package platformfinance_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"lapangango-api/internal/auth"
	"lapangango-api/internal/middleware"
	"lapangango-api/internal/platformfinance"
)

type expenseMutationStatusRepo struct {
	statuses map[string]string
	checks   int
}

func (r *expenseMutationStatusRepo) GetUserStatus(_ context.Context, userID string) (string, error) {
	r.checks++
	return r.statuses[userID], nil
}

type expenseMutationServiceStub struct {
	cancelCalls  int
	approveCalls int
}

func (s *expenseMutationServiceStub) ListExpenses(context.Context, platformfinance.ExpenseListQuery) (*platformfinance.ExpensePage, error) {
	return &platformfinance.ExpensePage{Items: []platformfinance.PlatformExpense{}}, nil
}

func (s *expenseMutationServiceStub) CreateDraft(context.Context, platformfinance.CreateExpenseRequest, string, string, string, string, string) (*platformfinance.PlatformExpense, bool, error) {
	return nil, false, nil
}

func (s *expenseMutationServiceStub) CancelDraft(_ context.Context, expenseID, _, _, _, _, _, _ string) (*platformfinance.PlatformExpense, bool, error) {
	s.cancelCalls++
	return &platformfinance.PlatformExpense{ID: expenseID, Status: "CANCELLED"}, false, nil
}

func (s *expenseMutationServiceStub) ApproveDraft(_ context.Context, expenseID, _, _, _, _, _ string) (*platformfinance.PlatformExpense, bool, error) {
	s.approveCalls++
	return &platformfinance.PlatformExpense{ID: expenseID, Status: "APPROVED"}, false, nil
}

type expenseMutationJournalStub struct{}

func (expenseMutationJournalStub) ListJournals(context.Context, platformfinance.JournalListQuery) (*platformfinance.JournalPage, error) {
	return &platformfinance.JournalPage{Items: []platformfinance.JournalListItem{}}, nil
}

func (expenseMutationJournalStub) GetSummary(context.Context, platformfinance.JournalListQuery) (*platformfinance.JournalSummary, error) {
	return &platformfinance.JournalSummary{}, nil
}

func TestExpenseMutationRoutesUseProductionAuthChain(t *testing.T) {
	gin.SetMode(gin.TestMode)
	const secret = "task-3b2-03-expense-auth-test-secret"
	expenseID := uuid.NewString()
	tests := []struct {
		name            string
		token           bool
		validToken      bool
		role            string
		status          string
		wantStatus      int
		wantStatusCall  int
		wantCancelCall  int
		wantApproveCall int
	}{
		{name: "missing jwt", wantStatus: http.StatusUnauthorized},
		{name: "invalid jwt", token: true, wantStatus: http.StatusUnauthorized},
		{name: "inactive superadmin", token: true, validToken: true, role: "SUPER_ADMIN", status: "SUSPENDED", wantStatus: http.StatusForbidden, wantStatusCall: 1},
		{name: "customer role", token: true, validToken: true, role: "CUSTOMER", status: "ACTIVE", wantStatus: http.StatusForbidden, wantStatusCall: 1},
		{name: "owner role", token: true, validToken: true, role: "OWNER", status: "ACTIVE", wantStatus: http.StatusForbidden, wantStatusCall: 1},
		{name: "staff role", token: true, validToken: true, role: "STAFF", status: "ACTIVE", wantStatus: http.StatusForbidden, wantStatusCall: 1},
		{name: "active superadmin", token: true, validToken: true, role: "SUPER_ADMIN", status: "ACTIVE", wantStatus: http.StatusOK, wantStatusCall: 1, wantCancelCall: 1, wantApproveCall: 1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			service := &expenseMutationServiceStub{}
			statusRepo := &expenseMutationStatusRepo{statuses: map[string]string{}}
			tokenService := auth.NewTokenService(secret, 1)
			router := gin.New()
			platformfinance.RegisterExpenseRoutes(
				router,
				middleware.Auth(tokenService),
				middleware.RequireActiveUser(statusRepo),
				middleware.RequireRole,
				service,
				expenseMutationJournalStub{},
			)

			req := httptest.NewRequest(http.MethodPost, "/admin/finance/expenses/"+expenseID+"/cancel", strings.NewReader(`{"reason":"duplicate invoice"}`))
			req.Header.Set("Idempotency-Key", "cancel-auth-test-key")
			if tt.token {
				if tt.validToken {
					userID := uuid.NewString()
					statusRepo.statuses[userID] = tt.status
					token, err := tokenService.Generate(auth.UserResponse{ID: userID, Email: "test@example.com", Role: tt.role, Status: tt.status})
					require.NoError(t, err)
					req.Header.Set("Authorization", "Bearer "+token)
				} else {
					req.Header.Set("Authorization", "Bearer invalid")
				}
			}
			response := httptest.NewRecorder()
			router.ServeHTTP(response, req)
			if response.Code != tt.wantStatus {
				t.Fatalf("cancel expected status %d, got %d: %s", tt.wantStatus, response.Code, response.Body.String())
			}
			if statusRepo.checks != tt.wantStatusCall {
				t.Fatalf("cancel expected active-user checks %d, got %d", tt.wantStatusCall, statusRepo.checks)
			}
			if service.cancelCalls != tt.wantCancelCall {
				t.Fatalf("cancel expected service calls %d, got %d", tt.wantCancelCall, service.cancelCalls)
			}

			statusRepo.checks = 0
			req = httptest.NewRequest(http.MethodPost, "/admin/finance/expenses/"+expenseID+"/approve", strings.NewReader(`{}`))
			req.Header.Set("Idempotency-Key", "approve-auth-test-key")
			if tt.token {
				if tt.validToken {
					userID := uuid.NewString()
					statusRepo.statuses[userID] = tt.status
					token, err := tokenService.Generate(auth.UserResponse{ID: userID, Email: "test@example.com", Role: tt.role, Status: tt.status})
					require.NoError(t, err)
					req.Header.Set("Authorization", "Bearer "+token)
				} else {
					req.Header.Set("Authorization", "Bearer invalid")
				}
			}
			response = httptest.NewRecorder()
			router.ServeHTTP(response, req)
			if response.Code != tt.wantStatus {
				t.Fatalf("approve expected status %d, got %d: %s", tt.wantStatus, response.Code, response.Body.String())
			}
			if statusRepo.checks != tt.wantStatusCall {
				t.Fatalf("approve expected active-user checks %d, got %d", tt.wantStatusCall, statusRepo.checks)
			}
			if service.approveCalls != tt.wantApproveCall {
				t.Fatalf("approve expected service calls %d, got %d", tt.wantApproveCall, service.approveCalls)
			}
		})
	}
}
