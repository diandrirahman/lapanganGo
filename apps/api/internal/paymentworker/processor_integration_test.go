package paymentworker

import (
	"context"
	"database/sql"
	"errors"
	"net/url"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"lapangango-api/internal/audit"
	"lapangango-api/internal/paymentoutbox"
	"lapangango-api/internal/payments"

	"github.com/golang-migrate/migrate/v4"
	"github.com/golang-migrate/migrate/v4/database/postgres"
	_ "github.com/golang-migrate/migrate/v4/source/file"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	_ "github.com/lib/pq"
)

const workerTestHash = "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"

func TestPaymentWorkerDisposableEvidenceGate(t *testing.T) {
	if os.Getenv("REQUIRE_PAYMENT_WORKER_DISPOSABLE") != "1" {
		t.Skip("worker disposable evidence gate is opt-in")
	}
	if os.Getenv("TEST_ROLLBACK_HARDENING_DISPOSABLE") != "1" {
		t.Fatal("worker disposable evidence required but TEST_ROLLBACK_HARDENING_DISPOSABLE is not 1")
	}
	if strings.TrimSpace(os.Getenv("ROLLBACK_HARDENING_TEST_DATABASE_URL")) == "" {
		t.Fatal("worker disposable evidence required but ROLLBACK_HARDENING_TEST_DATABASE_URL is empty")
	}
	t.Log("PAYMENT_WORKER_DISPOSABLE_SUITE_ENABLED")
}

func TestPaymentWorkerDisposableMigrationVersion(t *testing.T) {
	ctx, pool := openWorkerTestDB(t)
	var version int
	var dirty bool
	if err := pool.QueryRow(ctx, `SELECT version, dirty FROM schema_migrations`).Scan(&version, &dirty); err != nil {
		t.Fatal(err)
	}
	if version != 29 || dirty {
		t.Fatalf("disposable migration state = %d|%t; want 29|false", version, dirty)
	}
}

func TestProcessorCreateEnqueuesExactlyOneInquiry(t *testing.T) {
	ctx, pool := openWorkerTestDB(t)
	attempt, outbox, processor, _ := newWorkerProcessorFixture(t, ctx, pool, payments.PaymentStatusPending)
	create := enqueueWorkerCommand(t, ctx, pool, outbox, attempt, paymentoutbox.CommandPaymentCreate)
	claimed, err := outbox.ClaimNextForTypes(ctx, "worker:"+uuid.NewString(), 30*time.Second, []paymentoutbox.CommandType{paymentoutbox.CommandPaymentCreate})
	if err != nil {
		t.Fatal(err)
	}
	if err := processor.Process(ctx, claimed); err != nil {
		t.Fatalf("create processor: %v", err)
	}
	var inquiryCount int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM payment_provider_commands WHERE payment_attempt_id = $1 AND command_type = 'PAYMENT_INQUIRY'`, attempt.ID).Scan(&inquiryCount); err != nil {
		t.Fatal(err)
	}
	if inquiryCount != 1 {
		t.Fatalf("inquiry command count = %d; want 1", inquiryCount)
	}
	var createState paymentoutbox.CommandState
	if err := pool.QueryRow(ctx, `SELECT state FROM payment_provider_commands WHERE id = $1`, create.ID).Scan(&createState); err != nil {
		t.Fatal(err)
	}
	if createState != paymentoutbox.StateSucceeded {
		t.Fatalf("create state = %q; want SUCCEEDED", createState)
	}
}

func TestProcessorCreateTimeoutReusesSameAttemptAndCommand(t *testing.T) {
	ctx, pool := openWorkerTestDB(t)
	attempt, outbox, processor, adapter := newWorkerProcessorFixture(t, ctx, pool, payments.PaymentStatusPending)
	create := enqueueWorkerCommand(t, ctx, pool, outbox, attempt, paymentoutbox.CommandPaymentCreate)
	callCount := 0
	var requests []payments.CreatePaymentRequest
	adapter.createPayment = func(_ context.Context, req payments.CreatePaymentRequest) (payments.CreatePaymentResponse, error) {
		callCount++
		requests = append(requests, req)
		if callCount == 1 {
			return payments.CreatePaymentResponse{}, payments.ErrRetryableTimeout
		}
		return payments.CreatePaymentResponse{ProviderSessionID: "session-timeout-replay-0001", Status: payments.PaymentStatusPending, AmountRupiah: req.AmountRupiah, Currency: req.Currency}, nil
	}
	owner := "worker:" + uuid.NewString()
	claimed, err := outbox.ClaimNextForTypes(ctx, owner, 30*time.Second, []paymentoutbox.CommandType{paymentoutbox.CommandPaymentCreate})
	if err != nil {
		t.Fatal(err)
	}
	if err := processor.Process(ctx, claimed); err != nil {
		t.Fatalf("first create timeout: %v", err)
	}
	timedOutAttempt, err := payments.NewRepository(pool).GetAttemptByID(ctx, attempt.ID)
	if err != nil {
		t.Fatal(err)
	}
	if timedOutAttempt.State != payments.AttemptStatePending {
		t.Fatalf("attempt state after timeout = %q; want PENDING", timedOutAttempt.State)
	}
	assertWorkerCommand(t, ctx, pool, create.ID, paymentoutbox.StateRetryable, create.IdempotencyKey)
	assertPaymentFlowCounts(t, ctx, pool, attempt, 1, 0)
	waitForWorkerCommandAvailability(t, ctx, pool, create.ID)
	claimed, err = outbox.ClaimNextForTypes(ctx, owner, 30*time.Second, []paymentoutbox.CommandType{paymentoutbox.CommandPaymentCreate})
	if err != nil {
		t.Fatal(err)
	}
	if claimed.ID != create.ID {
		t.Fatalf("timeout replay leased new command %s; want %s", claimed.ID, create.ID)
	}
	if err := processor.Process(ctx, claimed); err != nil {
		t.Fatalf("replayed create: %v", err)
	}
	var attemptCount, createCount, inquiryCount int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM payment_attempts WHERE id = $1`, attempt.ID).Scan(&attemptCount); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `SELECT count(*) FILTER (WHERE command_type = 'PAYMENT_CREATE'), count(*) FILTER (WHERE command_type = 'PAYMENT_INQUIRY') FROM payment_provider_commands WHERE payment_attempt_id = $1`, attempt.ID).Scan(&createCount, &inquiryCount); err != nil {
		t.Fatal(err)
	}
	if attemptCount != 1 || createCount != 1 || inquiryCount != 1 || callCount != 2 {
		t.Fatalf("timeout replay counts attempt=%d create=%d inquiry=%d adapterCalls=%d", attemptCount, createCount, inquiryCount, callCount)
	}
	if len(requests) != 2 ||
		requests[0].AttemptID != requests[1].AttemptID ||
		requests[0].IdempotencyKey != requests[1].IdempotencyKey ||
		requests[0].RequestHash != requests[1].RequestHash ||
		requests[0].AmountRupiah != requests[1].AmountRupiah ||
		requests[0].Currency != requests[1].Currency ||
		requests[0].RequestedMethod != requests[1].RequestedMethod {
		t.Fatalf("create retry changed immutable request facts: %#v", requests)
	}
	finalAttempt, err := payments.NewRepository(pool).GetAttemptByID(ctx, attempt.ID)
	if err != nil {
		t.Fatal(err)
	}
	if finalAttempt.ProviderSessionID == nil || *finalAttempt.ProviderSessionID != "session-timeout-replay-0001" {
		t.Fatalf("session identity after replay = %#v", finalAttempt.ProviderSessionID)
	}
	assertWorkerCommand(t, ctx, pool, create.ID, paymentoutbox.StateSucceeded, create.IdempotencyKey)
	assertNoProductionFinanceWrites(t, ctx, pool, attempt.BookingID)
}

func TestProcessorCreateMalformedResultUsesTwoStrikeGuard(t *testing.T) {
	tests := []struct {
		name     string
		response func(payments.CreatePaymentRequest) payments.CreatePaymentResponse
	}{
		{
			name: "invalid status code",
			response: func(req payments.CreatePaymentRequest) payments.CreatePaymentResponse {
				return payments.CreatePaymentResponse{
					ProviderSessionID: "session-malformed-status-0001",
					StatusCode:        "INVALID STATUS",
					Status:            payments.PaymentStatusPending,
					AmountRupiah:      req.AmountRupiah,
					Currency:          req.Currency,
				}
			},
		},
		{
			name: "invalid provider session identity",
			response: func(req payments.CreatePaymentRequest) payments.CreatePaymentResponse {
				return payments.CreatePaymentResponse{
					ProviderSessionID: "invalid provider identity",
					Status:            payments.PaymentStatusPending,
					AmountRupiah:      req.AmountRupiah,
					Currency:          req.Currency,
				}
			},
		},
		{
			name: "invalid optional provider identity",
			response: func(req payments.CreatePaymentRequest) payments.CreatePaymentResponse {
				return payments.CreatePaymentResponse{
					ProviderSessionID:    "session-malformed-optional-0001",
					ProviderPaymentReqID: strings.Repeat("r", 192),
					Status:               payments.PaymentStatusPending,
					AmountRupiah:         req.AmountRupiah,
					Currency:             req.Currency,
				}
			},
		},
		{
			name: "provider request identity contains NUL",
			response: func(req payments.CreatePaymentRequest) payments.CreatePaymentResponse {
				return payments.CreatePaymentResponse{
					ProviderSessionID:    "session-malformed-control-0001",
					ProviderPaymentReqID: "payment-request-\x00invalid",
					Status:               payments.PaymentStatusPending,
					AmountRupiah:         req.AmountRupiah,
					Currency:             req.Currency,
				}
			},
		},
		{
			name: "credential-like provider session identity",
			response: func(req payments.CreatePaymentRequest) payments.CreatePaymentResponse {
				return payments.CreatePaymentResponse{
					ProviderSessionID: "secret-demo-provider-value-0001",
					Status:            payments.PaymentStatusPending,
					AmountRupiah:      req.AmountRupiah,
					Currency:          req.Currency,
				}
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			ctx, pool := openWorkerTestDB(t)
			attempt, outbox, processor, adapter := newWorkerProcessorFixture(t, ctx, pool, payments.PaymentStatusPending)
			command := enqueueWorkerCommand(t, ctx, pool, outbox, attempt, paymentoutbox.CommandPaymentCreate)
			adapterCalls := 0
			adapter.createPayment = func(_ context.Context, req payments.CreatePaymentRequest) (payments.CreatePaymentResponse, error) {
				adapterCalls++
				return tc.response(req), nil
			}

			for strike, wantState := range []paymentoutbox.CommandState{
				paymentoutbox.StateRetryable,
				paymentoutbox.StateTerminal,
			} {
				if strike > 0 {
					waitForWorkerCommandAvailability(t, ctx, pool, command.ID)
				}
				claimed := claimWorkerCommand(t, ctx, outbox, paymentoutbox.CommandPaymentCreate)
				if err := processor.Process(ctx, claimed); err != nil {
					t.Fatalf("malformed strike %d: %v", strike+1, err)
				}
				assertWorkerCommand(t, ctx, pool, command.ID, wantState, command.IdempotencyKey)
			}

			var malformedCount int
			var lastErrorCode *string
			if err := pool.QueryRow(ctx, `
				SELECT malformed_response_count, last_error_code
				FROM payment_provider_commands
				WHERE id = $1
			`, command.ID).Scan(&malformedCount, &lastErrorCode); err != nil {
				t.Fatal(err)
			}
			if malformedCount != 2 || lastErrorCode == nil || *lastErrorCode != "MALFORMED_RESPONSE" {
				t.Fatalf("malformed lifecycle count/code = %d/%v; want 2/MALFORMED_RESPONSE", malformedCount, lastErrorCode)
			}
			if adapterCalls != 2 {
				t.Fatalf("adapter calls = %d; want exactly two malformed strikes", adapterCalls)
			}
			if _, err := outbox.ClaimNextForTypes(ctx, "worker:"+uuid.NewString(), 30*time.Second, []paymentoutbox.CommandType{paymentoutbox.CommandPaymentCreate}); !errors.Is(err, paymentoutbox.ErrNoCommandAvailable) {
				t.Fatalf("terminal malformed create reclaimed again: %v", err)
			}
			stored, err := payments.NewRepository(pool).GetAttemptByID(ctx, attempt.ID)
			if err != nil {
				t.Fatal(err)
			}
			if stored.State != payments.AttemptStateCreated || stored.ProviderSessionID != nil {
				t.Fatalf("malformed create mutated attempt: %#v", stored)
			}
		})
	}
}

