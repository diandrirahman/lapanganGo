package middleware

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestCORSAllowsExpenseMutationHeadersOnPreflight(t *testing.T) {
	t.Setenv("CORS_ALLOWED_ORIGINS", "http://localhost:5173")
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(CORS())
	router.POST("/admin/finance/expenses", func(c *gin.Context) { c.Status(http.StatusCreated) })

	req := httptest.NewRequest(http.MethodOptions, "/admin/finance/expenses", nil)
	req.Header.Set("Origin", "http://localhost:5173")
	req.Header.Set("Access-Control-Request-Method", http.MethodPost)
	req.Header.Set("Access-Control-Request-Headers", "authorization,content-type,idempotency-key,x-request-deadline-ms")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("preflight status = %d, want %d", rec.Code, http.StatusNoContent)
	}
	allowedHeaders := strings.ToLower(rec.Header().Get("Access-Control-Allow-Headers"))
	for _, expected := range []string{"authorization", "content-type", "idempotency-key", "x-request-deadline-ms"} {
		if !strings.Contains(allowedHeaders, expected) {
			t.Errorf("Access-Control-Allow-Headers = %q, missing %q", allowedHeaders, expected)
		}
	}
}
