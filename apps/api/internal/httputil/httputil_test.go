package httputil_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"lapangango-api/internal/httputil"
)

func TestGetActorRole(t *testing.T) {
	gin.SetMode(gin.TestMode)

	t.Run("owner context yields OWNER role", func(t *testing.T) {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request, _ = http.NewRequest(http.MethodGet, "/", nil)
		c.Set("auth_is_owner", true)

		role := httputil.GetActorRole(c)
		if role != "OWNER" {
			t.Errorf("expected role OWNER, got %s", role)
		}
	})

	t.Run("staff context yields STAFF role", func(t *testing.T) {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request, _ = http.NewRequest(http.MethodGet, "/", nil)
		c.Set("auth_is_owner", false)

		role := httputil.GetActorRole(c)
		if role != "STAFF" {
			t.Errorf("expected role STAFF, got %s", role)
		}
	})
}
