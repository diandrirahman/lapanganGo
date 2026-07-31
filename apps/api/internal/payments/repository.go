package payments

import (
	"context"
	"errors"
	"lapangango-api/internal/audit"
	"lapangango-api/internal/paymentflow"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

var (
	ErrAttemptNotFound        = errors.New("payment attempt not found")
	ErrBookingNotFound        = errors.New("booking not found")
	ErrSnapshotNotFound       = errors.New("payment fee snapshot not found")
	ErrStateConflict          = errors.New("payment state conflict")
	ErrInvalidTransition      = errors.New("invalid payment state transition")
	ErrIdempotencyConflict    = errors.New("payment idempotency conflict")
	ErrCaptureConflict        = errors.New("payment capture conflict")
	ErrAlreadyCaptured        = errors.New("payment already captured")
	ErrPaymentIntegrity       = errors.New("payment integrity error")
	ErrInvalidCapture         = errors.New("invalid payment capture")
	ErrInvalidCreateAttempt   = errors.New("invalid payment attempt creation")
	ErrInvalidInquiryIdentity = errors.New("invalid payment inquiry identity")
	ErrBookingNotPayable      = errors.New("booking is not eligible for payment")
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
	BookingID            string
	CustomerID           string
	ActorUserID          *string
	ActorRole            string
	CorrelationID        string
	RejectActiveAttempt  bool
	RequireCanonicalHash bool
	Provider             Provider
	ProviderEnvironment  ProviderEnvironment
	RequestedMethod      RequestedMethod
	IntegrationMode      IntegrationMode
	CaptureMethod        CaptureMethod
	LocalReference       string
	RequestHash          string
	ExpiresAt            time.Time
	SuccessReturnURL     string
	CancelReturnURL      string
}

type PaymentCreationFacts struct {
	AmountRupiah     int64
	BookingCreatedAt time.Time
	BookingExpiresAt time.Time
}

type PaymentCreateContract struct {
	PaymentAttemptID   string
	RequestHash        string
	RequestedExpiresAt time.Time
	SuccessReturnURL   string
	CancelReturnURL    string
	CreatedAt          time.Time
}

type PaymentAttemptCreateFacts struct {
	Attempt  PaymentAttempt
	Contract PaymentCreateContract
}

type CustomerPaymentAttemptStatus struct {
	Attempt          PaymentAttempt
	CheckoutEligible bool
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

// ApplyCreateProviderResultParams contains only normalized provider facts.
// Raw provider bodies and credentials are intentionally not representable.
type ApplyCreateProviderResultParams struct {
	AttemptID            string
	Provider             Provider
	ProviderEnvironment  ProviderEnvironment
	ProviderSessionID    string
	ProviderPaymentReqID *string
	ProviderPaymentID    *string
	ProviderStatusCode   string
	CheckoutURL          *string
	ProviderExpiresAt    *time.Time
	Status               PaymentStatus
	AmountRupiah         int64
	Currency             Currency
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

// ApplyInquiryIdentityParams contains only provider-neutral identity facts
// discovered while querying an existing checkout session or payment object.
// It deliberately cannot carry state, amount, currency, expiry, or raw
// provider data.
type ApplyInquiryIdentityParams struct {
	AttemptID            string
	Provider             Provider
	ProviderEnvironment  ProviderEnvironment
	Scope                PaymentInquiryScope
	ProviderSessionID    *string
	ProviderPaymentReqID *string
	ProviderPaymentID    *string
	ProviderStatusCode   string
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

	attempt, _, err := r.CreateOrReplayAttemptTx(ctx, tx, params)
	if err != nil {
		return PaymentAttempt{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return PaymentAttempt{}, err
	}
	return attempt, nil
}

// CreateOrReplayAttemptTx creates or replays an attempt inside the caller's
// transaction. It never begins, commits, or rolls back the transaction. The
// bool result is true when the existing immutable attempt was replayed.
func (r *Repository) CreateOrReplayAttemptTx(ctx context.Context, tx pgx.Tx, params CreateAttemptParams) (PaymentAttempt, bool, error) {
	// PostgreSQL stores timestamptz at microsecond precision. Normalize before
	// idempotency comparison and hashing so the first insert and later replay
	// use the exact same immutable expiry value.
	params.ExpiresAt = params.ExpiresAt.UTC().Truncate(time.Microsecond)
	if err := validateCreateAttemptParams(params); err != nil {
		return PaymentAttempt{}, false, err
	}

	if err := paymentflow.LockBooking(ctx, tx, params.BookingID); err != nil {
		return PaymentAttempt{}, false, err
	}
	if err := lockCreateIdempotency(ctx, tx, params.Provider, params.ProviderEnvironment, params.LocalReference); err != nil {
		return PaymentAttempt{}, false, err
	}

	if existing, err := findAttemptByProviderReference(ctx, tx, params.Provider, params.ProviderEnvironment, params.LocalReference, true); err != nil {
		return PaymentAttempt{}, false, err
	} else if existing != nil {
		if !sameCreateAttemptIdentity(existing, params) {
			return PaymentAttempt{}, false, ErrIdempotencyConflict
		}
		if params.RequireCanonicalHash {
			contract, contractErr := findPaymentCreateContract(ctx, tx, existing.ID, true)
			if contractErr != nil {
				return PaymentAttempt{}, false, contractErr
			}
			if contract == nil {
				return PaymentAttempt{}, false, ErrPaymentIntegrity
			}
			if !samePaymentCreateContract(*contract, params) {
				return PaymentAttempt{}, false, ErrIdempotencyConflict
			}
		} else if !existing.ExpiresAt.Equal(params.ExpiresAt) {
			return PaymentAttempt{}, false, ErrIdempotencyConflict
		}
		if params.CustomerID != "" {
			if err := verifyPaymentBookingCustomer(ctx, tx, params.BookingID, params.CustomerID); err != nil {
				return PaymentAttempt{}, false, err
			}
		}
		return *existing, true, nil
	}

	var notExpired bool
	if err := tx.QueryRow(ctx, `SELECT $1::timestamptz > transaction_timestamp()`, params.ExpiresAt).Scan(&notExpired); err != nil {
		return PaymentAttempt{}, false, err
	}
	if !notExpired {
		return PaymentAttempt{}, false, ErrInvalidCreateAttempt
	}

	bookingExpiresAt, err := lockPaymentBooking(ctx, tx, params.BookingID, params.CustomerID)
	if err != nil {
		return PaymentAttempt{}, false, err
	}
	if params.ExpiresAt.After(bookingExpiresAt) {
		return PaymentAttempt{}, false, ErrInvalidCreateAttempt
	}
	if params.RejectActiveAttempt {
		var active bool
		if err := tx.QueryRow(ctx, `
			SELECT EXISTS (
				SELECT 1 FROM payment_attempts
				WHERE booking_id = $1 AND state IN ('CREATED', 'PENDING')
			)
		`, params.BookingID).Scan(&active); err != nil {
			return PaymentAttempt{}, false, err
		}
		if active {
			return PaymentAttempt{}, false, ErrStateConflict
		}
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
		return PaymentAttempt{}, false, ErrSnapshotNotFound
	}
	if err != nil {
		return PaymentAttempt{}, false, err
	}
	if params.RequireCanonicalHash {
		expectedHash := createRequestHash(createRequestHashInput{
			AmountRupiah:     snapshotAmount,
			BookingID:        params.BookingID,
			CancelReturnURL:  params.CancelReturnURL,
			ExpiresAt:        params.ExpiresAt,
			LocalReference:   params.LocalReference,
			RequestedMethod:  params.RequestedMethod,
			SuccessReturnURL: params.SuccessReturnURL,
		})
		if params.RequestHash != expectedHash {
			return PaymentAttempt{}, false, ErrInvalidCreateAttempt
		}
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
		return PaymentAttempt{}, false, ErrInvalidCreateAttempt
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
			return PaymentAttempt{}, false, ErrSnapshotNotFound
		}
		return PaymentAttempt{}, false, mapPaymentRepositoryError(err)
	}

	if params.RequireCanonicalHash {
		if err := insertPaymentCreateContract(ctx, tx, attempt.ID, params); err != nil {
			return PaymentAttempt{}, false, err
		}
	}

	if err := r.writePaymentAuditWithActor(ctx, tx, attempt, "", string(AttemptStateCreated), false,
		params.ActorUserID, params.ActorRole, params.CorrelationID); err != nil {
		return PaymentAttempt{}, false, err
	}
	return attempt, false, nil
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

// GetAttemptTx reads an attempt using the caller's transaction. Worker
// terminal/audit paths use this to keep the observed attempt facts and the
// command lifecycle in the same atomic boundary without taking a second
// mutable row lock after the command lease row has been finalized.
func (r *Repository) GetAttemptTx(ctx context.Context, tx pgx.Tx, id string) (PaymentAttempt, error) {
	if tx == nil {
		return PaymentAttempt{}, ErrPaymentIntegrity
	}
	if _, err := uuid.Parse(id); err != nil {
		return PaymentAttempt{}, ErrAttemptNotFound
	}
	var attempt PaymentAttempt
	err := scanPaymentAttempt(tx.QueryRow(ctx, paymentAttemptSelect+` WHERE id = $1`, id), &attempt)
	if errors.Is(err, pgx.ErrNoRows) {
		return PaymentAttempt{}, ErrAttemptNotFound
	}
	return attempt, err
}

// LockAttemptForFinalizationTx obtains the booking-flow lock before locking
// the latest attempt row. Worker finalizers use it before changing a leased
// command so a terminal attempt and the command's no-op completion share one
// atomic boundary and preserve the global booking -> attempt -> command order.
func (r *Repository) LockAttemptForFinalizationTx(ctx context.Context, tx pgx.Tx, id string) (PaymentAttempt, error) {
	if tx == nil {
		return PaymentAttempt{}, ErrPaymentIntegrity
	}
	if _, err := uuid.Parse(id); err != nil {
		return PaymentAttempt{}, ErrAttemptNotFound
	}
	var bookingID string
	if err := tx.QueryRow(ctx, `SELECT booking_id::text FROM payment_attempts WHERE id = $1::uuid`, id).Scan(&bookingID); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return PaymentAttempt{}, ErrAttemptNotFound
		}
		return PaymentAttempt{}, err
	}
	if err := paymentflow.LockBooking(ctx, tx, bookingID); err != nil {
		return PaymentAttempt{}, err
	}
	var attempt PaymentAttempt
	err := scanPaymentAttempt(tx.QueryRow(ctx, paymentAttemptSelect+` WHERE id = $1::uuid FOR UPDATE`, id), &attempt)
	if errors.Is(err, pgx.ErrNoRows) {
		return PaymentAttempt{}, ErrAttemptNotFound
	}
	return attempt, err
}

// GetAttemptByIDForCustomer applies ownership at the database boundary and
// deliberately returns the same not-found error for an unknown or foreign
// attempt to avoid resource enumeration.
func (r *Repository) GetAttemptByIDForCustomer(ctx context.Context, id, customerID string) (CustomerPaymentAttemptStatus, error) {
	if _, err := uuid.Parse(id); err != nil {
		return CustomerPaymentAttemptStatus{}, ErrAttemptNotFound
	}
	if _, err := uuid.Parse(customerID); err != nil {
		return CustomerPaymentAttemptStatus{}, ErrAttemptNotFound
	}
	var status CustomerPaymentAttemptStatus
	err := scanCustomerPaymentAttemptStatus(r.db.QueryRow(ctx, paymentAttemptStatusSelectForCustomer+`
		WHERE pa.id = $1 AND b.customer_id = $2`, id, customerID), &status)
	if errors.Is(err, pgx.ErrNoRows) {
		return CustomerPaymentAttemptStatus{}, ErrAttemptNotFound
	}
	return status, err
}

// GetAttemptByReferenceForCustomer allows the orchestration boundary to
// recover an original idempotent result before re-evaluating mutable booking
// eligibility. Unknown and foreign references remain indistinguishable.
func (r *Repository) GetAttemptByReferenceForCustomer(ctx context.Context, bookingID, customerID, localReference string) (PaymentAttempt, error) {
	if _, err := uuid.Parse(bookingID); err != nil {
		return PaymentAttempt{}, ErrAttemptNotFound
	}
	if _, err := uuid.Parse(customerID); err != nil || !isSafeLocalReference(localReference) {
		return PaymentAttempt{}, ErrAttemptNotFound
	}
	var attempt PaymentAttempt
	err := scanPaymentAttempt(r.db.QueryRow(ctx, paymentAttemptSelectForCustomer+`
		WHERE pa.booking_id = $1
		  AND b.customer_id = $2
		  AND pa.provider = $3
		  AND pa.provider_environment = $4
		  AND pa.local_reference = $5
	`, bookingID, customerID, ProviderXendit, ProviderEnvironmentTest, localReference), &attempt)
	if errors.Is(err, pgx.ErrNoRows) {
		return PaymentAttempt{}, ErrAttemptNotFound
	}
	return attempt, err
}

// GetAttemptCreateFactsByReferenceForCustomer returns the original immutable
// provider-create facts together with the current local attempt. It is used for
// replay so mutable provider result fields and current runtime configuration
// can never alter the canonical request.
func (r *Repository) GetAttemptCreateFactsByReferenceForCustomer(ctx context.Context, bookingID, customerID, localReference string) (PaymentAttemptCreateFacts, error) {
	attempt, err := r.GetAttemptByReferenceForCustomer(ctx, bookingID, customerID, localReference)
	if err != nil {
		return PaymentAttemptCreateFacts{}, err
	}
	contract, err := r.GetCreateContractByAttemptID(ctx, attempt.ID)
	if err != nil {
		if errors.Is(err, ErrAttemptNotFound) {
			return PaymentAttemptCreateFacts{}, ErrPaymentIntegrity
		}
		return PaymentAttemptCreateFacts{}, err
	}
	return PaymentAttemptCreateFacts{Attempt: attempt, Contract: contract}, nil
}

// GetAttemptByLocalReferenceForCustomer resolves the opaque checkout return
// reference without exposing whether a foreign reference exists.
func (r *Repository) GetAttemptByLocalReferenceForCustomer(ctx context.Context, customerID, localReference string) (CustomerPaymentAttemptStatus, error) {
	if _, err := uuid.Parse(customerID); err != nil || !isSafeLocalReference(localReference) {
		return CustomerPaymentAttemptStatus{}, ErrAttemptNotFound
	}
	var status CustomerPaymentAttemptStatus
	err := scanCustomerPaymentAttemptStatus(r.db.QueryRow(ctx, paymentAttemptStatusSelectForCustomer+`
		WHERE b.customer_id = $1
		  AND pa.provider = $2
		  AND pa.provider_environment = $3
		  AND pa.local_reference = $4
	`, customerID, ProviderXendit, ProviderEnvironmentTest, localReference), &status)
	if errors.Is(err, pgx.ErrNoRows) {
		return CustomerPaymentAttemptStatus{}, ErrAttemptNotFound
	}
	return status, err
}

// GetCreateContractByAttemptID is the future provider worker's durable source
// for the exact expiry and return URLs that were hashed at enqueue time.
func (r *Repository) GetCreateContractByAttemptID(ctx context.Context, attemptID string) (PaymentCreateContract, error) {
	if _, err := uuid.Parse(attemptID); err != nil {
		return PaymentCreateContract{}, ErrAttemptNotFound
	}
	contract, err := findPaymentCreateContract(ctx, r.db, attemptID, false)
	if err != nil {
		return PaymentCreateContract{}, err
	}
	if contract == nil {
		return PaymentCreateContract{}, ErrAttemptNotFound
	}
	return *contract, nil
}

func (r *Repository) GetPaymentCreationFacts(ctx context.Context, bookingID, customerID string) (PaymentCreationFacts, error) {
	if _, err := uuid.Parse(bookingID); err != nil {
		return PaymentCreationFacts{}, ErrBookingNotFound
	}
	if _, err := uuid.Parse(customerID); err != nil {
		return PaymentCreationFacts{}, ErrBookingNotFound
	}
	var facts PaymentCreationFacts
	var amount *int64
	var bookingExpiresAt *time.Time
	var status string
	var payable bool
	err := r.db.QueryRow(ctx, `
		SELECT b.status, b.created_at, b.expires_at,
		       COALESCE(b.expires_at > transaction_timestamp(), false),
		       (
		           SELECT s.customer_charge_amount_rupiah
		           FROM booking_fee_snapshots s
		           WHERE s.booking_id = b.id
		             AND s.booking_channel = 'MARKETPLACE_ONLINE'
		             AND s.finance_mode = 'SIMULATION'
		             AND s.currency = 'IDR'
		       )
		FROM bookings b
		WHERE b.id = $1 AND b.customer_id = $2
	`, bookingID, customerID).Scan(&status, &facts.BookingCreatedAt, &bookingExpiresAt, &payable, &amount)
	if errors.Is(err, pgx.ErrNoRows) {
		return PaymentCreationFacts{}, ErrBookingNotFound
	}
	if err != nil {
		return PaymentCreationFacts{}, err
	}
	if status != "PENDING_PAYMENT" || bookingExpiresAt == nil || !payable {
		return PaymentCreationFacts{}, ErrBookingNotPayable
	}
	if amount == nil {
		return PaymentCreationFacts{}, ErrSnapshotNotFound
	}
	facts.AmountRupiah = *amount
	facts.BookingExpiresAt = bookingExpiresAt.UTC()
	return facts, nil
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

// ApplyCreateProviderResultTx stores a normalized create result while keeping
// the attempt in a non-terminal state. A create response never proves capture;
// capture remains exclusive to an authenticated inquiry/webhook fact path.
func (r *Repository) ApplyCreateProviderResultTx(ctx context.Context, tx pgx.Tx, params ApplyCreateProviderResultParams) (PaymentAttempt, error) {
	if tx == nil || !validProviderResultIdentity(params) || params.Status != PaymentStatusPending ||
		params.ProviderStatusCode == "" || params.ProviderStatusCode == "CAPTURED" {
		return PaymentAttempt{}, ErrInvalidCapture
	}
	if params.CheckoutURL != nil && !safeCheckoutURL(*params.CheckoutURL) {
		return PaymentAttempt{}, ErrInvalidCreateAttempt
	}
	if params.ProviderExpiresAt != nil {
		value := params.ProviderExpiresAt.UTC().Truncate(time.Microsecond)
		params.ProviderExpiresAt = &value
	}

	var bookingID string
	if err := tx.QueryRow(ctx, `SELECT booking_id::text FROM payment_attempts WHERE id = $1::uuid`, params.AttemptID).Scan(&bookingID); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return PaymentAttempt{}, ErrAttemptNotFound
		}
		return PaymentAttempt{}, err
	}
	if err := paymentflow.LockBooking(ctx, tx, bookingID); err != nil {
		return PaymentAttempt{}, err
	}

	var current PaymentAttempt
	if err := scanPaymentAttempt(tx.QueryRow(ctx, paymentAttemptSelect+` WHERE id = $1::uuid FOR UPDATE`, params.AttemptID), &current); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return PaymentAttempt{}, ErrAttemptNotFound
		}
		return PaymentAttempt{}, err
	}
	if current.Provider != params.Provider || current.ProviderEnvironment != params.ProviderEnvironment ||
		current.AmountRupiah != params.AmountRupiah || current.Currency != params.Currency {
		return PaymentAttempt{}, ErrCaptureConflict
	}
	if current.ProviderSessionID != nil && *current.ProviderSessionID != params.ProviderSessionID {
		return PaymentAttempt{}, ErrCaptureConflict
	}
	if params.ProviderPaymentReqID != nil && current.ProviderPaymentReqID != nil && *current.ProviderPaymentReqID != *params.ProviderPaymentReqID {
		return PaymentAttempt{}, ErrCaptureConflict
	}
	if params.ProviderPaymentID != nil && current.ProviderPaymentID != nil && *current.ProviderPaymentID != *params.ProviderPaymentID {
		return PaymentAttempt{}, ErrCaptureConflict
	}
	if current.State != AttemptStateCreated && current.State != AttemptStatePending {
		// A terminal no-op is allowed only after every response identity that
		// is already bound locally has compared exactly. Identities that remain
		// unbound are not attached after terminal state. A conflicting late
		// result is surfaced to reconciliation as an idempotency breach.
		return current, nil
	}

	from := current.State
	query := `
		UPDATE payment_attempts
		SET state = 'PENDING',
		    provider_session_id = COALESCE($2, provider_session_id),
		    provider_payment_request_id = COALESCE($3, provider_payment_request_id),
		    provider_payment_id = COALESCE($4, provider_payment_id),
		    provider_status_code = $5,
		    checkout_url = COALESCE($6, checkout_url),
		    expires_at = CASE
		        WHEN $7::timestamptz IS NULL THEN expires_at
		        WHEN $7::timestamptz < expires_at THEN $7::timestamptz
		        ELSE expires_at
		    END,
		    updated_at = transaction_timestamp()
		WHERE id = $1::uuid AND state IN ('CREATED', 'PENDING')
		RETURNING id::text, booking_id::text, attempt_no, provider, provider_environment,
		          requested_method, integration_mode, capture_method, state, currency::text,
		          amount_rupiah, local_reference, request_hash, provider_session_id,
		          provider_payment_request_id, provider_payment_id, provider_status_code,
		          checkout_url, expires_at, captured_at, created_at, updated_at
	`
	var updated PaymentAttempt
	if err := scanPaymentAttempt(tx.QueryRow(ctx, query, params.AttemptID, params.ProviderSessionID,
		params.ProviderPaymentReqID, params.ProviderPaymentID, params.ProviderStatusCode,
		params.CheckoutURL, params.ProviderExpiresAt), &updated); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return PaymentAttempt{}, ErrStateConflict
		}
		return PaymentAttempt{}, mapPaymentRepositoryError(err)
	}
	if from != AttemptStatePending {
		if err := r.writePaymentAudit(ctx, tx, updated, string(from), string(AttemptStatePending), false); err != nil {
			return PaymentAttempt{}, err
		}
	}
	return updated, nil
}

