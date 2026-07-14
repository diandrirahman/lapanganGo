package bookings_test

import (
	"context"
	"errors"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/postgres"
	_ "github.com/golang-migrate/migrate/v4/source/file"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"lapangango-api/internal/bookings"
	"lapangango-api/internal/httputil"
	"lapangango-api/internal/platformfinance"
)

const (
	bookingMatrixEnabledEnv = "TEST_BOOKING_MATRIX_DISPOSABLE"
	bookingMatrixDSNEnv     = "BOOKING_MATRIX_TEST_DATABASE_URL"
)

type bookingMatrixFixture struct {
	ownerUserID    string
	customerUserID string
	superAdminID   string
	ownerProfileID string
	venueID        string
	courtID        string
	termID         string
	bookingDate    string
}

type bookingMatrixCounts struct {
	bookings         int
	snapshots        int
	offlineCustomers int
	ownerLedger      int
}

type matrixPostCommitTimeoutRepository struct {
	*bookings.Repository
	pool *pgxpool.Pool
}

func (r *matrixPostCommitTimeoutRepository) ExecuteBookingTx(ctx context.Context, fn func(pgx.Tx) error) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() {
		rollbackCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_ = tx.Rollback(rollbackCtx)
	}()

	if err := fn(tx); err != nil {
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return err
	}

	// Model the ambiguous outcome where PostgreSQL committed successfully but
	// the caller observed a transport timeout before receiving the acknowledgement.
	return context.DeadlineExceeded
}

type matrixFailingSnapshotRepository struct {
	err error
}

func (r *matrixFailingSnapshotRepository) InsertSnapshot(context.Context, platformfinance.SnapshotDBTX, platformfinance.CreateBookingFeeSnapshotParams) (*platformfinance.BookingFeeSnapshot, error) {
	return nil, r.err
}

func (r *matrixFailingSnapshotRepository) GetSnapshot(context.Context, platformfinance.SnapshotDBTX, string) (*platformfinance.BookingFeeSnapshot, error) {
	return nil, platformfinance.ErrBookingFeeSnapshotNotFound
}

type matrixPhantomSnapshotRepository struct{}

func (r *matrixPhantomSnapshotRepository) InsertSnapshot(_ context.Context, _ platformfinance.SnapshotDBTX, params platformfinance.CreateBookingFeeSnapshotParams) (*platformfinance.BookingFeeSnapshot, error) {
	// This deliberately violates the repository contract. It lets the matrix
	// prove that the deferred database guard still rejects the transaction.
	return &platformfinance.BookingFeeSnapshot{BookingID: params.BookingID}, nil
}

func (r *matrixPhantomSnapshotRepository) GetSnapshot(context.Context, platformfinance.SnapshotDBTX, string) (*platformfinance.BookingFeeSnapshot, error) {
	return nil, platformfinance.ErrBookingFeeSnapshotNotFound
}

