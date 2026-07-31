package payments

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/url"
	"strconv"
	"strings"
	"time"

	"lapangango-api/internal/audit"
	"lapangango-api/internal/paymentoutbox"
	"lapangango-api/internal/paymentreturn"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

var (
	ErrPaymentCapabilityDisabled = errors.New("sandbox payment create is disabled")
	ErrInvalidIdempotencyKey     = errors.New("invalid idempotency key")
	ErrInvalidPaymentMethod      = errors.New("invalid payment method")
	ErrPaymentAccessDenied       = errors.New("payment attempt not found")
	ErrPaymentAuditUnavailable   = errors.New("payment audit service is unavailable")
)

// OrchestratorOptions contains only non-secret runtime controls. Provider
// credentials are intentionally not needed by this task because provider work
// is performed by a later asynchronous worker.
type OrchestratorOptions struct {
	SandboxEnabled bool
	CreateEnabled  bool
	AttemptTTL     time.Duration
	ReturnOrigin   string
}

type CreateAttemptRequest struct {
	RequestedMethod RequestedMethod
}

type CreatePaymentResult struct {
	Attempt PaymentAttempt
	Replay  bool
}

type PaymentAttemptView struct {
	ID          string       `json:"id"`
	BookingID   string       `json:"booking_id"`
	State       AttemptState `json:"state"`
	ExpiresAt   time.Time    `json:"expires_at"`
	CheckoutURL *string      `json:"checkout_url,omitempty"`
}

type Orchestrator struct {
	db      *pgxpool.Pool
	attempt *Repository
	outbox  *paymentoutbox.Repository
	audit   audit.PlatformService
	options OrchestratorOptions
}

func NewOrchestrator(db *pgxpool.Pool, attemptRepo *Repository, outboxRepo *paymentoutbox.Repository, platformAudit audit.PlatformService, options OrchestratorOptions) *Orchestrator {
	if options.AttemptTTL <= 0 {
		options.AttemptTTL = 30 * time.Minute
	}
	return &Orchestrator{db: db, attempt: attemptRepo, outbox: outboxRepo, audit: platformAudit, options: options}
}

