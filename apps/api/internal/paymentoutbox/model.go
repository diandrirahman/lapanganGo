package paymentoutbox

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"regexp"
	"strconv"
	"time"

	"lapangango-api/internal/provideridentity"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

type CommandType string

const (
	CommandPaymentCreate  CommandType = "PAYMENT_CREATE"
	CommandPaymentInquiry CommandType = "PAYMENT_INQUIRY"
	CommandRefundCreate   CommandType = "REFUND_CREATE"
	CommandRefundInquiry  CommandType = "REFUND_INQUIRY"
)

func (c CommandType) IsValid() bool {
	switch c {
	case CommandPaymentCreate, CommandPaymentInquiry, CommandRefundCreate, CommandRefundInquiry:
		return true
	default:
		return false
	}
}

type AggregateType string

const (
	AggregatePaymentAttempt AggregateType = "PAYMENT_ATTEMPT"
	AggregatePaymentRefund  AggregateType = "PAYMENT_REFUND"
)

type CommandState string

const (
	StatePending   CommandState = "PENDING"
	StateLeased    CommandState = "LEASED"
	StateRetryable CommandState = "RETRYABLE"
	StateSucceeded CommandState = "SUCCEEDED"
	StateTerminal  CommandState = "TERMINAL"
)

// MaxLeaseDuration is the largest lease interval representable by the outbox
// lifecycle contract. Worker/processor constructors use the same bound so an
// adapter call can never require an impossible lease.
const MaxLeaseDuration = 24 * time.Hour

var (
	ErrInvalidCommand       = errors.New("invalid payment provider command")
	ErrCommandNotFound      = errors.New("payment provider command not found")
	ErrIdempotencyConflict  = errors.New("payment provider command idempotency conflict")
	ErrNoCommandAvailable   = errors.New("no payment provider command available")
	ErrLeaseConflict        = errors.New("payment provider command lease conflict")
	ErrRefundOutboxNotReady = errors.New("refund provider commands are not available before the refund migration")
)

type DBTX interface {
	Exec(context.Context, string, ...any) (pgconn.CommandTag, error)
	Query(context.Context, string, ...any) (pgx.Rows, error)
	QueryRow(context.Context, string, ...any) pgx.Row
}

// PaymentCommandPayload is the complete allowlisted payload persisted by the
// Phase 5B payment outbox. Provider credentials, provider response bodies,
// customer data, card data, and bank-account data have no representable field.
type PaymentCommandPayload struct {
	AttemptID       string `json:"attempt_id"`
	AmountRupiah    int64  `json:"amount_rupiah"`
	Currency        string `json:"currency"`
	RequestedMethod string `json:"requested_method"`
}

type EnqueueParams struct {
	CommandType      CommandType
	AggregateType    AggregateType
	AggregateID      string
	PaymentAttemptID string
	IdempotencyKey   string
	RequestHash      string
	Payload          PaymentCommandPayload
	AvailableAt      time.Time
}

type Command struct {
	ID                     string
	CommandType            CommandType
	AggregateType          AggregateType
	AggregateID            string
	PaymentAttemptID       *string
	IdempotencyKey         string
	RequestHash            string
	Payload                json.RawMessage
	State                  CommandState
	AttemptCount           int
	MalformedResponseCount int16
	AvailableAt            time.Time
	LeaseOwner             *string
	LeaseToken             *string
	LeaseExpiresAt         *time.Time
	LastErrorCode          *string
	ProviderReference      *string
	CreatedAt              time.Time
	UpdatedAt              time.Time
	CompletedAt            *time.Time
}

type EnqueueResult struct {
	Command  Command
	Replayed bool
}

type Repository struct {
	db *pgxpool.Pool
}

func NewRepository(db *pgxpool.Pool) *Repository {
	return &Repository{db: db}
}

const canonicalUUIDPattern = `[0-9a-f]{8}-[0-9a-f]{4}-[1-5][0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}`

var paymentCreateKeyPattern = regexp.MustCompile(`^payment:create:` + canonicalUUIDPattern + `:[1-9][0-9]{0,4}$`)
var paymentInquiryKeyPattern = regexp.MustCompile(`^payment:inquiry:` + canonicalUUIDPattern + `$`)
var safeHashPattern = regexp.MustCompile(`^[0-9a-f]{64}$`)
var leaseOwnerPattern = regexp.MustCompile(`^worker:` + canonicalUUIDPattern + `$`)
var providerReferenceDigestPattern = regexp.MustCompile(`^sha256:[0-9a-f]{64}$`)