func TestBookingRetryConcurrencyRollbackMatrix(t *testing.T) {
	if os.Getenv(bookingMatrixEnabledEnv) != "1" {
		t.Skip("set TEST_BOOKING_MATRIX_DISPOSABLE=1 to run the dedicated booking matrix")
	}
	baseDSN := strings.TrimSpace(os.Getenv(bookingMatrixDSNEnv))
	if baseDSN == "" {
		t.Fatal("BOOKING_MATRIX_TEST_DATABASE_URL must target a disposable PostgreSQL server")
	}

	dbName := "lapangango_booking_matrix_" + strings.ReplaceAll(uuid.NewString(), "-", "_")
	testDSN, dropDatabase := createBookingMatrixDatabase(t, baseDSN, dbName)
	defer dropDatabase()

	if err := runBookingMatrixMigrations(testDSN); err != nil {
		t.Fatalf("run migrations on booking matrix database: %v", err)
	}

	connectCtx, connectCancel := context.WithTimeout(context.Background(), 10*time.Second)
	poolConfig, err := pgxpool.ParseConfig(testDSN)
	if err != nil {
		connectCancel()
		t.Fatalf("parse booking matrix database config: %v", err)
	}
	poolConfig.MaxConns = 20
	pool, err := pgxpool.NewWithConfig(connectCtx, poolConfig)
	connectCancel()
	if err != nil {
		t.Fatalf("connect booking matrix database: %v", err)
	}
	defer pool.Close()

	t.Run("concurrent online create produces exactly one booking and snapshot", func(t *testing.T) {
		fixture := resetAndSetupBookingMatrix(t, pool)
		activateBookingMatrixCutover(t, pool, fixture)
		repo := bookings.NewRepository(pool)
		service := newBookingMatrixService(t, repo, actualResolverAdapter, platformfinance.NewBookingFeeSnapshotRepository())
		req := matrixOnlineRequest(fixture, "09:00", "10:00")

		errorsSeen := runConcurrentBookingCalls(5, func(ctx context.Context) error {
			_, err := service.CreateBooking(ctx, fixture.customerUserID, req)
			return err
		})
		assertConcurrentSingleWinner(t, errorsSeen)
		assertBookingMatrixCounts(t, pool, fixture, bookingMatrixCounts{bookings: 1, snapshots: 1})
	})

	t.Run("concurrent offline create produces one ledger", func(t *testing.T) {
		fixture := resetAndSetupBookingMatrix(t, pool)
		activateBookingMatrixCutover(t, pool, fixture)
		repo := bookings.NewRepository(pool)
		service := newBookingMatrixService(t, repo, actualResolverAdapter, platformfinance.NewBookingFeeSnapshotRepository())
		ownerCtx := matrixOwnerContext(fixture)
		req := matrixOfflineRequest(fixture, "10:00", "11:00")

		errorsSeen := runConcurrentBookingCalls(5, func(ctx context.Context) error {
			_, err := service.OwnerCreateOfflineBooking(ctx, ownerCtx, req)
			return err
		})
		assertConcurrentSingleWinner(t, errorsSeen)
		assertBookingMatrixCounts(t, pool, fixture, bookingMatrixCounts{
			bookings:         1,
			snapshots:        1,
			offlineCustomers: 1,
			ownerLedger:      1,
		})
	})

	t.Run("sequential retries preserve one effect", func(t *testing.T) {
		for _, testCase := range []struct {
			name     string
			offline  bool
			expected bookingMatrixCounts
		}{
			{name: "online", expected: bookingMatrixCounts{bookings: 1, snapshots: 1}},
			{name: "offline", offline: true, expected: bookingMatrixCounts{bookings: 1, snapshots: 1, offlineCustomers: 1, ownerLedger: 1}},
		} {
			testCase := testCase
			t.Run(testCase.name, func(t *testing.T) {
				fixture := resetAndSetupBookingMatrix(t, pool)
				activateBookingMatrixCutover(t, pool, fixture)
				service := newBookingMatrixService(t, bookings.NewRepository(pool), actualResolverAdapter, platformfinance.NewBookingFeeSnapshotRepository())
				ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
				defer cancel()

				var firstErr, retryErr error
				if testCase.offline {
					req := matrixOfflineRequest(fixture, "11:00", "12:00")
					_, firstErr = service.OwnerCreateOfflineBooking(ctx, matrixOwnerContext(fixture), req)
					_, retryErr = service.OwnerCreateOfflineBooking(ctx, matrixOwnerContext(fixture), req)
				} else {
					req := matrixOnlineRequest(fixture, "11:00", "12:00")
					_, firstErr = service.CreateBooking(ctx, fixture.customerUserID, req)
					_, retryErr = service.CreateBooking(ctx, fixture.customerUserID, req)
				}
				if firstErr != nil {
					t.Fatalf("first create: %v", firstErr)
				}
				if !errors.Is(retryErr, bookings.ErrOverlapBooking) {
					t.Fatalf("retry error = %v, want ErrOverlapBooking", retryErr)
				}
				assertBookingMatrixCounts(t, pool, fixture, testCase.expected)
			})
		}
	})

	t.Run("timeout after commit has stable retry state", func(t *testing.T) {
		for _, testCase := range []struct {
			name     string
			offline  bool
			expected bookingMatrixCounts
		}{
			{name: "online", expected: bookingMatrixCounts{bookings: 1, snapshots: 1}},
			{name: "offline", offline: true, expected: bookingMatrixCounts{bookings: 1, snapshots: 1, offlineCustomers: 1, ownerLedger: 1}},
		} {
			testCase := testCase
			t.Run(testCase.name, func(t *testing.T) {
				fixture := resetAndSetupBookingMatrix(t, pool)
				activateBookingMatrixCutover(t, pool, fixture)
				realRepo := bookings.NewRepository(pool)
				timeoutRepo := &matrixPostCommitTimeoutRepository{Repository: realRepo, pool: pool}
				timeoutService := newBookingMatrixService(t, timeoutRepo, actualResolverAdapter, platformfinance.NewBookingFeeSnapshotRepository())
				retryService := newBookingMatrixService(t, realRepo, actualResolverAdapter, platformfinance.NewBookingFeeSnapshotRepository())
				ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
				defer cancel()

				var firstErr, retryErr error
				if testCase.offline {
					req := matrixOfflineRequest(fixture, "12:00", "13:00")
					_, firstErr = timeoutService.OwnerCreateOfflineBooking(ctx, matrixOwnerContext(fixture), req)
					_, retryErr = retryService.OwnerCreateOfflineBooking(ctx, matrixOwnerContext(fixture), req)
				} else {
					req := matrixOnlineRequest(fixture, "12:00", "13:00")
					_, firstErr = timeoutService.CreateBooking(ctx, fixture.customerUserID, req)
					_, retryErr = retryService.CreateBooking(ctx, fixture.customerUserID, req)
				}
				if !errors.Is(firstErr, context.DeadlineExceeded) {
					t.Fatalf("ambiguous first response = %v, want context deadline exceeded", firstErr)
				}
				if !errors.Is(retryErr, bookings.ErrOverlapBooking) {
					t.Fatalf("retry error = %v, want ErrOverlapBooking", retryErr)
				}
				assertBookingMatrixCounts(t, pool, fixture, testCase.expected)
			})
		}
	})

	t.Run("resolver snapshot and commit failures leave no orphan", func(t *testing.T) {
		injectedResolverErr := errors.New("injected resolver failure")
		injectedSnapshotErr := errors.New("injected snapshot failure")
		failingResolver := func(context.Context, platformfinance.CommercialTermQueryer, string, time.Time) (*platformfinance.CommercialTerm, error) {
			return nil, injectedResolverErr
		}

		for _, path := range []struct {
			name    string
			offline bool
		}{
			{name: "online"},
			{name: "offline", offline: true},
		} {
			path := path
			for _, failure := range []struct {
				name         string
				resolver     bookings.ResolveEffectiveTermFunc
				snapshotRepo platformfinance.BookingFeeSnapshotRepository
				wantErr      error
				wantCommitPG bool
			}{
				{name: "resolver", resolver: failingResolver, snapshotRepo: platformfinance.NewBookingFeeSnapshotRepository(), wantErr: injectedResolverErr},
				{name: "snapshot", resolver: actualResolverAdapter, snapshotRepo: &matrixFailingSnapshotRepository{err: injectedSnapshotErr}, wantErr: injectedSnapshotErr},
				{name: "commit", resolver: actualResolverAdapter, snapshotRepo: &matrixPhantomSnapshotRepository{}, wantCommitPG: true},
			} {
				failure := failure
				t.Run(path.name+"/"+failure.name, func(t *testing.T) {
					fixture := resetAndSetupBookingMatrix(t, pool)
					activateBookingMatrixCutover(t, pool, fixture)
					service := newBookingMatrixService(t, bookings.NewRepository(pool), failure.resolver, failure.snapshotRepo)
					ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
					defer cancel()

					var err error
					if path.offline {
						_, err = service.OwnerCreateOfflineBooking(ctx, matrixOwnerContext(fixture), matrixOfflineRequest(fixture, "13:00", "14:00"))
					} else {
						_, err = service.CreateBooking(ctx, fixture.customerUserID, matrixOnlineRequest(fixture, "13:00", "14:00"))
					}
					if failure.wantCommitPG {
						assertSnapshotGuardError(t, err)
					} else if !errors.Is(err, failure.wantErr) {
						t.Fatalf("error = %v, want %v", err, failure.wantErr)
					}
					assertBookingMatrixCounts(t, pool, fixture, bookingMatrixCounts{})
				})
			}
		}
	})

	t.Run("post cutover boundary preserves legacy row and guards new rows", func(t *testing.T) {
		fixture := resetAndSetupBookingMatrix(t, pool)
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()

		preCutoverID := insertRawBookingAndCommit(t, ctx, pool, fixture, "07:00", "08:00", false)
		activateBookingMatrixCutover(t, pool, fixture)

		service := newBookingMatrixService(t, bookings.NewRepository(pool), actualResolverAdapter, platformfinance.NewBookingFeeSnapshotRepository())
		if _, err := service.CreateBooking(ctx, fixture.customerUserID, matrixOnlineRequest(fixture, "08:00", "09:00")); err != nil {
			t.Fatalf("create post-cutover online booking: %v", err)
		}
		if _, err := service.OwnerCreateOfflineBooking(ctx, matrixOwnerContext(fixture), matrixOfflineRequest(fixture, "09:00", "10:00")); err != nil {
			t.Fatalf("create post-cutover offline booking: %v", err)
		}
		postCutoverID := insertRawBookingAndCommit(t, ctx, pool, fixture, "10:00", "11:00", true)

		assertBookingMatrixCounts(t, pool, fixture, bookingMatrixCounts{
			bookings:         3,
			snapshots:        2,
			offlineCustomers: 1,
			ownerLedger:      1,
		})
		assertBookingSnapshotPresence(t, pool, preCutoverID, false)
		assertBookingAbsent(t, pool, postCutoverID)
	})
}

