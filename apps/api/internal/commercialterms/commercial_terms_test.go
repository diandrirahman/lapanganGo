package commercialterms_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"lapangango-api/internal/audit"
	"lapangango-api/internal/commercialterms"
	"lapangango-api/internal/database"
)

func setupRouter(repo commercialterms.Repository, dbPool *pgxpool.Pool, auditSvc audit.PlatformService) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.Default()

	svc := commercialterms.NewService(repo, dbPool, auditSvc)

	// Mock auth middlewares
	authMiddleware := func(c *gin.Context) {
		role := c.GetHeader("X-Mock-Role")
		if role == "" {
			c.AbortWithStatus(http.StatusUnauthorized)
			return
		}
		c.Set("auth_user_role", role)
		c.Next()
	}
	requireActiveUser := func(c *gin.Context) {
		status := c.GetHeader("X-Mock-Status")
		if status == "SUSPENDED" {
			c.AbortWithStatus(http.StatusForbidden)
			return
		}
		c.Next()
	}
	requireRole := func(roles ...string) gin.HandlerFunc {
		return func(c *gin.Context) {
			userRole := c.GetString("auth_user_role")
			for _, r := range roles {
				if userRole == r {
					c.Next()
					return
				}
			}
			c.AbortWithStatus(http.StatusForbidden)
		}
	}

	commercialterms.RegisterRoutes(r, authMiddleware, requireActiveUser, requireRole, svc)
	return r
}

func TestCommercialTermsPreview(t *testing.T) {
	// Preview doesn't need DB, we can unit test it right here.
	r := setupRouter(nil, nil, nil)

	t.Run("Valid preview 700 bps", func(t *testing.T) {
		reqBody := `{"commission_bps": 700, "finance_mode": "SIMULATION", "collection_method": "NONE", "valid_from": "2026-08-01T00:00:00Z"}`
		req, _ := http.NewRequest(http.MethodPost, "/admin/commercial-terms/preview", bytes.NewBufferString(reqBody))
		req.Header.Set("X-Mock-Role", "SUPER_ADMIN")

		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d. body: %s", w.Code, w.Body.String())
		}

		var res commercialterms.PreviewResponse
		json.Unmarshal(w.Body.Bytes(), &res)

		if len(res.Scenarios) != 3 {
			t.Fatalf("expected 3 scenarios")
		}

		// check 100k
		s1 := res.Scenarios[0]
		if s1.BookingAmountInt64 != 100000 || s1.ProjectedCommissionRupiah != 7000 || s1.ProjectedOwnerNetRupiah != 93000 {
			t.Fatalf("invalid calculation for 100k: %+v", s1)
		}

		// check 500k
		s3 := res.Scenarios[2]
		if s3.BookingAmountInt64 != 500000 || s3.ProjectedCommissionRupiah != 35000 || s3.ProjectedOwnerNetRupiah != 465000 {
			t.Fatalf("invalid calculation for 500k: %+v", s3)
		}
	})

	t.Run("Valid preview 500 bps", func(t *testing.T) {
		reqBody := `{"commission_bps": 500, "finance_mode": "SIMULATION", "collection_method": "NONE", "valid_from": "2026-08-01T00:00:00Z"}`
		req, _ := http.NewRequest(http.MethodPost, "/admin/commercial-terms/preview", bytes.NewBufferString(reqBody))
		req.Header.Set("X-Mock-Role", "SUPER_ADMIN")

		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d. body: %s", w.Code, w.Body.String())
		}

		var res commercialterms.PreviewResponse
		json.Unmarshal(w.Body.Bytes(), &res)

		// check 200k
		s2 := res.Scenarios[1]
		if s2.BookingAmountInt64 != 200000 || s2.ProjectedCommissionRupiah != 10000 || s2.ProjectedOwnerNetRupiah != 190000 {
			t.Fatalf("invalid calculation for 200k at 500 bps: %+v", s2)
		}
	})

	t.Run("Valid preview 0 bps", func(t *testing.T) {
		reqBody := `{"commission_bps": 0, "finance_mode": "SIMULATION", "collection_method": "NONE", "valid_from": "2026-08-01T00:00:00Z"}`
		req, _ := http.NewRequest(http.MethodPost, "/admin/commercial-terms/preview", bytes.NewBufferString(reqBody))
		req.Header.Set("X-Mock-Role", "SUPER_ADMIN")

		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d. body: %s", w.Code, w.Body.String())
		}

		var res commercialterms.PreviewResponse
		json.Unmarshal(w.Body.Bytes(), &res)

		// check 500k
		s3 := res.Scenarios[2]
		if s3.BookingAmountInt64 != 500000 || s3.ProjectedCommissionRupiah != 0 || s3.ProjectedOwnerNetRupiah != 500000 {
			t.Fatalf("invalid calculation for 500k at 0 bps: %+v", s3)
		}
	})

	t.Run("Invalid bps", func(t *testing.T) {
		reqBody := `{"commission_bps": 5000, "finance_mode": "SIMULATION", "collection_method": "NONE", "valid_from": "2026-08-01T00:00:00Z"}`
		req, _ := http.NewRequest(http.MethodPost, "/admin/commercial-terms/preview", bytes.NewBufferString(reqBody))
		req.Header.Set("X-Mock-Role", "SUPER_ADMIN")

		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		if w.Code != http.StatusBadRequest {
			t.Fatalf("expected 400 for out of bounds bps, got %d", w.Code)
		}
	})

	t.Run("LIVE mode rejected", func(t *testing.T) {
		reqBody := `{"commission_bps": 700, "finance_mode": "LIVE", "collection_method": "NONE", "valid_from": "2026-08-01T00:00:00Z"}`
		req, _ := http.NewRequest(http.MethodPost, "/admin/commercial-terms/preview", bytes.NewBufferString(reqBody))
		req.Header.Set("X-Mock-Role", "SUPER_ADMIN")

		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		if w.Code != http.StatusBadRequest {
			t.Fatalf("expected 400 for LIVE mode, got %d", w.Code)
		}
	})

	t.Run("Invalid window", func(t *testing.T) {
		reqBody := `{"commission_bps": 700, "finance_mode": "SIMULATION", "collection_method": "NONE", "valid_from": "2026-08-01T00:00:00Z", "valid_until": "2026-07-01T00:00:00Z"}`
		req, _ := http.NewRequest(http.MethodPost, "/admin/commercial-terms/preview", bytes.NewBufferString(reqBody))
		req.Header.Set("X-Mock-Role", "SUPER_ADMIN")

		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		if w.Code != http.StatusBadRequest {
			t.Fatalf("expected 400 for invalid window, got %d", w.Code)
		}
	})
}