func validProviderResultIdentity(params ApplyCreateProviderResultParams) bool {
	_, err := uuid.Parse(params.AttemptID)
	return err == nil &&
		params.Provider == ProviderXendit && params.ProviderEnvironment == ProviderEnvironmentTest &&
		ValidProviderIdentity(params.ProviderSessionID, true) &&
		isSafeProviderStatusCode(params.ProviderStatusCode) &&
		validOptionalProviderIdentity(params.ProviderPaymentReqID) &&
		validOptionalProviderIdentity(params.ProviderPaymentID) &&
		params.AmountRupiah > 0 && params.Currency == CurrencyIDR &&
		params.Status == PaymentStatusPending
}

// ApplyInquiryIdentityTx binds provider identities discovered by an inquiry
// without changing payment authority. It is intentionally transaction-only:
// the caller must mark the same leased command retryable/succeeded in the
// same transaction before committing.
func (r *Repository) ApplyInquiryIdentityTx(ctx context.Context, tx pgx.Tx, params ApplyInquiryIdentityParams) (PaymentAttempt, bool, error) {
	if tx == nil || !validInquiryIdentityParams(params) {
		return PaymentAttempt{}, false, ErrInvalidInquiryIdentity
	}

	var bookingID string
	if err := tx.QueryRow(ctx, `SELECT booking_id::text FROM payment_attempts WHERE id = $1::uuid`, params.AttemptID).Scan(&bookingID); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return PaymentAttempt{}, false, ErrAttemptNotFound
		}
		return PaymentAttempt{}, false, err
	}
	if err := paymentflow.LockBooking(ctx, tx, bookingID); err != nil {
		return PaymentAttempt{}, false, err
	}

	var current PaymentAttempt
	if err := scanPaymentAttempt(tx.QueryRow(ctx, paymentAttemptSelect+` WHERE id = $1::uuid FOR UPDATE`, params.AttemptID), &current); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return PaymentAttempt{}, false, ErrAttemptNotFound
		}
		return PaymentAttempt{}, false, err
	}
	if current.Provider != params.Provider || current.ProviderEnvironment != params.ProviderEnvironment {
		return current, false, ErrStateConflict
	}

	if params.Scope == PaymentInquiryScopeCheckoutSession {
		if current.ProviderSessionID != nil &&
			(params.ProviderSessionID == nil || *current.ProviderSessionID != *params.ProviderSessionID) {
			return current, false, ErrCaptureConflict
		}
		if current.ProviderPaymentReqID != nil &&
			(params.ProviderPaymentReqID == nil || *current.ProviderPaymentReqID != *params.ProviderPaymentReqID) {
			return current, false, ErrCaptureConflict
		}
	} else if params.Scope == PaymentInquiryScopePayment {
		if current.ProviderPaymentReqID != nil &&
			(params.ProviderPaymentReqID == nil || *current.ProviderPaymentReqID != *params.ProviderPaymentReqID) {
			return current, false, ErrCaptureConflict
		}
		if params.ProviderSessionID != nil && current.ProviderSessionID != nil && *current.ProviderSessionID != *params.ProviderSessionID {
			return current, false, ErrCaptureConflict
		}
		if current.ProviderPaymentID != nil && (params.ProviderPaymentID == nil || *current.ProviderPaymentID != *params.ProviderPaymentID) {
			return current, false, ErrCaptureConflict
		}
	} else {
		return current, false, ErrInvalidInquiryIdentity
	}
	if current.State != AttemptStatePending {
		return current, false, ErrStateConflict
	}
	if params.Scope == PaymentInquiryScopeCheckoutSession {
		if current.ProviderSessionID == nil || params.ProviderSessionID == nil {
			return current, false, ErrCaptureConflict
		}
		if params.ProviderPaymentReqID == nil {
			return current, false, ErrInvalidInquiryIdentity
		}
	} else if current.ProviderPaymentReqID == nil || params.ProviderPaymentReqID == nil {
		return current, false, ErrCaptureConflict
	}

	newIdentity := current.ProviderPaymentReqID == nil && params.ProviderPaymentReqID != nil || current.ProviderPaymentID == nil && params.ProviderPaymentID != nil
	var updated PaymentAttempt
	err := scanPaymentAttempt(tx.QueryRow(ctx, `
		UPDATE payment_attempts
		SET provider_payment_request_id = COALESCE($2, provider_payment_request_id),
		    provider_payment_id = COALESCE($3, provider_payment_id),
		    provider_status_code = $4,
		    updated_at = transaction_timestamp()
		WHERE id = $1::uuid AND state = 'PENDING'
		RETURNING id::text, booking_id::text, attempt_no, provider, provider_environment,
		          requested_method, integration_mode, capture_method, state, currency::text,
		          amount_rupiah, local_reference, request_hash, provider_session_id,
		          provider_payment_request_id, provider_payment_id, provider_status_code,
		          checkout_url, expires_at, captured_at, created_at, updated_at
	`, params.AttemptID, params.ProviderPaymentReqID, params.ProviderPaymentID, params.ProviderStatusCode), &updated)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return PaymentAttempt{}, false, ErrStateConflict
		}
		return PaymentAttempt{}, false, mapPaymentRepositoryError(err)
	}
	return updated, newIdentity, nil
}