func TestProcessorSessionHandoffPersistsIdentityAndCapturesOnPaymentScope(t *testing.T) {
	ctx, pool := openWorkerTestDB(t)
	attempt, outbox, processor, adapter := newWorkerProcessorFixture(t, ctx, pool, payments.PaymentStatusPending)
	// The create response is already persisted by the repository contract; the
	// test begins at the inquiry boundary to isolate the two-stage handoff.
	sessionID := "session-handoff-0001"
	reqID := "payment-request-handoff-0001"
	bindCreateResult(t, ctx, pool, attempt, sessionID)
	inquiryCommand := enqueueWorkerCommand(t, ctx, pool, outbox, attempt, paymentoutbox.CommandPaymentInquiry)
	claimed, err := outbox.ClaimNextForTypes(ctx, "worker:"+uuid.NewString(), 30*time.Second, []paymentoutbox.CommandType{paymentoutbox.CommandPaymentInquiry})
	if err != nil {
		t.Fatal(err)
	}
	adapter.getPaymentStatus = func(_ context.Context, req payments.GetPaymentStatusRequest) (payments.PaymentStatusResponse, error) {
		if req.ProviderSessionID != sessionID || req.ProviderPaymentReqID != "" {
			t.Fatalf("first inquiry identity = %#v", req)
		}
		return payments.PaymentStatusResponse{Scope: payments.PaymentInquiryScopeCheckoutSession, ProviderSessionID: sessionID, ProviderPaymentReqID: reqID, Status: payments.PaymentStatusPending}, nil
	}
	if err := processor.Process(ctx, claimed); err != nil {
		t.Fatalf("session handoff: %v", err)
	}
	stored, err := payments.NewRepository(pool).GetAttemptByID(ctx, attempt.ID)
	if err != nil || stored.ProviderPaymentReqID == nil || *stored.ProviderPaymentReqID != reqID {
		t.Fatalf("persisted payment request id = %#v, err=%v", stored.ProviderPaymentReqID, err)
	}
	var commandID string
	if err := pool.QueryRow(ctx, `SELECT id::text FROM payment_provider_commands WHERE payment_attempt_id = $1 AND command_type = 'PAYMENT_INQUIRY'`, attempt.ID).Scan(&commandID); err != nil {
		t.Fatal(err)
	}
	var commandState paymentoutbox.CommandState
	if err := pool.QueryRow(ctx, `SELECT state FROM payment_provider_commands WHERE id = $1`, commandID).Scan(&commandState); err != nil {
		t.Fatal(err)
	}
	if commandState != paymentoutbox.StateRetryable {
		t.Fatalf("handoff command state = %q; want RETRYABLE", commandState)
	}
	if commandID != inquiryCommand.ID {
		t.Fatalf("handoff changed inquiry command id: got %s want %s", commandID, inquiryCommand.ID)
	}
	var samePayload bool
	var storedHash string
	if err := pool.QueryRow(ctx, `SELECT request_hash, redacted_payload = $2::jsonb FROM payment_provider_commands WHERE id = $1`, inquiryCommand.ID, inquiryCommand.Payload).Scan(&storedHash, &samePayload); err != nil {
		t.Fatal(err)
	}
	if storedHash != inquiryCommand.RequestHash || !samePayload {
		t.Fatalf("handoff changed command request facts hash=%q same_payload=%v", storedHash, samePayload)
	}
	assertWorkerCommand(t, ctx, pool, inquiryCommand.ID, paymentoutbox.StateRetryable, inquiryCommand.IdempotencyKey)
	waitForWorkerCommandAvailability(t, ctx, pool, commandID)
	claimed, err = outbox.ClaimNextForTypes(ctx, "worker:"+uuid.NewString(), 30*time.Second, []paymentoutbox.CommandType{paymentoutbox.CommandPaymentInquiry})
	if err != nil {
		t.Fatal(err)
	}
	capturedAt := time.Now().UTC()
	adapter.getPaymentStatus = func(_ context.Context, req payments.GetPaymentStatusRequest) (payments.PaymentStatusResponse, error) {
		if req.ProviderPaymentReqID != reqID {
			t.Fatalf("second inquiry did not use persisted request id: %#v", req)
		}
		return payments.PaymentStatusResponse{Scope: payments.PaymentInquiryScopePayment, ProviderPaymentReqID: reqID, ProviderPaymentID: "payment-handoff-0001", Status: payments.PaymentStatusCaptured, AmountRupiah: attempt.AmountRupiah, Currency: attempt.Currency, CapturedAt: &capturedAt, PayloadHash: workerTestHash}, nil
	}
	if err := processor.Process(ctx, claimed); err != nil {
		t.Fatalf("payment capture after handoff: %v", err)
	}
	stored, err = payments.NewRepository(pool).GetAttemptByID(ctx, attempt.ID)
	if err != nil || stored.State != payments.AttemptStateCaptured {
		t.Fatalf("attempt after handoff capture = %#v, err=%v", stored, err)
	}
	var commandCount int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM payment_provider_commands WHERE payment_attempt_id = $1 AND command_type = 'PAYMENT_INQUIRY'`, attempt.ID).Scan(&commandCount); err != nil {
		t.Fatal(err)
	}
	if commandCount != 1 {
		t.Fatalf("inquiry command count = %d; want 1", commandCount)
	}
	var captureCount int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM payment_capture_facts WHERE payment_attempt_id = $1`, attempt.ID).Scan(&captureCount); err != nil {
		t.Fatal(err)
	}
	if captureCount != 1 {
		t.Fatalf("capture fact count = %d; want 1", captureCount)
	}
	var factCapturedAt time.Time
	var factHash string
	if err := pool.QueryRow(ctx, `SELECT captured_at, payload_hash FROM payment_capture_facts WHERE payment_attempt_id = $1`, attempt.ID).Scan(&factCapturedAt, &factHash); err != nil {
		t.Fatal(err)
	}
	if !factCapturedAt.Equal(capturedAt.UTC().Truncate(time.Microsecond)) || factHash != workerTestHash {
		t.Fatalf("capture evidence time=%s hash=%q", factCapturedAt, factHash)
	}
	assertWorkerCommand(t, ctx, pool, inquiryCommand.ID, paymentoutbox.StateSucceeded, inquiryCommand.IdempotencyKey)
	assertNoProductionFinanceWrites(t, ctx, pool, attempt.BookingID)
}

func TestProcessorAuditFailureRollsBackTerminalCommand(t *testing.T) {
	ctx, pool := openWorkerTestDB(t)
	attempt, outbox, processor, adapter := newWorkerProcessorFixture(t, ctx, pool, payments.PaymentStatusPending)
	processor.audit = failingAuditService{}
	sessionID := "session-audit-rollback"
	bindCreateResult(t, ctx, pool, attempt, sessionID)
	enqueueWorkerCommand(t, ctx, pool, outbox, attempt, paymentoutbox.CommandPaymentInquiry)
	claimed, err := outbox.ClaimNextForTypes(ctx, "worker:"+uuid.NewString(), 100*time.Millisecond, []paymentoutbox.CommandType{paymentoutbox.CommandPaymentInquiry})
	if err != nil {
		t.Fatal(err)
	}
	adapter.getPaymentStatus = func(context.Context, payments.GetPaymentStatusRequest) (payments.PaymentStatusResponse, error) {
		return payments.PaymentStatusResponse{Scope: payments.PaymentInquiryScopeCheckoutSession, ProviderSessionID: "wrong-session-0002", Status: payments.PaymentStatusPending}, nil
	}
	if err := processor.Process(ctx, claimed); err == nil {
		t.Fatal("audit failure was swallowed")
	}
	var state paymentoutbox.CommandState
	if err := pool.QueryRow(ctx, `SELECT state FROM payment_provider_commands WHERE id = $1`, claimed.ID).Scan(&state); err != nil {
		t.Fatal(err)
	}
	if state != paymentoutbox.StateLeased {
		t.Fatalf("command state after audit rollback = %q; want LEASED", state)
	}
	stored, err := payments.NewRepository(pool).GetAttemptByID(ctx, attempt.ID)
	if err != nil {
		t.Fatal(err)
	}
	var auditCount int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM platform_audit_logs WHERE entity_id = $1 AND action = $2`, attempt.ID, audit.ActionReconciliationException).Scan(&auditCount); err != nil {
		t.Fatal(err)
	}
	if stored.State != payments.AttemptStatePending || auditCount != 0 {
		t.Fatalf("audit rollback attempt=%q reconciliation_audits=%d", stored.State, auditCount)
	}
	waitForWorkerLeaseExpiry(t, ctx, pool, claimed.ID)
	processor.audit = audit.NewPlatformService(audit.NewPlatformRepository())
	adapter.getPaymentStatus = func(context.Context, payments.GetPaymentStatusRequest) (payments.PaymentStatusResponse, error) {
		return payments.PaymentStatusResponse{Scope: payments.PaymentInquiryScopeCheckoutSession, ProviderSessionID: sessionID, Status: payments.PaymentStatusPending}, nil
	}
	reclaimed := claimWorkerCommand(t, ctx, outbox, paymentoutbox.CommandPaymentInquiry)
	if reclaimed.ID != claimed.ID {
		t.Fatalf("audit rollback reclaimed command = %s; want %s", reclaimed.ID, claimed.ID)
	}
	if err := processor.Process(ctx, reclaimed); err != nil {
		t.Fatalf("process reclaimed audit command: %v", err)
	}
	assertWorkerCommand(t, ctx, pool, claimed.ID, paymentoutbox.StateRetryable, claimed.IdempotencyKey)
}

func TestProcessorStaleLeaseCannotCommitAfterReclaim(t *testing.T) {
	ctx, pool := openWorkerTestDB(t)
	attempt, outbox, processor, adapter := newWorkerProcessorFixture(t, ctx, pool, payments.PaymentStatusPending)
	bindCreateResult(t, ctx, pool, attempt, "session-stale-lease")
	enqueueWorkerCommand(t, ctx, pool, outbox, attempt, paymentoutbox.CommandPaymentInquiry)
	ownerA, ownerB := "worker:"+uuid.NewString(), "worker:"+uuid.NewString()
	claimedA, err := outbox.ClaimNextForTypes(ctx, ownerA, 20*time.Millisecond, []paymentoutbox.CommandType{paymentoutbox.CommandPaymentInquiry})
	if err != nil {
		t.Fatal(err)
	}
	waitForWorkerLeaseExpiry(t, ctx, pool, claimedA.ID)
	claimedB, err := outbox.ClaimNextForTypes(ctx, ownerB, 30*time.Second, []paymentoutbox.CommandType{paymentoutbox.CommandPaymentInquiry})
	if err != nil {
		t.Fatal(err)
	}
	adapter.getPaymentStatus = func(context.Context, payments.GetPaymentStatusRequest) (payments.PaymentStatusResponse, error) {
		return payments.PaymentStatusResponse{Scope: payments.PaymentInquiryScopeCheckoutSession, ProviderSessionID: "session-stale-lease", Status: payments.PaymentStatusPending}, nil
	}
	if err := processor.Process(ctx, claimedB); err != nil {
		t.Fatalf("reclaimed command processing: %v", err)
	}
	if err := processor.Process(ctx, claimedA); !errors.Is(err, paymentoutbox.ErrLeaseConflict) {
		t.Fatalf("stale lease process error = %v; want ErrLeaseConflict", err)
	}
	var state paymentoutbox.CommandState
	if err := pool.QueryRow(ctx, `SELECT state FROM payment_provider_commands WHERE id = $1`, claimedA.ID).Scan(&state); err != nil {
		t.Fatal(err)
	}
	if state != paymentoutbox.StateRetryable {
		t.Fatalf("state after stale processor = %q; want RETRYABLE", state)
	}
}

func TestTwoConcurrentProcessorsCommitExactlyOneCaptureEffect(t *testing.T) {
	ctx, pool := openWorkerTestDB(t)
	attempt, outbox, processorA, adapter := newWorkerProcessorFixture(t, ctx, pool, payments.PaymentStatusPending)
	sessionID := "session-concurrent-capture-0001"
	requestID := "request-concurrent-capture-0001"
	bindCreateResult(t, ctx, pool, attempt, sessionID)
	bindInquiryRequest(t, ctx, pool, attempt, sessionID, requestID, "")
	command := enqueueWorkerCommand(t, ctx, pool, outbox, attempt, paymentoutbox.CommandPaymentInquiry)
	claimedA, err := outbox.ClaimNextForTypes(ctx, "worker:"+uuid.NewString(), 100*time.Millisecond, []paymentoutbox.CommandType{paymentoutbox.CommandPaymentInquiry})
	if err != nil {
		t.Fatal(err)
	}
	processorB, err := NewProcessor(
		pool,
		payments.NewRepository(pool),
		outbox,
		adapter,
		ProcessorOptions{Audit: audit.NewPlatformService(audit.NewPlatformRepository()), AdapterTimeout: 2 * time.Second},
	)
	if err != nil {
		t.Fatal(err)
	}
	capturedAt := time.Now().UTC().Truncate(time.Microsecond)
	firstStarted := make(chan struct{})
	releaseFirst := make(chan struct{})
	var releaseOnce sync.Once
	defer releaseOnce.Do(func() { close(releaseFirst) })
	var callsMu sync.Mutex
	adapterCalls := 0
	adapter.getPaymentStatus = func(context.Context, payments.GetPaymentStatusRequest) (payments.PaymentStatusResponse, error) {
		callsMu.Lock()
		adapterCalls++
		call := adapterCalls
		callsMu.Unlock()
		if call == 1 {
			close(firstStarted)
			<-releaseFirst
		}
		return payments.PaymentStatusResponse{
			Scope:                payments.PaymentInquiryScopePayment,
			ProviderPaymentReqID: requestID,
			ProviderPaymentID:    "payment-concurrent-capture-0001",
			Status:               payments.PaymentStatusCaptured,
			AmountRupiah:         attempt.AmountRupiah,
			Currency:             attempt.Currency,
			CapturedAt:           &capturedAt,
			PayloadHash:          workerTestHash,
		}, nil
	}
	firstResult := make(chan error, 1)
	go func() { firstResult <- processorA.Process(ctx, claimedA) }()
	<-firstStarted
	waitForWorkerLeaseExpiry(t, ctx, pool, command.ID)
	claimedB := claimWorkerCommand(t, ctx, outbox, paymentoutbox.CommandPaymentInquiry)
	if claimedB.ID != command.ID {
		t.Fatalf("second processor claimed command %s; want %s", claimedB.ID, command.ID)
	}
	if err := processorB.Process(ctx, claimedB); err != nil {
		t.Fatalf("second processor capture: %v", err)
	}
	releaseOnce.Do(func() { close(releaseFirst) })
	if err := <-firstResult; !errors.Is(err, paymentoutbox.ErrLeaseConflict) {
		t.Fatalf("stale concurrent processor error = %v; want ErrLeaseConflict", err)
	}
	stored, err := payments.NewRepository(pool).GetAttemptByID(ctx, attempt.ID)
	if err != nil {
		t.Fatal(err)
	}
	if stored.State != payments.AttemptStateCaptured {
		t.Fatalf("concurrent attempt state = %q; want CAPTURED", stored.State)
	}
	var captureCount int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM payment_capture_facts WHERE payment_attempt_id = $1`, attempt.ID).Scan(&captureCount); err != nil {
		t.Fatal(err)
	}
	callsMu.Lock()
	finalAdapterCalls := adapterCalls
	callsMu.Unlock()
	if captureCount != 1 || finalAdapterCalls != 2 {
		t.Fatalf("concurrent effects capture=%d adapter_calls=%d; want 1/2", captureCount, finalAdapterCalls)
	}
	assertWorkerCommand(t, ctx, pool, command.ID, paymentoutbox.StateSucceeded, command.IdempotencyKey)
	assertPaymentFlowCounts(t, ctx, pool, attempt, 0, 1)
	assertNoProductionFinanceWrites(t, ctx, pool, attempt.BookingID)
}