func runBookingMatrixMigrations(testDSN string) error {
	paths := []string{
		"../../../../db/migrations",
		"../../../db/migrations",
		"../../db/migrations",
		"db/migrations",
	}
	var migrationsDirectory string
	for _, candidate := range paths {
		migration021 := filepath.Join(candidate, "021_platform_finance_cutover_guard.up.sql")
		if info, err := os.Stat(migration021); err == nil && !info.IsDir() {
			absolute, err := filepath.Abs(candidate)
			if err != nil {
				return err
			}
			migrationsDirectory = filepath.ToSlash(absolute)
			break
		}
	}
	if migrationsDirectory == "" {
		return errors.New("migration 021 directory not found")
	}

	migrator, err := migrate.New("file://"+migrationsDirectory, testDSN)
	if err != nil {
		return err
	}
	defer migrator.Close()
	if err := migrator.Up(); err != nil && !errors.Is(err, migrate.ErrNoChange) {
		return err
	}
	return nil
}

func createBookingMatrixDatabase(t *testing.T, baseDSN, dbName string) (string, func()) {
	t.Helper()
	parsed, err := url.Parse(baseDSN)
	if err != nil {
		t.Fatalf("parse booking matrix base DSN: %v", err)
	}
	parsed.Path = "/postgres"
	adminDSN := parsed.String()

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	adminPool, err := pgxpool.New(ctx, adminDSN)
	if err != nil {
		cancel()
		t.Fatalf("connect PostgreSQL admin database: %v", err)
	}
	identifier := pgx.Identifier{dbName}.Sanitize()
	if _, err := adminPool.Exec(ctx, "CREATE DATABASE "+identifier); err != nil {
		adminPool.Close()
		cancel()
		t.Fatalf("create booking matrix database: %v", err)
	}
	adminPool.Close()
	cancel()

	parsed.Path = "/" + dbName
	testDSN := parsed.String()
	drop := func() {
		dropCtx, dropCancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer dropCancel()
		pool, err := pgxpool.New(dropCtx, adminDSN)
		if err != nil {
			t.Errorf("connect admin database for cleanup: %v", err)
			return
		}
		defer pool.Close()
		if _, err := pool.Exec(dropCtx, `
			SELECT pg_terminate_backend(pid)
			FROM pg_stat_activity
			WHERE datname = $1 AND pid <> pg_backend_pid()
		`, dbName); err != nil {
			t.Errorf("terminate booking matrix database sessions: %v", err)
			return
		}
		if _, err := pool.Exec(dropCtx, "DROP DATABASE "+identifier); err != nil {
			t.Errorf("drop booking matrix database: %v", err)
		}
	}
	return testDSN, drop
}

