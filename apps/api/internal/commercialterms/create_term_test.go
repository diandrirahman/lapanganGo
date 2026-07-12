package commercialterms_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
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
	auditRepo := audit.NewPlatformRepository()
	return setupCreateRouterWithAudit(pool, adminID, audit.NewPlatformService(auditRepo))
}

func setupCreateRouterWithAudit(pool *pgxpool.Pool, adminID string, auditSvc audit.PlatformService) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.Default()

	repo := commercialterms.NewRepository(pool)
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

type failingCreatedAuditService struct{}

func (failingCreatedAuditService) Record(_ context.Context, _ audit.DBTX, params audit.CreatePlatformAuditLogParams) error {
	if params.Action == audit.ActionPlatformCommercialTermCreated {
		return errors.New("injected created audit failure")
	}
	return nil
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
	t.Cleanup(pool.Close)

	cleanupExec := func(query string, args ...any) {
		t.Helper()
		if _, err := pool.Exec(context.Background(), query, args...); err != nil {
			t.Errorf("test cleanup failed: %v", err)
		}
	}

	adminID := uuid.New().String()
	_, err = pool.Exec(ctx, "INSERT INTO users (id, name, email, password_hash, role) VALUES ($1, 'Admin Create', $2, 'hash', 'SUPER_ADMIN')", adminID, "admin."+adminID+"@test.local")
	if err != nil {
		t.Fatalf("failed to insert admin: %v", err)
	}

	t.Cleanup(func() {
		cleanupExec("DELETE FROM platform_commercial_terms WHERE created_by_user_id = $1", adminID)
		cleanupExec("DELETE FROM platform_audit_logs WHERE actor_user_id = $1", adminID)
		cleanupExec("DELETE FROM users WHERE id = $1", adminID)
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
			cleanupExec("DELETE FROM platform_commercial_terms WHERE owner_profile_id = $1", newOwnerID)
			cleanupExec("DELETE FROM owner_profiles WHERE id = $1", newOwnerID)
			cleanupExec("DELETE FROM users WHERE id = $1", newOwnerID)
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

		createdCount := 0
		conflictCount := 0
		for c := range codes {
			if c == http.StatusCreated {
				createdCount++
			}
			if c == http.StatusConflict {
				conflictCount++
			}
		}
		if createdCount != 1 || conflictCount != 1 {
			t.Errorf("expected exactly one 201 and one 409, got created=%d conflict=%d", createdCount, conflictCount)
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

	t.Run("LIVE replay with different payload conflicts", func(t *testing.T) {
		ik := uuid.New().String()
		oid := createTestOwner()
		validFrom := time.Now().Add(24 * time.Hour).UTC().Format(time.RFC3339Nano)
		firstBody := `{"owner_profile_id": "` + oid + `", "label": "Promo LIVE", "phase": "TRIAL", "finance_mode": "LIVE", "collection_method": "NONE", "commission_bps": 500, "valid_from": "` + validFrom + `"}`
		secondBody := `{"owner_profile_id": "` + oid + `", "label": "Different LIVE", "phase": "TRIAL", "finance_mode": "LIVE", "collection_method": "NONE", "commission_bps": 500, "valid_from": "` + validFrom + `"}`

		first, _ := http.NewRequest(http.MethodPost, "/admin/commercial-terms", bytes.NewBufferString(firstBody))
		first.Header.Set("Idempotency-Key", ik)
		firstResponse := httptest.NewRecorder()
		r.ServeHTTP(firstResponse, first)
		if firstResponse.Code != http.StatusForbidden {
			t.Fatalf("expected initial LIVE request to return 403, got %d", firstResponse.Code)
		}

		second, _ := http.NewRequest(http.MethodPost, "/admin/commercial-terms", bytes.NewBufferString(secondBody))
		second.Header.Set("Idempotency-Key", ik)
		secondResponse := httptest.NewRecorder()
		r.ServeHTTP(secondResponse, second)
		if secondResponse.Code != http.StatusConflict {
			t.Fatalf("expected changed LIVE replay to return 409, got %d", secondResponse.Code)
		}
	})

	t.Run("Idempotency key cannot cross from LIVE to SIMULATION", func(t *testing.T) {
		ik := uuid.New().String()
		oid := createTestOwner()
		validFrom := time.Now().Add(24 * time.Hour).UTC().Format(time.RFC3339Nano)
		liveBody := `{"owner_profile_id": "` + oid + `", "label": "Cross Outcome", "phase": "TRIAL", "finance_mode": "LIVE", "collection_method": "NONE", "commission_bps": 500, "valid_from": "` + validFrom + `"}`
		simulationBody := `{"owner_profile_id": "` + oid + `", "label": "Cross Outcome", "phase": "TRIAL", "finance_mode": "SIMULATION", "collection_method": "NONE", "commission_bps": 500, "valid_from": "` + validFrom + `"}`

		live, _ := http.NewRequest(http.MethodPost, "/admin/commercial-terms", bytes.NewBufferString(liveBody))
		live.Header.Set("Idempotency-Key", ik)
		liveResponse := httptest.NewRecorder()
		r.ServeHTTP(liveResponse, live)
		if liveResponse.Code != http.StatusForbidden {
			t.Fatalf("expected LIVE request to return 403, got %d", liveResponse.Code)
		}

		simulation, _ := http.NewRequest(http.MethodPost, "/admin/commercial-terms", bytes.NewBufferString(simulationBody))
		simulation.Header.Set("Idempotency-Key", ik)
		simulationResponse := httptest.NewRecorder()
		r.ServeHTTP(simulationResponse, simulation)
		if simulationResponse.Code != http.StatusConflict {
			t.Fatalf("expected cross-outcome replay to return 409, got %d", simulationResponse.Code)
		}

		var terms, rejectedAudits, createdAudits int
		if err := pool.QueryRow(ctx, "SELECT count(*) FROM platform_commercial_terms WHERE owner_profile_id = $1", oid).Scan(&terms); err != nil {
			t.Fatalf("failed to count terms: %v", err)
		}
		if err := pool.QueryRow(ctx, "SELECT count(*) FROM platform_audit_logs WHERE correlation_id = $1 AND action = $2", ik, audit.ActionPlatformCommercialTermLiveRejected).Scan(&rejectedAudits); err != nil {
			t.Fatalf("failed to count rejected audits: %v", err)
		}
		if err := pool.QueryRow(ctx, "SELECT count(*) FROM platform_audit_logs WHERE correlation_id = $1 AND action = $2", ik, audit.ActionPlatformCommercialTermCreated).Scan(&createdAudits); err != nil {
			t.Fatalf("failed to count created audits: %v", err)
		}
		if terms != 0 || rejectedAudits != 1 || createdAudits != 0 {
			t.Fatalf("unexpected cross-outcome state: terms=%d rejected_audits=%d created_audits=%d", terms, rejectedAudits, createdAudits)
		}
	})

	t.Run("Idempotency key cannot cross from SIMULATION to LIVE", func(t *testing.T) {
		ik := uuid.New().String()
		oid := createTestOwner()
		validFrom := time.Now().Add(24 * time.Hour).UTC().Format(time.RFC3339Nano)
		simulationBody := `{"owner_profile_id": "` + oid + `", "label": "Cross Outcome", "phase": "TRIAL", "finance_mode": "SIMULATION", "collection_method": "NONE", "commission_bps": 500, "valid_from": "` + validFrom + `"}`
		liveBody := `{"owner_profile_id": "` + oid + `", "label": "Cross Outcome", "phase": "TRIAL", "finance_mode": "LIVE", "collection_method": "NONE", "commission_bps": 500, "valid_from": "` + validFrom + `"}`

		simulation, _ := http.NewRequest(http.MethodPost, "/admin/commercial-terms", bytes.NewBufferString(simulationBody))
		simulation.Header.Set("Idempotency-Key", ik)
		simulationResponse := httptest.NewRecorder()
		r.ServeHTTP(simulationResponse, simulation)
		if simulationResponse.Code != http.StatusCreated {
			t.Fatalf("expected SIMULATION request to return 201, got %d: %s", simulationResponse.Code, simulationResponse.Body.String())
		}

		live, _ := http.NewRequest(http.MethodPost, "/admin/commercial-terms", bytes.NewBufferString(liveBody))
		live.Header.Set("Idempotency-Key", ik)
		liveResponse := httptest.NewRecorder()
		r.ServeHTTP(liveResponse, live)
		if liveResponse.Code != http.StatusConflict {
			t.Fatalf("expected cross-outcome replay to return 409, got %d", liveResponse.Code)
		}

		var terms, rejectedAudits, createdAudits int
		if err := pool.QueryRow(ctx, "SELECT count(*) FROM platform_commercial_terms WHERE owner_profile_id = $1", oid).Scan(&terms); err != nil {
			t.Fatalf("failed to count terms: %v", err)
		}
		if err := pool.QueryRow(ctx, "SELECT count(*) FROM platform_audit_logs WHERE correlation_id = $1 AND action = $2", ik, audit.ActionPlatformCommercialTermLiveRejected).Scan(&rejectedAudits); err != nil {
			t.Fatalf("failed to count rejected audits: %v", err)
		}
		if err := pool.QueryRow(ctx, "SELECT count(*) FROM platform_audit_logs WHERE correlation_id = $1 AND action = $2", ik, audit.ActionPlatformCommercialTermCreated).Scan(&createdAudits); err != nil {
			t.Fatalf("failed to count created audits: %v", err)
		}
		if terms != 1 || rejectedAudits != 0 || createdAudits != 1 {
			t.Fatalf("unexpected cross-outcome state: terms=%d rejected_audits=%d created_audits=%d", terms, rejectedAudits, createdAudits)
		}
	})

	t.Run("Global Create", func(t *testing.T) {
		ik := uuid.New().String()
		label := "Promo Global " + uuid.NewString()
		var previousGlobalID string
		if err := pool.QueryRow(ctx, "SELECT id FROM platform_commercial_terms WHERE owner_profile_id IS NULL AND valid_until IS NULL").Scan(&previousGlobalID); err != nil {
			t.Fatalf("failed to locate current global term: %v", err)
		}
		reqBody := `{"label": "` + label + `", "phase": "TRIAL", "finance_mode": "SIMULATION", "collection_method": "NONE", "commission_bps": 500, "valid_from": "` + time.Now().Add(24*time.Hour).UTC().Format(time.RFC3339Nano) + `"}`

		req, _ := http.NewRequest(http.MethodPost, "/admin/commercial-terms", bytes.NewBufferString(reqBody))
		req.Header.Set("Idempotency-Key", ik)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		if w.Code != http.StatusCreated {
			t.Fatalf("expected 201 for global create, got %d", w.Code)
		}
		var created commercialterms.CommercialTerm
		if err := json.Unmarshal(w.Body.Bytes(), &created); err != nil {
			t.Fatalf("failed to decode global create response: %v", err)
		}

		t.Cleanup(func() {
			cleanupExec("DELETE FROM platform_commercial_terms WHERE id = $1", created.ID)
			cleanupExec("UPDATE platform_commercial_terms SET valid_until = NULL WHERE id = $1", previousGlobalID)
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

	t.Run("Correct supersession, adjacent boundaries, and historical immutability", func(t *testing.T) {
		oid := createTestOwner()
		historicalID := uuid.NewString()
		historicalFrom := time.Now().Add(-48 * time.Hour).UTC().Truncate(time.Microsecond)
		historicalUntil := time.Now().Add(-24 * time.Hour).UTC().Truncate(time.Microsecond)
		_, err := pool.Exec(ctx, `
			INSERT INTO platform_commercial_terms (
				id, owner_profile_id, label, phase, finance_mode, collection_method,
				commission_bps, valid_from, valid_until, created_by_user_id
			) VALUES ($1, $2, 'Immutable Historical', 'TRIAL', 'SIMULATION', 'NONE', 50, $3, $4, $5)
		`, historicalID, oid, historicalFrom, historicalUntil, adminID)
		if err != nil {
			t.Fatalf("failed to insert historical fixture: %v", err)
		}

		ik1 := uuid.New().String()
		validFrom1 := time.Now().Add(24 * time.Hour).UTC().Truncate(time.Microsecond)
		reqBody1 := `{"owner_profile_id": "` + oid + `", "label": "Term 1", "phase": "TRIAL", "finance_mode": "SIMULATION", "collection_method": "NONE", "commission_bps": 100, "valid_from": "` + validFrom1.Format(time.RFC3339Nano) + `"}`
		req1, _ := http.NewRequest(http.MethodPost, "/admin/commercial-terms", bytes.NewBufferString(reqBody1))
		req1.Header.Set("Idempotency-Key", ik1)
		w1 := httptest.NewRecorder()
		r.ServeHTTP(w1, req1)
		if w1.Code != http.StatusCreated {
			t.Fatalf("failed to create term 1: %d", w1.Code)
		}
		var term1 commercialterms.CommercialTerm
		if err := json.Unmarshal(w1.Body.Bytes(), &term1); err != nil {
			t.Fatalf("failed to decode term 1: %v", err)
		}

		ik2 := uuid.New().String()
		validFrom2 := time.Now().Add(48 * time.Hour).UTC().Truncate(time.Microsecond)
		reqBody2 := `{"owner_profile_id": "` + oid + `", "label": "Term 2", "phase": "STANDARD", "finance_mode": "SIMULATION", "collection_method": "NONE", "commission_bps": 200, "valid_from": "` + validFrom2.Format(time.RFC3339Nano) + `"}`
		req2, _ := http.NewRequest(http.MethodPost, "/admin/commercial-terms", bytes.NewBufferString(reqBody2))
		req2.Header.Set("Idempotency-Key", ik2)
		w2 := httptest.NewRecorder()
		r.ServeHTTP(w2, req2)
		if w2.Code != http.StatusCreated {
			t.Fatalf("failed to supersede with term 2: %d", w2.Code)
		}
		var term2 commercialterms.CommercialTerm
		if err := json.Unmarshal(w2.Body.Bytes(), &term2); err != nil {
			t.Fatalf("failed to decode term 2: %v", err)
		}

		var oldValidUntil time.Time
		if err := pool.QueryRow(ctx, "SELECT valid_until FROM platform_commercial_terms WHERE id = $1", term1.ID).Scan(&oldValidUntil); err != nil {
			t.Fatalf("failed to read superseded term: %v", err)
		}
		if !oldValidUntil.Equal(validFrom2) {
			t.Fatalf("old.valid_until=%s, expected %s", oldValidUntil, validFrom2)
		}
		if term2.SupersedesID == nil || *term2.SupersedesID != term1.ID {
			t.Fatalf("new.supersedes_id does not reference old term")
		}

		var supersededAuditCount, createdAuditCount int
		if err := pool.QueryRow(ctx, "SELECT count(*) FROM platform_audit_logs WHERE correlation_id = $1 AND action = 'PLATFORM_COMMERCIAL_TERM_SUPERSEDED'", ik2).Scan(&supersededAuditCount); err != nil {
			t.Fatalf("failed to count superseded audit: %v", err)
		}
		if err := pool.QueryRow(ctx, "SELECT count(*) FROM platform_audit_logs WHERE correlation_id = $1 AND action = 'PLATFORM_COMMERCIAL_TERM_CREATED'", ik2).Scan(&createdAuditCount); err != nil {
			t.Fatalf("failed to count created audit: %v", err)
		}
		if supersededAuditCount != 1 || createdAuditCount != 1 {
			t.Fatalf("expected one superseded and one created audit, got %d and %d", supersededAuditCount, createdAuditCount)
		}

		var historicalLabel string
		var historicalBps int
		var historicalFromAfter, historicalUntilAfter time.Time
		if err := pool.QueryRow(ctx, "SELECT label, commission_bps, valid_from, valid_until FROM platform_commercial_terms WHERE id = $1", historicalID).Scan(&historicalLabel, &historicalBps, &historicalFromAfter, &historicalUntilAfter); err != nil {
			t.Fatalf("failed to read historical fixture: %v", err)
		}
		if historicalLabel != "Immutable Historical" || historicalBps != 50 || !historicalFromAfter.Equal(historicalFrom) || !historicalUntilAfter.Equal(historicalUntil) {
			t.Fatalf("historical term changed during supersession")
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
		if _, err := pool.Exec(ctx, "INSERT INTO platform_audit_logs (id, actor_user_id, actor_role, action, entity_type, correlation_id) VALUES ($1, $2, 'SUPER_ADMIN', 'PLATFORM_COMMERCIAL_TERM_CREATED', 'PLATFORM_COMMERCIAL_TERM', $3)", uuid.NewString(), adminID, ik); err != nil {
			t.Fatalf("failed to insert first duplicate marker fixture: %v", err)
		}
		if _, err := pool.Exec(ctx, "INSERT INTO platform_audit_logs (id, actor_user_id, actor_role, action, entity_type, correlation_id) VALUES ($1, $2, 'SUPER_ADMIN', 'PLATFORM_COMMERCIAL_TERM_CREATED', 'PLATFORM_COMMERCIAL_TERM', $3)", uuid.NewString(), adminID, ik); err != nil {
			t.Fatalf("failed to insert second duplicate marker fixture: %v", err)
		}

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
		if err := pool.QueryRow(ctx, "SELECT count(*) FROM platform_audit_logs WHERE correlation_id = $1", ik).Scan(&countBefore); err != nil {
			t.Fatalf("failed to count audit logs before replay: %v", err)
		}

		req2, _ := http.NewRequest(http.MethodPost, "/admin/commercial-terms", bytes.NewBufferString(reqBody))
		req2.Header.Set("Idempotency-Key", ik)
		w2 := httptest.NewRecorder()
		r.ServeHTTP(w2, req2)
		if w2.Code != http.StatusCreated {
			t.Fatalf("expected 201 for replay, got %d", w2.Code)
		}

		var countAfter int
		if err := pool.QueryRow(ctx, "SELECT count(*) FROM platform_audit_logs WHERE correlation_id = $1", ik).Scan(&countAfter); err != nil {
			t.Fatalf("failed to count audit logs after replay: %v", err)
		}

		if countBefore != countAfter {
			t.Fatalf("expected %d audit logs, got %d after replay", countBefore, countAfter)
		}
	})

	t.Run("Timeout after commit replays after valid_from passed", func(t *testing.T) {
		ik := uuid.NewString()
		oid := createTestOwner()
		validFrom := time.Now().Add(2 * time.Second).UTC().Truncate(time.Microsecond)
		reqBody := `{"owner_profile_id": "` + oid + `", "label": "Timeout Replay", "phase": "TRIAL", "finance_mode": "SIMULATION", "collection_method": "NONE", "commission_bps": 100, "valid_from": "` + validFrom.Format(time.RFC3339Nano) + `"}`

		req1, _ := http.NewRequest(http.MethodPost, "/admin/commercial-terms", bytes.NewBufferString(reqBody))
		req1.Header.Set("Idempotency-Key", ik)
		w1 := httptest.NewRecorder()
		r.ServeHTTP(w1, req1)
		if w1.Code != http.StatusCreated {
			t.Fatalf("expected initial 201, got %d: %s", w1.Code, w1.Body.String())
		}
		var first commercialterms.CommercialTerm
		if err := json.Unmarshal(w1.Body.Bytes(), &first); err != nil {
			t.Fatalf("failed to decode initial response: %v", err)
		}

		time.Sleep(3 * time.Second)

		req2, _ := http.NewRequest(http.MethodPost, "/admin/commercial-terms", bytes.NewBufferString(reqBody))
		req2.Header.Set("Idempotency-Key", ik)
		w2 := httptest.NewRecorder()
		r.ServeHTTP(w2, req2)
		if w2.Code != http.StatusCreated {
			t.Fatalf("expected replay 201 after valid_from passed, got %d: %s", w2.Code, w2.Body.String())
		}
		var replay commercialterms.CommercialTerm
		if err := json.Unmarshal(w2.Body.Bytes(), &replay); err != nil {
			t.Fatalf("failed to decode replay response: %v", err)
		}
		if replay.ID != first.ID || replay.Label != first.Label || replay.Status != first.Status || !replay.ValidFrom.Equal(first.ValidFrom) {
			t.Fatalf("replay response differs from original: first=%+v replay=%+v", first, replay)
		}
		var auditCount int
		if err := pool.QueryRow(ctx, "SELECT count(*) FROM platform_audit_logs WHERE correlation_id = $1", ik).Scan(&auditCount); err != nil {
			t.Fatalf("failed to count timeout replay audits: %v", err)
		}
		if auditCount != 1 {
			t.Fatalf("expected one audit after timeout replay, got %d", auditCount)
		}
	})

	t.Run("Audit failure rolls back supersession and insert", func(t *testing.T) {
		ik1 := uuid.New().String()
		oid := createTestOwner()
		validFrom1 := time.Now().Add(24 * time.Hour).UTC().Truncate(time.Microsecond)
		reqBody1 := `{"owner_profile_id": "` + oid + `", "label": "First Term", "phase": "TRIAL", "finance_mode": "SIMULATION", "collection_method": "NONE", "commission_bps": 100, "valid_from": "` + validFrom1.Format(time.RFC3339Nano) + `"}`
		req1, _ := http.NewRequest(http.MethodPost, "/admin/commercial-terms", bytes.NewBufferString(reqBody1))
		req1.Header.Set("Idempotency-Key", ik1)
		w1 := httptest.NewRecorder()
		r.ServeHTTP(w1, req1)
		if w1.Code != http.StatusCreated {
			t.Fatalf("expected 201, got %d", w1.Code)
		}

		var first commercialterms.CommercialTerm
		if err := json.Unmarshal(w1.Body.Bytes(), &first); err != nil {
			t.Fatalf("failed to decode first term response: %v", err)
		}

		ik2 := uuid.New().String()
		validFrom2 := validFrom1.Add(24 * time.Hour)
		reqBody2 := `{"owner_profile_id": "` + oid + `", "label": "Second Term", "phase": "TRIAL", "finance_mode": "SIMULATION", "collection_method": "NONE", "commission_bps": 200, "valid_from": "` + validFrom2.Format(time.RFC3339Nano) + `"}`
		req2, _ := http.NewRequest(http.MethodPost, "/admin/commercial-terms", bytes.NewBufferString(reqBody2))
		req2.Header.Set("Idempotency-Key", ik2)
		w2 := httptest.NewRecorder()
		failingRouter := setupCreateRouterWithAudit(pool, adminID, failingCreatedAuditService{})
		failingRouter.ServeHTTP(w2, req2)
		if w2.Code != http.StatusInternalServerError {
			t.Fatalf("expected 500 from injected audit failure, got %d", w2.Code)
		}

		var oldValidUntil *time.Time
		if err := pool.QueryRow(ctx, "SELECT valid_until FROM platform_commercial_terms WHERE id = $1", first.ID).Scan(&oldValidUntil); err != nil {
			t.Fatalf("failed to query old term after audit failure: %v", err)
		}
		if oldValidUntil != nil {
			t.Fatalf("old term valid_until survived rollback: %v", oldValidUntil)
		}

		var newTermCount, auditCount int
		if err := pool.QueryRow(ctx, "SELECT count(*) FROM platform_commercial_terms WHERE owner_profile_id = $1 AND label = 'Second Term'", oid).Scan(&newTermCount); err != nil {
			t.Fatalf("failed to count rolled-back new terms: %v", err)
		}
		if err := pool.QueryRow(ctx, "SELECT count(*) FROM platform_audit_logs WHERE correlation_id = $1", ik2).Scan(&auditCount); err != nil {
			t.Fatalf("failed to count rolled-back audits: %v", err)
		}
		if newTermCount != 0 || auditCount != 0 {
			t.Fatalf("audit failure left partial writes: terms=%d audits=%d", newTermCount, auditCount)
		}
	})
}