func TestProcessorExactRetryRecoversAfterLocalTransactionFailure(t *testing.T) {
	ctx, pool := openWorkerTestDB(t)
	attempt, outbox, processor, adapter := newWorkerProcessorFixture(t, ctx, pool, payments.PaymentStatusPending)
	sessionID := "session-local-tx-recovery-0001"
	requestID := "request-local-tx-recovery-0001"
	bindCreateResult(t, ctx, pool, attempt, sessionID)
	bindInquiryRequest(t, ctx, pool, attempt, sessionID, requestID, "")
	command := enqueueWorkerCommand(t, ctx, pool, outbox, attempt, paymentoutbox.CommandPaymentInquiry)
	claimed, err := outbox.ClaimNextForTypes(ctx, "worker:"+uuid.NewString(), time.Second, []paymentoutbox.CommandType{paymentoutbox.CommandPaymentInquiry})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `
		CREATE FUNCTION fail_worker_command_success_for_test()
		RETURNS trigger
		LANGUAGE plpgsql
		AS $$
		BEGIN
			RAISE EXCEPTION 'forced local transaction failure';
		END;
		$$;

		CREATE TRIGGER fail_worker_command_success_for_test
		BEFORE UPDATE OF state ON payment_provider_commands
		FOR EACH ROW
		EXECUTE FUNCTION fail_worker_command_success_for_test();
	`); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), `DROP TRIGGER IF EXISTS fail_worker_command_success_for_test ON payment_provider_commands`)
		_, _ = pool.Exec(context.Background(), `DROP FUNCTION IF EXISTS fail_worker_command_success_for_test()`)
	})

	capturedAt := time.Now().UTC().Truncate(time.Microsecond)
	var requests []payments.GetPaymentStatusRequest
	adapter.getPaymentStatus = func(_ context.Context, req payments.GetPaymentStatusRequest) (payments.PaymentStatusResponse, error) {
		requests = append(requests, req)
		return payments.PaymentStatusResponse{
			Scope:                payments.PaymentInquiryScopePayment,
			ProviderPaymentReqID: requestID,
			ProviderPaymentID:    "payment-localtx-recovery-0001",
			Status:               payments.PaymentStatusCaptured,
			AmountRupiah:         attempt.AmountRupiah,
			Currency:             attempt.Currency,
			CapturedAt:           &capturedAt,
			PayloadHash:          workerTestHash,
		}, nil
	}
	if err := processor.Process(ctx, claimed); err == nil {
		t.Fatal("forced local transaction failure was swallowed")
	}
	var commandState paymentoutbox.CommandState
	var captureCount int
	if err := pool.QueryRow(ctx, `SELECT state FROM payment_provider_commands WHERE id = $1`, command.ID).Scan(&commandState); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM payment_capture_facts WHERE payment_attempt_id = $1`, attempt.ID).Scan(&captureCount); err != nil {
		t.Fatal(err)
	}
	stored, err := payments.NewRepository(pool).GetAttemptByID(ctx, attempt.ID)
	if err != nil {
		t.Fatal(err)
	}
	if commandState != paymentoutbox.StateLeased || captureCount != 0 || stored.State != payments.AttemptStatePending {
		t.Fatalf("failed local tx residue command=%q capture=%d attempt=%q; want LEASED/0/PENDING", commandState, captureCount, stored.State)
	}
	if _, err := pool.Exec(ctx, `DROP TRIGGER fail_worker_command_success_for_test ON payment_provider_commands`); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `DROP FUNCTION fail_worker_command_success_for_test()`); err != nil {
		t.Fatal(err)
	}
	waitForWorkerLeaseExpiry(t, ctx, pool, command.ID)
	reclaimed := claimWorkerCommand(t, ctx, outbox, paymentoutbox.CommandPaymentInquiry)
	if reclaimed.ID != command.ID {
		t.Fatalf("recovery claimed command %s; want %s", reclaimed.ID, command.ID)
	}
	if err := processor.Process(ctx, reclaimed); err != nil {
		t.Fatalf("exact retry recovery: %v", err)
	}
	stored, err = payments.NewRepository(pool).GetAttemptByID(ctx, attempt.ID)
	if err != nil {
		t.Fatal(err)
	}
	if stored.State != payments.AttemptStateCaptured {
		var recoveredCommandState paymentoutbox.CommandState
		var recoveredErrorCode *string
		if err := pool.QueryRow(ctx, `SELECT state, last_error_code FROM payment_provider_commands WHERE id = $1`, command.ID).Scan(&recoveredCommandState, &recoveredErrorCode); err != nil {
			t.Fatal(err)
		}
		errorCode := ""
		if recoveredErrorCode != nil {
			errorCode = *recoveredErrorCode
		}
		t.Fatalf("recovered attempt state = %q command=%q error=%q session=%v request=%v payment=%v requests=%#v; want CAPTURED/SUCCEEDED",
			stored.State, recoveredCommandState, errorCode, stored.ProviderSessionID, stored.ProviderPaymentReqID, stored.ProviderPaymentID, requests)
	}
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM payment_capture_facts WHERE payment_attempt_id = $1`, attempt.ID).Scan(&captureCount); err != nil {
		t.Fatal(err)
	}
	if len(requests) != 2 ||
		requests[0].AttemptID != requests[1].AttemptID ||
		requests[0].IdempotencyKey != requests[1].IdempotencyKey ||
		requests[0].ProviderPaymentReqID != requests[1].ProviderPaymentReqID ||
		captureCount != 1 {
		t.Fatalf("exact recovery requests=%#v capture_count=%d", requests, captureCount)
	}
	assertWorkerCommand(t, ctx, pool, command.ID, paymentoutbox.StateSucceeded, command.IdempotencyKey)
	assertPaymentFlowCounts(t, ctx, pool, attempt, 0, 1)
	assertNoProductionFinanceWrites(t, ctx, pool, attempt.BookingID)
}

func TestProcessorInquiryOutcomeMatrix(t *testing.T) {
	tests := []struct {
		name          string
		withRequest   bool
		wantRequestID bool
		response      func(payments.PaymentAttempt, string, string) (payments.PaymentStatusResponse, error)
		wantState     payments.AttemptState
		wantCommand   paymentoutbox.CommandState
		wantPaymentID string
	}{
		{
			name: "session pending without request id",
			response: func(_ payments.PaymentAttempt, sessionID, _ string) (payments.PaymentStatusResponse, error) {
				return payments.PaymentStatusResponse{Scope: payments.PaymentInquiryScopeCheckoutSession, ProviderSessionID: sessionID, Status: payments.PaymentStatusPending}, nil
			},
			wantState: payments.AttemptStatePending, wantCommand: paymentoutbox.StateRetryable,
		},
		{
			name: "inquiry timeout",
			response: func(payments.PaymentAttempt, string, string) (payments.PaymentStatusResponse, error) {
				return payments.PaymentStatusResponse{}, payments.ErrRetryableTimeout
			},
			wantState: payments.AttemptStatePending, wantCommand: paymentoutbox.StateRetryable,
		},
		{
			name: "session expired",
			response: func(_ payments.PaymentAttempt, sessionID, _ string) (payments.PaymentStatusResponse, error) {
				return payments.PaymentStatusResponse{Scope: payments.PaymentInquiryScopeCheckoutSession, ProviderSessionID: sessionID, StatusCode: "EXPIRED", Status: payments.PaymentStatusExpired}, nil
			},
			wantState: payments.AttemptStateExpired, wantCommand: paymentoutbox.StateSucceeded,
		},
		{
			name: "session cancelled",
			response: func(_ payments.PaymentAttempt, sessionID, _ string) (payments.PaymentStatusResponse, error) {
				return payments.PaymentStatusResponse{Scope: payments.PaymentInquiryScopeCheckoutSession, ProviderSessionID: sessionID, StatusCode: "CANCELLED", Status: payments.PaymentStatusCancelled}, nil
			},
			wantState: payments.AttemptStateCancelled, wantCommand: paymentoutbox.StateSucceeded,
		},
		{
			name: "session expired with newly discovered request id",
			response: func(_ payments.PaymentAttempt, sessionID, requestID string) (payments.PaymentStatusResponse, error) {
				return payments.PaymentStatusResponse{
					Scope:                payments.PaymentInquiryScopeCheckoutSession,
					ProviderSessionID:    sessionID,
					ProviderPaymentReqID: requestID,
					StatusCode:           "EXPIRED",
					Status:               payments.PaymentStatusExpired,
				}, nil
			},
			wantRequestID: true,
			wantState:     payments.AttemptStatePending,
			wantCommand:   paymentoutbox.StateRetryable,
		},
		{
			name: "session cancelled with newly discovered request id",
			response: func(_ payments.PaymentAttempt, sessionID, requestID string) (payments.PaymentStatusResponse, error) {
				return payments.PaymentStatusResponse{
					Scope:                payments.PaymentInquiryScopeCheckoutSession,
					ProviderSessionID:    sessionID,
					ProviderPaymentReqID: requestID,
					StatusCode:           "CANCELLED",
					Status:               payments.PaymentStatusCancelled,
				}, nil
			},
			wantRequestID: true,
			wantState:     payments.AttemptStatePending,
			wantCommand:   paymentoutbox.StateRetryable,
		},
		{
			name:        "payment pending",
			withRequest: true,
			response: func(attempt payments.PaymentAttempt, _ string, requestID string) (payments.PaymentStatusResponse, error) {
				return payments.PaymentStatusResponse{Scope: payments.PaymentInquiryScopePayment, ProviderPaymentReqID: requestID, Status: payments.PaymentStatusPending, AmountRupiah: attempt.AmountRupiah, Currency: attempt.Currency}, nil
			},
			wantState: payments.AttemptStatePending, wantCommand: paymentoutbox.StateRetryable,
		},
		{
			name:        "payment failed",
			withRequest: true,
			response: func(attempt payments.PaymentAttempt, _ string, requestID string) (payments.PaymentStatusResponse, error) {
				return terminalPaymentResponse(attempt, requestID, payments.PaymentStatusFailed), nil
			},
			wantState: payments.AttemptStateFailed, wantCommand: paymentoutbox.StateSucceeded, wantPaymentID: "payment-failed-0001",
		},
		{
			name:        "payment expired",
			withRequest: true,
			response: func(attempt payments.PaymentAttempt, _ string, requestID string) (payments.PaymentStatusResponse, error) {
				return terminalPaymentResponse(attempt, requestID, payments.PaymentStatusExpired), nil
			},
			wantState: payments.AttemptStateExpired, wantCommand: paymentoutbox.StateSucceeded, wantPaymentID: "payment-expired-0001",
		},
		{
			name:        "payment cancelled",
			withRequest: true,
			response: func(attempt payments.PaymentAttempt, _ string, requestID string) (payments.PaymentStatusResponse, error) {
				return terminalPaymentResponse(attempt, requestID, payments.PaymentStatusCancelled), nil
			},
			wantState: payments.AttemptStateCancelled, wantCommand: paymentoutbox.StateSucceeded, wantPaymentID: "payment-cancelled-0001",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			ctx, pool := openWorkerTestDB(t)
			attempt, outbox, processor, adapter := newWorkerProcessorFixture(t, ctx, pool, payments.PaymentStatusPending)
			sessionID := "session-" + strings.ReplaceAll(uuid.NewString(), "-", "")
			requestID := "request-" + strings.ReplaceAll(uuid.NewString(), "-", "")
			bindCreateResult(t, ctx, pool, attempt, sessionID)
			if tc.withRequest {
				bindInquiryRequest(t, ctx, pool, attempt, sessionID, requestID, "")
			}
			command := enqueueWorkerCommand(t, ctx, pool, outbox, attempt, paymentoutbox.CommandPaymentInquiry)
			claimed := claimWorkerCommand(t, ctx, outbox, paymentoutbox.CommandPaymentInquiry)
			adapter.getPaymentStatus = func(context.Context, payments.GetPaymentStatusRequest) (payments.PaymentStatusResponse, error) {
				return tc.response(attempt, sessionID, requestID)
			}
			if err := processor.Process(ctx, claimed); err != nil {
				t.Fatalf("process inquiry: %v", err)
			}
			stored, err := payments.NewRepository(pool).GetAttemptByID(ctx, attempt.ID)
			if err != nil {
				t.Fatal(err)
			}
			if stored.State != tc.wantState {
				t.Fatalf("attempt state = %q; want %q", stored.State, tc.wantState)
			}
			if tc.wantRequestID &&
				(stored.ProviderPaymentReqID == nil || *stored.ProviderPaymentReqID != requestID) {
				t.Fatalf("new request identity = %#v; want %q", stored.ProviderPaymentReqID, requestID)
			}
			if tc.wantPaymentID != "" &&
				(stored.ProviderPaymentID == nil || *stored.ProviderPaymentID != tc.wantPaymentID ||
					stored.ProviderStatusCode == nil || *stored.ProviderStatusCode != string(tc.wantState)) {
				t.Fatalf("terminal payment identity/status = %#v/%#v; want %q/%q",
					stored.ProviderPaymentID, stored.ProviderStatusCode, tc.wantPaymentID, tc.wantState)
			}
			assertWorkerCommand(t, ctx, pool, command.ID, tc.wantCommand, command.IdempotencyKey)
			assertPaymentFlowCounts(t, ctx, pool, attempt, 0, 1)
			assertNoProductionFinanceWrites(t, ctx, pool, attempt.BookingID)
		})
	}
}