func TestCommercialTermsAuthMatrix(t *testing.T) {
	r := setupRouter(nil, nil, nil)

	tests := []struct {
		name       string
		role       string
		status     string
		statusCode int
	}{
		{"Active Super Admin", "SUPER_ADMIN", "ACTIVE", http.StatusOK},
		{"Suspended Super Admin", "SUPER_ADMIN", "SUSPENDED", http.StatusForbidden},
		{"Active Staff", "STAFF", "ACTIVE", http.StatusForbidden},
		{"Active Owner", "OWNER", "ACTIVE", http.StatusForbidden},
		{"Active Customer", "CUSTOMER", "ACTIVE", http.StatusForbidden},
		{"Anonymous", "", "", http.StatusUnauthorized},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reqBody := `{"commission_bps": 700, "finance_mode": "SIMULATION", "collection_method": "NONE", "valid_from": "2026-08-01T00:00:00Z"}`
			req, _ := http.NewRequest(http.MethodPost, "/admin/commercial-terms/preview", bytes.NewBufferString(reqBody))
			if tt.role != "" {
				req.Header.Set("X-Mock-Role", tt.role)
			}
			if tt.status != "" {
				req.Header.Set("X-Mock-Status", tt.status)
			}

			w := httptest.NewRecorder()
			r.ServeHTTP(w, req)

			if w.Code != tt.statusCode {
				t.Fatalf("expected %d, got %d", tt.statusCode, w.Code)
			}
		})
	}
}