func validInquiryIdentityParams(params ApplyInquiryIdentityParams) bool {
	_, err := uuid.Parse(params.AttemptID)
	if err != nil || params.Provider != ProviderXendit || params.ProviderEnvironment != ProviderEnvironmentTest || !params.Scope.IsValid() || !isSafeProviderStatusCode(params.ProviderStatusCode) {
		return false
	}
	if !validOptionalProviderIdentity(params.ProviderSessionID) ||
		!validOptionalProviderIdentity(params.ProviderPaymentReqID) ||
		!validOptionalProviderIdentity(params.ProviderPaymentID) {
		return false
	}
	switch params.Scope {
	case PaymentInquiryScopeCheckoutSession:
		return params.ProviderSessionID != nil && params.ProviderPaymentReqID != nil && params.ProviderPaymentID == nil
	case PaymentInquiryScopePayment:
		return params.ProviderPaymentReqID != nil
	default:
		return false
	}
}

func isSafeProviderStatusCode(value string) bool {
	if value == "" || len(value) > 64 || value != strings.TrimSpace(value) {
		return false
	}
	for _, char := range value {
		if (char < 'A' || char > 'Z') && (char < 'a' || char > 'z') &&
			(char < '0' || char > '9') && char != '_' && char != '-' {
			return false
		}
	}
	return true
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
	attempt, err := r.TransitionStateTx(ctx, tx, id, expected, next)
	if err != nil {
		return PaymentAttempt{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return PaymentAttempt{}, err
	}
	return attempt, nil
}

// TransitionStateTx performs a guarded state transition in the caller's
// transaction. CAPTURED remains exclusive to RecordCaptureTx.
func (r *Repository) TransitionStateTx(ctx context.Context, tx pgx.Tx, id string, expected, next AttemptState) (PaymentAttempt, error) {
	if tx == nil {
		return PaymentAttempt{}, ErrPaymentIntegrity
	}
	if _, err := uuid.Parse(id); err != nil {
		return PaymentAttempt{}, ErrAttemptNotFound
	}
	if !allowedStateTransition(expected, next) {
		return PaymentAttempt{}, ErrInvalidTransition
	}

	var current AttemptState
	err := tx.QueryRow(ctx, `SELECT state FROM payment_attempts WHERE id = $1 FOR UPDATE`, id).Scan(&current)
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
	result, err := r.RecordCaptureTx(ctx, tx, params)
	if err != nil {
		return CaptureResult{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return CaptureResult{}, err
	}
	return result, nil
}

// RecordCaptureTx applies an authenticated capture inside the caller's
// transaction. It never begins, commits, or rolls back a transaction. This is
// required when a provider command lifecycle and capture fact must commit
// atomically.
func (r *Repository) RecordCaptureTx(ctx context.Context, tx pgx.Tx, params CaptureParams) (CaptureResult, error) {
	if tx == nil {
		return CaptureResult{}, ErrPaymentIntegrity
	}
	if err := validateCaptureParams(params); err != nil {
		return CaptureResult{}, err
	}
	// PostgreSQL TIMESTAMPTZ stores microsecond precision. Canonicalize before
	// persistence so an identical retry compares equal to the first fact.
	params.CapturedAt = params.CapturedAt.UTC().Truncate(time.Microsecond)
	params.ObservedAt = params.ObservedAt.UTC().Truncate(time.Microsecond)

	var bookingID string
	err := tx.QueryRow(ctx, `
		SELECT booking_id::text
		FROM payment_attempts
		WHERE id = $1
	`, params.AttemptID).Scan(&bookingID)
	if errors.Is(err, pgx.ErrNoRows) {
		return CaptureResult{}, ErrAttemptNotFound
	}
	if err != nil {
		return CaptureResult{}, err
	}
	if err := paymentflow.LockBooking(ctx, tx, bookingID); err != nil {
		return CaptureResult{}, err
	}

	var attempt PaymentAttempt
	if err := scanPaymentAttempt(tx.QueryRow(ctx, paymentAttemptSelect+` WHERE id = $1 FOR UPDATE`, params.AttemptID), &attempt); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return CaptureResult{}, ErrAttemptNotFound
		}
		return CaptureResult{}, err
	}
	if attempt.BookingID != bookingID {
		return CaptureResult{}, ErrPaymentIntegrity
	}

	var bookingStatus string
	var bookingExpired bool
	err = tx.QueryRow(ctx, `
		SELECT status, COALESCE(expires_at <= transaction_timestamp(), false)
		FROM bookings
		WHERE id = $1
	`, bookingID).Scan(&bookingStatus, &bookingExpired)
	if errors.Is(err, pgx.ErrNoRows) {
		return CaptureResult{}, ErrPaymentIntegrity
	}
	if err != nil {
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
		return CaptureResult{Attempt: attempt, Fact: *existing, Duplicate: true}, nil
	}

	lateCapture := bookingStatus == "CANCELLED" || bookingExpired
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
	return CaptureResult{Attempt: updated, Fact: fact, LateCapture: lateCapture}, nil
}