func TestProcessorMismatchAndMalformedMatrix(t *testing.T) {
	tests := []struct {
		name        string
		withRequest bool
		withPayment bool
		malformed   bool
		response    func(payments.PaymentAttempt, string, string, string) payments.PaymentStatusResponse
	}{
		{
			name: "wrong session id",
			response: func(_ payments.PaymentAttempt, _ string, _ string, _ string) payments.PaymentStatusResponse {
				return payments.PaymentStatusResponse{Scope: payments.PaymentInquiryScopeCheckoutSession, ProviderSessionID: "wrong-session-0002", Status: payments.PaymentStatusPending}
			},
		},
		{
			name: "wrong request id", withRequest: true,
			response: func(_ payments.PaymentAttempt, _ string, _ string, _ string) payments.PaymentStatusResponse {
				return payments.PaymentStatusResponse{Scope: payments.PaymentInquiryScopePayment, ProviderPaymentReqID: "wrong-request-0002", Status: payments.PaymentStatusPending}
			},
		},
		{
			name: "wrong payment id", withRequest: true, withPayment: true,
			response: func(_ payments.PaymentAttempt, _ string, requestID string, _ string) payments.PaymentStatusResponse {
				return payments.PaymentStatusResponse{Scope: payments.PaymentInquiryScopePayment, ProviderPaymentReqID: requestID, ProviderPaymentID: "wrong-payment-0002", Status: payments.PaymentStatusPending}
			},
		},
		{
			name: "amount mismatch", withRequest: true,
			response: func(attempt payments.PaymentAttempt, _ string, requestID string, _ string) payments.PaymentStatusResponse {
				capturedAt := time.Now().UTC()
				return payments.PaymentStatusResponse{Scope: payments.PaymentInquiryScopePayment, ProviderPaymentReqID: requestID, ProviderPaymentID: "payment-amount-mismatch-0001", Status: payments.PaymentStatusCaptured, AmountRupiah: attempt.AmountRupiah + 1, Currency: attempt.Currency, CapturedAt: &capturedAt, PayloadHash: workerTestHash}
			},
		},
		{
			name: "currency mismatch", withRequest: true,
			response: func(attempt payments.PaymentAttempt, _ string, requestID string, _ string) payments.PaymentStatusResponse {
				capturedAt := time.Now().UTC()
				return payments.PaymentStatusResponse{Scope: payments.PaymentInquiryScopePayment, ProviderPaymentReqID: requestID, ProviderPaymentID: "payment-currency-mismatch-0001", Status: payments.PaymentStatusCaptured, AmountRupiah: attempt.AmountRupiah, Currency: payments.Currency("USD"), CapturedAt: &capturedAt, PayloadHash: workerTestHash}
			},
		},
		{
			name: "pending amount mismatch", withRequest: true,
			response: func(attempt payments.PaymentAttempt, _ string, requestID string, _ string) payments.PaymentStatusResponse {
				return payments.PaymentStatusResponse{Scope: payments.PaymentInquiryScopePayment, ProviderPaymentReqID: requestID, Status: payments.PaymentStatusPending, AmountRupiah: attempt.AmountRupiah + 1, Currency: attempt.Currency}
			},
		},
		{
			name: "pending currency mismatch", withRequest: true,
			response: func(attempt payments.PaymentAttempt, _ string, requestID string, _ string) payments.PaymentStatusResponse {
				return payments.PaymentStatusResponse{Scope: payments.PaymentInquiryScopePayment, ProviderPaymentReqID: requestID, Status: payments.PaymentStatusPending, AmountRupiah: attempt.AmountRupiah, Currency: payments.Currency("USD")}
			},
		},
		{
			name: "missing capture evidence", withRequest: true, malformed: true,
			response: func(attempt payments.PaymentAttempt, _ string, requestID string, _ string) payments.PaymentStatusResponse {
				return payments.PaymentStatusResponse{Scope: payments.PaymentInquiryScopePayment, ProviderPaymentReqID: requestID, ProviderPaymentID: "payment-missing-evidence-0001", Status: payments.PaymentStatusCaptured, AmountRupiah: attempt.AmountRupiah, Currency: attempt.Currency}
			},
		},
		{
			name: "provider payment identity contains control character", withRequest: true, malformed: true,
			response: func(_ payments.PaymentAttempt, _ string, requestID string, _ string) payments.PaymentStatusResponse {
				return payments.PaymentStatusResponse{Scope: payments.PaymentInquiryScopePayment, ProviderPaymentReqID: requestID, ProviderPaymentID: "payment-\x00invalid", Status: payments.PaymentStatusPending}
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			ctx, pool := openWorkerTestDB(t)
			attempt, outbox, processor, adapter := newWorkerProcessorFixture(t, ctx, pool, payments.PaymentStatusPending)
			sessionID := "session-" + strings.ReplaceAll(uuid.NewString(), "-", "")
			requestID := "request-" + strings.ReplaceAll(uuid.NewString(), "-", "")
			paymentID := "payment-" + strings.ReplaceAll(uuid.NewString(), "-", "")
			bindCreateResult(t, ctx, pool, attempt, sessionID)
			if tc.withRequest {
				bindInquiryRequest(t, ctx, pool, attempt, sessionID, requestID, "")
			}
			if tc.withPayment {
				bindInquiryRequest(t, ctx, pool, attempt, sessionID, requestID, paymentID)
			}
			command := enqueueWorkerCommand(t, ctx, pool, outbox, attempt, paymentoutbox.CommandPaymentInquiry)
			response := tc.response(attempt, sessionID, requestID, paymentID)
			adapter.getPaymentStatus = func(context.Context, payments.GetPaymentStatusRequest) (payments.PaymentStatusResponse, error) {
				return response, nil
			}
			claimed := claimWorkerCommand(t, ctx, outbox, paymentoutbox.CommandPaymentInquiry)
			if err := processor.Process(ctx, claimed); err != nil {
				t.Fatalf("first mismatch/malformed process: %v", err)
			}
			wantState := paymentoutbox.StateTerminal
			if tc.malformed {
				wantState = paymentoutbox.StateRetryable
			}
			assertWorkerCommand(t, ctx, pool, command.ID, wantState, command.IdempotencyKey)
			if tc.malformed {
				waitForWorkerCommandAvailability(t, ctx, pool, command.ID)
				claimed = claimWorkerCommand(t, ctx, outbox, paymentoutbox.CommandPaymentInquiry)
				if err := processor.Process(ctx, claimed); err != nil {
					t.Fatalf("second malformed process: %v", err)
				}
				assertWorkerCommand(t, ctx, pool, command.ID, paymentoutbox.StateTerminal, command.IdempotencyKey)
			}
			stored, err := payments.NewRepository(pool).GetAttemptByID(ctx, attempt.ID)
			if err != nil {
				t.Fatal(err)
			}
			if stored.State != payments.AttemptStatePending {
				t.Fatalf("invalid response mutated attempt to %q", stored.State)
			}
			var captureCount, auditCount int
			if err := pool.QueryRow(ctx, `SELECT count(*) FROM payment_capture_facts WHERE payment_attempt_id = $1`, attempt.ID).Scan(&captureCount); err != nil {
				t.Fatal(err)
			}
			if err := pool.QueryRow(ctx, `SELECT count(*) FROM platform_audit_logs WHERE entity_id = $1 AND action = $2`, attempt.ID, audit.ActionReconciliationException).Scan(&auditCount); err != nil {
				t.Fatal(err)
			}
			if captureCount != 0 || auditCount != 1 {
				t.Fatalf("invalid response effects capture=%d reconciliation_audit=%d", captureCount, auditCount)
			}
			assertNoProductionFinanceWrites(t, ctx, pool, attempt.BookingID)
		})
	}
}

func TestProcessorLateCaptureKeepsBookingCancelledAndFinanceIsolated(t *testing.T) {
	ctx, pool := openWorkerTestDB(t)
	attempt, outbox, processor, adapter := newWorkerProcessorFixture(t, ctx, pool, payments.PaymentStatusPending)
	sessionID := "session-late-capture-0001"
	requestID := "request-late-capture-0001"
	bindCreateResult(t, ctx, pool, attempt, sessionID)
	bindInquiryRequest(t, ctx, pool, attempt, sessionID, requestID, "")
	command := enqueueWorkerCommand(t, ctx, pool, outbox, attempt, paymentoutbox.CommandPaymentInquiry)
	claimed := claimWorkerCommand(t, ctx, outbox, paymentoutbox.CommandPaymentInquiry)

	started := make(chan struct{})
	release := make(chan struct{})
	var releaseOnce sync.Once
	defer releaseOnce.Do(func() { close(release) })
	capturedAt := time.Now().UTC()
	adapter.getPaymentStatus = func(context.Context, payments.GetPaymentStatusRequest) (payments.PaymentStatusResponse, error) {
		close(started)
		<-release
		return payments.PaymentStatusResponse{Scope: payments.PaymentInquiryScopePayment, ProviderPaymentReqID: requestID, ProviderPaymentID: "payment-late-capture-0001", Status: payments.PaymentStatusCaptured, AmountRupiah: attempt.AmountRupiah, Currency: attempt.Currency, CapturedAt: &capturedAt, PayloadHash: workerTestHash}, nil
	}
	result := make(chan error, 1)
	go func() { result <- processor.Process(ctx, claimed) }()
	<-started
	if _, err := pool.Exec(ctx, `UPDATE bookings SET status = 'CANCELLED', updated_at = transaction_timestamp() WHERE id = $1`, attempt.BookingID); err != nil {
		t.Fatal(err)
	}
	if _, err := payments.NewRepository(pool).TransitionState(ctx, attempt.ID, payments.AttemptStatePending, payments.AttemptStateCancelled); err != nil {
		t.Fatal(err)
	}
	releaseOnce.Do(func() { close(release) })
	if err := <-result; err != nil {
		t.Fatalf("late capture process: %v", err)
	}

	stored, err := payments.NewRepository(pool).GetAttemptByID(ctx, attempt.ID)
	if err != nil {
		t.Fatal(err)
	}
	var bookingStatus string
	var captureCount, lateAuditCount int
	if err := pool.QueryRow(ctx, `SELECT status FROM bookings WHERE id = $1`, attempt.BookingID).Scan(&bookingStatus); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM payment_capture_facts WHERE payment_attempt_id = $1`, attempt.ID).Scan(&captureCount); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM platform_audit_logs WHERE entity_id = $1 AND action = $2 AND metadata->>'reason' = 'LATE_CAPTURE'`, attempt.ID, audit.ActionReconciliationException).Scan(&lateAuditCount); err != nil {
		t.Fatal(err)
	}
	if stored.State != payments.AttemptStateCaptured || bookingStatus != "CANCELLED" || captureCount != 1 || lateAuditCount != 1 {
		t.Fatalf("late capture state=%q booking=%q capture=%d audit=%d", stored.State, bookingStatus, captureCount, lateAuditCount)
	}
	assertWorkerCommand(t, ctx, pool, command.ID, paymentoutbox.StateSucceeded, command.IdempotencyKey)
	assertNoProductionFinanceWrites(t, ctx, pool, attempt.BookingID)
}