// CreatePayment creates the local payment attempt and provider command in one
// transaction. It never calls a provider and never changes booking payment
// status.
func (o *Orchestrator) CreatePayment(ctx context.Context, customerID, bookingID, idempotencyKey string, req CreateAttemptRequest) (CreatePaymentResult, error) {
	customerUUID, err := uuid.Parse(customerID)
	if err != nil {
		return CreatePaymentResult{}, ErrPaymentAccessDenied
	}
	bookingUUID, err := uuid.Parse(bookingID)
	if err != nil {
		return CreatePaymentResult{}, ErrBookingNotFound
	}
	customerID = customerUUID.String()
	bookingID = bookingUUID.String()
	if !validRequestedMethod(req.RequestedMethod) {
		return CreatePaymentResult{}, ErrInvalidPaymentMethod
	}
	if !validIdempotencyKey(idempotencyKey) {
		return CreatePaymentResult{}, ErrInvalidIdempotencyKey
	}
	if o.audit == nil {
		return CreatePaymentResult{}, ErrPaymentAuditUnavailable
	}

	if !o.options.SandboxEnabled || !o.options.CreateEnabled {
		requestHash := createDisabledRequestFingerprint(bookingID, req.RequestedMethod)
		if err := o.recordCreateDisabled(ctx, customerID, req.RequestedMethod, requestHash); err != nil {
			return CreatePaymentResult{}, err
		}
		return CreatePaymentResult{}, ErrPaymentCapabilityDisabled
	}

	localReference := deterministicLocalReference(bookingID, idempotencyKey)

	var amountRupiah int64
	var expiresAt time.Time
	var successReturnURL string
	var cancelReturnURL string
	existing, err := o.attempt.GetAttemptCreateFactsByReferenceForCustomer(ctx, bookingID, customerID, localReference)
	switch {
	case err == nil:
		// Replay is derived from the immutable original attempt and must remain
		// available after booking/provider state or runtime configuration changes.
		amountRupiah = existing.Attempt.AmountRupiah
		expiresAt = existing.Contract.RequestedExpiresAt
		successReturnURL = existing.Contract.SuccessReturnURL
		cancelReturnURL = existing.Contract.CancelReturnURL
	case errors.Is(err, ErrAttemptNotFound):
		successReturnURL, cancelReturnURL, err = normalizedPaymentReturnURLs(o.options.ReturnOrigin, localReference)
		if err != nil {
			return CreatePaymentResult{}, ErrInvalidCreateAttempt
		}
		creationFacts, factsErr := o.attempt.GetPaymentCreationFacts(ctx, bookingID, customerID)
		if factsErr != nil {
			return CreatePaymentResult{}, factsErr
		}
		amountRupiah = creationFacts.AmountRupiah
		expiresAt = creationFacts.BookingCreatedAt.UTC().Add(o.options.AttemptTTL)
		if creationFacts.BookingExpiresAt.Before(expiresAt) {
			expiresAt = creationFacts.BookingExpiresAt.UTC()
		}
	default:
		return CreatePaymentResult{}, err
	}
	requestHash := createRequestHash(createRequestHashInput{
		AmountRupiah:     amountRupiah,
		BookingID:        bookingID,
		CancelReturnURL:  cancelReturnURL,
		ExpiresAt:        expiresAt,
		LocalReference:   localReference,
		RequestedMethod:  req.RequestedMethod,
		SuccessReturnURL: successReturnURL,
	})
	actorRole := "CUSTOMER"
	params := CreateAttemptParams{
		BookingID:            bookingID,
		CustomerID:           customerID,
		ActorUserID:          stringPointer(customerID),
		ActorRole:            actorRole,
		CorrelationID:        localReference,
		Provider:             ProviderXendit,
		ProviderEnvironment:  ProviderEnvironmentTest,
		RequestedMethod:      req.RequestedMethod,
		IntegrationMode:      IntegrationModePaymentLink,
		CaptureMethod:        CaptureMethodAutomatic,
		LocalReference:       localReference,
		RequestHash:          requestHash,
		ExpiresAt:            expiresAt,
		SuccessReturnURL:     successReturnURL,
		CancelReturnURL:      cancelReturnURL,
		RejectActiveAttempt:  true,
		RequireCanonicalHash: true,
	}

	tx, err := o.db.Begin(ctx)
	if err != nil {
		return CreatePaymentResult{}, err
	}
	defer tx.Rollback(ctx)

	attempt, attemptReplay, err := o.attempt.CreateOrReplayAttemptTx(ctx, tx, params)
	if err != nil {
		return CreatePaymentResult{}, err
	}

	commandKey := paymentoutbox.DeterministicCreateKey(attempt.BookingID, attempt.AttemptNo)
	commandResult, err := o.outbox.EnqueueTx(ctx, tx, paymentoutbox.EnqueueParams{
		CommandType:      paymentoutbox.CommandPaymentCreate,
		AggregateType:    paymentoutbox.AggregatePaymentAttempt,
		AggregateID:      attempt.ID,
		PaymentAttemptID: attempt.ID,
		IdempotencyKey:   commandKey,
		RequestHash:      attempt.RequestHash,
		Payload: paymentoutbox.PaymentCommandPayload{
			AttemptID:       attempt.ID,
			AmountRupiah:    attempt.AmountRupiah,
			Currency:        string(attempt.Currency),
			RequestedMethod: string(attempt.RequestedMethod),
		},
	})
	if err != nil {
		return CreatePaymentResult{}, err
	}
	if !commandResult.Replayed {
		entityID := attempt.ID
		if err := o.audit.Record(ctx, tx, audit.CreatePlatformAuditLogParams{
			ActorUserID:   stringPointer(customerID),
			ActorRole:     actorRole,
			Action:        audit.ActionPaymentCommandEnqueued,
			EntityType:    audit.EntityPaymentAttempt,
			EntityID:      &entityID,
			CorrelationID: stringPointer(attempt.LocalReference),
			Metadata: map[string]any{
				"attempt_no":   int(attempt.AttemptNo),
				"command_type": string(paymentoutbox.CommandPaymentCreate),
			},
		}); err != nil {
			return CreatePaymentResult{}, err
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return CreatePaymentResult{}, err
	}
	return CreatePaymentResult{Attempt: attempt, Replay: attemptReplay}, nil
}

func (o *Orchestrator) GetPaymentAttempt(ctx context.Context, customerID, attemptID string) (PaymentAttemptView, error) {
	if _, err := uuid.Parse(customerID); err != nil {
		return PaymentAttemptView{}, ErrPaymentAccessDenied
	}
	status, err := o.attempt.GetAttemptByIDForCustomer(ctx, attemptID, customerID)
	if err != nil {
		if errors.Is(err, ErrAttemptNotFound) {
			return PaymentAttemptView{}, ErrPaymentAccessDenied
		}
		return PaymentAttemptView{}, err
	}
	return paymentAttemptView(status.Attempt, status.CheckoutEligible), nil
}

// GetPaymentAttemptByReference safely resolves the opaque reference used by
// hosted-checkout return URLs. The browser result remains non-authoritative;
// callers receive only the same normalized local state as the ID endpoint.
func (o *Orchestrator) GetPaymentAttemptByReference(ctx context.Context, customerID, localReference string) (PaymentAttemptView, error) {
	if _, err := uuid.Parse(customerID); err != nil || !isSafeLocalReference(localReference) {
		return PaymentAttemptView{}, ErrPaymentAccessDenied
	}
	status, err := o.attempt.GetAttemptByLocalReferenceForCustomer(ctx, customerID, localReference)
	if err != nil {
		if errors.Is(err, ErrAttemptNotFound) {
			return PaymentAttemptView{}, ErrPaymentAccessDenied
		}
		return PaymentAttemptView{}, err
	}
	return paymentAttemptView(status.Attempt, status.CheckoutEligible), nil
}

func paymentAttemptView(attempt PaymentAttempt, checkoutEligible bool) PaymentAttemptView {
	var checkoutURL *string
	if checkoutEligible &&
		attempt.State == AttemptStatePending &&
		attempt.CheckoutURL != nil &&
		safeCheckoutURL(*attempt.CheckoutURL) {
		value := *attempt.CheckoutURL
		checkoutURL = &value
	}
	return PaymentAttemptView{
		ID:          attempt.ID,
		BookingID:   attempt.BookingID,
		State:       attempt.State,
		ExpiresAt:   attempt.ExpiresAt,
		CheckoutURL: checkoutURL,
	}
}

func (o *Orchestrator) recordCreateDisabled(ctx context.Context, customerID string, method RequestedMethod, requestFingerprint string) error {
	tx, err := o.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	correlation := "payment:create:disabled:" + requestFingerprint
	if err := o.audit.Record(ctx, tx, audit.CreatePlatformAuditLogParams{
		ActorUserID:   stringPointer(customerID),
		ActorRole:     "CUSTOMER",
		Action:        audit.ActionPaymentCreateFlagOffRejected,
		EntityType:    audit.EntityPaymentAttempt,
		CorrelationID: &correlation,
		Metadata: map[string]any{
			"reason":              "CREATE_DISABLED",
			"requested_method":    string(method),
			"request_fingerprint": requestFingerprint,
		},
	}); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func validRequestedMethod(method RequestedMethod) bool {
	switch method {
	case RequestedMethodBCAVA, RequestedMethodQRIS, RequestedMethodCard:
		return true
	default:
		return false
	}
}

func validIdempotencyKey(value string) bool {
	if value == "" || len(value) > 128 || value != strings.TrimSpace(value) ||
		strings.Contains(value, ",") {
		return false
	}
	for _, r := range value {
		if r < 0x21 || r > 0x7e {
			return false
		}
	}
	return true
}

func deterministicLocalReference(bookingID, idempotencyKey string) string {
	digest := sha256.Sum256([]byte(bookingID + "\x00" + idempotencyKey))
	return "pa-" + hex.EncodeToString(digest[:])[:60]
}

type createRequestHashInput struct {
	AmountRupiah     int64
	BookingID        string
	CancelReturnURL  string
	ExpiresAt        time.Time
	LocalReference   string
	RequestedMethod  RequestedMethod
	SuccessReturnURL string
}

func createRequestHash(input createRequestHashInput) string {
	// Field declaration order is intentionally lexicographic by JSON key. The
	// standard encoder preserves struct field order, producing the frozen
	// canonical JSON representation without insignificant whitespace.
	payload := struct {
		AmountRupiah        string              `json:"amount_rupiah"`
		BookingID           string              `json:"booking_id"`
		CancelReturnURL     string              `json:"cancel_return_url"`
		CaptureMethod       CaptureMethod       `json:"capture_method"`
		Currency            Currency            `json:"currency"`
		ExpiresAt           string              `json:"expires_at"`
		IntegrationMode     IntegrationMode     `json:"integration_mode"`
		LocalReference      string              `json:"local_reference"`
		Provider            Provider            `json:"provider"`
		ProviderEnvironment ProviderEnvironment `json:"provider_environment"`
		RequestedMethod     RequestedMethod     `json:"requested_method"`
		SuccessReturnURL    string              `json:"success_return_url"`
	}{
		AmountRupiah:        strconv.FormatInt(input.AmountRupiah, 10),
		BookingID:           input.BookingID,
		CancelReturnURL:     input.CancelReturnURL,
		CaptureMethod:       CaptureMethodAutomatic,
		Currency:            CurrencyIDR,
		ExpiresAt:           input.ExpiresAt.UTC().Format(time.RFC3339Nano),
		IntegrationMode:     IntegrationModePaymentLink,
		LocalReference:      input.LocalReference,
		Provider:            ProviderXendit,
		ProviderEnvironment: ProviderEnvironmentTest,
		RequestedMethod:     input.RequestedMethod,
		SuccessReturnURL:    input.SuccessReturnURL,
	}
	encoded, _ := json.Marshal(payload)
	digest := sha256.Sum256(encoded)
	return hex.EncodeToString(digest[:])
}

func createDisabledRequestFingerprint(bookingID string, method RequestedMethod) string {
	payload := struct {
		BookingID       string          `json:"booking_id"`
		RequestedMethod RequestedMethod `json:"requested_method"`
	}{BookingID: bookingID, RequestedMethod: method}
	encoded, _ := json.Marshal(payload)
	digest := sha256.Sum256(encoded)
	return hex.EncodeToString(digest[:])
}

func stringPointer(value string) *string { return &value }

func safeCheckoutURL(value string) bool {
	parsed, err := url.Parse(value)
	if err != nil || parsed.Scheme != "https" || parsed.Host == "" || parsed.User != nil ||
		parsed.Port() != "" || parsed.RawQuery != "" || parsed.Fragment != "" {
		return false
	}
	switch strings.ToLower(parsed.Hostname()) {
	case "checkout-staging.xendit.co":
		const prefix = "/sessions/ps-"
		token := strings.TrimPrefix(parsed.EscapedPath(), prefix)
		return strings.HasPrefix(parsed.EscapedPath(), prefix) &&
			parsed.EscapedPath() == prefix+token && safeCheckoutToken(token, 8, 128)
	case "dev.xen.to":
		slug := strings.TrimPrefix(parsed.EscapedPath(), "/")
		return parsed.EscapedPath() == "/"+slug && safeCheckoutToken(slug, 4, 128)
	default:
		return false
	}
}

func safeCheckoutToken(value string, minLength, maxLength int) bool {
	if len(value) < minLength || len(value) > maxLength {
		return false
	}
	for _, r := range value {
		if (r < 'a' || r > 'z') && (r < 'A' || r > 'Z') &&
			(r < '0' || r > '9') && r != '-' && r != '_' {
			return false
		}
	}
	return true
}

func normalizedPaymentReturnURLs(origin, attemptReference string) (string, string, error) {
	normalizedOrigin, err := paymentreturn.NormalizeOrigin(origin)
	if err != nil || !isSafeLocalReference(attemptReference) {
		return "", "", errors.New("invalid payment return origin")
	}
	opaqueReference := url.PathEscape(attemptReference)
	return normalizedOrigin + "/payments/return/" + opaqueReference + "/success",
		normalizedOrigin + "/payments/return/" + opaqueReference + "/cancel", nil
}

func validPaymentReturnURLs(attemptReference, successURL, cancelURL string) bool {
	if !isSafeLocalReference(attemptReference) {
		return false
	}
	expectedSuccessPath := "/payments/return/" + attemptReference + "/success"
	expectedCancelPath := "/payments/return/" + attemptReference + "/cancel"
	success, successOK := parseStoredPaymentReturnURL(successURL, expectedSuccessPath)
	cancel, cancelOK := parseStoredPaymentReturnURL(cancelURL, expectedCancelPath)
	return successOK && cancelOK &&
		strings.EqualFold(success.Scheme, cancel.Scheme) &&
		strings.EqualFold(success.Host, cancel.Host)
}

func parseStoredPaymentReturnURL(value, expectedPath string) (*url.URL, bool) {
	parsed, err := url.Parse(value)
	if err != nil || parsed.Scheme != "https" || parsed.Host == "" || parsed.User != nil ||
		parsed.RawQuery != "" || parsed.Fragment != "" || parsed.Path != expectedPath ||
		parsed.EscapedPath() != expectedPath || parsed.Host != strings.ToLower(parsed.Host) {
		return nil, false
	}
	return parsed, true
}
