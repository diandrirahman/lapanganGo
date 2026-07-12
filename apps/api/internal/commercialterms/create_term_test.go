package commercialterms_test

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"lapangango-api/internal/audit"
	"lapangango-api/internal/commercialterms"
	"lapangango-api/internal/database"
)

func setupCreateRouter(pool *pgxpool.Pool, adminID string) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.Default()

	repo := commercialterms.NewRepository(pool)
	auditRepo := audit.NewPlatformRepository()
	auditSvc := audit.NewPlatformService(auditRepo)
	svc := commercialterms.NewService(repo, pool, auditSvc)

	authMiddleware := func(c *gin.Context) {
		c.Set("auth_user_id", adminID)
		c.Set("auth_actor_user_id", adminID)
		c.Set("auth_user_role", "SUPER_ADMIN")
		c.Next()
	}
	requireActiveUser := func(c *gin.Context) { c.Next() }
	requireRole := func(roles ...string) gin.HandlerFunc {
		return func(c *gin.Context) { c.Next() }
	}

	commercialterms.RegisterRoutes(r, authMiddleware, requireActiveUser, requireRole, svc)
	return r
}

func TestCommercialTermsCreate_Integration(t *testing.T) {
	if os.Getenv("TEST_INTEGRATION") != "1" {
		t.Skip("Skipping integration tests. Set TEST_INTEGRATION=1 to run.")
	}
	dbURL := os.Getenv("TEST_DATABASE_URL")
	if dbURL == "" {
		t.Skip("TEST_DATABASE_URL not set")
	}

	ctx := context.Background()
	pool, err := database.NewPostgresPool(ctx, dbURL)
	if err != nil {
		t.Fatalf("Failed to connect to database: %v", err)
	}
	defer pool.Close()

	adminID := uuid.New().String()
	_, err = pool.Exec(ctx, "INSERT INTO users (id, name, email, password_hash, role) VALUES ($1, 'Admin Create', $2, 'hash', 'SUPER_ADMIN')", adminID, "admin."+adminID+"@test.local")
	if err != nil {
		t.Fatalf("failed to insert admin: %v", err)
	}

	t.Cleanup(func() {
		pool.Exec(ctx, "DELETE FROM platform_commercial_terms WHERE created_by_user_id = $1", adminID)
		pool.Exec(ctx, "DELETE FROM platform_audit_logs WHERE actor_user_id = $1", adminID)
		pool.Exec(ctx, "DELETE FROM users WHERE id = $1", adminID)
	})

	r := setupCreateRouter(pool, adminID)

	createTestOwner := func() string {
		newOwnerID := uuid.New().String()
		_, err := pool.Exec(ctx, "INSERT INTO users (id, name, email, password_hash, role) VALUES ($1, 'Owner Create', $2, 'hash', 'OWNER')", newOwnerID, "owner."+newOwnerID+"@test.local")
		if err != nil {
			t.Fatalf("failed to insert owner user: %v", err)
		}
		_, err = pool.Exec(ctx, "INSERT INTO owner_profiles (id, user_id, business_name, verification_status) VALUES ($1, $2, 'BC', 'APPROVED')", newOwnerID, newOwnerID)
		if err != nil {
			t.Fatalf("failed to insert owner profile: %v", err)
		}
		t.Cleanup(func() {
			pool.Exec(ctx, "DELETE FROM platform_commercial_terms WHERE owner_profile_id = $1", newOwnerID)
			pool.Exec(ctx, "DELETE FROM owner_profiles WHERE id = $1", newOwnerID)
			pool.Exec(ctx, "DELETE FROM users WHERE id = $1", newOwnerID)
		})
		return newOwnerID
	}

	t.Run("Valid 0 bps", func(t *testing.T) {
		ik := uuid.New().String()
		oid := createTestOwner()
		reqBody := `{"owner_profile_id": "` + oid + `", "label": "Promo 0", "phase": "TRIAL", "finance_mode": "SIMULATION", "collection_method": "NONE", "commission_bps": 0, "valid_from": "` + time.Now().Add(24*time.Hour).UTC().Format(time.RFC3339Nano) + `"}`

		req, _ := http.NewRequest(http.MethodPost, "/admin/commercial-terms", bytes.NewBufferString(reqBody))
		req.Header.Set("Idempotency-Key", ik)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		if w.Code != http.StatusCreated {
			t.Fatalf("expected 201, got %d. body: %s", w.Code, w.Body.String())
		}
	})

	t.Run("Missing commission_bps", func(t *testing.T) {
		ik := uuid.New().String()
		oid := createTestOwner()
		reqBody := `{"owner_profile_id": "` + oid + `", "label": "Promo Missing BPS", "phase": "TRIAL", "finance_mode": "SIMULATION", "collection_method": "NONE", "valid_from": "` + time.Now().Add(24*time.Hour).UTC().Format(time.RFC3339Nano) + `"}`

		req, _ := http.NewRequest(http.MethodPost, "/admin/commercial-terms", bytes.NewBufferString(reqBody))
		req.Header.Set("Idempotency-Key", ik)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		if w.Code != http.StatusBadRequest {
			t.Fatalf("expected 400, got %d", w.Code)
		}
	})

	t.Run("valid_from now/past rejected", func(t *testing.T) {
		ik := uuid.New().String()
		oid := createTestOwner()
		reqBody := `{"owner_profile_id": "` + oid + `", "label": "Promo Past", "phase": "TRIAL", "finance_mode": "SIMULATION", "collection_method": "NONE", "commission_bps": 500, "valid_from": "` + time.Now().Add(-1*time.Hour).UTC().Format(time.RFC3339Nano) + `"}`

		req, _ := http.NewRequest(http.MethodPost, "/admin/commercial-terms", bytes.NewBufferString(reqBody))
		req.Header.Set("Idempotency-Key", ik)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		if w.Code != http.StatusBadRequest {
			t.Fatalf("expected 400, got %d", w.Code)
		}
	})

	t.Run("nonexistent owner 404", func(t *testing.T) {
		ik := uuid.New().String()
		fakeID := uuid.New().String()
		reqBody := `{"owner_profile_id": "` + fakeID + `", "label": "Promo Fake", "phase": "TRIAL", "finance_mode": "SIMULATION", "collection_method": "NONE", "commission_bps": 500, "valid_from": "` + time.Now().Add(24*time.Hour).UTC().Format(time.RFC3339Nano) + `"}`

		req, _ := http.NewRequest(http.MethodPost, "/admin/commercial-terms", bytes.NewBufferString(reqBody))
		req.Header.Set("Idempotency-Key", ik)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		if w.Code != http.StatusNotFound {
			t.Fatalf("expected 404, got %d", w.Code)
		}
	})

	t.Run("Concurrency Idempotency Test", func(t *testing.T) {
		ik := uuid.New().String()
		oid := createTestOwner()
		reqBody := `{"owner_profile_id": "` + oid + `", "label": "Concurrent", "phase": "TRIAL", "finance_mode": "SIMULATION", "collection_method": "NONE", "commission_bps": 500, "valid_from": "` + time.Now().Add(48*time.Hour).UTC().Format(time.RFC3339Nano) + `"}`

		var wg sync.WaitGroup
		results := make(chan int, 5)

		for i := 0; i < 5; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				req, _ := http.NewRequest(http.MethodPost, "/admin/commercial-terms", bytes.NewBufferString(reqBody))
				req.Header.Set("Idempotency-Key", ik)
				w := httptest.NewRecorder()
				r.ServeHTTP(w, req)
				results <- w.Code
			}()
		}
		wg.Wait()
		close(results)

		successCount := 0
		for code := range results {
			if code == http.StatusCreated {
				successCount++
			} else if code != http.StatusConflict && code != http.StatusInternalServerError {
				t.Errorf("unexpected code %d", code)
			}
		}
		if successCount != 5 {
			t.Errorf("expected all 5 concurrent identical requests to succeed (1 create, 4 replay), got %d success", successCount)
		}
	})

	t.Run("Concurrency Overlap Scope Lock Test", func(t *testing.T) {
		ik1 := uuid.New().String()
		ik2 := uuid.New().String()
		oid := createTestOwner()

		// Use exact same timestamp for true payload overlap conflict test
		exactTimeStr := time.Now().Add(24 * time.Hour).UTC().Format(time.RFC3339Nano)

		reqBody1 := `{"owner_profile_id": "` + oid + `", "label": "Race1", "phase": "TRIAL", "finance_mode": "SIMULATION", "collection_method": "NONE", "commission_bps": 500, "valid_from": "` + exactTimeStr + `"}`
		reqBody2 := `{"owner_profile_id": "` + oid + `", "label": "Race2", "phase": "STANDARD", "finance_mode": "SIMULATION", "collection_method": "NONE", "commission_bps": 600, "valid_from": "` + exactTimeStr + `"}`

		var wg sync.WaitGroup
		codes := make(chan int, 2)
		wg.Add(2)
		go func() {
			defer wg.Done()
			req, _ := http.NewRequest(http.MethodPost, "/admin/commercial-terms", bytes.NewBufferString(reqBody1))
			req.Header.Set("Idempotency-Key", ik1)
			w := httptest.NewRecorder()
			r.ServeHTTP(w, req)
			codes <- w.Code
		}()
		go func() {
			defer wg.Done()
			req, _ := http.NewRequest(http.MethodPost, "/admin/commercial-terms", bytes.NewBufferString(reqBody2))
			req.Header.Set("Idempotency-Key", ik2)
			w := httptest.NewRecorder()
			r.ServeHTTP(w, req)
			codes <- w.Code
		}()
		wg.Wait()
		close(codes)

		has201 := false
		hasError := false
		for c := range codes {
			if c == http.StatusCreated {
				has201 = true
			}
			if c == http.StatusConflict || c == http.StatusBadRequest {
				hasError = true
			}
		}
		if !has201 || !hasError {
			t.Errorf("expected exactly one 201 and one 409/400, got codes. has201=%v hasError=%v", has201, hasError)
		}
	})

	t.Run("Idempotency different payload", func(t *testing.T) {
		ik := uuid.New().String()
		oid := createTestOwner()
		reqBody1 := `{"owner_profile_id": "` + oid + `", "label": "First", "phase": "TRIAL", "finance_mode": "SIMULATION", "collection_method": "NONE", "commission_bps": 500, "valid_from": "` + time.Now().Add(24*time.Hour).UTC().Format(time.RFC3339Nano) + `"}`
		reqBody2 := `{"owner_profile_id": "` + oid + `", "label": "Second", "phase": "TRIAL", "finance_mode": "SIMULATION", "collection_method": "NONE", "commission_bps": 500, "valid_from": "` + time.Now().Add(24*time.Hour).UTC().Format(time.RFC3339Nano) + `"}`

		req1, _ := http.NewRequest(http.MethodPost, "/admin/commercial-terms", bytes.NewBufferString(reqBody1))
		req1.Header.Set("Idempotency-Key", ik)
		w1 := httptest.NewRecorder()
		r.ServeHTTP(w1, req1)
		if w1.Code != http.StatusCreated {
			t.Fatalf("first request failed: %d", w1.Code)
		}

		req2, _ := http.NewRequest(http.MethodPost, "/admin/commercial-terms", bytes.NewBufferString(reqBody2))
		req2.Header.Set("Idempotency-Key", ik)
		w2 := httptest.NewRecorder()
		r.ServeHTTP(w2, req2)
		if w2.Code != http.StatusConflict {
			t.Fatalf("expected 409 for diff payload with same key, got %d", w2.Code)
		}
	})

	t.Run("LIVE owner validation rejection", func(t *testing.T) {
		ik := uuid.New().String()
		oid := createTestOwner()
		reqBody := `{"owner_profile_id": "` + oid + `", "label": "Promo LIVE", "phase": "TRIAL", "finance_mode": "LIVE", "collection_method": "NONE", "commission_bps": 500, "valid_from": "` + time.Now().Add(24*time.Hour).UTC().Format(time.RFC3339Nano) + `"}`

		req, _ := http.NewRequest(http.MethodPost, "/admin/commercial-terms", bytes.NewBufferString(reqBody))
		req.Header.Set("Idempotency-Key", ik)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		if w.Code != http.StatusForbidden {
			t.Fatalf("expected 403, got %d", w.Code)
		}

		var count int
		pool.QueryRow(ctx, "SELECT count(*) FROM platform_audit_logs WHERE correlation_id = $1 AND action = 'PLATFORM_COMMERCIAL_TERM_LIVE_REJECTED'", ik).Scan(&count)
		if count != 1 {
			t.Fatalf("expected audit log for LIVE_REJECTED")
		}
	})

	t.Run("Replay LIVE identical", func(t *testing.T) {
		ik := uuid.New().String()
		oid := createTestOwner()
		reqBody := `{"owner_profile_id": "` + oid + `", "label": "Promo LIVE", "phase": "TRIAL", "finance_mode": "LIVE", "collection_method": "NONE", "commission_bps": 500, "valid_from": "` + time.Now().Add(24*time.Hour).UTC().Format(time.RFC3339Nano) + `"}`

		req1, _ := http.NewRequest(http.MethodPost, "/admin/commercial-terms", bytes.NewBufferString(reqBody))
		req1.Header.Set("Idempotency-Key", ik)
		w1 := httptest.NewRecorder()
		r.ServeHTTP(w1, req1)

		req2, _ := http.NewRequest(http.MethodPost, "/admin/commercial-terms", bytes.NewBufferString(reqBody))
		req2.Header.Set("Idempotency-Key", ik)
		w2 := httptest.NewRecorder()
		r.ServeHTTP(w2, req2)

		if w2.Code != http.StatusForbidden {
			t.Fatalf("expected 403 replay, got %d", w2.Code)
		}
	})

	t.Run("Global Create", func(t *testing.T) {
		ik := uuid.New().String()
		reqBody := `{"label": "Promo Global", "phase": "TRIAL", "finance_mode": "SIMULATION", "collection_method": "NONE", "commission_bps": 500, "valid_from": "` + time.Now().Add(24*time.Hour).UTC().Format(time.RFC3339Nano) + `"}`

		req, _ := http.NewRequest(http.MethodPost, "/admin/commercial-terms", bytes.NewBufferString(reqBody))
		req.Header.Set("Idempotency-Key", ik)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		if w.Code != http.StatusCreated {
			t.Fatalf("expected 201 for global create, got %d", w.Code)
		}

		t.Cleanup(func() {
			pool.Exec(ctx, "DELETE FROM platform_commercial_terms WHERE label = 'Promo Global'")
			pool.Exec(ctx, "UPDATE platform_commercial_terms SET valid_until = NULL WHERE owner_profile_id IS NULL AND label != 'Promo Global'")
		})
	})

	t.Run("Full Auth Matrix", func(t *testing.T) {
		reqBody := `{"label": "Auth", "phase": "TRIAL", "finance_mode": "SIMULATION", "collection_method": "NONE", "commission_bps": 500, "valid_from": "` + time.Now().Add(24*time.Hour).UTC().Format(time.RFC3339Nano) + `"}`

		cases := []struct {
			name           string
			role           string
			status         string
			expectedStatus int
		}{
			{"Anonymous", "", "", http.StatusUnauthorized},
			{"Suspended_Super_Admin", "SUPER_ADMIN", "SUSPENDED", http.StatusForbidden},
			{"Customer", "CUSTOMER", "ACTIVE", http.StatusForbidden},
			{"Owner", "OWNER", "ACTIVE", http.StatusForbidden},
			{"Staff", "STAFF", "ACTIVE", http.StatusForbidden},
		}

		for _, tc := range cases {
			t.Run(tc.name, func(t *testing.T) {
				ik := uuid.New().String()
				rMock := gin.Default()
				rMock.Use(func(c *gin.Context) {
					if tc.role != "" {
						c.Set("auth_user_id", "some_user")
						c.Set("auth_user_role", tc.role)
						c.Set("auth_user_status", tc.status)
					}
					c.Next()
				})
				commercialterms.RegisterRoutes(rMock, func(c *gin.Context) {
					if c.GetString("auth_user_id") == "" {
						c.AbortWithStatus(http.StatusUnauthorized)
					}
					c.Next()
				}, func(c *gin.Context) {
					if c.GetString("auth_user_status") != "ACTIVE" {
						c.AbortWithStatus(http.StatusForbidden)
					}
					c.Next()
				}, func(roles ...string) gin.HandlerFunc {
					return func(c *gin.Context) {
						role := c.GetString("auth_user_role")
						allowed := false
						for _, r := range roles {
							if r == role {
								allowed = true
								break
							}
						}
						if !allowed {
							c.AbortWithStatus(http.StatusForbidden)
						}
					}
				}, commercialterms.NewService(commercialterms.NewRepository(pool), pool, audit.NewPlatformService(audit.NewPlatformRepository())))

				req, _ := http.NewRequest(http.MethodPost, "/admin/commercial-terms", bytes.NewBufferString(reqBody))
				req.Header.Set("Idempotency-Key", ik)
				w := httptest.NewRecorder()
				rMock.ServeHTTP(w, req)
				if w.Code != tc.expectedStatus {
					t.Fatalf("expected %d for %s, got %d", tc.expectedStatus, tc.name, w.Code)
				}
			})
		}
	})

	t.Run("Method absence", func(t *testing.T) {
		reqPATCH, _ := http.NewRequest(http.MethodPatch, "/admin/commercial-terms", nil)
		wPATCH := httptest.NewRecorder()
		r.ServeHTTP(wPATCH, reqPATCH)
		if wPATCH.Code != http.StatusNotFound && wPATCH.Code != http.StatusMethodNotAllowed {
			t.Fatalf("expected 404/405 for PATCH, got %d", wPATCH.Code)
		}

		reqPUT, _ := http.NewRequest(http.MethodPut, "/admin/commercial-terms", nil)
		wPUT := httptest.NewRecorder()
		r.ServeHTTP(wPUT, reqPUT)
		if wPUT.Code != http.StatusNotFound && wPUT.Code != http.StatusMethodNotAllowed {
			t.Fatalf("expected 404/405 for PUT, got %d", wPUT.Code)
		}

		reqDELETE, _ := http.NewRequest(http.MethodDelete, "/admin/commercial-terms", nil)
		wDELETE := httptest.NewRecorder()
		r.ServeHTTP(wDELETE, reqDELETE)
		if wDELETE.Code != http.StatusNotFound && wDELETE.Code != http.StatusMethodNotAllowed {
			t.Fatalf("expected 404/405 for DELETE, got %d", wDELETE.Code)
		}
	})

	t.Run("Correct supersession & adjacent boundaries", func(t *testing.T) {
		oid := createTestOwner()
		ik1 := uuid.New().String()
		validFrom1 := time.Now().Add(24 * time.Hour).UTC().Format(time.RFC3339Nano)
		reqBody1 := `{"owner_profile_id": "` + oid + `", "label": "Term 1", "phase": "TRIAL", "finance_mode": "SIMULATION", "collection_method": "NONE", "commission_bps": 100, "valid_from": "` + validFrom1 + `"}`
		req1, _ := http.NewRequest(http.MethodPost, "/admin/commercial-terms", bytes.NewBufferString(reqBody1))
		req1.Header.Set("Idempotency-Key", ik1)
		w1 := httptest.NewRecorder()
		r.ServeHTTP(w1, req1)
		if w1.Code != http.StatusCreated {
			t.Fatalf("failed to create term 1: %d", w1.Code)
		}

		ik2 := uuid.New().String()
		validFrom2 := time.Now().Add(48 * time.Hour).UTC().Format(time.RFC3339Nano)
		reqBody2 := `{"owner_profile_id": "` + oid + `", "label": "Term 2", "phase": "STANDARD", "finance_mode": "SIMULATION", "collection_method": "NONE", "commission_bps": 200, "valid_from": "` + validFrom2 + `"}`
		req2, _ := http.NewRequest(http.MethodPost, "/admin/commercial-terms", bytes.NewBufferString(reqBody2))
		req2.Header.Set("Idempotency-Key", ik2)
		w2 := httptest.NewRecorder()
		r.ServeHTTP(w2, req2)
		if w2.Code != http.StatusCreated {
			t.Fatalf("failed to supersede with term 2: %d", w2.Code)
		}

		// Verify supersession boundary
		var count int
		pool.QueryRow(ctx, "SELECT count(*) FROM platform_commercial_terms WHERE owner_profile_id = $1 AND supersedes_id IS NOT NULL", oid).Scan(&count)
		if count != 1 {
			t.Fatalf("expected exactly 1 superseded term, got %d", count)
		}
	})
	t.Run("Missing Idempotency Key", func(t *testing.T) {
		reqBody := `{"label": "Promo", "phase": "TRIAL", "finance_mode": "SIMULATION", "collection_method": "NONE", "commission_bps": 500, "valid_from": "` + time.Now().Add(24*time.Hour).UTC().Format(time.RFC3339Nano) + `"}`
		req, _ := http.NewRequest(http.MethodPost, "/admin/commercial-terms", bytes.NewBufferString(reqBody))
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		if w.Code != http.StatusBadRequest {
			t.Fatalf("expected 400 for missing idempotency key, got %d", w.Code)
		}
	})

	t.Run("Invalid collection/bps", func(t *testing.T) {
		ik := uuid.New().String()
		reqBody := `{"label": "Promo", "phase": "TRIAL", "finance_mode": "SIMULATION", "collection_method": "GATEWAY", "commission_bps": 5000, "valid_from": "` + time.Now().Add(24*time.Hour).UTC().Format(time.RFC3339Nano) + `"}`
		req, _ := http.NewRequest(http.MethodPost, "/admin/commercial-terms", bytes.NewBufferString(reqBody))
		req.Header.Set("Idempotency-Key", ik)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		if w.Code != http.StatusBadRequest {
			t.Fatalf("expected 400 for invalid collection/bps, got %d", w.Code)
		}
	})

	t.Run("Duplicate Audit Marker Fail-Closed", func(t *testing.T) {
		ik := uuid.New().String()
		pool.Exec(ctx, "INSERT INTO platform_audit_logs (id, actor_user_id, actor_role, action, entity_type, correlation_id) VALUES ($1, $2, 'SUPER_ADMIN', 'PLATFORM_COMMERCIAL_TERM_CREATED', 'PLATFORM_COMMERCIAL_TERM', $3)", uuid.NewString(), adminID, ik)
		pool.Exec(ctx, "INSERT INTO platform_audit_logs (id, actor_user_id, actor_role, action, entity_type, correlation_id) VALUES ($1, $2, 'SUPER_ADMIN', 'PLATFORM_COMMERCIAL_TERM_CREATED', 'PLATFORM_COMMERCIAL_TERM', $3)", uuid.NewString(), adminID, ik)
		
		reqBody := `{"label": "Promo", "phase": "TRIAL", "finance_mode": "SIMULATION", "collection_method": "NONE", "commission_bps": 500, "valid_from": "` + time.Now().Add(24*time.Hour).UTC().Format(time.RFC3339Nano) + `"}`
		req, _ := http.NewRequest(http.MethodPost, "/admin/commercial-terms", bytes.NewBufferString(reqBody))
		req.Header.Set("Idempotency-Key", ik)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		if w.Code != http.StatusInternalServerError {
			t.Fatalf("expected 500 for duplicate audit marker, got %d", w.Code)
		}
	})

	t.Run("Replay does not add audit logs", func(t *testing.T) {
		ik := uuid.New().String()
		oid := createTestOwner()
		reqBody := `{"owner_profile_id": "` + oid + `", "label": "Replay Audit", "phase": "TRIAL", "finance_mode": "SIMULATION", "collection_method": "NONE", "commission_bps": 100, "valid_from": "` + time.Now().Add(24*time.Hour).UTC().Format(time.RFC3339Nano) + `"}`
		
		req1, _ := http.NewRequest(http.MethodPost, "/admin/commercial-terms", bytes.NewBufferString(reqBody))
		req1.Header.Set("Idempotency-Key", ik)
		w1 := httptest.NewRecorder()
		r.ServeHTTP(w1, req1)
		if w1.Code != http.StatusCreated {
			t.Fatalf("expected 201, got %d", w1.Code)
		}

		var countBefore int
		pool.QueryRow(ctx, "SELECT count(*) FROM platform_audit_logs").Scan(&countBefore)

		req2, _ := http.NewRequest(http.MethodPost, "/admin/commercial-terms", bytes.NewBufferString(reqBody))
		req2.Header.Set("Idempotency-Key", ik)
		w2 := httptest.NewRecorder()
		r.ServeHTTP(w2, req2)
		if w2.Code != http.StatusCreated {
			t.Fatalf("expected 201 for replay, got %d", w2.Code)
		}

		var countAfter int
		pool.QueryRow(ctx, "SELECT count(*) FROM platform_audit_logs").Scan(&countAfter)

		if countBefore != countAfter {
			t.Fatalf("expected %d audit logs, got %d after replay", countBefore, countAfter)
		}
	})

	t.Run("Historical row strictly unchanged and rollback audit on failure", func(t *testing.T) {
		ik1 := uuid.New().String()
		oid := createTestOwner()
		validFrom1 := time.Now().Add(24*time.Hour).UTC().Truncate(time.Microsecond)
		reqBody1 := `{"owner_profile_id": "` + oid + `", "label": "First Term", "phase": "TRIAL", "finance_mode": "SIMULATION", "collection_method": "NONE", "commission_bps": 100, "valid_from": "` + validFrom1.Format(time.RFC3339Nano) + `"}`
		req1, _ := http.NewRequest(http.MethodPost, "/admin/commercial-terms", bytes.NewBufferString(reqBody1))
		req1.Header.Set("Idempotency-Key", ik1)
		w1 := httptest.NewRecorder()
		r.ServeHTTP(w1, req1)
		if w1.Code != http.StatusCreated {
			t.Fatalf("expected 201, got %d", w1.Code)
		}

		// Ensure first term is there
		var term1ID, term1Label string
		var term1ValidUntil *time.Time
		err := pool.QueryRow(ctx, "SELECT id, label, valid_until FROM platform_commercial_terms WHERE owner_profile_id = $1 AND label = 'First Term'", oid).Scan(&term1ID, &term1Label, &term1ValidUntil)
		if err != nil {
			t.Fatalf("failed to query first term: %v", err)
		}

		// Trigger failure (invalid valid_from <= old.valid_from)
		ik2 := uuid.New().String()
		validFrom2 := validFrom1.Add(-1 * time.Hour) // Past!
		reqBody2 := `{"owner_profile_id": "` + oid + `", "label": "Second Term", "phase": "TRIAL", "finance_mode": "SIMULATION", "collection_method": "NONE", "commission_bps": 200, "valid_from": "` + validFrom2.Format(time.RFC3339Nano) + `"}`
		req2, _ := http.NewRequest(http.MethodPost, "/admin/commercial-terms", bytes.NewBufferString(reqBody2))
		req2.Header.Set("Idempotency-Key", ik2)
		w2 := httptest.NewRecorder()
		r.ServeHTTP(w2, req2)
		if w2.Code != http.StatusConflict {
			t.Fatalf("expected 409 conflict, got %d", w2.Code)
		}

		// Verify historical row unchanged
		var term1ValidUntilAfter *time.Time
		var term1LabelAfter string
		err = pool.QueryRow(ctx, "SELECT label, valid_until FROM platform_commercial_terms WHERE id = $1", term1ID).Scan(&term1LabelAfter, &term1ValidUntilAfter)
		if err != nil {
			t.Fatalf("failed to query first term after failure: %v", err)
		}
		if term1LabelAfter != term1Label {
			t.Fatalf("historical label changed")
		}
		if term1ValidUntilAfter != nil {
			t.Fatalf("historical valid_until changed to %v, expected nil", term1ValidUntilAfter)
		}

		// Verify no audit log for failed tx
		var count int
		pool.QueryRow(ctx, "SELECT count(*) FROM platform_audit_logs WHERE correlation_id = $1", ik2).Scan(&count)
		if count > 0 {
			t.Fatalf("expected 0 audit logs for failed tx, got %d", count)
		}
	})
}
