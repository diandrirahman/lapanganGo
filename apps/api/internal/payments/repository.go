package payments

import (
	"context"
	"errors"
	"lapangango-api/internal/audit"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

var (
	ErrAttemptNotFound      = errors.New("payment attempt not found")
	ErrBookingNotFound      = errors.New("booking not found")
	ErrSnapshotNotFound     = errors.New("payment fee snapshot not found")
	ErrStateConflict        = errors.New("payment state conflict")
	ErrInvalidTransition    = errors.New("invalid payment state transition")
	ErrIdempotencyConflict  = errors.New("payment idempotency conflict")
	ErrCaptureConflict      = errors.New("payment capture conflict")
	ErrAlreadyCaptured      = errors.New("payment already captured")
	ErrPaymentIntegrity     = errors.New("payment integrity error")
	ErrInvalidCapture       = errors.New("invalid payment capture")
	ErrInvalidCreateAttempt = errors.New("invalid payment attempt creation")
)

type DBTX interface {
	Exec(context.Context, string, ...any) (pgconn.CommandTag, error)
	Query(context.Context, string, ...any) (pgx.Rows, error)
	QueryRow(context.Context, string, ...any) pgx.Row
}

type Repository struct {
	db           *pgxpool.Pool
	auditService audit.PlatformService
}

type CreateAttemptParams struct {
	BookingID           string
	Provider            Provider
	ProviderEnvironment ProviderEnvironment
	RequestedMethod     RequestedMethod
	IntegrationMode     IntegrationMode
	CaptureMethod       CaptureMethod
	LocalReference      string
	RequestHash         string
	ExpiresAt           time.Time
}

type PaymentAttempt struct {
	ID                   string
	BookingID            string
	AttemptNo            int16
	Provider             Provider
	ProviderEnvironment  ProviderEnvironment
	RequestedMethod      RequestedMethod
	IntegrationMode      IntegrationMode
	CaptureMethod        CaptureMethod
	State                AttemptState
	Currency             Currency
	AmountRupiah         int64
	LocalReference       string
	RequestHash          string
	ProviderSessionID    *string
	ProviderPaymentReqID *string
	ProviderPaymentID    *string
	ProviderStatusCode   *string
	CheckoutURL          *string
	ExpiresAt            time.Time
	CapturedAt           *time.Time
	CreatedAt            time.Time
	UpdatedAt            time.Time
}

type CaptureParams struct {
	AttemptID            string
	Provider             Provider
	ProviderEnvironment  ProviderEnvironment
	ProviderPaymentID    string
	ProviderPaymentReqID *string
	AmountRupiah         int64
	Currency             Currency
	CapturedAt           time.Time
	ObservedAt           time.Time
	Authority            string
	SourceReference      string
	PayloadHash          string
}

type CaptureResult struct {
	Attempt     PaymentAttempt
	Fact        PaymentCaptureFact
	Duplicate   bool
	LateCapture bool
}

type PaymentCaptureFact struct {
	ID                   string
	PaymentAttemptID     string
	Provider             Provider
	ProviderEnvironment  ProviderEnvironment
	ProviderPaymentID    string
	ProviderPaymentReqID *string
	AmountRupiah         int64
	Currency             Currency
	CapturedAt           time.Time
	ObservedAt           time.Time
	Authority            string
	SourceReference      string
	PayloadHash          string
	CreatedAt            time.Time
}

func NewRepository(db *pgxpool.Pool) *Repository {
	return &Repository{
		db:           db,
		auditService: audit.NewPlatformService(audit.NewPlatformRepository()),
	}
}

// CreateOrReplayAttempt creates a local attempt from the immutable fee
// snapshot. It never accepts an amount from the caller.
func (r *Repository) CreateOrReplayAttempt(ctx context.Context, params CreateAttemptParams) (PaymentAttempt, error) {
	if err := validateCreateAttemptParams(params); err != nil {
		return PaymentAttempt{}, err
	}

	tx, err := r.db.Begin(ctx)
	if err != nil {
		return PaymentAttempt{}, err
	}
	defer tx.Rollback(ctx)

	if err := lockCreateIdempotency(ctx, tx, params.Provider, params.ProviderEnvironment, params.LocalReference); err != nil {
		return PaymentAttempt{}, err
	}

	if existing, err := findAttemptByProviderReference(ctx, tx, params.Provider, params.ProviderEnvironment, params.LocalReference, true); err != nil {
		return PaymentAttempt{}, err
	} else if existing != nil {
		if !sameCreateAttempt(existing, params) {
			return PaymentAttempt{}, ErrIdempotencyConflict
		}
		if err := tx.Commit(ctx); err != nil {
			return PaymentAttempt{}, err
		}
		return *existing, nil
	}

	if !params.ExpiresAt.After(time.Now()) {
		return PaymentAttempt{}, ErrInvalidCreateAttempt
	}

	if err := lockBooking(ctx, tx, params.BookingID); err != nil {
		return PaymentAttempt{}, err
	}

	var snapshotAmount int64
	err = tx.QueryRow(ctx, `
		SELECT customer_charge_amount_rupiah
		FROM booking_fee_snapshots
		WHERE booking_id = $1
		  AND booking_channel = 'MARKETPLACE_ONLINE'
		  AND finance_mode = 'SIMULATION'
		  AND currency = 'IDR'
		FOR KEY SHARE
	`, params.BookingID).Scan(&snapshotAmount)
	if errors.Is(err, pgx.ErrNoRows) {
		return PaymentAttempt{}, ErrSnapshotNotFound
	}
	if err != nil {
		return PaymentAttempt{}, err
	}

	if _, err := ValidatePaymentAttemptInput(PaymentAttemptInput{
		Provider:            params.Provider,
		ProviderEnvironment: params.ProviderEnvironment,
		RequestedMethod:     params.RequestedMethod,
		IntegrationMode:     params.IntegrationMode,
		CaptureMethod:       params.CaptureMethod,
		State:               AttemptStateCreated,
		Currency:            CurrencyIDR,
		AmountRupiah:        strconv.FormatInt(snapshotAmount, 10),
		LocalReference:      params.LocalReference,
		RequestHash:         params.RequestHash,
	}); err != nil {
		return PaymentAttempt{}, ErrInvalidCreateAttempt
	}

	var attempt PaymentAttempt
	err = scanPaymentAttempt(tx.QueryRow(ctx, `
		INSERT INTO payment_attempts (
			booking_id, attempt_no, provider, provider_environment, requested_method,
			integration_mode, capture_method, state, currency, amount_rupiah,
			local_reference, request_hash, expires_at
		)
		SELECT $1::uuid,
		       COALESCE(MAX(pa.attempt_no), 0) + 1,
		       $2::varchar, $3::varchar, $4::varchar, $5::varchar, $6::varchar,
		       'CREATED'::varchar, 'IDR'::char(3), s.customer_charge_amount_rupiah,
		       $7::varchar, $8::varchar, $9::timestamptz
		FROM booking_fee_snapshots s
		LEFT JOIN payment_attempts pa ON pa.booking_id = s.booking_id
		WHERE s.booking_id = $1
		  AND s.booking_channel = 'MARKETPLACE_ONLINE'
		  AND s.finance_mode = 'SIMULATION'
		  AND s.currency = 'IDR'
		GROUP BY s.booking_id, s.customer_charge_amount_rupiah
		RETURNING id::text, booking_id::text, attempt_no, provider, provider_environment,
		          requested_method, integration_mode, capture_method, state, currency::text,
		          amount_rupiah, local_reference, request_hash, provider_session_id,
		          provider_payment_request_id, provider_payment_id, provider_status_code,
		          checkout_url, expires_at, captured_at, created_at, updated_at
	`, params.BookingID, params.Provider, params.ProviderEnvironment, params.RequestedMethod,
		params.IntegrationMode, params.CaptureMethod, params.LocalReference, params.RequestHash,
		params.ExpiresAt), &attempt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return PaymentAttempt{}, ErrSnapshotNotFound
		}
		return PaymentAttempt{}, mapPaymentRepositoryError(err)
	}

	if err := r.writePaymentAudit(ctx, tx, attempt, "", string(AttemptStateCreated), false); err != nil {
		return PaymentAttempt{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return PaymentAttempt{}, err
	}
	return attempt, nil
}

func (r *Repository) GetAttemptByID(ctx context.Context, id string) (PaymentAttempt, error) {
	if _, err := uuid.Parse(id); err != nil {
		return PaymentAttempt{}, ErrAttemptNotFound
	}
	var attempt PaymentAttempt
	err := scanPaymentAttempt(r.db.QueryRow(ctx, paymentAttemptSelect+` WHERE id = $1`, id), &attempt)
	if errors.Is(err, pgx.ErrNoRows) {
		return PaymentAttempt{}, ErrAttemptNotFound
	}
	return attempt, err
}

func (r *Repository) GetAttemptsByBooking(ctx context.Context, bookingID string) ([]PaymentAttempt, error) {
	if _, err := uuid.Parse(bookingID); err != nil {
		return nil, ErrBookingNotFound
	}
	rows, err := r.db.Query(ctx, paymentAttemptSelect+` WHERE booking_id = $1 ORDER BY attempt_no ASC`, bookingID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	attempts := make([]PaymentAttempt, 0)
	for rows.Next() {
		var attempt PaymentAttempt
		if err := scanPaymentAttempt(rows, &attempt); err != nil {
			return nil, err
		}
		attempts = append(attempts, attempt)
	}
	return attempts, rows.Err()
}

func (r *Repository) GetNextAttemptNumber(ctx context.Context, bookingID string) (int16, error) {
	if _, err := uuid.Parse(bookingID); err != nil {
		return 0, ErrBookingNotFound
	}
	var bookingExists bool
	if err := r.db.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM bookings WHERE id = $1)`, bookingID).Scan(&bookingExists); err != nil {
		return 0, err
	}
	if !bookingExists {
		return 0, ErrBookingNotFound
	}
	var next int16
	err := r.db.QueryRow(ctx, `
		SELECT (COALESCE(MAX(attempt_no), 0) + 1)::smallint
		FROM payment_attempts
		WHERE booking_id = $1
	`, bookingID).Scan(&next)
	return next, err
}

// TransitionState performs a compare-and-set transition. CAPTURED is only
// reachable through RecordCapture, which atomically writes the capture fact.
func (r *Repository) TransitionState(ctx context.Context, id string, expected, next AttemptState) (PaymentAttempt, error) {
	if _, err := uuid.Parse(id); err != nil {
		return PaymentAttempt{}, ErrAttemptNotFound
	}
	if !allowedStateTransition(expected, next) {
		return PaymentAttempt{}, ErrInvalidTransition
	}

	tx, err := r.db.Begin(ctx)
	if err != nil {
		return PaymentAttempt{}, err
	}
	defer tx.Rollback(ctx)

	var current AttemptState
	err = tx.QueryRow(ctx, `SELECT state FROM payment_attempts WHERE id = $1 FOR UPDATE`, id).Scan(&current)
	if errors.Is(err, pgx.ErrNoRows) {
		return PaymentAttempt{}, ErrAttemptNotFound
	}
	if err != nil {
		return PaymentAttempt{}, err
	}
	if current != expected {
		return PaymentAttempt{}, ErrStateConflict
	}

	var attempt PaymentAttempt
	err = scanPaymentAttempt(tx.QueryRow(ctx, `
		UPDATE payment_attempts
		SET state = $2, updated_at = transaction_timestamp()
		WHERE id = $1 AND state = $3
		RETURNING id::text, booking_id::text, attempt_no, provider, provider_environment,
		          requested_method, integration_mode, capture_method, state, currency::text,
		          amount_rupiah, local_reference, request_hash, provider_session_id,
		          provider_payment_request_id, provider_payment_id, provider_status_code,
		          checkout_url, expires_at, captured_at, created_at, updated_at
	`, id, next, expected), &attempt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return PaymentAttempt{}, ErrStateConflict
		}
		return PaymentAttempt{}, mapPaymentRepositoryError(err)
	}
	if err := r.writePaymentAudit(ctx, tx, attempt, string(expected), string(next), false); err != nil {
		return PaymentAttempt{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return PaymentAttempt{}, err
	}
	return attempt, nil
}

// RecordCapture changes the attempt and inserts its immutable capture fact in
// one transaction. A duplicate identical fact is a no-op; a late capture is
// returned with LateCapture=true and never reopens booking fulfillment.
func (r *Repository) RecordCapture(ctx context.Context, params CaptureParams) (CaptureResult, error) {
	if err := validateCaptureParams(params); err != nil {
		return CaptureResult{}, err
	}
	// PostgreSQL TIMESTAMPTZ stores microsecond precision. Canonicalize before
	// persistence so an identical retry compares equal to the first fact.
	params.CapturedAt = params.CapturedAt.UTC().Truncate(time.Microsecond)
	params.ObservedAt = params.ObservedAt.UTC().Truncate(time.Microsecond)

	tx, err := r.db.Begin(ctx)
	if err != nil {
		return CaptureResult{}, err
	}
	defer tx.Rollback(ctx)

	var attempt PaymentAttempt
	if err := scanPaymentAttempt(tx.QueryRow(ctx, paymentAttemptSelect+` WHERE id = $1 FOR UPDATE`, params.AttemptID), &attempt); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return CaptureResult{}, ErrAttemptNotFound
		}
		return CaptureResult{}, err
	}

	if attempt.Provider != params.Provider || attempt.ProviderEnvironment != params.ProviderEnvironment ||
		attempt.Currency != params.Currency || attempt.AmountRupiah != params.AmountRupiah ||
		(attempt.ProviderPaymentID != nil && *attempt.ProviderPaymentID != params.ProviderPaymentID) ||
		(attempt.ProviderPaymentReqID != nil && !sameOptionalString(attempt.ProviderPaymentReqID, params.ProviderPaymentReqID)) {
		return CaptureResult{}, ErrCaptureConflict
	}

	if existing, err := findCaptureByAttemptOrProviderID(ctx, tx, params.AttemptID, params.Provider, params.ProviderEnvironment, params.ProviderPaymentID); err != nil {
		return CaptureResult{}, err
	} else if existing != nil {
		if existing.PaymentAttemptID != params.AttemptID || !sameCapture(*existing, params) {
			return CaptureResult{}, ErrCaptureConflict
		}
		if attempt.State != AttemptStateCaptured || attempt.CapturedAt == nil {
			return CaptureResult{}, ErrPaymentIntegrity
		}
		if err := tx.Commit(ctx); err != nil {
			return CaptureResult{}, err
		}
		return CaptureResult{Attempt: attempt, Fact: *existing, Duplicate: true}, nil
	}

	lateCapture := false
	switch attempt.State {
	case AttemptStatePending:
	case AttemptStateFailed, AttemptStateExpired, AttemptStateCancelled:
		lateCapture = true
	case AttemptStateCaptured:
		return CaptureResult{}, ErrPaymentIntegrity
	default:
		return CaptureResult{}, ErrInvalidTransition
	}

	var updated PaymentAttempt
	err = scanPaymentAttempt(tx.QueryRow(ctx, `
		UPDATE payment_attempts
		SET state = 'CAPTURED', captured_at = $2, provider_payment_id = $3,
		    provider_payment_request_id = $4, provider_status_code = 'CAPTURED',
		    updated_at = transaction_timestamp()
		WHERE id = $1 AND state = $5 AND captured_at IS NULL
		RETURNING id::text, booking_id::text, attempt_no, provider, provider_environment,
		          requested_method, integration_mode, capture_method, state, currency::text,
		          amount_rupiah, local_reference, request_hash, provider_session_id,
		          provider_payment_request_id, provider_payment_id, provider_status_code,
		          checkout_url, expires_at, captured_at, created_at, updated_at
	`, params.AttemptID, params.CapturedAt, params.ProviderPaymentID, params.ProviderPaymentReqID, attempt.State), &updated)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return CaptureResult{}, ErrStateConflict
		}
		return CaptureResult{}, mapPaymentRepositoryError(err)
	}

	var fact PaymentCaptureFact
	err = scanPaymentCaptureFact(tx.QueryRow(ctx, `
		INSERT INTO payment_capture_facts (
			payment_attempt_id, provider, provider_environment, provider_payment_id,
			provider_payment_request_id, amount_rupiah, currency, captured_at, observed_at,
			authority, source_reference, payload_hash
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)
		RETURNING id::text, payment_attempt_id::text, provider, provider_environment,
		          provider_payment_id, provider_payment_request_id, amount_rupiah, currency::text,
		          captured_at, observed_at, authority, source_reference, payload_hash, created_at
	`, params.AttemptID, params.Provider, params.ProviderEnvironment, params.ProviderPaymentID,
		params.ProviderPaymentReqID, params.AmountRupiah, params.Currency, params.CapturedAt,
		params.ObservedAt, params.Authority, params.SourceReference, params.PayloadHash), &fact)
	if err != nil {
		return CaptureResult{}, mapPaymentRepositoryError(err)
	}
	if err := r.writePaymentAudit(ctx, tx, updated, string(attempt.State), string(AttemptStateCaptured), lateCapture); err != nil {
		return CaptureResult{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return CaptureResult{}, err
	}
	return CaptureResult{Attempt: updated, Fact: fact, LateCapture: lateCapture}, nil
}

const paymentAttemptSelect = `
	SELECT id::text, booking_id::text, attempt_no, provider, provider_environment,
	       requested_method, integration_mode, capture_method, state, currency::text,
	       amount_rupiah, local_reference, request_hash, provider_session_id,
	       provider_payment_request_id, provider_payment_id, provider_status_code,
	       checkout_url, expires_at, captured_at, created_at, updated_at
	FROM payment_attempts`

func scanPaymentAttempt(row interface{ Scan(...any) error }, attempt *PaymentAttempt) error {
	return row.Scan(&attempt.ID, &attempt.BookingID, &attempt.AttemptNo, &attempt.Provider,
		&attempt.ProviderEnvironment, &attempt.RequestedMethod, &attempt.IntegrationMode,
		&attempt.CaptureMethod, &attempt.State, &attempt.Currency, &attempt.AmountRupiah,
		&attempt.LocalReference, &attempt.RequestHash, &attempt.ProviderSessionID,
		&attempt.ProviderPaymentReqID, &attempt.ProviderPaymentID, &attempt.ProviderStatusCode,
		&attempt.CheckoutURL, &attempt.ExpiresAt, &attempt.CapturedAt, &attempt.CreatedAt,
		&attempt.UpdatedAt)
}

func scanPaymentCaptureFact(row interface{ Scan(...any) error }, fact *PaymentCaptureFact) error {
	return row.Scan(&fact.ID, &fact.PaymentAttemptID, &fact.Provider, &fact.ProviderEnvironment,
		&fact.ProviderPaymentID, &fact.ProviderPaymentReqID, &fact.AmountRupiah, &fact.Currency,
		&fact.CapturedAt, &fact.ObservedAt, &fact.Authority, &fact.SourceReference,
		&fact.PayloadHash, &fact.CreatedAt)
}

func validateCreateAttemptParams(params CreateAttemptParams) error {
	if _, err := uuid.Parse(params.BookingID); err != nil || params.ExpiresAt.IsZero() {
		return ErrInvalidCreateAttempt
	}
	if params.Provider != ProviderXendit || params.ProviderEnvironment != ProviderEnvironmentTest ||
		!isAllowedRequestedMethod(params.RequestedMethod) || params.IntegrationMode != IntegrationModePaymentLink ||
		params.CaptureMethod != CaptureMethodAutomatic || !isSafeLocalReference(params.LocalReference) ||
		!isLowerSHA256(params.RequestHash) {
		return ErrInvalidCreateAttempt
	}
	return nil
}

func validateCaptureParams(params CaptureParams) error {
	if _, err := uuid.Parse(params.AttemptID); err != nil || params.Provider != ProviderXendit ||
		params.ProviderEnvironment != ProviderEnvironmentTest || params.Currency != CurrencyIDR ||
		params.AmountRupiah <= 0 || !isBoundedReference(params.ProviderPaymentID, 191) ||
		!isBoundedOptionalReference(params.ProviderPaymentReqID, 191) ||
		(params.Authority != "VERIFIED_WEBHOOK" && params.Authority != "AUTHENTICATED_INQUIRY") ||
		!isBoundedReference(params.SourceReference, 191) || !isLowerSHA256(params.PayloadHash) ||
		params.CapturedAt.IsZero() || params.ObservedAt.IsZero() || params.ObservedAt.Before(params.CapturedAt) {
		return ErrInvalidCapture
	}
	return nil
}

func lockCreateIdempotency(ctx context.Context, db DBTX, provider Provider, environment ProviderEnvironment, reference string) error {
	_, err := db.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended('payments:create:' || $1 || ':' || $2 || ':' || $3, 0))`, provider, environment, reference)
	return err
}

