package platformfinance

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"lapangango-api/internal/httputil"
)

const ActiveBookingFeeCalculationVersion = BookingFeeCalculationVersionV1

var (
	ErrCutoverAlreadyActive  = errors.New("CUTOVER_ALREADY_ACTIVE")
	ErrCutoverActorForbidden = errors.New("CUTOVER_ACTOR_FORBIDDEN")
	ErrInvalidCutoverParams  = errors.New("INVALID_CUTOVER_PARAMS")
	ErrCutoverLockTimeout    = errors.New("CUTOVER_LOCK_TIMEOUT")
	ErrCutoverIntegrity      = errors.New("CUTOVER_INTEGRITY")
	ErrCutoverNotActive      = errors.New("CUTOVER_NOT_ACTIVE")
)

type CutoverRecord struct {
	ID                 int16
	SnapshotCutoverAt  time.Time
	CalculationVersion string
	ReleaseReference   string
	CreatedByUserID    string
	CreatedAt          time.Time
}

type ActivateCutoverParams struct {
	CalculationVersion string
	ReleaseReference   string
	ActorUserID        string
	LockTimeout        time.Duration
}

type CutoverRepository interface {
	ActivateCutover(ctx context.Context, params ActivateCutoverParams) (*CutoverRecord, error)
	GetActiveCutover(ctx context.Context) (*CutoverRecord, error)
}

type cutoverActivator struct {
	db *pgxpool.Pool
}

