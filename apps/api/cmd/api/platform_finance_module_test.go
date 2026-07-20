package main

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"lapangango-api/internal/platformfinance"

	"github.com/gin-gonic/gin"
)

func dummyAuthMiddleware(c *gin.Context) {
	c.Set("userID", "test-user")
	c.Next()
}

func dummyRequireActiveUser(c *gin.Context) {
	c.Next()
}

func dummyRequireRoleFactory(roles ...string) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Set("role", roles[0])
		c.Next()
	}
}

func TestPlatformFinanceAdminModule_Disabled(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()

	buildCount := 0
	buildServices := func() (platformfinance.Service, platformfinance.ExpenseService, platformfinance.JournalReadService, error) {
		buildCount++
		return nil, nil, nil, nil
	}

	err := registerPlatformFinanceAdminModule(
		r,
		false,
		dummyAuthMiddleware,
		dummyRequireActiveUser,
		dummyRequireRoleFactory,
		buildServices,
	)

	if err != nil {
		t.Fatalf("failed to register module: %v", err)
	}

	if buildCount != 0 {
		t.Errorf("expected buildServices to be called 0 times when disabled, got %d", buildCount)
	}

	routes := []struct {
		method string
		path   string
	}{
		{"GET", "/admin/finance/summary"},
		{"GET", "/admin/finance/breakdown"},
		{"GET", "/admin/finance/expenses"},
		{"POST", "/admin/finance/expenses"},
		{"POST", "/admin/finance/expenses/1/cancel"},
		{"POST", "/admin/finance/expenses/1/approve"},
		{"POST", "/admin/finance/expenses/1/post"},
		{"POST", "/admin/finance/expenses/1/void"},
		{"GET", "/admin/finance/journals"},
	}

	for _, route := range routes {
		req, _ := http.NewRequest(route.method, route.path, nil)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		if w.Code != http.StatusNotFound {
			t.Errorf("expected 404 for disabled route %s %s, got %d", route.method, route.path, w.Code)
		}
	}
}

func TestPlatformFinanceAdminModule_Enabled(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()

	buildCount := 0
	buildServices := func() (platformfinance.Service, platformfinance.ExpenseService, platformfinance.JournalReadService, error) {
		buildCount++
		// We return nil services. This might panic if the handlers actually call them,
		// but since we only check route registration, we can just check if routes exist.
		// Wait, RegisterRoutes might wrap them. Let's return mock services if needed.
		// Actually, returning nil is fine for just route registration.
		return nil, nil, nil, nil
	}

	err := registerPlatformFinanceAdminModule(
		r,
		true,
		dummyAuthMiddleware,
		dummyRequireActiveUser,
		dummyRequireRoleFactory,
		buildServices,
	)

	if err != nil {
		t.Fatalf("failed to register module: %v", err)
	}

	if buildCount != 1 {
		t.Errorf("expected buildServices to be called 1 time when enabled, got %d", buildCount)
	}

	expectedRoutes := map[string]bool{
		"/admin/finance/summary": false,
		"/admin/finance/breakdown": false,
		"/admin/finance/expenses": false,
		"/admin/finance/expenses/:id/cancel": false,
		"/admin/finance/expenses/:id/approve": false,
		"/admin/finance/expenses/:id/post": false,
		"/admin/finance/expenses/:id/void": false,
		"/admin/finance/journals": false,
	}

	for _, routeInfo := range r.Routes() {
		if _, exists := expectedRoutes[routeInfo.Path]; exists {
			expectedRoutes[routeInfo.Path] = true
		}
	}

	for route, found := range expectedRoutes {
		if !found {
			t.Errorf("expected route %s to be registered, but it was not found", route)
		}
	}
}