func lockBooking(ctx context.Context, db DBTX, bookingID string) error {
	var id string
	err := db.QueryRow(ctx, `SELECT id::text FROM bookings WHERE id = $1 FOR UPDATE`, bookingID).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrBookingNotFound
	}
	return err
}

func findAttemptByProviderReference(ctx context.Context, db DBTX, provider Provider, environment ProviderEnvironment, reference string, lock bool) (*PaymentAttempt, error) {
	query := paymentAttemptSelect + ` WHERE provider = $1 AND provider_environment = $2 AND local_reference = $3`
	if lock {
		query += ` FOR UPDATE`
	}
	var attempt PaymentAttempt
	err := scanPaymentAttempt(db.QueryRow(ctx, query, provider, environment, reference), &attempt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	return &attempt, err
}

func sameCreateAttempt(existing *PaymentAttempt, params CreateAttemptParams) bool {
	return existing.BookingID == params.BookingID && existing.Provider == params.Provider &&
		existing.ProviderEnvironment == params.ProviderEnvironment && existing.RequestedMethod == params.RequestedMethod &&
		existing.IntegrationMode == params.IntegrationMode && existing.CaptureMethod == params.CaptureMethod &&
		existing.LocalReference == params.LocalReference && existing.RequestHash == params.RequestHash
}

func findCaptureByAttemptOrProviderID(ctx context.Context, db DBTX, attemptID string, provider Provider, environment ProviderEnvironment, providerPaymentID string) (*PaymentCaptureFact, error) {
	rows, err := db.Query(ctx, `
		SELECT id::text, payment_attempt_id::text, provider, provider_environment,
		       provider_payment_id, provider_payment_request_id, amount_rupiah, currency::text,
		       captured_at, observed_at, authority, source_reference, payload_hash, created_at
		FROM payment_capture_facts
		WHERE payment_attempt_id = $1
		   OR (provider = $2 AND provider_environment = $3 AND provider_payment_id = $4)
		FOR UPDATE
	`, attemptID, provider, environment, providerPaymentID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var found *PaymentCaptureFact
	for rows.Next() {
		var fact PaymentCaptureFact
		if err := scanPaymentCaptureFact(rows, &fact); err != nil {
			return nil, err
		}
		if found != nil {
			return nil, ErrPaymentIntegrity
		}
		found = &fact
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return found, nil
}

func sameCapture(existing PaymentCaptureFact, params CaptureParams) bool {
	return existing.Provider == params.Provider && existing.ProviderEnvironment == params.ProviderEnvironment &&
		existing.ProviderPaymentID == params.ProviderPaymentID &&
		sameOptionalString(existing.ProviderPaymentReqID, params.ProviderPaymentReqID) &&
		existing.AmountRupiah == params.AmountRupiah && existing.Currency == params.Currency &&
		existing.CapturedAt.Equal(params.CapturedAt)
}

func sameOptionalString(left, right *string) bool {
	if left == nil || right == nil {
		return left == nil && right == nil
	}
	return *left == *right
}

func allowedStateTransition(from, to AttemptState) bool {
	switch from {
	case AttemptStateCreated:
		return to == AttemptStatePending || to == AttemptStateCancelled
	case AttemptStatePending:
		return to == AttemptStateFailed || to == AttemptStateExpired || to == AttemptStateCancelled
	default:
		return false
	}
}

func (r *Repository) writePaymentAudit(ctx context.Context, db DBTX, attempt PaymentAttempt, from, to string, lateCapture bool) error {
	metadata := map[string]any{
		"to_state":     to,
		"attempt_no":   int(attempt.AttemptNo),
		"late_capture": lateCapture,
	}
	if from != "" {
		metadata["from_state"] = from
	}

	entityID := attempt.ID
	correlationID := attempt.LocalReference
	if err := r.auditService.Record(ctx, db, audit.CreatePlatformAuditLogParams{
		ActorRole:     "SYSTEM",
		Action:        audit.ActionPaymentStateTransition,
		EntityType:    audit.EntityPaymentAttempt,
		EntityID:      &entityID,
		CorrelationID: &correlationID,
		Metadata:      metadata,
	}); err != nil {
		return err
	}

	if !lateCapture {
		return nil
	}
	return r.auditService.Record(ctx, db, audit.CreatePlatformAuditLogParams{
		ActorRole:     "SYSTEM",
		Action:        audit.ActionReconciliationException,
		EntityType:    audit.EntityPaymentAttempt,
		EntityID:      &entityID,
		CorrelationID: &correlationID,
		Metadata: map[string]any{
			"from_state": from,
			"to_state":   to,
			"attempt_no": int(attempt.AttemptNo),
			"reason":     "LATE_CAPTURE",
		},
	})
}

func isBoundedReference(value string, max int) bool {
	return value != "" && value == strings.TrimSpace(value) && len(value) <= max
}

func isBoundedOptionalReference(value *string, max int) bool {
	return value == nil || isBoundedReference(*value, max)
}

func mapPaymentRepositoryError(err error) error {
	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) {
		return err
	}
	if pgErr.Code != "23505" {
		return err
	}
	switch pgErr.ConstraintName {
	case "uq_payment_attempt_booking_captured", "uq_payment_capture_fact_attempt":
		return ErrAlreadyCaptured
	case "uq_payment_capture_fact_provider_payment", "uq_payment_capture_fact_provider_request":
		return ErrCaptureConflict
	case "uq_payment_attempt_booking_attempt_no", "uq_payment_attempt_provider_reference":
		return ErrIdempotencyConflict
	default:
		return ErrCaptureConflict
	}
}