type QueryQuerier interface {
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

func VerifyTriggerIntegrity(ctx context.Context, q QueryQuerier) (bool, error) {
	var triggerValid bool
	err := q.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1
			FROM pg_trigger t
			JOIN pg_class c
			  ON c.oid = t.tgrelid
			JOIN pg_namespace relation_ns
			  ON relation_ns.oid = c.relnamespace
			JOIN pg_proc p
			  ON p.oid = t.tgfoid
			JOIN pg_namespace function_ns
			  ON function_ns.oid = p.pronamespace
			WHERE t.tgname = 'booking_snapshot_required_after_cutover'
			  AND relation_ns.nspname = 'public'
			  AND c.relname = 'bookings'
			  AND function_ns.nspname = 'public'
			  AND p.proname = 'enforce_booking_snapshot_after_cutover'
			  AND p.pronargs = 0
			  AND t.tgconstraint <> 0
			  AND t.tgisinternal = false
			  AND t.tgenabled IN ('O', 'A')
			  AND t.tgdeferrable = true
			  AND t.tginitdeferred = true
			  AND t.tgtype = 5
		)
	`).Scan(&triggerValid)
	if err != nil {
		return false, err
	}
	return triggerValid, nil
}

func NewCutoverActivator(db *pgxpool.Pool) (CutoverRepository, error) {
	if db == nil {
		return nil, fmt.Errorf("database pool cannot be nil")
	}
	return &cutoverActivator{db: db}, nil
}

func (a *cutoverActivator) ActivateCutover(ctx context.Context, params ActivateCutoverParams) (*CutoverRecord, error) {
	// 1. Validate
	if !httputil.IsUUID(params.ActorUserID) {
		return nil, ErrInvalidCutoverParams
	}
	ref := strings.TrimSpace(params.ReleaseReference)
	if ref == "" || len(ref) > 255 {
		return nil, ErrInvalidCutoverParams
	}
	if params.CalculationVersion != ActiveBookingFeeCalculationVersion {
		return nil, ErrInvalidCutoverParams
	}
	if params.LockTimeout <= 0 || params.LockTimeout > 10*time.Minute {
		return nil, ErrInvalidCutoverParams
	}

	// 2. Begin transaction
	tx, err := a.db.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer func() {
		if tx != nil {
			ctxRollback, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer cancel()
			_ = tx.Rollback(ctxRollback)
		}
	}()

	// 3. Set transaction-local lock timeout
	timeoutMs := params.LockTimeout.Milliseconds()
	_, err = tx.Exec(ctx, fmt.Sprintf("SET LOCAL lock_timeout = '%dms'", timeoutMs))
	if err != nil {
		return nil, err
	}

	// 4. Acquire transaction advisory lock
	_, err = tx.Exec(ctx, "SELECT pg_advisory_xact_lock(74239857)")
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "55P03" {
			return nil, ErrCutoverLockTimeout
		}
		return nil, err
	}

	// 5. Verify actor from users
	var role, status string
	err = tx.QueryRow(ctx, "SELECT role::text, status::text FROM users WHERE id = $1", params.ActorUserID).Scan(&role, &status)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrCutoverActorForbidden
		}
		return nil, err
	}
	if role != "SUPER_ADMIN" || status != "ACTIVE" {
		return nil, ErrCutoverActorForbidden
	}

	// 6. Recheck platform_finance_cutovers
	var exists bool
	err = tx.QueryRow(ctx, "SELECT EXISTS(SELECT 1 FROM platform_finance_cutovers WHERE id = 1)").Scan(&exists)
	if err != nil {
		return nil, err
	}
	if exists {
		return nil, ErrCutoverAlreadyActive
	}

	// 7. Acquire booking deny-create window
	_, err = tx.Exec(ctx, "LOCK TABLE bookings IN SHARE ROW EXCLUSIVE MODE")
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "55P03" {
			return nil, ErrCutoverLockTimeout
		}
		return nil, err
	}

	// 8. Recheck cutover row after lock
	err = tx.QueryRow(ctx, "SELECT EXISTS(SELECT 1 FROM platform_finance_cutovers WHERE id = 1)").Scan(&exists)
	if err != nil {
		return nil, err
	}
	if exists {
		return nil, ErrCutoverAlreadyActive
	}

	// 8.5 Verify deferred constraint trigger contract is valid
	triggerValid, err := VerifyTriggerIntegrity(ctx, tx)
	if err != nil {
		return nil, err
	}
	if !triggerValid {
		return nil, ErrCutoverIntegrity
	}

	// 9. Obtain exact timestamp from PostgreSQL after lock
	var cutoverAt time.Time
	err = tx.QueryRow(ctx, "SELECT clock_timestamp()").Scan(&cutoverAt)
	if err != nil {
		return nil, err
	}

	// 10. Insert singleton cutover row
	_, err = tx.Exec(ctx, `
		INSERT INTO platform_finance_cutovers (snapshot_cutover_at, calculation_version, release_reference, created_by_user_id)
		VALUES ($1, $2, $3, $4)
	`, cutoverAt, params.CalculationVersion, ref, params.ActorUserID)
	if err != nil {
		return nil, err
	}

	// 11. Read the inserted record back
	var r CutoverRecord
	var createdByUUID string
	err = tx.QueryRow(ctx, `
		SELECT id, snapshot_cutover_at, calculation_version, release_reference, created_by_user_id::text, created_at
		FROM platform_finance_cutovers
		WHERE id = 1
	`).Scan(&r.ID, &r.SnapshotCutoverAt, &r.CalculationVersion, &r.ReleaseReference, &createdByUUID, &r.CreatedAt)
	if err != nil {
		return nil, err
	}
	r.CreatedByUserID = createdByUUID

	// 12. Commit
	err = tx.Commit(ctx)
	if err != nil {
		return nil, err
	}

	// 13. Return the immutable record
	return &r, nil
}

func (a *cutoverActivator) GetActiveCutover(ctx context.Context) (*CutoverRecord, error) {
	rows, err := a.db.Query(ctx, `
		SELECT id, snapshot_cutover_at, calculation_version, release_reference, created_by_user_id::text, created_at
		FROM platform_finance_cutovers
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var records []CutoverRecord
	for rows.Next() {
		var r CutoverRecord
		var createdByUUID string
		err := rows.Scan(&r.ID, &r.SnapshotCutoverAt, &r.CalculationVersion, &r.ReleaseReference, &createdByUUID, &r.CreatedAt)
		if err != nil {
			return nil, err
		}
		r.CreatedByUserID = createdByUUID
		records = append(records, r)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	if len(records) == 0 {
		return nil, ErrCutoverNotActive
	}
	if len(records) > 1 {
		return nil, ErrCutoverIntegrity
	}
	return &records[0], nil
}