const paymentAttemptSelect = `
	SELECT id::text, booking_id::text, attempt_no, provider, provider_environment,
	       requested_method, integration_mode, capture_method, state, currency::text,
	       amount_rupiah, local_reference, request_hash, provider_session_id,
	       provider_payment_request_id, provider_payment_id, provider_status_code,
	       checkout_url, expires_at, captured_at, created_at, updated_at
	FROM payment_attempts`

const paymentAttemptSelectForCustomer = `
	SELECT pa.id::text, pa.booking_id::text, pa.attempt_no, pa.provider, pa.provider_environment,
	       pa.requested_method, pa.integration_mode, pa.capture_method, pa.state, pa.currency::text,
	       pa.amount_rupiah, pa.local_reference, pa.request_hash, pa.provider_session_id,
	       pa.provider_payment_request_id, pa.provider_payment_id, pa.provider_status_code,
	       pa.checkout_url, pa.expires_at, pa.captured_at, pa.created_at, pa.updated_at
	FROM payment_attempts pa
	JOIN bookings b ON b.id = pa.booking_id`

const paymentAttemptStatusSelectForCustomer = `
	SELECT pa.id::text, pa.booking_id::text, pa.attempt_no, pa.provider, pa.provider_environment,
	       pa.requested_method, pa.integration_mode, pa.capture_method, pa.state, pa.currency::text,
	       pa.amount_rupiah, pa.local_reference, pa.request_hash, pa.provider_session_id,
	       pa.provider_payment_request_id, pa.provider_payment_id, pa.provider_status_code,
	       pa.checkout_url, pa.expires_at, pa.captured_at, pa.created_at, pa.updated_at,
	       (
	           pa.state = 'PENDING'
	           AND pa.expires_at > transaction_timestamp()
	           AND b.status = 'PENDING_PAYMENT'
	           AND b.expires_at IS NOT NULL
	           AND b.expires_at > transaction_timestamp()
	       ) AS checkout_eligible
	FROM payment_attempts pa
	JOIN bookings b ON b.id = pa.booking_id`

