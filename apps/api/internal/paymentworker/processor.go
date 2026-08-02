package paymentworker

import (
	"bytes"
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"reflect"
	"strings"
	"time"

	"lapangango-api/internal/audit"
	"lapangango-api/internal/paymentoutbox"
	"lapangango-api/internal/payments"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

var (
	ErrProcessorUnavailable = errors.New("payment worker processor unavailable")
	ErrMalformedCommand     = errors.New("malformed payment provider command")
)

type ProcessorOptions struct {
	Audit          audit.PlatformService
	Now            func() time.Time
	RetryPolicy    RetryPolicy
	AdapterTimeout time.Duration
}

// paymentAttemptRepository keeps the worker coupled to payment lifecycle
// operations rather than to the concrete PostgreSQL repository. Production
// still injects *payments.Repository; the seam also permits deterministic
// classification tests for transient read failures.
type paymentAttemptRepository interface {
	GetAttemptByID(context.Context, string) (payments.PaymentAttempt, error)
	GetCreateContractByAttemptID(context.Context, string) (payments.PaymentCreateContract, error)
	ApplyCreateProviderResultTx(context.Context, pgx.Tx, payments.ApplyCreateProviderResultParams) (payments.PaymentAttempt, error)
	ApplyInquiryIdentityTx(context.Context, pgx.Tx, payments.ApplyInquiryIdentityParams) (payments.PaymentAttempt, bool, error)
	RecordCaptureTx(context.Context, pgx.Tx, payments.CaptureParams) (payments.CaptureResult, error)
	TransitionStateTx(context.Context, pgx.Tx, string, payments.AttemptState, payments.AttemptState) (payments.PaymentAttempt, error)
	LockAttemptForFinalizationTx(context.Context, pgx.Tx, string) (payments.PaymentAttempt, error)
	GetAttemptTx(context.Context, pgx.Tx, string) (payments.PaymentAttempt, error)
}

// Processor owns the local transaction around normalized adapter results. It
// never imports provider SDK types and it can only call an injected adapter.
type Processor struct {
	db             *pgxpool.Pool
	attempts       paymentAttemptRepository
	outbox         *paymentoutbox.Repository
	audit          audit.PlatformService
	adapter        payments.PaymentAdapter
	now            func() time.Time
	policy         RetryPolicy
	adapterTimeout time.Duration
}

// CallTimeout is the maximum provider call duration the worker lease must
// cover. It is intentionally exposed through the narrow worker seam rather
// than leaking processor internals into worker construction.
func (p *Processor) CallTimeout() time.Duration {
	if p == nil {
		return 0
	}
	return p.adapterTimeout
}

func NewProcessor(db *pgxpool.Pool, attempts paymentAttemptRepository, outbox *paymentoutbox.Repository, adapter payments.PaymentAdapter, options ProcessorOptions) (*Processor, error) {
	if db == nil || isNilDependency(attempts) || outbox == nil || isNilDependency(adapter) || isNilDependency(options.Audit) {
		return nil, ErrProcessorUnavailable
	}
	if options.Now == nil {
		options.Now = time.Now
	}
	if options.RetryPolicy.InitialDelay == 0 && options.RetryPolicy.MaxDelay == 0 {
		options.RetryPolicy = DefaultRetryPolicy()
	} else {
		if options.RetryPolicy.InitialDelay <= 0 ||
			!paymentoutbox.ValidateRetryDelay(options.RetryPolicy.InitialDelay) ||
			!paymentoutbox.ValidateRetryDelay(options.RetryPolicy.MaxDelay) ||
			options.RetryPolicy.InitialDelay > options.RetryPolicy.MaxDelay {
			return nil, paymentoutbox.ErrInvalidCommand
		}
		if options.RetryPolicy.Jitter == nil {
			options.RetryPolicy.Jitter = deterministicJitter
		}
	}
	if options.AdapterTimeout == 0 {
		options.AdapterTimeout = 10 * time.Second
	} else if options.AdapterTimeout < 0 || options.AdapterTimeout >= paymentoutbox.MaxLeaseDuration {
		return nil, paymentoutbox.ErrInvalidCommand
	}
	return &Processor{
		db:             db,
		attempts:       attempts,
		outbox:         outbox,
		audit:          options.Audit,
		adapter:        adapter,
		now:            options.Now,
		policy:         options.RetryPolicy,
		adapterTimeout: options.AdapterTimeout,
	}, nil
}

func isNilDependency(value any) bool {
	if value == nil {
		return true
	}
	reflected := reflect.ValueOf(value)
	switch reflected.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		return reflected.IsNil()
	default:
		return false
	}
}

func (p *Processor) Process(ctx context.Context, command paymentoutbox.Command) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if !commandLeaseActive(command, p.now()) {
		return paymentoutbox.ErrLeaseConflict
	}
	payload, err := decodePayload(command.Payload)
	if err != nil || payload.AttemptID != *command.PaymentAttemptID || command.AggregateID != payload.AttemptID {
		return p.finishTerminal(ctx, command, "INVALID_REQUEST", "REFERENCE_MISMATCH")
	}
	switch command.CommandType {
	case paymentoutbox.CommandPaymentCreate:
		return p.processCreate(ctx, command, payload)
	case paymentoutbox.CommandPaymentInquiry:
		return p.processInquiry(ctx, command, payload)
	default:
		return paymentoutbox.ErrInvalidCommand
	}
}