func ValidateEnqueueParams(params EnqueueParams) ([]byte, error) {
	if params.CommandType == CommandRefundCreate || params.CommandType == CommandRefundInquiry {
		return nil, ErrRefundOutboxNotReady
	}
	if !params.CommandType.IsValid() || params.AggregateType != AggregatePaymentAttempt ||
		(params.CommandType != CommandPaymentCreate && params.CommandType != CommandPaymentInquiry) {
		return nil, ErrInvalidCommand
	}
	aggregateID, err := uuid.Parse(params.AggregateID)
	if err != nil || aggregateID.String() != params.AggregateID {
		return nil, ErrInvalidCommand
	}
	if params.PaymentAttemptID == "" || params.PaymentAttemptID != params.AggregateID {
		return nil, ErrInvalidCommand
	}
	paymentAttemptID, err := uuid.Parse(params.PaymentAttemptID)
	if err != nil || paymentAttemptID.String() != params.PaymentAttemptID {
		return nil, ErrInvalidCommand
	}
	if !validateIdempotencyKey(params.CommandType, params.IdempotencyKey) ||
		!safeHashPattern.MatchString(params.RequestHash) {
		return nil, ErrInvalidCommand
	}
	payloadAttemptID, err := uuid.Parse(params.Payload.AttemptID)
	if err != nil || payloadAttemptID.String() != params.Payload.AttemptID ||
		params.Payload.AttemptID != params.AggregateID ||
		params.Payload.AmountRupiah <= 0 ||
		params.Payload.Currency != "IDR" ||
		!isAllowedRequestedMethod(params.Payload.RequestedMethod) {
		return nil, ErrInvalidCommand
	}
	payload, err := json.Marshal(params.Payload)
	if err != nil || len(payload) > 16384 {
		return nil, ErrInvalidCommand
	}
	return payload, nil
}

func validateIdempotencyKey(commandType CommandType, value string) bool {
	switch commandType {
	case CommandPaymentCreate:
		return paymentCreateKeyPattern.MatchString(value)
	case CommandPaymentInquiry:
		return paymentInquiryKeyPattern.MatchString(value)
	default:
		return false
	}
}

func deterministicIdempotencyKey(
	commandType CommandType,
	bookingID, paymentAttemptID string,
	attemptNo int16,
) string {
	switch commandType {
	case CommandPaymentCreate:
		return "payment:create:" + bookingID + ":" + strconv.FormatInt(int64(attemptNo), 10)
	case CommandPaymentInquiry:
		return "payment:inquiry:" + paymentAttemptID
	default:
		return ""
	}
}

// DeterministicCreateKey exposes the frozen create-payment namespace to the
// orchestration layer without exposing any provider-specific implementation.
func DeterministicCreateKey(bookingID string, attemptNo int16) string {
	return deterministicIdempotencyKey(CommandPaymentCreate, bookingID, "", attemptNo)
}

// DeterministicInquiryKey exposes the reserved uncertain-payment inquiry
// namespace. Task 5B-06 persists and claims this provider-neutral command but
// does not execute a provider inquiry; execution remains scoped to Task 5B-07.
func DeterministicInquiryKey(paymentAttemptID string) string {
	return deterministicIdempotencyKey(CommandPaymentInquiry, "", paymentAttemptID, 0)
}

func isAllowedRequestedMethod(value string) bool {
	switch value {
	case "BCA_VA", "QRIS", "CARD":
		return true
	default:
		return false
	}
}

func validateProviderReference(value string) bool {
	return providerReferenceDigestPattern.MatchString(value)
}

// DigestProviderReference converts an opaque provider identifier into the only
// representation allowed in the outbox. Raw provider IDs, URLs, account data,
// card data, credentials, and response fragments must never be persisted here.
func DigestProviderReference(value string) (string, error) {
	if !validateRawProviderReference(value) {
		return "", ErrInvalidCommand
	}
	digest := sha256.Sum256([]byte(value))
	return "sha256:" + hex.EncodeToString(digest[:]), nil
}

func validateRawProviderReference(value string) bool {
	return provideridentity.Valid(value, true)
}

func validateLeaseOwner(owner string) bool {
	return leaseOwnerPattern.MatchString(owner)
}

// ValidateLeaseOwner exposes the exact lifecycle owner contract to worker
// constructors so startup validation cannot drift from repository validation.
func ValidateLeaseOwner(owner string) bool {
	return validateLeaseOwner(owner)
}

func validateLeaseToken(token string) bool {
	parsed, err := uuid.Parse(token)
	return err == nil && parsed.String() == token
}

func validateLeaseDuration(value time.Duration) bool {
	return value >= time.Microsecond &&
		value <= MaxLeaseDuration &&
		value%time.Microsecond == 0
}

// ValidateLeaseDuration exposes the exact persistence contract used by claim
// operations so worker construction cannot accept a lease the repository will
// reject later.
func ValidateLeaseDuration(value time.Duration) bool {
	return validateLeaseDuration(value)
}

func validateRetryDelay(value time.Duration) bool {
	return value >= 0 &&
		value <= 24*time.Hour &&
		value%time.Microsecond == 0
}

// ValidateRetryDelay exposes the exact persistence contract used by retry
// finalizers so retry-policy validation cannot drift from the repository.
func ValidateRetryDelay(value time.Duration) bool {
	return validateRetryDelay(value)
}

func validateRetryableErrorCode(code string) bool {
	switch code {
	case "RETRYABLE_TIMEOUT", "RETRYABLE_PROVIDER", "RATE_LIMITED", "MALFORMED_RESPONSE":
		return true
	default:
		return false
	}
}

func validateTerminalErrorCode(code string) bool {
	switch code {
	case "AUTHENTICATION_FAILED", "INVALID_REQUEST", "IDEMPOTENCY_CONFLICT",
		"REFERENCE_MISMATCH", "AMOUNT_MISMATCH", "CURRENCY_MISMATCH",
		"TERMINAL_PROVIDER":
		return true
	default:
		return false
	}
}