func TestCommercialTermsIntegration(t *testing.T) {
	if os.Getenv("TEST_INTEGRATION") != "1" {
		t.Skip("Skipping integration tests. Set TEST_INTEGRATION=1 to run.")
	}
	dbURL := os.Getenv("TEST_DATABASE_URL")
	if dbURL == "" {
		t.Skip("TEST_DATABASE_URL not set, skipping integration test")
	}

	ctx := context.Background()
	pool, err := database.NewPostgresPool(ctx, dbURL)
	if err != nil {
		t.Fatalf("Failed to connect to database: %v", err)
	}
	defer pool.Close()

	// Re-insert global seed if missing (safeguard)
	_, err = pool.Exec(ctx, `
		INSERT INTO platform_commercial_terms (
			id, owner_profile_id, label, phase,
			finance_mode, collection_method, commission_bps,
			valid_from, valid_until, supersedes_id, created_by_user_id
		) VALUES (
			'00000000-0000-0000-0000-000000000000',
			NULL, 'Platform Frozen 500 bps (Mock)', 'TRIAL',
			'SIMULATION', 'NONE', 500,
			'2025-01-01T00:00:00Z', NULL, NULL, NULL
		) ON CONFLICT (id) DO UPDATE SET valid_until = NULL, supersedes_id = NULL
	`)
	if err != nil {
		t.Fatalf("Failed to re-insert global seed: %v", err)
	}

	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("Failed to begin transaction: %v", err)
	}
	defer tx.Rollback(ctx)

	// Insert test data
	adminID := uuid.New().String()
	tx.Exec(ctx, "INSERT INTO users (id, name, email, password_hash, role) VALUES ($1, 'Admin', $2, 'hash', 'SUPER_ADMIN')", adminID, "admin."+adminID+"@test.local")

	ownerID1 := uuid.New().String()
	_, err = tx.Exec(ctx, "INSERT INTO users (id, name, email, password_hash, role) VALUES ($1, 'Owner 1', $2, 'hash', 'OWNER')", ownerID1, "owner1."+ownerID1+"@test.local")
	if err != nil {
		t.Fatalf("err user 1: %v", err)
	}
	_, err = tx.Exec(ctx, "INSERT INTO owner_profiles (id, user_id, business_name, verification_status) VALUES ($1, $2, 'B1', 'APPROVED')", ownerID1, ownerID1)
	if err != nil {
		t.Fatalf("err owner 1: %v", err)
	}

	ownerID2 := uuid.New().String()
	_, err = tx.Exec(ctx, "INSERT INTO users (id, name, email, password_hash, role) VALUES ($1, 'Owner 2', $2, 'hash', 'OWNER')", ownerID2, "owner2."+ownerID2+"@test.local")
	if err != nil {
		t.Fatalf("err user 2: %v", err)
	}
	_, err = tx.Exec(ctx, "INSERT INTO owner_profiles (id, user_id, business_name, verification_status) VALUES ($1, $2, 'B2', 'APPROVED')", ownerID2, ownerID2)
	if err != nil {
		t.Fatalf("err owner 2: %v", err)
	}
	t.Cleanup(func() {
		pool.Exec(ctx, "DELETE FROM platform_commercial_terms WHERE owner_profile_id IN ($1, $2)", ownerID1, ownerID2)
		pool.Exec(ctx, "DELETE FROM owner_profiles WHERE id IN ($1, $2)", ownerID1, ownerID2)
		pool.Exec(ctx, "DELETE FROM users WHERE id IN ($1, $2)", ownerID1, ownerID2)
	})

	now := time.Now()
	yesterday := now.Add(-24 * time.Hour)
	tomorrow := now.Add(24 * time.Hour)

	// Global default is already seeded by migration 019
	var globalDefaultID string
	err = tx.QueryRow(ctx, "SELECT id FROM platform_commercial_terms WHERE owner_profile_id IS NULL AND valid_until IS NULL").Scan(&globalDefaultID)
	if err != nil {
		t.Fatalf("err querying seeded global: %v", err)
	}

	// Owner 1 historical
	owner1HistID := uuid.New().String()
	_, err = tx.Exec(ctx, "INSERT INTO platform_commercial_terms (id, owner_profile_id, label, phase, finance_mode, collection_method, commission_bps, valid_from, valid_until, created_by_user_id) VALUES ($1, $2, 'Hist', 'TRIAL', 'SIMULATION', 'NONE', 0, $3, $4, $5)", owner1HistID, ownerID1, yesterday.Add(-24*time.Hour), yesterday, adminID)
	if err != nil {
		t.Fatalf("err inserting hist: %v", err)
	}

	// Owner 1 current
	owner1CurrID := uuid.New().String()
	_, err = tx.Exec(ctx, "INSERT INTO platform_commercial_terms (id, owner_profile_id, label, phase, finance_mode, collection_method, commission_bps, valid_from, valid_until, supersedes_id, created_by_user_id) VALUES ($1, $2, 'Curr', 'STANDARD', 'SIMULATION', 'NONE', 500, $3, $4, $5, $6)", owner1CurrID, ownerID1, yesterday, tomorrow, owner1HistID, adminID)
	if err != nil {
		t.Fatalf("err inserting curr: %v", err)
	}

	// Owner 1 scheduled
	owner1SchedID := uuid.New().String()
	_, err = tx.Exec(ctx, "INSERT INTO platform_commercial_terms (id, owner_profile_id, label, phase, finance_mode, collection_method, commission_bps, valid_from, supersedes_id, created_by_user_id) VALUES ($1, $2, 'Sched', 'STANDARD', 'SIMULATION', 'NONE', 600, $3, $4, $5)", owner1SchedID, ownerID1, tomorrow, owner1CurrID, adminID)
	if err != nil {
		t.Fatalf("err inserting sched: %v", err)
	}

	repo := commercialterms.NewRepository(tx)
	auditRepo := audit.NewPlatformRepository()
	auditSvc := audit.NewPlatformService(auditRepo)
	r := setupRouter(repo, pool, auditSvc)

	t.Run("Get ALL terms (no filters)", func(t *testing.T) {
		req, _ := http.NewRequest(http.MethodGet, "/admin/commercial-terms", nil)
		req.Header.Set("X-Mock-Role", "SUPER_ADMIN")
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d. body: %s", w.Code, w.Body.String())
		}

		var res commercialterms.PaginatedTermsResponse
		json.Unmarshal(w.Body.Bytes(), &res)

		if res.TotalItems != 4 {
			t.Fatalf("expected 4 total items, got %d", res.TotalItems)
		}
		// Check sorting: scheduled (valid_from tomorrow) should be first
		if res.Data[0].ID != owner1SchedID {
			t.Fatalf("expected scheduled to be first due to valid_from desc")
		}
	})

	t.Run("Get GLOBAL terms", func(t *testing.T) {
		req, _ := http.NewRequest(http.MethodGet, "/admin/commercial-terms?scope=GLOBAL", nil)
		req.Header.Set("X-Mock-Role", "SUPER_ADMIN")
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		var res commercialterms.PaginatedTermsResponse
		json.Unmarshal(w.Body.Bytes(), &res)

		if res.TotalItems != 1 || res.Data[0].ID != globalDefaultID {
			t.Fatalf("expected 1 global item")
		}
		if res.Data[0].Status != "CURRENT" {
			t.Fatalf("expected global to be CURRENT, got %s", res.Data[0].Status)
		}
	})

	t.Run("Get OWNER terms filtered by owner_profile_id", func(t *testing.T) {
		req, _ := http.NewRequest(http.MethodGet, "/admin/commercial-terms?scope=OWNER&owner_profile_id="+ownerID1, nil)
		req.Header.Set("X-Mock-Role", "SUPER_ADMIN")
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		var res commercialterms.PaginatedTermsResponse
		json.Unmarshal(w.Body.Bytes(), &res)

		if res.TotalItems != 3 {
			t.Fatalf("expected 3 owner items")
		}
	})

	t.Run("Get OWNER terms missing owner_profile_id returns 400", func(t *testing.T) {
		req, _ := http.NewRequest(http.MethodGet, "/admin/commercial-terms?scope=OWNER", nil)
		req.Header.Set("X-Mock-Role", "SUPER_ADMIN")
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		if w.Code != http.StatusBadRequest {
			t.Fatalf("expected 400, got %d", w.Code)
		}
	})

	t.Run("Filter by status CURRENT", func(t *testing.T) {
		req, _ := http.NewRequest(http.MethodGet, "/admin/commercial-terms?status=CURRENT", nil)
		req.Header.Set("X-Mock-Role", "SUPER_ADMIN")
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		var res commercialterms.PaginatedTermsResponse
		json.Unmarshal(w.Body.Bytes(), &res)

		// Should find Global Default and Owner 1 Current
		if res.TotalItems != 2 {
			t.Fatalf("expected 2 current items, got %d", res.TotalItems)
		}
		for _, item := range res.Data {
			if item.Status != "CURRENT" {
				t.Fatalf("expected item status CURRENT, got %s", item.Status)
			}
		}
	})

	t.Run("Empty state returns items: [] not null", func(t *testing.T) {
		req, _ := http.NewRequest(http.MethodGet, "/admin/commercial-terms?scope=OWNER&owner_profile_id="+ownerID2, nil)
		req.Header.Set("X-Mock-Role", "SUPER_ADMIN")
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		var res commercialterms.PaginatedTermsResponse
		json.Unmarshal(w.Body.Bytes(), &res)

		if res.TotalItems != 0 {
			t.Fatalf("expected 0 items")
		}
		if res.Data == nil {
			t.Fatalf("expected Data to be [] not nil")
		}
	})

	t.Run("Pagination limits", func(t *testing.T) {
		req, _ := http.NewRequest(http.MethodGet, "/admin/commercial-terms?limit=2&page=1", nil)
		req.Header.Set("X-Mock-Role", "SUPER_ADMIN")
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		var res commercialterms.PaginatedTermsResponse
		json.Unmarshal(w.Body.Bytes(), &res)

		if len(res.Data) != 2 {
			t.Fatalf("expected 2 items due to limit")
		}
		if res.TotalPages != 2 {
			t.Fatalf("expected 2 pages for 4 items with limit 2")
		}
	})
}