func scanPaymentAttempt(row interface{ Scan(...any) error }, attempt *PaymentAttempt) error {
	return row.Scan(&attempt.ID, &attempt.BookingID, &attempt.AttemptNo, &attempt.Provider,
		&attempt.ProviderEnvironment, &attempt.RequestedMethod, &attempt.IntegrationMode,
		&attempt.CaptureMethod, &attempt.State, &attempt.Currency, &attempt.AmountRupiah,
		&attempt.LocalReference, &attempt.RequestHash, &attempt.ProviderSessionID,
		&attempt.ProviderPaymentReqID, &attempt.ProviderPaymentID, &attempt.ProviderStatusCode,
		&attempt.CheckoutURL, &attempt.ExpiresAt, &attempt.CapturedAt, &attempt.CreatedAt,
		&attempt.UpdatedAt)
}

func scanCustomerPaymentAttemptStatus(
	row interface{ Scan(...any) error },
	status *CustomerPaymentAttemptStatus,
) error {
	attempt := &status.Attempt
	return row.Scan(&attempt.ID, &attempt.BookingID, &attempt.AttemptNo, &attempt.Provider,
		&attempt.ProviderEnvironment, &attempt.RequestedMethod, &attempt.IntegrationMode,
		&attempt.CaptureMethod, &attempt.State, &attempt.Currency, &attempt.AmountRupiah,
		&attempt.LocalReference, &attempt.RequestHash, &attempt.ProviderSessionID,
		&attempt.ProviderPaymentReqID, &attempt.ProviderPaymentID, &attempt.ProviderStatusCode,
		&attempt.CheckoutURL, &attempt.ExpiresAt, &attempt.CapturedAt, &attempt.CreatedAt,
		&attempt.UpdatedAt, &status.CheckoutEligible)
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
	if params.RequireCanonicalHash &&
		!validPaymentReturnURLs(params.LocalReference, params.SuccessReturnURL, params.CancelReturnURL) {
		return ErrInvalidCreateAttempt
	}
	return nil
}