func resetAndSetupBookingMatrix(t *testing.T, pool *pgxpool.Pool) bookingMatrixFixture {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	if _, err := pool.Exec(ctx, `
		TRUNCATE TABLE
			platform_finance_cutovers,
			booking_fee_snapshots,
			offline_booking_customers,
			owner_finance_transactions,
			bookings,
			court_blocked_slots,
			court_operating_hours,
			courts,
			venues,
			platform_commercial_terms,
			owner_profiles,
			users
		CASCADE
	`); err != nil {
		t.Fatalf("reset booking matrix database: %v", err)
	}

	var fixture bookingMatrixFixture
	suffix := strings.ReplaceAll(uuid.NewString(), "-", "")
	if err := pool.QueryRow(ctx, `
		INSERT INTO users (name, email, password_hash, role, status)
		VALUES ('Matrix Owner', $1, 'hash', 'OWNER', 'ACTIVE')
		RETURNING id::text
	`, "matrix_owner_"+suffix+"@example.test").Scan(&fixture.ownerUserID); err != nil {
		t.Fatalf("insert matrix owner: %v", err)
	}
	if err := pool.QueryRow(ctx, `
		INSERT INTO users (name, email, password_hash, role, status)
		VALUES ('Matrix Customer', $1, 'hash', 'CUSTOMER', 'ACTIVE')
		RETURNING id::text
	`, "matrix_customer_"+suffix+"@example.test").Scan(&fixture.customerUserID); err != nil {
		t.Fatalf("insert matrix customer: %v", err)
	}
	if err := pool.QueryRow(ctx, `
		INSERT INTO users (name, email, password_hash, role, status)
		VALUES ('Matrix Super Admin', $1, 'hash', 'SUPER_ADMIN', 'ACTIVE')
		RETURNING id::text
	`, "matrix_admin_"+suffix+"@example.test").Scan(&fixture.superAdminID); err != nil {
		t.Fatalf("insert matrix super admin: %v", err)
	}
	if err := pool.QueryRow(ctx, `
		INSERT INTO owner_profiles (user_id, business_name, verification_status)
		VALUES ($1, 'Matrix Business', 'APPROVED')
		RETURNING id::text
	`, fixture.ownerUserID).Scan(&fixture.ownerProfileID); err != nil {
		t.Fatalf("insert matrix owner profile: %v", err)
	}
	if err := pool.QueryRow(ctx, `
		INSERT INTO venues (owner_profile_id, name, address, city, status)
		VALUES ($1, 'Matrix Venue', 'Matrix Address', 'Jakarta', 'ACTIVE')
		RETURNING id::text
	`, fixture.ownerProfileID).Scan(&fixture.venueID); err != nil {
		t.Fatalf("insert matrix venue: %v", err)
	}

	var sportID string
	if err := pool.QueryRow(ctx, `SELECT id::text FROM sports WHERE status = 'ACTIVE' ORDER BY name LIMIT 1`).Scan(&sportID); err != nil {
		t.Fatalf("select seeded sport: %v", err)
	}
	if err := pool.QueryRow(ctx, `
		INSERT INTO courts (venue_id, sport_id, name, location_type, price_per_hour, status)
		VALUES ($1, $2, 'Matrix Court', 'INDOOR', 100000, 'ACTIVE')
		RETURNING id::text
	`, fixture.venueID, sportID).Scan(&fixture.courtID); err != nil {
		t.Fatalf("insert matrix court: %v", err)
	}
	for day := 0; day < 7; day++ {
		if _, err := pool.Exec(ctx, `
			INSERT INTO court_operating_hours (court_id, day_of_week, open_time, close_time, is_closed)
			VALUES ($1, $2, '00:00', '23:59', false)
		`, fixture.courtID, day); err != nil {
			t.Fatalf("insert matrix operating hours for day %d: %v", day, err)
		}
	}
	if err := pool.QueryRow(ctx, `
		INSERT INTO platform_commercial_terms (
			owner_profile_id, label, phase, finance_mode, collection_method, commission_bps, valid_from
		) VALUES ($1, 'Matrix Standard', 'STANDARD', 'SIMULATION', 'NONE', 700, '2000-01-01')
		RETURNING id::text
	`, fixture.ownerProfileID).Scan(&fixture.termID); err != nil {
		t.Fatalf("insert matrix commercial term: %v", err)
	}

	loc, err := time.LoadLocation("Asia/Jakarta")
	if err != nil {
		t.Fatalf("load Jakarta timezone: %v", err)
	}
	fixture.bookingDate = time.Now().In(loc).AddDate(0, 0, 3).Format("2006-01-02")
	return fixture
}