func (p *Processor) processCreate(ctx context.Context, command paymentoutbox.Command, payload paymentoutbox.PaymentCommandPayload) error {
	attempt, err := p.attempts.GetAttemptByID(ctx, payload.AttemptID)
	if err != nil {
		if errors.Is(err, payments.ErrAttemptNotFound) {
			return p.finishMissingAttemptTerminal(ctx, command, "INVALID_REQUEST")
		}
		return err
	}
	if !commandFactsMatch(command, attempt, payload) {
		return p.finishTerminal(ctx, command, "REFERENCE_MISMATCH", "REFERENCE_MISMATCH")
	}
	execution := decideCommandExecution(command, attempt, p.now())
	switch execution.Action {
	case executionRejectLease:
		return paymentoutbox.ErrLeaseConflict
	case executionLocalNoop:
		return p.finishNoop(ctx, command, attempt)
	case executionLocalCreateRecovery:
		return p.finishKnownCreateIdentity(ctx, command)
	case executionLocalTerminal:
		return p.finishTerminal(ctx, command, "TERMINAL_PROVIDER", execution.Reason)
	case executionCallCreate:
		// Continue to the provider call below.
	default:
		return paymentoutbox.ErrInvalidCommand
	}
	contract, err := p.attempts.GetCreateContractByAttemptID(ctx, attempt.ID)
	if err != nil {
		if errors.Is(err, payments.ErrAttemptNotFound) {
			return p.finishTerminal(ctx, command, "INVALID_REQUEST", "PROVIDER_CONTRACT_BLOCKED")
		}
		return err
	}
	if !commandLeaseActive(command, p.now()) {
		return paymentoutbox.ErrLeaseConflict
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	adapterCtx, cancel := context.WithTimeout(ctx, p.adapterTimeout)
	response, err := p.adapter.CreatePayment(adapterCtx, payments.CreatePaymentRequest{
		AttemptID:        attempt.ID,
		AmountRupiah:     attempt.AmountRupiah,
		Currency:         attempt.Currency,
		RequestedMethod:  attempt.RequestedMethod,
		IntegrationMode:  attempt.IntegrationMode,
		CaptureMethod:    attempt.CaptureMethod,
		LocalReference:   attempt.LocalReference,
		IdempotencyKey:   command.IdempotencyKey,
		RequestHash:      command.RequestHash,
		ExpiresAt:        contract.RequestedExpiresAt,
		SuccessReturnURL: contract.SuccessReturnURL,
		CancelReturnURL:  contract.CancelReturnURL,
	})
	cancel()
	if err != nil {
		return p.finishAdapterError(ctx, command, attempt, err, true)
	}
	createResult, valid := classifyCreateResult(attempt, response)
	if !valid {
		return p.finishMalformed(ctx, command, "PROVIDER_CONTRACT_BLOCKED")
	}
	tx, err := p.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	appliedAttempt, err := p.attempts.ApplyCreateProviderResultTx(ctx, tx, payments.ApplyCreateProviderResultParams{
		AttemptID:            attempt.ID,
		Provider:             attempt.Provider,
		ProviderEnvironment:  attempt.ProviderEnvironment,
		ProviderSessionID:    createResult.ProviderSessionID,
		ProviderPaymentReqID: createResult.ProviderPaymentReqID,
		ProviderPaymentID:    createResult.ProviderPaymentID,
		ProviderStatusCode:   createResult.ProviderStatusCode,
		CheckoutURL:          optionalString(response.CheckoutURL),
		ProviderExpiresAt:    optionalTime(response.ExpiresAt),
		Status:               response.Status,
		AmountRupiah:         response.AmountRupiah,
		Currency:             response.Currency,
	})
	if err != nil {
		_ = tx.Rollback(ctx)
		switch {
		case errors.Is(err, payments.ErrInvalidCreateAttempt),
			errors.Is(err, payments.ErrInvalidCapture):
			return p.finishMalformed(ctx, command, "PROVIDER_CONTRACT_BLOCKED")
		case errors.Is(err, payments.ErrCaptureConflict):
			return p.finishTerminal(ctx, command, "REFERENCE_MISMATCH", "REFERENCE_MISMATCH")
		default:
			return err
		}
	}
	if isTerminalAttemptState(appliedAttempt.State) {
		if err := p.finishNoopTx(ctx, tx, command, appliedAttempt); err != nil {
			return err
		}
		return tx.Commit(ctx)
	}
	if err := p.finishCreateSuccessTx(ctx, tx, command, appliedAttempt, createResult.ProviderReference); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// finishKnownCreateIdentity recovers a create command whose provider identity
// was already committed by an exact prior attempt. It never calls the
// provider: under the attempt lock it normalizes CREATED to PENDING, ensures
// the one deterministic inquiry command, and completes the create command in
// the same transaction.
func (p *Processor) finishKnownCreateIdentity(ctx context.Context, command paymentoutbox.Command) error {
	tx, err := p.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	current, err := p.lockAttemptForFinalizationTx(ctx, tx, command)
	if err != nil {
		return err
	}
	decision := decideCommandExecution(command, current, p.now())
	switch decision.Action {
	case executionRejectLease:
		return paymentoutbox.ErrLeaseConflict
	case executionLocalNoop:
		if err := p.finishNoopTx(ctx, tx, command, current); err != nil {
			return err
		}
		return tx.Commit(ctx)
	case executionLocalTerminal:
		if err := p.finishTerminalTx(ctx, tx, command, "TERMINAL_PROVIDER", decision.Reason); err != nil {
			return err
		}
		return tx.Commit(ctx)
	case executionLocalCreateRecovery:
		// Continue below.
	default:
		return payments.ErrStateConflict
	}
	if current.State == payments.AttemptStateCreated {
		current, err = p.attempts.TransitionStateTx(
			ctx,
			tx,
			current.ID,
			payments.AttemptStateCreated,
			payments.AttemptStatePending,
		)
		if err != nil {
			return err
		}
	}
	providerReference, err := paymentoutbox.DigestProviderReference(preferredProviderIdentity(current))
	if err != nil {
		return err
	}
	if err := p.finishCreateSuccessTx(ctx, tx, command, current, providerReference); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (p *Processor) finishCreateSuccessTx(
	ctx context.Context,
	tx pgx.Tx,
	command paymentoutbox.Command,
	attempt payments.PaymentAttempt,
	providerReference string,
) error {
	inquiry, err := p.outbox.EnqueueTx(ctx, tx, paymentoutbox.EnqueueParams{
		CommandType:      paymentoutbox.CommandPaymentInquiry,
		AggregateType:    paymentoutbox.AggregatePaymentAttempt,
		AggregateID:      attempt.ID,
		PaymentAttemptID: attempt.ID,
		IdempotencyKey:   paymentoutbox.DeterministicInquiryKey(attempt.ID),
		RequestHash:      attempt.RequestHash,
		Payload: paymentoutbox.PaymentCommandPayload{
			AttemptID:       attempt.ID,
			AmountRupiah:    attempt.AmountRupiah,
			Currency:        string(attempt.Currency),
			RequestedMethod: string(attempt.RequestedMethod),
		},
	})
	if err != nil {
		return err
	}
	if !inquiry.Replayed {
		if err := p.recordCommandAudit(ctx, tx, attempt, paymentoutbox.CommandPaymentInquiry); err != nil {
			return err
		}
	}
	_, err = p.outbox.MarkSucceededTx(
		ctx,
		tx,
		command.ID,
		*command.LeaseOwner,
		*command.LeaseToken,
		providerReference,
	)
	return err
}

type classifiedCreateResult struct {
	ProviderSessionID    string
	ProviderPaymentReqID *string
	ProviderPaymentID    *string
	ProviderStatusCode   string
	ProviderReference    string
}

// classifyCreateResult is the single structural classification boundary for a
// normalized create response. Invalid provider output is malformed evidence,
// not an identity mismatch against an already-bound local fact. Exact replay
// conflicts remain the repository's responsibility under the attempt lock.
func classifyCreateResult(attempt payments.PaymentAttempt, response payments.CreatePaymentResponse) (classifiedCreateResult, bool) {
	if response.Status != payments.PaymentStatusPending ||
		response.AmountRupiah != attempt.AmountRupiah ||
		response.Currency != attempt.Currency ||
		!validWorkerProviderIdentity(response.ProviderSessionID, true) ||
		!validWorkerProviderIdentity(response.ProviderPaymentReqID, false) ||
		!validWorkerProviderIdentity(response.ProviderPaymentID, false) {
		return classifiedCreateResult{}, false
	}
	statusCode := response.StatusCode
	if statusCode == "" {
		statusCode = string(response.Status)
	}
	if !safeProviderStatusCode(statusCode) || statusCode == string(payments.PaymentStatusCaptured) {
		return classifiedCreateResult{}, false
	}
	providerReference, err := paymentoutbox.DigestProviderReference(response.ProviderSessionID)
	if err != nil {
		return classifiedCreateResult{}, false
	}
	return classifiedCreateResult{
		ProviderSessionID:    response.ProviderSessionID,
		ProviderPaymentReqID: optionalString(response.ProviderPaymentReqID),
		ProviderPaymentID:    optionalString(response.ProviderPaymentID),
		ProviderStatusCode:   statusCode,
		ProviderReference:    providerReference,
	}, true
}

func validWorkerProviderIdentity(value string, required bool) bool {
	return payments.ValidProviderIdentity(value, required)
}

func (p *Processor) processInquiry(ctx context.Context, command paymentoutbox.Command, payload paymentoutbox.PaymentCommandPayload) error {
	attemptID := *command.PaymentAttemptID
	attempt, err := p.attempts.GetAttemptByID(ctx, attemptID)
	if err != nil {
		if errors.Is(err, payments.ErrAttemptNotFound) {
			return p.finishMissingAttemptTerminal(ctx, command, "INVALID_REQUEST")
		}
		return err
	}
	if !commandFactsMatch(command, attempt, payload) {
		return p.finishTerminal(ctx, command, "REFERENCE_MISMATCH", "REFERENCE_MISMATCH")
	}
	execution := decideCommandExecution(command, attempt, p.now())
	switch execution.Action {
	case executionRejectLease:
		return paymentoutbox.ErrLeaseConflict
	case executionLocalNoop:
		return p.finishNoop(ctx, command, attempt)
	case executionLocalRetry:
		return p.finishRetryable(ctx, command, "RETRYABLE_PROVIDER")
	case executionLocalTerminal:
		return p.finishTerminal(ctx, command, "TERMINAL_PROVIDER", execution.Reason)
	case executionCallInquiry:
		// Continue to the provider call below.
	default:
		return paymentoutbox.ErrInvalidCommand
	}
	if !commandLeaseActive(command, p.now()) {
		return paymentoutbox.ErrLeaseConflict
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	adapterCtx, cancel := context.WithTimeout(ctx, p.adapterTimeout)
	response, err := p.adapter.GetPaymentStatus(adapterCtx, payments.GetPaymentStatusRequest{
		AttemptID:            attempt.ID,
		ProviderSessionID:    valueOrEmpty(attempt.ProviderSessionID),
		ProviderPaymentReqID: valueOrEmpty(attempt.ProviderPaymentReqID),
		ProviderPaymentID:    valueOrEmpty(attempt.ProviderPaymentID),
		IdempotencyKey:       command.IdempotencyKey,
	})
	cancel()
	if err != nil {
		return p.finishAdapterError(ctx, command, attempt, err, false)
	}
	decision := decideInquiryResponse(attempt, response)
	if decision.Kind == inquiryRejectMalformed {
		return p.finishMalformed(ctx, command, "PROVIDER_CONTRACT_BLOCKED")
	}
	if decision.Kind == inquiryRejectMismatch {
		return p.finishTerminal(ctx, command, decision.Reason, decision.Reason)
	}
	if decision.Kind == inquiryBindIdentityAndRetry {
		return p.bindInquiryIdentityAndRetry(ctx, command, attempt, response)
	}
	if decision.Kind == inquiryRetry {
		return p.finishRetryable(ctx, command, "RETRYABLE_PROVIDER")
	}
	if decision.Kind != inquiryCapture && decision.Kind != inquiryTerminalPayment {
		if decision.Reason == "MALFORMED_RESPONSE" {
			return p.finishMalformed(ctx, command, "PROVIDER_CONTRACT_BLOCKED")
		}
		return p.finishTerminal(ctx, command, decision.Reason, decision.Reason)
	}
	if response.Status == payments.PaymentStatusCaptured && response.CapturedAt == nil {
		return p.finishMalformed(ctx, command, "PROVIDER_CONTRACT_BLOCKED")
	}
	providerID := response.ProviderPaymentID
	if providerID == "" {
		providerID = response.ProviderPaymentReqID
	}
	if providerID == "" && attempt.ProviderSessionID != nil {
		providerID = *attempt.ProviderSessionID
	}
	providerReference, err := paymentoutbox.DigestProviderReference(providerID)
	if err != nil {
		return p.finishMalformed(ctx, command, "PROVIDER_CONTRACT_BLOCKED")
	}
	tx, err := p.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if response.Scope == payments.PaymentInquiryScopePayment && response.Status != payments.PaymentStatusCaptured {
		current, _, identityErr := p.attempts.ApplyInquiryIdentityTx(ctx, tx, inquiryIdentityParams(attempt, response))
		if identityErr != nil {
			if errors.Is(identityErr, payments.ErrStateConflict) && isTerminalAttemptState(current.State) {
				if err := p.finishNoopTx(ctx, tx, command, current); err != nil {
					return err
				}
				return tx.Commit(ctx)
			}
			_ = tx.Rollback(ctx)
			switch {
			case errors.Is(identityErr, payments.ErrCaptureConflict):
				return p.finishTerminal(ctx, command, "REFERENCE_MISMATCH", "REFERENCE_MISMATCH")
			case errors.Is(identityErr, payments.ErrInvalidInquiryIdentity):
				return p.finishMalformed(ctx, command, "PROVIDER_CONTRACT_BLOCKED")
			default:
				return identityErr
			}
		}
	}
	switch response.Status {
	case payments.PaymentStatusCaptured:
		capturedAt := response.CapturedAt
		_, err = p.attempts.RecordCaptureTx(ctx, tx, payments.CaptureParams{
			AttemptID:            attempt.ID,
			Provider:             attempt.Provider,
			ProviderEnvironment:  attempt.ProviderEnvironment,
			ProviderPaymentID:    response.ProviderPaymentID,
			ProviderPaymentReqID: optionalString(response.ProviderPaymentReqID),
			AmountRupiah:         response.AmountRupiah,
			Currency:             response.Currency,
			CapturedAt:           capturedAt.UTC(),
			ObservedAt:           p.now().UTC(),
			Authority:            "AUTHENTICATED_INQUIRY",
			SourceReference:      command.IdempotencyKey,
			PayloadHash:          response.PayloadHash,
		})
	case payments.PaymentStatusFailed, payments.PaymentStatusExpired, payments.PaymentStatusCancelled:
		_, err = p.attempts.TransitionStateTx(ctx, tx, attempt.ID, payments.AttemptStatePending, payments.AttemptState(response.Status))
		if errors.Is(err, payments.ErrStateConflict) {
			err = nil
		}
	default:
		_ = tx.Rollback(ctx)
		return p.finishMalformed(ctx, command, "PROVIDER_CONTRACT_BLOCKED")
	}
	if err != nil {
		_ = tx.Rollback(ctx)
		switch {
		case errors.Is(err, payments.ErrCaptureConflict):
			return p.finishTerminal(ctx, command, "REFERENCE_MISMATCH", "REFERENCE_MISMATCH")
		case errors.Is(err, payments.ErrInvalidCapture):
			return p.finishMalformed(ctx, command, "PROVIDER_CONTRACT_BLOCKED")
		default:
			return err
		}
	}
	if _, err := p.outbox.MarkSucceededTx(ctx, tx, command.ID, *command.LeaseOwner, *command.LeaseToken, providerReference); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (p *Processor) bindInquiryIdentityAndRetry(ctx context.Context, command paymentoutbox.Command, attempt payments.PaymentAttempt, response payments.PaymentStatusResponse) error {
	tx, err := p.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	statusCode := response.StatusCode
	if statusCode == "" {
		statusCode = string(response.Status)
	}
	current, _, err := p.attempts.ApplyInquiryIdentityTx(ctx, tx, inquiryIdentityParamsWithStatus(attempt, response, statusCode))
	if err != nil {
		if errors.Is(err, payments.ErrStateConflict) && isTerminalAttemptState(current.State) {
			if err := p.finishNoopTx(ctx, tx, command, current); err != nil {
				return err
			}
			return tx.Commit(ctx)
		}
		_ = tx.Rollback(ctx)
		if errors.Is(err, payments.ErrCaptureConflict) {
			return p.finishTerminal(ctx, command, "REFERENCE_MISMATCH", "REFERENCE_MISMATCH")
		}
		if errors.Is(err, payments.ErrInvalidInquiryIdentity) {
			return p.finishMalformed(ctx, command, "PROVIDER_CONTRACT_BLOCKED")
		}
		return err
	}
	if _, err := p.outbox.MarkRetryableTx(ctx, tx, command.ID, *command.LeaseOwner, *command.LeaseToken, "RETRYABLE_PROVIDER", p.policy.Delay(command.IdempotencyKey, command.AttemptCount, 0)); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func inquiryIdentityParams(attempt payments.PaymentAttempt, response payments.PaymentStatusResponse) payments.ApplyInquiryIdentityParams {
	statusCode := response.StatusCode
	if statusCode == "" {
		statusCode = string(response.Status)
	}
	return inquiryIdentityParamsWithStatus(attempt, response, statusCode)
}

func inquiryIdentityParamsWithStatus(attempt payments.PaymentAttempt, response payments.PaymentStatusResponse, statusCode string) payments.ApplyInquiryIdentityParams {
	return payments.ApplyInquiryIdentityParams{
		AttemptID:            attempt.ID,
		Provider:             attempt.Provider,
		ProviderEnvironment:  attempt.ProviderEnvironment,
		Scope:                response.Scope,
		ProviderSessionID:    optionalString(response.ProviderSessionID),
		ProviderPaymentReqID: optionalString(response.ProviderPaymentReqID),
		ProviderPaymentID:    optionalString(response.ProviderPaymentID),
		ProviderStatusCode:   statusCode,
	}
}

func (p *Processor) finishAdapterError(ctx context.Context, command paymentoutbox.Command, attempt payments.PaymentAttempt, err error, create bool) error {
	normalized := payments.NormalizeAdapterError(err)
	code := string(normalized.Code())
	switch normalized.Code() {
	case payments.AdapterErrorRetryableTimeout, payments.AdapterErrorRetryableProvider, payments.AdapterErrorRateLimited, payments.AdapterErrorMalformedResponse:
		if normalized.Code() == payments.AdapterErrorMalformedResponse {
			return p.finishMalformed(ctx, command, "PROVIDER_CONTRACT_BLOCKED")
		}
		if create && normalized.Code() == payments.AdapterErrorRetryableTimeout && attempt.State == payments.AttemptStateCreated {
			tx, txErr := p.db.Begin(ctx)
			if txErr != nil {
				return txErr
			}
			defer tx.Rollback(ctx)
			current, txErr := p.attempts.LockAttemptForFinalizationTx(ctx, tx, attempt.ID)
			if txErr != nil {
				return txErr
			}
			if isTerminalAttemptState(current.State) {
				if txErr := p.finishNoopTx(ctx, tx, command, current); txErr != nil {
					return txErr
				}
				return tx.Commit(ctx)
			}
			if current.State == payments.AttemptStateCreated {
				if _, txErr = p.attempts.TransitionStateTx(ctx, tx, attempt.ID, payments.AttemptStateCreated, payments.AttemptStatePending); txErr != nil {
					return txErr
				}
			} else if current.State != payments.AttemptStatePending {
				return payments.ErrStateConflict
			}
			if _, txErr = p.outbox.MarkRetryableTx(ctx, tx, command.ID, *command.LeaseOwner, *command.LeaseToken, code, p.policy.Delay(command.IdempotencyKey, command.AttemptCount, normalized.RetryAfter)); txErr != nil {
				return txErr
			}
			return tx.Commit(ctx)
		}
		return p.finishRetryableWithDelay(ctx, command, code, p.policy.Delay(command.IdempotencyKey, command.AttemptCount, normalized.RetryAfter))
	default:
		return p.finishTerminal(ctx, command, code, code)
	}
}

func (p *Processor) finishRetryable(ctx context.Context, command paymentoutbox.Command, code string) error {
	return p.finishRetryableWithDelay(ctx, command, code, p.policy.Delay(command.IdempotencyKey, command.AttemptCount, 0))
}

func (p *Processor) finishRetryableWithDelay(ctx context.Context, command paymentoutbox.Command, code string, delay time.Duration) error {
	tx, err := p.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	current, err := p.lockAttemptForFinalizationTx(ctx, tx, command)
	if err != nil {
		return err
	}
	if isTerminalAttemptState(current.State) {
		if err := p.finishNoopTx(ctx, tx, command, current); err != nil {
			return err
		}
		return tx.Commit(ctx)
	}
	if _, err := p.outbox.MarkRetryableTx(ctx, tx, command.ID, *command.LeaseOwner, *command.LeaseToken, code, delay); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// finishMalformed uses the existing two-strike outbox guard: the first
// malformed normalized result is retryable, while the second becomes a
// terminal incident. The attempt itself is never changed by either outcome.
func (p *Processor) finishMalformed(ctx context.Context, command paymentoutbox.Command, reason string) error {
	tx, err := p.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	current, err := p.lockAttemptForFinalizationTx(ctx, tx, command)
	if err != nil {
		return err
	}
	if isTerminalAttemptState(current.State) {
		if err := p.finishNoopTx(ctx, tx, command, current); err != nil {
			return err
		}
		return tx.Commit(ctx)
	}
	finished, err := p.outbox.MarkRetryableTx(ctx, tx, command.ID, *command.LeaseOwner, *command.LeaseToken, "MALFORMED_RESPONSE", p.policy.Delay(command.IdempotencyKey, command.AttemptCount, 0))
	if err != nil {
		return err
	}
	if finished.State == paymentoutbox.StateTerminal {
		if err := p.recordReconciliationTx(ctx, tx, command, reason); err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}

func (p *Processor) lockAttemptForFinalizationTx(ctx context.Context, tx pgx.Tx, command paymentoutbox.Command) (payments.PaymentAttempt, error) {
	if command.PaymentAttemptID == nil {
		return payments.PaymentAttempt{}, ErrMalformedCommand
	}
	return p.attempts.LockAttemptForFinalizationTx(ctx, tx, *command.PaymentAttemptID)
}

func (p *Processor) finishNoop(ctx context.Context, command paymentoutbox.Command, attempt payments.PaymentAttempt) error {
	tx, err := p.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if err := p.finishNoopTx(ctx, tx, command, attempt); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (p *Processor) finishNoopTx(ctx context.Context, tx pgx.Tx, command paymentoutbox.Command, attempt payments.PaymentAttempt) error {
	providerID := valueOrEmpty(attempt.ProviderPaymentID)
	if providerID == "" {
		providerID = valueOrEmpty(attempt.ProviderPaymentReqID)
	}
	if providerID == "" {
		providerID = valueOrEmpty(attempt.ProviderSessionID)
	}
	contractBlocked := false
	providerReference, err := paymentoutbox.DigestProviderReference(providerID)
	if providerID == "" || err != nil {
		// A terminal local no-op must never be held hostage by a missing or
		// outbox-incompatible provider identity. Complete it with an auditable
		// local reference; incompatible stored identity is also reconciled.
		contractBlocked = providerID != "" && err != nil
		providerReference, err = paymentoutbox.DigestProviderReference("local:" + attempt.ID)
		if err != nil {
			return err
		}
	}
	if _, err := p.outbox.MarkSucceededTx(ctx, tx, command.ID, *command.LeaseOwner, *command.LeaseToken, providerReference); err != nil {
		return err
	}
	if contractBlocked {
		return p.recordReconciliationForAttemptTx(ctx, tx, attempt, "PROVIDER_CONTRACT_BLOCKED")
	}
	return nil
}

func (p *Processor) finishMissingAttemptTerminal(ctx context.Context, command paymentoutbox.Command, code string) error {
	tx, err := p.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if _, err := p.outbox.MarkTerminalTx(ctx, tx, command.ID, *command.LeaseOwner, *command.LeaseToken, code); err != nil {
		return err
	}
	if command.PaymentAttemptID == nil {
		return ErrMalformedCommand
	}
	entityID := *command.PaymentAttemptID
	correlation := command.IdempotencyKey
	if err := p.audit.Record(ctx, tx, audit.CreatePlatformAuditLogParams{
		ActorRole:     "SYSTEM",
		Action:        audit.ActionPaymentCommandInvariantViolation,
		EntityType:    audit.EntityPaymentAttempt,
		EntityID:      &entityID,
		CorrelationID: &correlation,
		Metadata: map[string]any{
			"command_type": string(command.CommandType),
			"reason":       "ATTEMPT_NOT_FOUND",
		},
	}); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (p *Processor) finishTerminal(ctx context.Context, command paymentoutbox.Command, code, reason string) error {
	tx, err := p.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if err := p.finishTerminalTx(ctx, tx, command, code, reason); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (p *Processor) finishTerminalTx(ctx context.Context, tx pgx.Tx, command paymentoutbox.Command, code, reason string) error {
	if _, err := p.outbox.MarkTerminalTx(ctx, tx, command.ID, *command.LeaseOwner, *command.LeaseToken, code); err != nil {
		return err
	}
	return p.recordReconciliationTx(ctx, tx, command, reason)
}

func (p *Processor) recordReconciliationTx(ctx context.Context, tx pgx.Tx, command paymentoutbox.Command, reason string) error {
	if command.PaymentAttemptID == nil {
		return ErrMalformedCommand
	}
	attempt, err := p.attempts.GetAttemptTx(ctx, tx, *command.PaymentAttemptID)
	if err != nil {
		return err
	}
	return p.recordReconciliationForAttemptTx(ctx, tx, attempt, reason)
}

func (p *Processor) recordReconciliationForAttemptTx(ctx context.Context, tx pgx.Tx, attempt payments.PaymentAttempt, reason string) error {
	entityID := attempt.ID
	correlation := attempt.LocalReference
	return p.audit.Record(ctx, tx, audit.CreatePlatformAuditLogParams{
		ActorRole: "SYSTEM", Action: audit.ActionReconciliationException,
		EntityType: audit.EntityPaymentAttempt, EntityID: &entityID, CorrelationID: &correlation,
		Metadata: map[string]any{
			"from_state": string(attempt.State), "to_state": string(attempt.State),
			"attempt_no": int(attempt.AttemptNo), "reason": reason,
		},
	})
}

func (p *Processor) recordCommandAudit(ctx context.Context, tx pgx.Tx, attempt payments.PaymentAttempt, commandType paymentoutbox.CommandType) error {
	entityID := attempt.ID
	correlation := attempt.LocalReference
	return p.audit.Record(ctx, tx, audit.CreatePlatformAuditLogParams{
		ActorRole: "SYSTEM", Action: audit.ActionPaymentCommandEnqueued,
		EntityType: audit.EntityPaymentAttempt, EntityID: &entityID, CorrelationID: &correlation,
		Metadata: map[string]any{"attempt_no": int(attempt.AttemptNo), "command_type": string(commandType)},
	})
}

type inquiryDecisionKind string

const (
	inquiryRetry                inquiryDecisionKind = "RETRY"
	inquiryBindIdentityAndRetry inquiryDecisionKind = "BIND_IDENTITY_AND_RETRY"
	inquiryCapture              inquiryDecisionKind = "CAPTURE"
	inquiryTerminalPayment      inquiryDecisionKind = "TERMINAL_PAYMENT"
	inquiryRejectMismatch       inquiryDecisionKind = "REJECT_MISMATCH"
	inquiryRejectMalformed      inquiryDecisionKind = "REJECT_MALFORMED"
)

type inquiryDecision struct {
	Kind   inquiryDecisionKind
	Reason string
}

func decideInquiryResponse(attempt payments.PaymentAttempt, response payments.PaymentStatusResponse) inquiryDecision {
	if !response.Scope.IsValid() || !safeProviderStatusCode(response.StatusCode) || !validInquiryStatus(response.Status) {
		return inquiryDecision{Kind: inquiryRejectMalformed, Reason: "MALFORMED_RESPONSE"}
	}
	if !validWorkerProviderIdentity(response.ProviderSessionID, response.Scope == payments.PaymentInquiryScopeCheckoutSession) ||
		!validWorkerProviderIdentity(response.ProviderPaymentReqID, response.Scope == payments.PaymentInquiryScopePayment) ||
		!validWorkerProviderIdentity(response.ProviderPaymentID, false) {
		return inquiryDecision{Kind: inquiryRejectMalformed, Reason: "MALFORMED_RESPONSE"}
	}
	if response.Scope == payments.PaymentInquiryScopeCheckoutSession {
		if attempt.ProviderSessionID == nil || response.ProviderSessionID != *attempt.ProviderSessionID {
			return inquiryDecision{Kind: inquiryRejectMismatch, Reason: "REFERENCE_MISMATCH"}
		}
		if response.ProviderPaymentID != "" {
			return inquiryDecision{Kind: inquiryRejectMalformed, Reason: "MALFORMED_RESPONSE"}
		}
		if attempt.ProviderPaymentReqID != nil && response.ProviderPaymentReqID != "" && *attempt.ProviderPaymentReqID != response.ProviderPaymentReqID {
			return inquiryDecision{Kind: inquiryRejectMismatch, Reason: "REFERENCE_MISMATCH"}
		}
		if reason := optionalInquiryMoneyMismatch(attempt, response); reason != "" {
			return inquiryDecision{Kind: inquiryRejectMismatch, Reason: reason}
		}
		if response.ProviderPaymentReqID != "" && attempt.ProviderPaymentReqID == nil {
			// A newly discovered Payment Request is more authoritative than the
			// Session's terminal-looking status. Bind it and continue recovery
			// through Payment scope before deciding the local terminal state.
			return inquiryDecision{Kind: inquiryBindIdentityAndRetry}
		}
		switch response.Status {
		case payments.PaymentStatusPending:
			return inquiryDecision{Kind: inquiryRetry}
		case payments.PaymentStatusExpired, payments.PaymentStatusCancelled:
			return inquiryDecision{Kind: inquiryTerminalPayment}
		default:
			return inquiryDecision{Kind: inquiryRejectMalformed, Reason: "MALFORMED_RESPONSE"}
		}
	}
	if attempt.ProviderPaymentReqID == nil || response.ProviderPaymentReqID != *attempt.ProviderPaymentReqID {
		return inquiryDecision{Kind: inquiryRejectMismatch, Reason: "REFERENCE_MISMATCH"}
	}
	if response.ProviderSessionID != "" && attempt.ProviderSessionID != nil && response.ProviderSessionID != *attempt.ProviderSessionID {
		return inquiryDecision{Kind: inquiryRejectMismatch, Reason: "REFERENCE_MISMATCH"}
	}
	if attempt.ProviderPaymentID != nil && response.ProviderPaymentID != *attempt.ProviderPaymentID {
		return inquiryDecision{Kind: inquiryRejectMismatch, Reason: "REFERENCE_MISMATCH"}
	}
	if reason := optionalInquiryMoneyMismatch(attempt, response); reason != "" {
		return inquiryDecision{Kind: inquiryRejectMismatch, Reason: reason}
	}
	if response.Status == payments.PaymentStatusPending {
		if response.ProviderPaymentID != "" && attempt.ProviderPaymentID == nil {
			return inquiryDecision{Kind: inquiryBindIdentityAndRetry}
		}
		return inquiryDecision{Kind: inquiryRetry}
	}
	if response.AmountRupiah != attempt.AmountRupiah {
		return inquiryDecision{Kind: inquiryRejectMismatch, Reason: "AMOUNT_MISMATCH"}
	}
	if response.Currency != attempt.Currency {
		return inquiryDecision{Kind: inquiryRejectMismatch, Reason: "CURRENCY_MISMATCH"}
	}
	if response.Status == payments.PaymentStatusCaptured {
		if response.ProviderPaymentID == "" || response.CapturedAt == nil || response.CapturedAt.IsZero() || !validEvidenceHash(response.PayloadHash) {
			return inquiryDecision{Kind: inquiryRejectMalformed, Reason: "MALFORMED_RESPONSE"}
		}
		return inquiryDecision{Kind: inquiryCapture}
	}
	return inquiryDecision{Kind: inquiryTerminalPayment}
}

func optionalInquiryMoneyMismatch(attempt payments.PaymentAttempt, response payments.PaymentStatusResponse) string {
	if response.AmountRupiah != 0 && response.AmountRupiah != attempt.AmountRupiah {
		return "AMOUNT_MISMATCH"
	}
	if response.Currency != "" && response.Currency != attempt.Currency {
		return "CURRENCY_MISMATCH"
	}
	return ""
}

// validateInquiryResponse remains as a compact compatibility helper for
// existing unit callers; production processing uses the typed decision above.
func validateInquiryResponse(attempt payments.PaymentAttempt, response payments.PaymentStatusResponse) string {
	decision := decideInquiryResponse(attempt, response)
	if decision.Kind == inquiryRetry || decision.Kind == inquiryBindIdentityAndRetry {
		return ""
	}
	return decision.Reason
}

func validInquiryStatus(status payments.PaymentStatus) bool {
	return status == payments.PaymentStatusPending || status.IsTerminal()
}

func safeProviderStatusCode(value string) bool {
	if value == "" {
		return true
	}
	if len(value) > 64 || value != strings.TrimSpace(value) {
		return false
	}
	for _, char := range value {
		if (char < 'A' || char > 'Z') && (char < 'a' || char > 'z') && (char < '0' || char > '9') && char != '_' && char != '-' {
			return false
		}
	}
	return true
}

func commandFactsMatch(command paymentoutbox.Command, attempt payments.PaymentAttempt, payload paymentoutbox.PaymentCommandPayload) bool {
	if command.RequestHash != attempt.RequestHash || payload.AttemptID != attempt.ID ||
		payload.AmountRupiah != attempt.AmountRupiah || payload.Currency != string(attempt.Currency) ||
		payload.RequestedMethod != string(attempt.RequestedMethod) {
		return false
	}
	switch command.CommandType {
	case paymentoutbox.CommandPaymentCreate:
		return command.IdempotencyKey == paymentoutbox.DeterministicCreateKey(attempt.BookingID, attempt.AttemptNo)
	case paymentoutbox.CommandPaymentInquiry:
		return command.IdempotencyKey == paymentoutbox.DeterministicInquiryKey(attempt.ID)
	default:
		return false
	}
}

func validEvidenceHash(value string) bool {
	if len(value) != 64 {
		return false
	}
	decoded, err := hex.DecodeString(value)
	return err == nil && len(decoded) == 32 && value == strings.ToLower(value)
}

func decodePayload(raw json.RawMessage) (paymentoutbox.PaymentCommandPayload, error) {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	var payload paymentoutbox.PaymentCommandPayload
	if err := decoder.Decode(&payload); err != nil {
		return paymentoutbox.PaymentCommandPayload{}, err
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		return paymentoutbox.PaymentCommandPayload{}, ErrMalformedCommand
	}
	if payload.AttemptID == "" || payload.AmountRupiah <= 0 || payload.Currency != "IDR" || payload.RequestedMethod == "" {
		return paymentoutbox.PaymentCommandPayload{}, ErrMalformedCommand
	}
	return payload, nil
}

func optionalString(value string) *string {
	if value == "" {
		return nil
	}
	return &value
}

func optionalTime(value time.Time) *time.Time {
	if value.IsZero() {
		return nil
	}
	return &value
}

func valueOrEmpty(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

func isTerminalAttemptState(state payments.AttemptState) bool {
	switch state {
	case payments.AttemptStateCaptured, payments.AttemptStateFailed, payments.AttemptStateExpired, payments.AttemptStateCancelled:
		return true
	default:
		return false
	}
}
