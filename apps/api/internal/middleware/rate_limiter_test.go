package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
)

func TestRateLimiter(t *testing.T) {
	gin.SetMode(gin.TestMode)

	// Create a rate limiter with 2 requests per second
	rl1 := NewRateLimiter("", "general", 2, time.Second)
	rl2 := NewRateLimiter("", "auth", 2, time.Second)

	r := gin.New()
	r.GET("/test1", rl1, func(c *gin.Context) {
		c.Status(http.StatusOK)
	})
	r.GET("/test2", rl2, func(c *gin.Context) {
		c.Status(http.StatusOK)
	})

	req1, _ := http.NewRequest(http.MethodGet, "/test1", nil)
	req1.RemoteAddr = "127.0.0.1:12345"

	req2, _ := http.NewRequest(http.MethodGet, "/test2", nil)
	req2.RemoteAddr = "127.0.0.1:12345"

	// Request 1 to general: should pass
	w1 := httptest.NewRecorder()
	r.ServeHTTP(w1, req1)
	if w1.Code != http.StatusOK {
		t.Errorf("Expected 200 OK for 1st request, got %d", w1.Code)
	}

	// Request 2 to general: should pass
	w2 := httptest.NewRecorder()
	r.ServeHTTP(w2, req1)
	if w2.Code != http.StatusOK {
		t.Errorf("Expected 200 OK for 2nd request, got %d", w2.Code)
	}

	// Request 3 to general: should fail with 429
	w3 := httptest.NewRecorder()
	r.ServeHTTP(w3, req1)
	if w3.Code != http.StatusTooManyRequests {
		t.Errorf("Expected 429 Too Many Requests for 3rd request, got %d", w3.Code)
	}

	// Request 1 to auth: should pass (isolated from general)
	w4 := httptest.NewRecorder()
	r.ServeHTTP(w4, req2)
	if w4.Code != http.StatusOK {
		t.Errorf("Expected 200 OK for auth request, got %d", w4.Code)
	}

	// Wait for window to pass
	time.Sleep(1100 * time.Millisecond)

	// Request 4 to general: should pass again
	w5 := httptest.NewRecorder()
	r.ServeHTTP(w5, req1)
	if w5.Code != http.StatusOK {
		t.Errorf("Expected 200 OK for 4th request, got %d", w5.Code)
	}
}