func activateBookingMatrixCutover(t *testing.T, pool *pgxpool.Pool, fixture bookingMatrixFixture) {
	t.Helper()
	activator, err := platformfinance.NewCutoverActivator(pool)
	if err != nil {
		t.Fatalf("create cutover activator: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	if _, err := activator.ActivateCutover(ctx, platformfinance.ActivateCutoverParams{
		CalculationVersion: platformfinance.ActiveBookingFeeCalculationVersion,
		ReleaseReference:   "task-2b2-06-matrix",
		ActorUserID:        fixture.superAdminID,
		LockTimeout:        5 * time.Second,
	}); err != nil {
		t.Fatalf("activate cutover: %v", err)
	}
}

func newBookingMatrixService(
	t *testing.T,
	repository bookings.BookingRepository,
	resolver bookings.ResolveEffectiveTermFunc,
	snapshotRepo platformfinance.BookingFeeSnapshotRepository,
) *bookings.Service {
	t.Helper()
	orchestrator, err := bookings.NewSnapshotOrchestrator(resolver, platformfinance.CalculateBookingFees, snapshotRepo)
	if err != nil {
		t.Fatalf("create snapshot orchestrator: %v", err)
	}
	return bookings.NewService(repository, 15, nil, nil, orchestrator)
}

func matrixOnlineRequest(fixture bookingMatrixFixture, startTime, endTime string) bookings.CreateBookingRequest {
	return bookings.CreateBookingRequest{
		CourtID:     fixture.courtID,
		BookingDate: fixture.bookingDate,
		StartTime:   startTime,
		EndTime:     endTime,
	}
}