func validateCaptureParams(params CaptureParams) error {
	if _, err := uuid.Parse(params.AttemptID); err != nil || params.Provider != ProviderXendit ||
		params.ProviderEnvironment != ProviderEnvironmentTest || params.Currency != CurrencyIDR ||
		params.AmountRupiah <= 0 || !ValidProviderIdentity(params.ProviderPaymentID, true) ||
		!validOptionalProviderIdentity(params.ProviderPaymentReqID) ||
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

func lockPaymentBooking(ctx context.Context, db DBTX, bookingID, customerID string) (time.Time, error) {
	var id string
	var status string
	var expiresAt *time.Time
	var notExpired bool
	query := `SELECT id::text, status, expires_at, COALESCE(expires_at > transaction_timestamp(), false) FROM bookings WHERE id = $1`
	args := []any{bookingID}
	if customerID != "" {
		query += ` AND customer_id = $2`
		args = append(args, customerID)
	}
	query += ` FOR UPDATE`
	if err := db.QueryRow(ctx, query, args...).Scan(&id, &status, &expiresAt, &notExpired); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return time.Time{}, ErrBookingNotFound
		}
		return time.Time{}, err
	}
	if customerID != "" && (status != "PENDING_PAYMENT" || expiresAt == nil || !notExpired) {
		return time.Time{}, ErrBookingNotPayable
	}
	if expiresAt == nil || !notExpired {
		return time.Time{}, ErrInvalidCreateAttempt
	}
	return expiresAt.UTC(), nil
}