func TestProcessorDuplicateAndOutOfOrderRemainSingleEffect(t *testing.T) {
	ctx, pool := openWorkerTestDB(t)
	attempt, outbox, processor, adapter := newWorkerProcessorFixture(t, ctx, pool, payments.PaymentStatusPending)
	sessionID := "session-outof-order-0001"
	requestID := "request-outof-order-0001"
	paymentID := "payment-outof-order-0001"
	bindCreateResult(t, ctx, pool, attempt, sessionID)
	bindInquiryRequest(t, ctx, pool, attempt, sessionID, requestID, "")
	command := enqueueWorkerCommand(t, ctx, pool, outbox, attempt, paymentoutbox.CommandPaymentInquiry)
	claimed := claimWorkerCommand(t, ctx, outbox, paymentoutbox.CommandPaymentInquiry)
	capturedAt := time.Now().UTC().Add(-time.Second)
	capture := payments.CaptureParams{AttemptID: attempt.ID, Provider: attempt.Provider, ProviderEnvironment: attempt.ProviderEnvironment, ProviderPaymentID: paymentID, ProviderPaymentReqID: &requestID, AmountRupiah: attempt.AmountRupiah, Currency: attempt.Currency, CapturedAt: capturedAt, ObservedAt: capturedAt.Add(time.Second), Authority: "AUTHENTICATED_INQUIRY", SourceReference: command.IdempotencyKey, PayloadHash: workerTestHash}
	first, err := payments.NewRepository(pool).RecordCapture(ctx, capture)
	if err != nil {
		t.Fatal(err)
	}
	duplicate, err := payments.NewRepository(pool).RecordCapture(ctx, capture)
	if err != nil {
		t.Fatal(err)
	}
	if first.Duplicate || !duplicate.Duplicate || first.Fact.ID != duplicate.Fact.ID {
		t.Fatalf("capture replay first=%#v duplicate=%#v", first, duplicate)
	}
	adapter.getPaymentStatus = func(context.Context, payments.GetPaymentStatusRequest) (payments.PaymentStatusResponse, error) {
		t.Fatal("terminal stale command must not call adapter")
		return payments.PaymentStatusResponse{}, nil
	}
	if err := processor.Process(ctx, claimed); err != nil {
		t.Fatalf("terminal stale command: %v", err)
	}
	stored, err := payments.NewRepository(pool).GetAttemptByID(ctx, attempt.ID)
	if err != nil {
		t.Fatal(err)
	}
	if stored.State != payments.AttemptStateCaptured {
		t.Fatalf("failure/out-of-order downgraded captured attempt to %q", stored.State)
	}
	var captureCount int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM payment_capture_facts WHERE payment_attempt_id = $1`, attempt.ID).Scan(&captureCount); err != nil {
		t.Fatal(err)
	}
	if captureCount != 1 {
		t.Fatalf("capture count = %d; want 1", captureCount)
	}
	assertWorkerCommand(t, ctx, pool, command.ID, paymentoutbox.StateSucceeded, command.IdempotencyKey)
	assertNoProductionFinanceWrites(t, ctx, pool, attempt.BookingID)
}

func TestProcessorTerminalRaceFinalizesLeasedCommand(t *testing.T) {
	t.Run("inquiry handoff after terminal transition", func(t *testing.T) {
		ctx, pool := openWorkerTestDB(t)
		attempt, outbox, processor, adapter := newWorkerProcessorFixture(t, ctx, pool, payments.PaymentStatusPending)
		sessionID := "session-terminal-race-0001"
		bindCreateResult(t, ctx, pool, attempt, sessionID)
		command := enqueueWorkerCommand(t, ctx, pool, outbox, attempt, paymentoutbox.CommandPaymentInquiry)
		claimed := claimWorkerCommand(t, ctx, outbox, paymentoutbox.CommandPaymentInquiry)
		started, release := make(chan struct{}), make(chan struct{})
		var releaseOnce sync.Once
		defer releaseOnce.Do(func() { close(release) })
		adapter.getPaymentStatus = func(context.Context, payments.GetPaymentStatusRequest) (payments.PaymentStatusResponse, error) {
			close(started)
			<-release
			return payments.PaymentStatusResponse{Scope: payments.PaymentInquiryScopeCheckoutSession, ProviderSessionID: sessionID, ProviderPaymentReqID: "request-terminal-race-0001", Status: payments.PaymentStatusPending}, nil
		}
		result := make(chan error, 1)
		go func() { result <- processor.Process(ctx, claimed) }()
		<-started
		if _, err := payments.NewRepository(pool).TransitionState(ctx, attempt.ID, payments.AttemptStatePending, payments.AttemptStateCancelled); err != nil {
			t.Fatal(err)
		}
		releaseOnce.Do(func() { close(release) })
		if err := <-result; err != nil {
			t.Fatalf("terminal inquiry race: %v", err)
		}
		stored, err := payments.NewRepository(pool).GetAttemptByID(ctx, attempt.ID)
		if err != nil {
			t.Fatal(err)
		}
		if stored.State != payments.AttemptStateCancelled || stored.ProviderPaymentReqID != nil {
			t.Fatalf("terminal race mutated attempt: %#v", stored)
		}
		assertWorkerCommand(t, ctx, pool, command.ID, paymentoutbox.StateSucceeded, command.IdempotencyKey)
	})

	t.Run("inquiry identity mismatch after terminal transition", func(t *testing.T) {
		ctx, pool := openWorkerTestDB(t)
		attempt, outbox, processor, adapter := newWorkerProcessorFixture(t, ctx, pool, payments.PaymentStatusPending)
		sessionID := "session-terminal-identity-race-0001"
		winnerRequestID := "request-terminal-identity-winner-0001"
		bindCreateResult(t, ctx, pool, attempt, sessionID)
		command := enqueueWorkerCommand(t, ctx, pool, outbox, attempt, paymentoutbox.CommandPaymentInquiry)
		claimed := claimWorkerCommand(t, ctx, outbox, paymentoutbox.CommandPaymentInquiry)
		started, release := make(chan struct{}), make(chan struct{})
		var releaseOnce sync.Once
		defer releaseOnce.Do(func() { close(release) })
		adapter.getPaymentStatus = func(context.Context, payments.GetPaymentStatusRequest) (payments.PaymentStatusResponse, error) {
			close(started)
			<-release
			return payments.PaymentStatusResponse{
				Scope:                payments.PaymentInquiryScopeCheckoutSession,
				ProviderSessionID:    sessionID,
				ProviderPaymentReqID: "request-terminal-identity-loser-0001",
				Status:               payments.PaymentStatusPending,
			}, nil
		}
		result := make(chan error, 1)
		go func() { result <- processor.Process(ctx, claimed) }()
		<-started
		bindInquiryRequest(t, ctx, pool, attempt, sessionID, winnerRequestID, "")
		if _, err := payments.NewRepository(pool).TransitionState(ctx, attempt.ID, payments.AttemptStatePending, payments.AttemptStateCancelled); err != nil {
			t.Fatal(err)
		}
		releaseOnce.Do(func() { close(release) })
		if err := <-result; err != nil {
			t.Fatalf("terminal inquiry identity mismatch: %v", err)
		}
		stored, err := payments.NewRepository(pool).GetAttemptByID(ctx, attempt.ID)
		if err != nil {
			t.Fatal(err)
		}
		if stored.State != payments.AttemptStateCancelled ||
			stored.ProviderPaymentReqID == nil || *stored.ProviderPaymentReqID != winnerRequestID {
			t.Fatalf("terminal inquiry mismatch mutated attempt: %#v", stored)
		}
		assertWorkerCommand(t, ctx, pool, command.ID, paymentoutbox.StateTerminal, command.IdempotencyKey)
		var auditCount int
		if err := pool.QueryRow(ctx, `SELECT count(*) FROM platform_audit_logs WHERE entity_id = $1 AND action = $2`, attempt.ID, audit.ActionReconciliationException).Scan(&auditCount); err != nil {
			t.Fatal(err)
		}
		if auditCount != 1 {
			t.Fatalf("reconciliation audit count = %d; want 1", auditCount)
		}
	})

	t.Run("create response after terminal transition", func(t *testing.T) {
		ctx, pool := openWorkerTestDB(t)
		attempt, outbox, processor, adapter := newWorkerProcessorFixture(t, ctx, pool, payments.PaymentStatusPending)
		command := enqueueWorkerCommand(t, ctx, pool, outbox, attempt, paymentoutbox.CommandPaymentCreate)
		claimed := claimWorkerCommand(t, ctx, outbox, paymentoutbox.CommandPaymentCreate)
		sessionID := "session-create-terminal-race-0001"
		started, release := make(chan struct{}), make(chan struct{})
		var releaseOnce sync.Once
		defer releaseOnce.Do(func() { close(release) })
		adapter.createPayment = func(_ context.Context, req payments.CreatePaymentRequest) (payments.CreatePaymentResponse, error) {
			close(started)
			<-release
			return payments.CreatePaymentResponse{ProviderSessionID: sessionID, Status: payments.PaymentStatusPending, AmountRupiah: req.AmountRupiah, Currency: req.Currency}, nil
		}
		result := make(chan error, 1)
		go func() { result <- processor.Process(ctx, claimed) }()
		<-started
		bindCreateResult(t, ctx, pool, attempt, sessionID)
		if _, err := payments.NewRepository(pool).TransitionState(ctx, attempt.ID, payments.AttemptStatePending, payments.AttemptStateCancelled); err != nil {
			t.Fatal(err)
		}
		releaseOnce.Do(func() { close(release) })
		if err := <-result; err != nil {
			t.Fatalf("terminal create race: %v", err)
		}
		assertWorkerCommand(t, ctx, pool, command.ID, paymentoutbox.StateSucceeded, command.IdempotencyKey)
		assertPaymentFlowCounts(t, ctx, pool, attempt, 1, 0)
	})

	t.Run("create response identity mismatch after terminal transition", func(t *testing.T) {
		ctx, pool := openWorkerTestDB(t)
		attempt, outbox, processor, adapter := newWorkerProcessorFixture(t, ctx, pool, payments.PaymentStatusPending)
		command := enqueueWorkerCommand(t, ctx, pool, outbox, attempt, paymentoutbox.CommandPaymentCreate)
		claimed := claimWorkerCommand(t, ctx, outbox, paymentoutbox.CommandPaymentCreate)
		winnerSessionID := "session-create-race-winner-0001"
		started, release := make(chan struct{}), make(chan struct{})
		var releaseOnce sync.Once
		defer releaseOnce.Do(func() { close(release) })
		adapter.createPayment = func(_ context.Context, req payments.CreatePaymentRequest) (payments.CreatePaymentResponse, error) {
			close(started)
			<-release
			return payments.CreatePaymentResponse{ProviderSessionID: "session-create-race-loser-0001", Status: payments.PaymentStatusPending, AmountRupiah: req.AmountRupiah, Currency: req.Currency}, nil
		}
		result := make(chan error, 1)
		go func() { result <- processor.Process(ctx, claimed) }()
		<-started
		bindCreateResult(t, ctx, pool, attempt, winnerSessionID)
		if _, err := payments.NewRepository(pool).TransitionState(ctx, attempt.ID, payments.AttemptStatePending, payments.AttemptStateCancelled); err != nil {
			t.Fatal(err)
		}
		releaseOnce.Do(func() { close(release) })
		if err := <-result; err != nil {
			t.Fatalf("terminal create identity mismatch: %v", err)
		}
		stored, err := payments.NewRepository(pool).GetAttemptByID(ctx, attempt.ID)
		if err != nil {
			t.Fatal(err)
		}
		if stored.State != payments.AttemptStateCancelled || stored.ProviderSessionID == nil || *stored.ProviderSessionID != winnerSessionID {
			t.Fatalf("terminal mismatch mutated attempt: %#v", stored)
		}
		assertWorkerCommand(t, ctx, pool, command.ID, paymentoutbox.StateTerminal, command.IdempotencyKey)
		var auditCount int
		if err := pool.QueryRow(ctx, `SELECT count(*) FROM platform_audit_logs WHERE entity_id = $1 AND action = $2`, attempt.ID, audit.ActionReconciliationException).Scan(&auditCount); err != nil {
			t.Fatal(err)
		}
		if auditCount != 1 {
			t.Fatalf("reconciliation audit count = %d; want 1", auditCount)
		}
		assertPaymentFlowCounts(t, ctx, pool, attempt, 1, 0)
	})
}

func TestProcessorTerminalRaceFinalizesRetryOutcomesAsNoop(t *testing.T) {
	inquiryCases := []struct {
		name     string
		response func(string) (payments.PaymentStatusResponse, error)
	}{
		{
			name: "adapter timeout",
			response: func(string) (payments.PaymentStatusResponse, error) {
				return payments.PaymentStatusResponse{}, payments.ErrRetryableTimeout
			},
		},
		{
			name: "pending response",
			response: func(sessionID string) (payments.PaymentStatusResponse, error) {
				return payments.PaymentStatusResponse{
					Scope:             payments.PaymentInquiryScopeCheckoutSession,
					ProviderSessionID: sessionID,
					Status:            payments.PaymentStatusPending,
				}, nil
			},
		},
		{
			name: "first malformed response",
			response: func(sessionID string) (payments.PaymentStatusResponse, error) {
				return payments.PaymentStatusResponse{
					Scope:             payments.PaymentInquiryScopeCheckoutSession,
					ProviderSessionID: sessionID,
					Status:            payments.PaymentStatusFailed,
				}, nil
			},
		},
	}
	for _, tc := range inquiryCases {
		t.Run("inquiry "+tc.name, func(t *testing.T) {
			ctx, pool := openWorkerTestDB(t)
			attempt, outbox, processor, adapter := newWorkerProcessorFixture(t, ctx, pool, payments.PaymentStatusPending)
			sessionID := "session-retry-race-" + strings.ReplaceAll(tc.name, " ", "-") + "-0001"
			bindCreateResult(t, ctx, pool, attempt, sessionID)
			command := enqueueWorkerCommand(t, ctx, pool, outbox, attempt, paymentoutbox.CommandPaymentInquiry)
			claimed := claimWorkerCommand(t, ctx, outbox, paymentoutbox.CommandPaymentInquiry)
			started, release := make(chan struct{}), make(chan struct{})
			var releaseOnce sync.Once
			defer releaseOnce.Do(func() { close(release) })
			adapter.getPaymentStatus = func(context.Context, payments.GetPaymentStatusRequest) (payments.PaymentStatusResponse, error) {
				close(started)
				<-release
				return tc.response(sessionID)
			}
			result := make(chan error, 1)
			go func() { result <- processor.Process(ctx, claimed) }()
			<-started
			if _, err := payments.NewRepository(pool).TransitionState(ctx, attempt.ID, payments.AttemptStatePending, payments.AttemptStateCancelled); err != nil {
				t.Fatal(err)
			}
			releaseOnce.Do(func() { close(release) })
			if err := <-result; err != nil {
				t.Fatalf("terminal retry race: %v", err)
			}
			assertWorkerCommand(t, ctx, pool, command.ID, paymentoutbox.StateSucceeded, command.IdempotencyKey)
			var malformedCount int
			if err := pool.QueryRow(ctx, `SELECT malformed_response_count FROM payment_provider_commands WHERE id = $1`, command.ID).Scan(&malformedCount); err != nil {
				t.Fatal(err)
			}
			if malformedCount != 0 {
				t.Fatalf("malformed response count = %d; want 0 after terminal no-op", malformedCount)
			}
		})
	}

	t.Run("create timeout", func(t *testing.T) {
		ctx, pool := openWorkerTestDB(t)
		attempt, outbox, processor, adapter := newWorkerProcessorFixture(t, ctx, pool, payments.PaymentStatusPending)
		command := enqueueWorkerCommand(t, ctx, pool, outbox, attempt, paymentoutbox.CommandPaymentCreate)
		claimed := claimWorkerCommand(t, ctx, outbox, paymentoutbox.CommandPaymentCreate)
		sessionID := "session-create-timeout-terminal-race-0001"
		started, release := make(chan struct{}), make(chan struct{})
		var releaseOnce sync.Once
		defer releaseOnce.Do(func() { close(release) })
		adapter.createPayment = func(context.Context, payments.CreatePaymentRequest) (payments.CreatePaymentResponse, error) {
			close(started)
			<-release
			return payments.CreatePaymentResponse{}, payments.ErrRetryableTimeout
		}
		result := make(chan error, 1)
		go func() { result <- processor.Process(ctx, claimed) }()
		<-started
		bindCreateResult(t, ctx, pool, attempt, sessionID)
		if _, err := payments.NewRepository(pool).TransitionState(ctx, attempt.ID, payments.AttemptStatePending, payments.AttemptStateCancelled); err != nil {
			t.Fatal(err)
		}
		releaseOnce.Do(func() { close(release) })
		if err := <-result; err != nil {
			t.Fatalf("terminal create timeout race: %v", err)
		}
		assertWorkerCommand(t, ctx, pool, command.ID, paymentoutbox.StateSucceeded, command.IdempotencyKey)
	})
}

func TestProcessorReclaimsExpiredTerminalLeaseWithoutProviderCall(t *testing.T) {
	for _, commandType := range []paymentoutbox.CommandType{
		paymentoutbox.CommandPaymentCreate,
		paymentoutbox.CommandPaymentInquiry,
	} {
		t.Run(string(commandType), func(t *testing.T) {
			ctx, pool := openWorkerTestDB(t)
			attempt, outbox, processor, adapter := newWorkerProcessorFixture(t, ctx, pool, payments.PaymentStatusPending)
			commandLabel := "create"
			if commandType == paymentoutbox.CommandPaymentInquiry {
				commandLabel = "inquiry"
			}
			sessionID := "session-terminal-reclaim-" + commandLabel + "-0001"
			if commandType == paymentoutbox.CommandPaymentInquiry {
				bindCreateResult(t, ctx, pool, attempt, sessionID)
			}
			command := enqueueWorkerCommand(t, ctx, pool, outbox, attempt, commandType)
			original, err := outbox.ClaimNextForTypes(
				ctx,
				"worker:"+uuid.NewString(),
				20*time.Millisecond,
				[]paymentoutbox.CommandType{commandType},
			)
			if err != nil {
				t.Fatal(err)
			}
			if commandType == paymentoutbox.CommandPaymentCreate {
				bindCreateResult(t, ctx, pool, attempt, sessionID)
			}
			if _, err := payments.NewRepository(pool).TransitionState(
				ctx,
				attempt.ID,
				payments.AttemptStatePending,
				payments.AttemptStateCancelled,
			); err != nil {
				t.Fatal(err)
			}
			waitForWorkerLeaseExpiry(t, ctx, pool, command.ID)

			reclaimed, err := outbox.ClaimNextForTypes(
				ctx,
				"worker:"+uuid.NewString(),
				30*time.Second,
				[]paymentoutbox.CommandType{commandType},
			)
			if err != nil {
				t.Fatalf("reclaim terminal attempt command: %v", err)
			}
			if reclaimed.ID != command.ID || reclaimed.LeaseToken == nil ||
				original.LeaseToken == nil || *reclaimed.LeaseToken == *original.LeaseToken {
				t.Fatalf("terminal reclaim did not CAS/rotate lease: original=%#v reclaimed=%#v", original, reclaimed)
			}
			if _, err := outbox.ClaimNextForTypes(
				ctx,
				"worker:"+uuid.NewString(),
				30*time.Second,
				[]paymentoutbox.CommandType{commandType},
			); !errors.Is(err, paymentoutbox.ErrNoCommandAvailable) {
				t.Fatalf("same terminal command claimed twice: %v", err)
			}

			adapter.createPayment = func(context.Context, payments.CreatePaymentRequest) (payments.CreatePaymentResponse, error) {
				t.Fatal("terminal create reclaim called provider")
				return payments.CreatePaymentResponse{}, nil
			}
			adapter.getPaymentStatus = func(context.Context, payments.GetPaymentStatusRequest) (payments.PaymentStatusResponse, error) {
				t.Fatal("terminal inquiry reclaim called provider")
				return payments.PaymentStatusResponse{}, nil
			}
			if err := processor.Process(ctx, reclaimed); err != nil {
				t.Fatalf("terminal local finalizer: %v", err)
			}
			assertWorkerCommand(t, ctx, pool, command.ID, paymentoutbox.StateSucceeded, command.IdempotencyKey)
		})
	}
}

func TestProcessorCreateWithKnownIdentityUsesLocalRecoveryWithoutProviderCall(t *testing.T) {
	for _, tc := range []struct {
		name        string
		legacyState bool
	}{
		{name: "pending identity"},
		{name: "legacy created identity", legacyState: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx, pool := openWorkerTestDB(t)
			attempt, outbox, processor, adapter := newWorkerProcessorFixture(t, ctx, pool, payments.PaymentStatusPending)
			command := enqueueWorkerCommand(t, ctx, pool, outbox, attempt, paymentoutbox.CommandPaymentCreate)
			sessionID := "session-known-before-create-retry-0001"
			if tc.legacyState {
				if _, err := pool.Exec(ctx, `
					UPDATE payment_attempts
					SET provider_session_id = $2
					WHERE id = $1
				`, attempt.ID, sessionID); err != nil {
					t.Fatal(err)
				}
			} else {
				bindCreateResult(t, ctx, pool, attempt, sessionID)
			}
			claimed := claimWorkerCommand(t, ctx, outbox, paymentoutbox.CommandPaymentCreate)

			var providerCalls int
			adapter.createPayment = func(context.Context, payments.CreatePaymentRequest) (payments.CreatePaymentResponse, error) {
				providerCalls++
				return payments.CreatePaymentResponse{}, nil
			}
			if err := processor.Process(ctx, claimed); err != nil {
				t.Fatalf("local create recovery: %v", err)
			}
			if providerCalls != 0 {
				t.Fatalf("known provider identity triggered %d create call(s); want 0", providerCalls)
			}
			stored, err := payments.NewRepository(pool).GetAttemptByID(ctx, attempt.ID)
			if err != nil {
				t.Fatal(err)
			}
			if stored.State != payments.AttemptStatePending {
				t.Fatalf("local recovery attempt state = %q; want PENDING", stored.State)
			}
			assertWorkerCommand(t, ctx, pool, command.ID, paymentoutbox.StateSucceeded, command.IdempotencyKey)
			assertPaymentFlowCounts(t, ctx, pool, attempt, 1, 1)
			assertNoProductionFinanceWrites(t, ctx, pool, attempt.BookingID)
		})
	}
}

func TestProcessorKnownIdentityRecoveryRollsBackAtomicallyOnAuditFailure(t *testing.T) {
	ctx, pool := openWorkerTestDB(t)
	attempt, outbox, processor, adapter := newWorkerProcessorFixture(t, ctx, pool, payments.PaymentStatusPending)
	command := enqueueWorkerCommand(t, ctx, pool, outbox, attempt, paymentoutbox.CommandPaymentCreate)
	bindCreateResult(t, ctx, pool, attempt, "session-local-recovery-rollback-0001")
	claimed, err := outbox.ClaimNextForTypes(
		ctx,
		"worker:"+uuid.NewString(),
		100*time.Millisecond,
		[]paymentoutbox.CommandType{paymentoutbox.CommandPaymentCreate},
	)
	if err != nil {
		t.Fatal(err)
	}
	adapter.createPayment = func(context.Context, payments.CreatePaymentRequest) (payments.CreatePaymentResponse, error) {
		t.Fatal("known identity recovery called provider")
		return payments.CreatePaymentResponse{}, nil
	}
	processor.audit = failingAuditService{}
	if err := processor.Process(ctx, claimed); err == nil {
		t.Fatal("local recovery audit failure was swallowed")
	}
	assertWorkerCommand(t, ctx, pool, command.ID, paymentoutbox.StateLeased, command.IdempotencyKey)
	assertPaymentFlowCounts(t, ctx, pool, attempt, 1, 0)

	waitForWorkerLeaseExpiry(t, ctx, pool, command.ID)
	processor.audit = audit.NewPlatformService(audit.NewPlatformRepository())
	reclaimed := claimWorkerCommand(t, ctx, outbox, paymentoutbox.CommandPaymentCreate)
	if reclaimed.ID != command.ID {
		t.Fatalf("local recovery reclaimed command = %s; want %s", reclaimed.ID, command.ID)
	}
	if err := processor.Process(ctx, reclaimed); err != nil {
		t.Fatalf("local recovery retry: %v", err)
	}
	assertWorkerCommand(t, ctx, pool, command.ID, paymentoutbox.StateSucceeded, command.IdempotencyKey)
	assertPaymentFlowCounts(t, ctx, pool, attempt, 1, 1)
	assertNoProductionFinanceWrites(t, ctx, pool, attempt.BookingID)
}

func TestProcessorInquiryWithInvalidStoredIdentityNeverCallsProvider(t *testing.T) {
	for _, tc := range []struct {
		name      string
		sessionID *string
		requestID *string
		paymentID *string
	}{
		{
			name:      "payment ID without payment request",
			paymentID: strptr("payment-orphan-identity-0001"),
		},
		{
			name:      "credential-like session identity",
			sessionID: strptr("secret-demo-provider-value"),
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx, pool := openWorkerTestDB(t)
			attempt, outbox, processor, adapter := newWorkerProcessorFixture(t, ctx, pool, payments.PaymentStatusPending)
			if _, err := pool.Exec(ctx, `
				UPDATE payment_attempts
				SET state = 'PENDING',
				    provider_session_id = $2,
				    provider_payment_request_id = $3,
				    provider_payment_id = $4
				WHERE id = $1
			`, attempt.ID, tc.sessionID, tc.requestID, tc.paymentID); err != nil {
				t.Fatal(err)
			}
			command := enqueueWorkerCommand(t, ctx, pool, outbox, attempt, paymentoutbox.CommandPaymentInquiry)
			claimed := claimWorkerCommand(t, ctx, outbox, paymentoutbox.CommandPaymentInquiry)

			var providerCalls int
			adapter.getPaymentStatus = func(context.Context, payments.GetPaymentStatusRequest) (payments.PaymentStatusResponse, error) {
				providerCalls++
				return payments.PaymentStatusResponse{Status: payments.PaymentStatusPending}, nil
			}
			if err := processor.Process(ctx, claimed); err != nil {
				t.Fatalf("invalid identity local finalizer: %v", err)
			}
			if providerCalls != 0 {
				t.Fatalf("invalid stored identity triggered %d inquiry call(s); want 0", providerCalls)
			}
			assertWorkerCommand(t, ctx, pool, command.ID, paymentoutbox.StateTerminal, command.IdempotencyKey)
			var auditCount int
			if err := pool.QueryRow(ctx, `
				SELECT count(*)
				FROM platform_audit_logs
				WHERE entity_id = $1
				  AND action = $2
				  AND metadata->>'reason' = 'PROVIDER_CONTRACT_BLOCKED'
			`, attempt.ID, audit.ActionReconciliationException).Scan(&auditCount); err != nil {
				t.Fatal(err)
			}
			if auditCount != 1 {
				t.Fatalf("provider contract audit count = %d; want 1", auditCount)
			}
			assertNoProductionFinanceWrites(t, ctx, pool, attempt.BookingID)
		})
	}
}

func TestProcessorExpiredLeaseNeverCallsProvider(t *testing.T) {
	for _, commandType := range []paymentoutbox.CommandType{
		paymentoutbox.CommandPaymentCreate,
		paymentoutbox.CommandPaymentInquiry,
	} {
		t.Run(string(commandType), func(t *testing.T) {
			ctx, pool := openWorkerTestDB(t)
			attempt, outbox, processor, adapter := newWorkerProcessorFixture(t, ctx, pool, payments.PaymentStatusPending)
			if commandType == paymentoutbox.CommandPaymentInquiry {
				bindCreateResult(t, ctx, pool, attempt, "session-expired-provider-call-0001")
			}
			command := enqueueWorkerCommand(t, ctx, pool, outbox, attempt, commandType)
			claimed, err := outbox.ClaimNextForTypes(
				ctx,
				"worker:"+uuid.NewString(),
				20*time.Millisecond,
				[]paymentoutbox.CommandType{commandType},
			)
			if err != nil {
				t.Fatal(err)
			}
			waitForWorkerLeaseExpiry(t, ctx, pool, command.ID)

			var providerCalls int
			adapter.createPayment = func(context.Context, payments.CreatePaymentRequest) (payments.CreatePaymentResponse, error) {
				providerCalls++
				return payments.CreatePaymentResponse{}, nil
			}
			adapter.getPaymentStatus = func(context.Context, payments.GetPaymentStatusRequest) (payments.PaymentStatusResponse, error) {
				providerCalls++
				return payments.PaymentStatusResponse{}, nil
			}
			if err := processor.Process(ctx, claimed); !errors.Is(err, paymentoutbox.ErrLeaseConflict) {
				t.Fatalf("expired lease process error = %v; want ErrLeaseConflict", err)
			}
			if providerCalls != 0 {
				t.Fatalf("expired lease triggered %d provider call(s); want 0", providerCalls)
			}
		})
	}
}

func TestProcessorPreservesLeaseOnTransientRepositoryReadFailure(t *testing.T) {
	for _, tc := range []struct {
		name         string
		commandType  paymentoutbox.CommandType
		failAttempt  bool
		failContract bool
	}{
		{name: "create attempt read", commandType: paymentoutbox.CommandPaymentCreate, failAttempt: true},
		{name: "create contract read", commandType: paymentoutbox.CommandPaymentCreate, failContract: true},
		{name: "inquiry attempt read", commandType: paymentoutbox.CommandPaymentInquiry, failAttempt: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx, pool := openWorkerTestDB(t)
			attempt, outbox, processor, adapter := newWorkerProcessorFixture(t, ctx, pool, payments.PaymentStatusPending)
			if tc.commandType == paymentoutbox.CommandPaymentInquiry {
				bindCreateResult(t, ctx, pool, attempt, "session-transient-read-0001")
			}
			command := enqueueWorkerCommand(t, ctx, pool, outbox, attempt, tc.commandType)
			claimed := claimWorkerCommand(t, ctx, outbox, tc.commandType)
			transientErr := errors.New("temporary repository read failure")
			faults := &faultingPaymentAttemptRepository{Repository: payments.NewRepository(pool)}
			if tc.failAttempt {
				faults.getAttemptErr = transientErr
			}
			if tc.failContract {
				faults.getContractErr = transientErr
			}
			processor.attempts = faults
			adapter.createPayment = func(context.Context, payments.CreatePaymentRequest) (payments.CreatePaymentResponse, error) {
				t.Fatal("provider create must not be called after repository read failure")
				return payments.CreatePaymentResponse{}, nil
			}
			adapter.getPaymentStatus = func(context.Context, payments.GetPaymentStatusRequest) (payments.PaymentStatusResponse, error) {
				t.Fatal("provider inquiry must not be called after repository read failure")
				return payments.PaymentStatusResponse{}, nil
			}

			if err := processor.Process(ctx, claimed); !errors.Is(err, transientErr) {
				t.Fatalf("processor error = %v; want transient repository error", err)
			}
			assertWorkerCommand(t, ctx, pool, command.ID, paymentoutbox.StateLeased, command.IdempotencyKey)
		})
	}
}

func TestProcessorTerminalNoopFallsBackFromOutboxIncompatibleIdentity(t *testing.T) {
	ctx, pool := openWorkerTestDB(t)
	attempt, outbox, processor, adapter := newWorkerProcessorFixture(t, ctx, pool, payments.PaymentStatusPending)
	sessionID := "session-terminal-fallback-0001"
	requestID := "opaque/id"
	bindCreateResult(t, ctx, pool, attempt, sessionID)
	// Simulate a legacy value that predates the repository identity boundary.
	// Current repository methods must reject it; terminal local finalization
	// still needs to complete without calling the provider.
	if _, err := pool.Exec(ctx, `
		UPDATE payment_attempts
		SET provider_payment_request_id = $2
		WHERE id = $1
	`, attempt.ID, requestID); err != nil {
		t.Fatal(err)
	}
	command := enqueueWorkerCommand(t, ctx, pool, outbox, attempt, paymentoutbox.CommandPaymentInquiry)
	if _, err := outbox.ClaimNextForTypes(
		ctx,
		"worker:"+uuid.NewString(),
		20*time.Millisecond,
		[]paymentoutbox.CommandType{paymentoutbox.CommandPaymentInquiry},
	); err != nil {
		t.Fatal(err)
	}
	if _, err := payments.NewRepository(pool).TransitionState(
		ctx,
		attempt.ID,
		payments.AttemptStatePending,
		payments.AttemptStateCancelled,
	); err != nil {
		t.Fatal(err)
	}
	waitForWorkerLeaseExpiry(t, ctx, pool, command.ID)
	reclaimed := claimWorkerCommand(t, ctx, outbox, paymentoutbox.CommandPaymentInquiry)
	adapter.getPaymentStatus = func(context.Context, payments.GetPaymentStatusRequest) (payments.PaymentStatusResponse, error) {
		t.Fatal("terminal no-op fallback called provider")
		return payments.PaymentStatusResponse{}, nil
	}

	if err := processor.Process(ctx, reclaimed); err != nil {
		t.Fatalf("terminal no-op fallback: %v", err)
	}
	assertWorkerCommand(t, ctx, pool, command.ID, paymentoutbox.StateSucceeded, command.IdempotencyKey)
	wantReference, err := paymentoutbox.DigestProviderReference("local:" + attempt.ID)
	if err != nil {
		t.Fatal(err)
	}
	var providerReference *string
	if err := pool.QueryRow(ctx, `SELECT provider_reference FROM payment_provider_commands WHERE id = $1`, command.ID).Scan(&providerReference); err != nil {
		t.Fatal(err)
	}
	if providerReference == nil || *providerReference != wantReference {
		t.Fatalf("provider reference = %v; want local fallback %q", providerReference, wantReference)
	}
	var auditCount int
	if err := pool.QueryRow(ctx, `
		SELECT count(*) FROM platform_audit_logs
		WHERE entity_id = $1 AND action = $2 AND metadata->>'reason' = 'PROVIDER_CONTRACT_BLOCKED'
	`, attempt.ID, audit.ActionReconciliationException).Scan(&auditCount); err != nil {
		t.Fatal(err)
	}
	if auditCount != 1 {
		t.Fatalf("provider identity fallback audit count = %d; want 1", auditCount)
	}
}

func TestProcessorFinalizesMissingAttemptInvariantWithoutAttemptRead(t *testing.T) {
	for _, commandType := range []paymentoutbox.CommandType{
		paymentoutbox.CommandPaymentCreate,
		paymentoutbox.CommandPaymentInquiry,
	} {
		t.Run(string(commandType), func(t *testing.T) {
			ctx, pool := openWorkerTestDB(t)
			attempt, outbox, processor, adapter := newWorkerProcessorFixture(t, ctx, pool, payments.PaymentStatusPending)
			if commandType == paymentoutbox.CommandPaymentInquiry {
				bindCreateResult(t, ctx, pool, attempt, "session-missing-attempt-0001")
			}
			command := enqueueWorkerCommand(t, ctx, pool, outbox, attempt, commandType)
			claimed := claimWorkerCommand(t, ctx, outbox, commandType)
			processor.attempts = &faultingPaymentAttemptRepository{
				Repository:      payments.NewRepository(pool),
				getAttemptErr:   payments.ErrAttemptNotFound,
				getAttemptTxErr: payments.ErrAttemptNotFound,
			}
			adapter.createPayment = func(context.Context, payments.CreatePaymentRequest) (payments.CreatePaymentResponse, error) {
				t.Fatal("missing attempt create called provider")
				return payments.CreatePaymentResponse{}, nil
			}
			adapter.getPaymentStatus = func(context.Context, payments.GetPaymentStatusRequest) (payments.PaymentStatusResponse, error) {
				t.Fatal("missing attempt inquiry called provider")
				return payments.PaymentStatusResponse{}, nil
			}

			if err := processor.Process(ctx, claimed); err != nil {
				t.Fatalf("missing attempt invariant finalizer: %v", err)
			}
			assertWorkerCommand(t, ctx, pool, command.ID, paymentoutbox.StateTerminal, command.IdempotencyKey)
			var lastErrorCode *string
			var auditCount int
			if err := pool.QueryRow(ctx, `SELECT last_error_code FROM payment_provider_commands WHERE id = $1`, command.ID).Scan(&lastErrorCode); err != nil {
				t.Fatal(err)
			}
			if lastErrorCode == nil || *lastErrorCode != "INVALID_REQUEST" {
				t.Fatalf("missing attempt error code = %v; want INVALID_REQUEST", lastErrorCode)
			}
			if err := pool.QueryRow(ctx, `
				SELECT count(*) FROM platform_audit_logs
				WHERE entity_id = $1 AND action = 'PAYMENT_COMMAND_INVARIANT_VIOLATION'
			`, attempt.ID).Scan(&auditCount); err != nil {
				t.Fatal(err)
			}
			if auditCount != 1 {
				t.Fatalf("missing attempt invariant audit count = %d; want 1", auditCount)
			}
		})
	}
}

type faultingPaymentAttemptRepository struct {
	*payments.Repository
	getAttemptErr   error
	getContractErr  error
	getAttemptTxErr error
}

func (r *faultingPaymentAttemptRepository) GetAttemptByID(ctx context.Context, id string) (payments.PaymentAttempt, error) {
	if r.getAttemptErr != nil {
		return payments.PaymentAttempt{}, r.getAttemptErr
	}
	return r.Repository.GetAttemptByID(ctx, id)
}

func (r *faultingPaymentAttemptRepository) GetCreateContractByAttemptID(ctx context.Context, attemptID string) (payments.PaymentCreateContract, error) {
	if r.getContractErr != nil {
		return payments.PaymentCreateContract{}, r.getContractErr
	}
	return r.Repository.GetCreateContractByAttemptID(ctx, attemptID)
}

func (r *faultingPaymentAttemptRepository) GetAttemptTx(ctx context.Context, tx pgx.Tx, id string) (payments.PaymentAttempt, error) {
	if r.getAttemptTxErr != nil {
		return payments.PaymentAttempt{}, r.getAttemptTxErr
	}
	return r.Repository.GetAttemptTx(ctx, tx, id)
}

func terminalPaymentResponse(attempt payments.PaymentAttempt, requestID string, status payments.PaymentStatus) payments.PaymentStatusResponse {
	return payments.PaymentStatusResponse{
		Scope:                payments.PaymentInquiryScopePayment,
		ProviderPaymentReqID: requestID,
		ProviderPaymentID:    "payment-" + strings.ToLower(string(status)) + "-0001",
		StatusCode:           string(status),
		Status:               status,
		AmountRupiah:         attempt.AmountRupiah,
		Currency:             attempt.Currency,
	}
}

func bindInquiryRequest(t *testing.T, ctx context.Context, pool *pgxpool.Pool, attempt payments.PaymentAttempt, sessionID, requestID, paymentID string) {
	t.Helper()
	repo := payments.NewRepository(pool)
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)
	params := payments.ApplyInquiryIdentityParams{
		AttemptID:            attempt.ID,
		Provider:             attempt.Provider,
		ProviderEnvironment:  attempt.ProviderEnvironment,
		Scope:                payments.PaymentInquiryScopeCheckoutSession,
		ProviderSessionID:    &sessionID,
		ProviderPaymentReqID: &requestID,
		ProviderStatusCode:   "PENDING",
	}
	if paymentID != "" {
		params.Scope = payments.PaymentInquiryScopePayment
		params.ProviderPaymentID = &paymentID
	}
	if _, _, err := repo.ApplyInquiryIdentityTx(ctx, tx, params); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
}

func claimWorkerCommand(t *testing.T, ctx context.Context, outbox *paymentoutbox.Repository, commandType paymentoutbox.CommandType) paymentoutbox.Command {
	t.Helper()
	command, err := outbox.ClaimNextForTypes(ctx, "worker:"+uuid.NewString(), 30*time.Second, []paymentoutbox.CommandType{commandType})
	if err != nil {
		t.Fatal(err)
	}
	return command
}

func waitForWorkerCommandAvailability(t *testing.T, ctx context.Context, pool *pgxpool.Pool, commandID string) {
	t.Helper()
	var availableAt time.Time
	if err := pool.QueryRow(ctx, `SELECT available_at FROM payment_provider_commands WHERE id = $1`, commandID).Scan(&availableAt); err != nil {
		t.Fatal(err)
	}
	if delay := time.Until(availableAt); delay > 0 {
		time.Sleep(delay + time.Millisecond)
	}
}

func waitForWorkerLeaseExpiry(t *testing.T, ctx context.Context, pool *pgxpool.Pool, commandID string) {
	t.Helper()
	var expiresAt time.Time
	if err := pool.QueryRow(ctx, `SELECT lease_expires_at FROM payment_provider_commands WHERE id = $1`, commandID).Scan(&expiresAt); err != nil {
		t.Fatal(err)
	}
	if delay := time.Until(expiresAt); delay > 0 {
		time.Sleep(delay + 10*time.Millisecond)
	}
}

func assertWorkerCommand(t *testing.T, ctx context.Context, pool *pgxpool.Pool, commandID string, wantState paymentoutbox.CommandState, wantKey string) {
	t.Helper()
	var state paymentoutbox.CommandState
	var key string
	if err := pool.QueryRow(ctx, `SELECT state, idempotency_key FROM payment_provider_commands WHERE id = $1`, commandID).Scan(&state, &key); err != nil {
		t.Fatal(err)
	}
	if state != wantState || key != wantKey {
		t.Fatalf("command state/key = %q/%q; want %q/%q", state, key, wantState, wantKey)
	}
}

func assertPaymentFlowCounts(t *testing.T, ctx context.Context, pool *pgxpool.Pool, attempt payments.PaymentAttempt, wantCreate, wantInquiry int) {
	t.Helper()
	var attempts, contracts, creates, inquiries, captures int
	if err := pool.QueryRow(ctx, `
		SELECT
			(SELECT count(*) FROM payment_attempts WHERE id = $1),
			(SELECT count(*) FROM payment_create_contracts WHERE payment_attempt_id = $1),
			(SELECT count(*) FROM payment_provider_commands WHERE payment_attempt_id = $1 AND command_type = 'PAYMENT_CREATE'),
			(SELECT count(*) FROM payment_provider_commands WHERE payment_attempt_id = $1 AND command_type = 'PAYMENT_INQUIRY'),
			(SELECT count(*) FROM payment_capture_facts WHERE payment_attempt_id = $1)
	`, attempt.ID).Scan(&attempts, &contracts, &creates, &inquiries, &captures); err != nil {
		t.Fatal(err)
	}
	if attempts != 1 || contracts != 1 || creates != wantCreate || inquiries != wantInquiry || captures > 1 {
		t.Fatalf("flow counts attempt=%d contract=%d create=%d inquiry=%d capture=%d", attempts, contracts, creates, inquiries, captures)
	}
}

func assertNoProductionFinanceWrites(t *testing.T, ctx context.Context, pool *pgxpool.Pool, bookingID string) {
	t.Helper()
	var ownerTransactions, journals, ledgerEntries int
	if err := pool.QueryRow(ctx, `
		SELECT
			(SELECT count(*) FROM owner_finance_transactions WHERE booking_id = $1),
			(SELECT count(*) FROM platform_journals WHERE booking_id = $1),
			(SELECT count(*) FROM platform_ledger_entries e JOIN platform_journals j ON j.id = e.journal_id WHERE j.booking_id = $1)
	`, bookingID).Scan(&ownerTransactions, &journals, &ledgerEntries); err != nil {
		t.Fatal(err)
	}
	if ownerTransactions != 0 || journals != 0 || ledgerEntries != 0 {
		t.Fatalf("production finance residue owner_transactions=%d journals=%d ledger_entries=%d", ownerTransactions, journals, ledgerEntries)
	}
}

type scriptedWorkerAdapter struct {
	createPayment    func(context.Context, payments.CreatePaymentRequest) (payments.CreatePaymentResponse, error)
	getPaymentStatus func(context.Context, payments.GetPaymentStatusRequest) (payments.PaymentStatusResponse, error)
}

func (a *scriptedWorkerAdapter) CreatePayment(ctx context.Context, req payments.CreatePaymentRequest) (payments.CreatePaymentResponse, error) {
	if a.createPayment != nil {
		return a.createPayment(ctx, req)
	}
	return payments.CreatePaymentResponse{ProviderSessionID: "session-created-0001", Status: payments.PaymentStatusPending, AmountRupiah: req.AmountRupiah, Currency: req.Currency}, nil
}
func (a *scriptedWorkerAdapter) GetPaymentStatus(ctx context.Context, req payments.GetPaymentStatusRequest) (payments.PaymentStatusResponse, error) {
	if a.getPaymentStatus != nil {
		return a.getPaymentStatus(ctx, req)
	}
	return payments.PaymentStatusResponse{Scope: payments.PaymentInquiryScopeCheckoutSession, ProviderSessionID: req.ProviderSessionID, Status: payments.PaymentStatusPending}, nil
}
func (a *scriptedWorkerAdapter) VerifyWebhook(context.Context, payments.VerifyWebhookRequest) (payments.WebhookVerification, error) {
	return payments.WebhookVerification{}, payments.ErrFakeAdapterUnscripted
}
func (a *scriptedWorkerAdapter) ParseWebhook(context.Context, payments.ParseWebhookRequest) (payments.WebhookEvent, error) {
	return payments.WebhookEvent{}, payments.ErrFakeAdapterUnscripted
}
func (a *scriptedWorkerAdapter) RequestRefund(context.Context, payments.RefundRequest) (payments.RefundResponse, error) {
	return payments.RefundResponse{}, payments.ErrFakeAdapterUnscripted
}
func (a *scriptedWorkerAdapter) GetRefundStatus(context.Context, payments.GetRefundStatusRequest) (payments.RefundStatusResponse, error) {
	return payments.RefundStatusResponse{}, payments.ErrFakeAdapterUnscripted
}

func newWorkerProcessorFixture(t *testing.T, ctx context.Context, pool *pgxpool.Pool, _ payments.PaymentStatus) (payments.PaymentAttempt, *paymentoutbox.Repository, *Processor, *scriptedWorkerAdapter) {
	t.Helper()
	bookingID := seedWorkerBooking(t, ctx, pool)
	repo := payments.NewRepository(pool)
	attempt, err := repo.CreateOrReplayAttempt(ctx, payments.CreateAttemptParams{
		BookingID: bookingID, Provider: payments.ProviderXendit, ProviderEnvironment: payments.ProviderEnvironmentTest,
		RequestedMethod: payments.RequestedMethodQRIS, IntegrationMode: payments.IntegrationModePaymentLink,
		CaptureMethod: payments.CaptureMethodAutomatic, LocalReference: "worker-test-" + uuid.NewString(), RequestHash: workerTestHash,
		ExpiresAt: time.Now().UTC().Add(time.Hour),
	})
	if err != nil {
		t.Fatal(err)
	}
	successReturnURL := "https://demo.test/payments/return/" + attempt.LocalReference + "/success"
	cancelReturnURL := "https://demo.test/payments/return/" + attempt.LocalReference + "/cancel"
	if _, err := pool.Exec(ctx, `
		INSERT INTO payment_create_contracts (
			payment_attempt_id,
			request_hash,
			requested_expires_at,
			success_return_url,
			cancel_return_url
		)
		VALUES ($1, $2, $3, $4, $5)
	`, attempt.ID, attempt.RequestHash, attempt.ExpiresAt, successReturnURL, cancelReturnURL); err != nil {
		t.Fatal(err)
	}
	adapter := &scriptedWorkerAdapter{}
	outbox := paymentoutbox.NewRepository(pool)
	processor, err := NewProcessor(pool, repo, outbox, adapter, ProcessorOptions{Audit: audit.NewPlatformService(audit.NewPlatformRepository()), AdapterTimeout: 2 * time.Second, RetryPolicy: RetryPolicy{InitialDelay: time.Millisecond, MaxDelay: time.Second}})
	if err != nil {
		t.Fatal(err)
	}
	return attempt, outbox, processor, adapter
}

func enqueueWorkerCommand(t *testing.T, ctx context.Context, pool *pgxpool.Pool, outbox *paymentoutbox.Repository, attempt payments.PaymentAttempt, commandType paymentoutbox.CommandType) paymentoutbox.Command {
	t.Helper()
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	result, err := outbox.EnqueueTx(ctx, tx, paymentoutbox.EnqueueParams{
		CommandType: commandType, AggregateType: paymentoutbox.AggregatePaymentAttempt, AggregateID: attempt.ID, PaymentAttemptID: attempt.ID,
		IdempotencyKey: func() string {
			if commandType == paymentoutbox.CommandPaymentCreate {
				return paymentoutbox.DeterministicCreateKey(attempt.BookingID, attempt.AttemptNo)
			}
			return paymentoutbox.DeterministicInquiryKey(attempt.ID)
		}(),
		RequestHash: attempt.RequestHash, Payload: paymentoutbox.PaymentCommandPayload{AttemptID: attempt.ID, AmountRupiah: attempt.AmountRupiah, Currency: string(attempt.Currency), RequestedMethod: string(attempt.RequestedMethod)},
	})
	if err != nil {
		_ = tx.Rollback(ctx)
		t.Fatal(err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	return result.Command
}

func bindCreateResult(t *testing.T, ctx context.Context, pool *pgxpool.Pool, attempt payments.PaymentAttempt, sessionID string) {
	t.Helper()
	repo := payments.NewRepository(pool)
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := repo.ApplyCreateProviderResultTx(ctx, tx, payments.ApplyCreateProviderResultParams{AttemptID: attempt.ID, Provider: attempt.Provider, ProviderEnvironment: attempt.ProviderEnvironment, ProviderSessionID: sessionID, ProviderStatusCode: "PENDING", Status: payments.PaymentStatusPending, AmountRupiah: attempt.AmountRupiah, Currency: attempt.Currency}); err != nil {
		_ = tx.Rollback(ctx)
		t.Fatal(err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
}

type failingAuditService struct{}

func (failingAuditService) Record(context.Context, audit.DBTX, audit.CreatePlatformAuditLogParams) error {
	return errors.New("audit failure")
}

func openWorkerTestDB(t *testing.T) (context.Context, *pgxpool.Pool) {
	t.Helper()
	if os.Getenv("TEST_ROLLBACK_HARDENING_DISPOSABLE") != "1" {
		t.Skip("disposable payment worker test is opt-in")
	}
	adminDSN := os.Getenv("ROLLBACK_HARDENING_TEST_DATABASE_URL")
	if adminDSN == "" {
		t.Fatal("ROLLBACK_HARDENING_TEST_DATABASE_URL is required")
	}
	parsed, err := url.Parse(adminDSN)
	if err != nil || parsed.Path == "" || parsed.Path == "/" {
		t.Fatal("invalid admin DSN")
	}
	dbName := "lapangango_worker_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	adminDB, err := sql.Open("postgres", adminDSN)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := adminDB.Exec("CREATE DATABASE " + dbName); err != nil {
		adminDB.Close()
		t.Fatal(err)
	}
	adminDB.Close()
	var pool *pgxpool.Pool
	t.Cleanup(func() {
		if pool != nil {
			pool.Close()
		}
		cleanup, err := sql.Open("postgres", adminDSN)
		if err == nil {
			defer cleanup.Close()
			_, _ = cleanup.Exec(`SELECT pg_terminate_backend(pid) FROM pg_stat_activity WHERE datname = $1`, dbName)
			_, _ = cleanup.Exec("DROP DATABASE " + dbName)
		}
	})
	parsed.Path = "/" + dbName
	targetDSN := parsed.String()
	targetDB, err := sql.Open("postgres", targetDSN)
	if err != nil {
		t.Fatal(err)
	}
	defer targetDB.Close()
	driver, err := postgres.WithInstance(targetDB, &postgres.Config{})
	if err != nil {
		t.Fatal(err)
	}
	m, err := migrate.NewWithDatabaseInstance("file://../../../../db/migrations", "postgres", driver)
	if err != nil {
		t.Fatal(err)
	}
	if err := m.Up(); err != nil && err != migrate.ErrNoChange {
		t.Fatal(err)
	}
	m.Close()
	pool, err = pgxpool.New(context.Background(), targetDSN)
	if err != nil {
		t.Fatal(err)
	}
	if err := pool.Ping(context.Background()); err != nil {
		pool.Close()
		t.Fatal(err)
	}
	return context.Background(), pool
}

func seedWorkerBooking(t *testing.T, ctx context.Context, pool *pgxpool.Pool) string {
	t.Helper()
	suffix := strings.ReplaceAll(uuid.NewString(), "-", "")
	customerID, ownerID, profileID, venueID, courtID, bookingID := uuid.NewString(), uuid.NewString(), uuid.NewString(), uuid.NewString(), uuid.NewString(), uuid.NewString()
	if _, err := pool.Exec(ctx, `INSERT INTO users (id,name,email,password_hash,role,status) VALUES ($1,'worker customer',$2,'hash','CUSTOMER','ACTIVE'),($3,'worker owner',$4,'hash','OWNER','ACTIVE')`, customerID, "customer-"+suffix+"@example.test", ownerID, "owner-"+suffix+"@example.test"); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO owner_profiles (id,user_id,business_name,verification_status) VALUES ($1,$2,'Worker Owner','APPROVED')`, profileID, ownerID); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO venues (id,owner_profile_id,name,address,city,status) VALUES ($1,$2,$3,'Test address','Jakarta','ACTIVE')`, venueID, profileID, "Worker Venue "+suffix); err != nil {
		t.Fatal(err)
	}
	var sportID string
	if err := pool.QueryRow(ctx, `SELECT id FROM sports WHERE name='Futsal' LIMIT 1`).Scan(&sportID); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO courts (id,venue_id,sport_id,name,location_type,price_per_hour,status) VALUES ($1,$2,$3,$4,'INDOOR',10000,'ACTIVE')`, courtID, venueID, sportID, "Worker Court "+suffix); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO bookings (id,customer_id,court_id,booking_date,start_time,end_time,total_price,status,expires_at) VALUES ($1,$2,$3,CURRENT_DATE+1,'10:00','11:00',10000,'PENDING_PAYMENT',transaction_timestamp()+interval '2 hours')`, bookingID, customerID, courtID); err != nil {
		t.Fatal(err)
	}
	var termID string
	if err := pool.QueryRow(ctx, `SELECT id FROM platform_commercial_terms WHERE owner_profile_id IS NULL LIMIT 1`).Scan(&termID); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO booking_fee_snapshots (booking_id,owner_profile_id,venue_id,commercial_term_id,terms_source,booking_channel,finance_mode,original_price_rupiah,owner_price_adjustment_rupiah,final_booking_price_rupiah,customer_charge_amount_rupiah,commission_basis_amount_rupiah,commission_bps,commission_amount_rupiah,owner_net_amount_rupiah,calculation_version) SELECT $1,$2,$3,$4,'POLICY','MARKETPLACE_ONLINE','SIMULATION',10000,0,10000,10000,10000,700,700,9300,'WORKER_TEST_V1'`, bookingID, profileID, venueID, termID); err != nil {
		t.Fatal(err)
	}
	return bookingID
}

var _ payments.PaymentAdapter = (*scriptedWorkerAdapter)(nil)
var _ audit.PlatformService = (*failingAuditService)(nil)