func matrixOfflineRequest(fixture bookingMatrixFixture, startTime, endTime string) bookings.OwnerCreateOfflineBookingRequest {
	return bookings.OwnerCreateOfflineBookingRequest{
		VenueID:      fixture.venueID,
		CourtID:      fixture.courtID,
		BookingDate:  fixture.bookingDate,
		StartTime:    startTime,
		EndTime:      endTime,
		CustomerName: "Matrix Walk-in",
		TotalPrice:   100000,
		Status:       "PAID",
	}
}

func matrixOwnerContext(fixture bookingMatrixFixture) httputil.OwnerContext {
	return httputil.OwnerContext{
		ActorUserID:          fixture.ownerUserID,
		ActorRole:            "OWNER",
		EffectiveOwnerUserID: fixture.ownerUserID,
		OwnerProfileID:       fixture.ownerProfileID,
		IsOwner:              true,
	}
}

func runConcurrentBookingCalls(count int, call func(context.Context) error) []error {
	results := make(chan error, count)
	for i := 0; i < count; i++ {
		go func() {
			ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
			defer cancel()
			results <- call(ctx)
		}()
	}
	errorsSeen := make([]error, 0, count)
	for i := 0; i < count; i++ {
		errorsSeen = append(errorsSeen, <-results)
	}
	return errorsSeen
}