func verifyPaymentBookingCustomer(ctx context.Context, db DBTX, bookingID, customerID string) error {
	var id string
	err := db.QueryRow(ctx, `
		SELECT id::text FROM bookings
		WHERE id = $1 AND customer_id = $2
		FOR UPDATE
	`, bookingID, customerID).Scan(&id)
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

func sameCreateAttemptIdentity(existing *PaymentAttempt, params CreateAttemptParams) bool {
	return existing.BookingID == params.BookingID && existing.Provider == params.Provider &&
		existing.ProviderEnvironment == params.ProviderEnvironment && existing.RequestedMethod == params.RequestedMethod &&
		existing.IntegrationMode == params.IntegrationMode && existing.CaptureMethod == params.CaptureMethod &&
		existing.LocalReference == params.LocalReference && existing.RequestHash == params.RequestHash
}

func samePaymentCreateContract(existing PaymentCreateContract, params CreateAttemptParams) bool {
	return existing.RequestHash == params.RequestHash &&
		existing.RequestedExpiresAt.Equal(params.ExpiresAt) &&
		existing.SuccessReturnURL == params.SuccessReturnURL &&
		existing.CancelReturnURL == params.CancelReturnURL
}

func insertPaymentCreateContract(ctx context.Context, db DBTX, attemptID string, params CreateAttemptParams) error {
	_, err := db.Exec(ctx, `
		INSERT INTO payment_create_contracts (
			payment_attempt_id, request_hash, requested_expires_at,
			success_return_url, cancel_return_url
		) VALUES ($1, $2, $3, $4, $5)
	`, attemptID, params.RequestHash, params.ExpiresAt,
		params.SuccessReturnURL, params.CancelReturnURL)
	return err
}

func findPaymentCreateContract(ctx context.Context, db DBTX, attemptID string, lock bool) (*PaymentCreateContract, error) {
	query := `
		SELECT payment_attempt_id::text, request_hash, requested_expires_at,
		       success_return_url, cancel_return_url, created_at
		FROM payment_create_contracts
		WHERE payment_attempt_id = $1`
	if lock {
		query += ` FOR UPDATE`
	}
	var contract PaymentCreateContract
	err := db.QueryRow(ctx, query, attemptID).Scan(
		&contract.PaymentAttemptID,
		&contract.RequestHash,
		&contract.RequestedExpiresAt,
		&contract.SuccessReturnURL,
		&contract.CancelReturnURL,
		&contract.CreatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	return &contract, err
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
	return r.writePaymentAuditWithActor(ctx, db, attempt, from, to, lateCapture, nil, "SYSTEM", "")
}

func (r *Repository) writePaymentAuditWithActor(ctx context.Context, db DBTX, attempt PaymentAttempt, from, to string, lateCapture bool, actorUserID *string, actorRole, correlationID string) error {
	action := audit.ActionPaymentStateTransition
	metadata := map[string]any{"attempt_no": int(attempt.AttemptNo)}
	if from == "" {
		action = audit.ActionPaymentAttemptCreated
	} else {
		metadata["from_state"] = from
		metadata["to_state"] = to
		metadata["late_capture"] = lateCapture
	}

	entityID := attempt.ID
	if correlationID == "" {
		correlationID = attempt.LocalReference
	}
	if actorRole == "" {
		actorRole = "SYSTEM"
	}
	if err := r.auditService.Record(ctx, db, audit.CreatePlatformAuditLogParams{
		ActorUserID:   actorUserID,
		ActorRole:     actorRole,
		Action:        action,
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

func mapPaymentRepositoryError(err error) error {
	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) {
		return err
	}
	if pgErr.Code == "55000" &&
		pgErr.Message == "sandbox payment attempt blocked by legacy booking payment facts" {
		return ErrBookingNotPayable
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