func assertConcurrentSingleWinner(t *testing.T, errorsSeen []error) {
	t.Helper()
	successes := 0
	overlaps := 0
	for _, err := range errorsSeen {
		switch {
		case err == nil:
			successes++
		case errors.Is(err, bookings.ErrOverlapBooking):
			overlaps++
		default:
			t.Fatalf("unexpected concurrent create error: %v", err)
		}
	}
	if successes != 1 || overlaps != len(errorsSeen)-1 {
		t.Fatalf("concurrent outcomes: successes=%d overlaps=%d total=%d", successes, overlaps, len(errorsSeen))
	}
}

func assertBookingMatrixCounts(t *testing.T, pool *pgxpool.Pool, fixture bookingMatrixFixture, want bookingMatrixCounts) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	var got bookingMatrixCounts
	if err := pool.QueryRow(ctx, `
		SELECT
			(SELECT count(*) FROM bookings WHERE court_id = $1),
			(SELECT count(*) FROM booking_fee_snapshots s JOIN bookings b ON b.id = s.booking_id WHERE b.court_id = $1),
			(SELECT count(*) FROM offline_booking_customers o JOIN bookings b ON b.id = o.booking_id WHERE b.court_id = $1),
			(SELECT count(*) FROM owner_finance_transactions WHERE venue_id = $2 AND source = 'BOOKING')
	`, fixture.courtID, fixture.venueID).Scan(&got.bookings, &got.snapshots, &got.offlineCustomers, &got.ownerLedger); err != nil {
		t.Fatalf("read booking matrix counts: %v", err)
	}
	if got != want {
		t.Fatalf("booking matrix counts = %+v, want %+v", got, want)
	}
}

func assertSnapshotGuardError(t *testing.T, err error) {
	t.Helper()
	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) {
		t.Fatalf("commit error = %v, want PostgreSQL constraint error", err)
	}
	if pgErr.Code != "23514" || pgErr.ConstraintName != "booking_snapshot_required_after_cutover" {
		t.Fatalf("commit error code=%s constraint=%s", pgErr.Code, pgErr.ConstraintName)
	}
}

func insertRawBookingAndCommit(
	t *testing.T,
	ctx context.Context,
	pool *pgxpool.Pool,
	fixture bookingMatrixFixture,
	startTime string,
	endTime string,
	wantGuardFailure bool,
) string {
	t.Helper()
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin raw booking transaction: %v", err)
	}
	defer func() {
		rollbackCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_ = tx.Rollback(rollbackCtx)
	}()

	var bookingID string
	if err := tx.QueryRow(ctx, `
		INSERT INTO bookings (
			customer_id, court_id, booking_date, start_time, end_time,
			original_price, discount_amount, final_price, total_price, status
		) VALUES ($1, $2, $3, $4, $5, 100000, 0, 100000, 100000, 'PAID')
		RETURNING id::text
	`, fixture.customerUserID, fixture.courtID, fixture.bookingDate, startTime, endTime).Scan(&bookingID); err != nil {
		t.Fatalf("insert raw booking: %v", err)
	}
	commitErr := tx.Commit(ctx)
	if wantGuardFailure {
		assertSnapshotGuardError(t, commitErr)
	} else if commitErr != nil {
		t.Fatalf("commit pre-cutover raw booking: %v", commitErr)
	}
	return bookingID
}

func assertBookingSnapshotPresence(t *testing.T, pool *pgxpool.Pool, bookingID string, want bool) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	var exists bool
	if err := pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM booking_fee_snapshots WHERE booking_id = $1)`, bookingID).Scan(&exists); err != nil {
		t.Fatalf("check booking snapshot presence: %v", err)
	}
	if exists != want {
		t.Fatalf("snapshot presence for %s = %t, want %t", bookingID, exists, want)
	}
}

func assertBookingAbsent(t *testing.T, pool *pgxpool.Pool, bookingID string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	var exists bool
	if err := pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM bookings WHERE id = $1)`, bookingID).Scan(&exists); err != nil {
		t.Fatalf("check rejected booking: %v", err)
	}
	if exists {
		t.Fatalf("post-cutover raw booking %s survived rejected commit", bookingID)
	}
}
