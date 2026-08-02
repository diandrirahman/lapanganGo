package paymentworker

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"lapangango-api/internal/audit"
	"lapangango-api/internal/paymentoutbox"
	"lapangango-api/internal/payments"

	"github.com/jackc/pgx/v5/pgxpool"
)

func TestDecodePayloadRejectsUnknownOrTrailingFields(t *testing.T) {
	valid := paymentoutbox.PaymentCommandPayload{
		AttemptID:       "00000000-0000-4000-8000-000000000001",
		AmountRupiah:    10000,
		Currency:        "IDR",
		RequestedMethod: "QRIS",
	}
	raw, err := json.Marshal(valid)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := decodePayload(raw); err != nil {
		t.Fatalf("valid payload rejected: %v", err)
	}

	unknown := []byte(`{"attempt_id":"00000000-0000-4000-8000-000000000001","amount_rupiah":10000,"currency":"IDR","requested_method":"QRIS","forbidden_field":"must-not-exist"}`)
	if _, err := decodePayload(unknown); err == nil {
		t.Fatal("unknown field was accepted")
	}
	trailing := append(append([]byte(nil), raw...), []byte(` {}`)...)
	if _, err := decodePayload(trailing); err == nil {
		t.Fatal("trailing JSON value was accepted")
	}
	malformedTrailing := append(append([]byte(nil), raw...), []byte(` {`)...)
	if _, err := decodePayload(malformedTrailing); err == nil {
		t.Fatal("malformed trailing JSON was accepted")
	}
}

func TestValidateInquiryResponseRejectsMismatchesAndRequiresCaptureEvidence(t *testing.T) {
	attempt := payments.PaymentAttempt{
		ProviderSessionID:    strptr("session-test-0001"),
		ProviderPaymentReqID: strptr("payment-request-test-0001"),
		ProviderPaymentID:    strptr("payment-test-0001"),
		AmountRupiah:         10000,
		Currency:             payments.CurrencyIDR,
	}
	if reason := validateInquiryResponse(attempt, payments.PaymentStatusResponse{
		Scope:                payments.PaymentInquiryScopePayment,
		ProviderPaymentReqID: "payment-request-test-0001",
		ProviderPaymentID:    "payment-test-0001",
		Status:               payments.PaymentStatusCaptured,
		AmountRupiah:         10000,
		Currency:             payments.CurrencyIDR,
	}); reason != "MALFORMED_RESPONSE" {
		t.Fatalf("missing capture evidence reason = %q", reason)
	}

	if reason := validateInquiryResponse(attempt, payments.PaymentStatusResponse{
		Scope:                payments.PaymentInquiryScopePayment,
		ProviderPaymentReqID: "other-request-0002",
		ProviderPaymentID:    "payment-test-0001",
		Status:               payments.PaymentStatusPending,
	}); reason != "REFERENCE_MISMATCH" {
		t.Fatalf("reference mismatch reason = %q", reason)
	}

	if reason := validateInquiryResponse(attempt, payments.PaymentStatusResponse{
		Scope:                payments.PaymentInquiryScopePayment,
		ProviderPaymentReqID: "payment-request-test-0001",
		ProviderPaymentID:    "payment-test-0001",
		Status:               payments.PaymentStatusCaptured,
		AmountRupiah:         10001,
		Currency:             payments.CurrencyIDR,
		PayloadHash:          "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
	}); reason != "AMOUNT_MISMATCH" {
		t.Fatalf("amount mismatch reason = %q", reason)
	}
}

func TestValidateInquiryResponseAllowsPendingWithoutAuthoritativeAmount(t *testing.T) {
	attempt := payments.PaymentAttempt{ProviderSessionID: strptr("session-test-0001"), AmountRupiah: 10000, Currency: payments.CurrencyIDR}
	if reason := validateInquiryResponse(attempt, payments.PaymentStatusResponse{Scope: payments.PaymentInquiryScopeCheckoutSession, ProviderSessionID: "session-test-0001", Status: payments.PaymentStatusPending}); reason != "" {
		t.Fatalf("pending response reason = %q", reason)
	}
	if reason := validateInquiryResponse(attempt, payments.PaymentStatusResponse{Scope: payments.PaymentInquiryScopeCheckoutSession, ProviderSessionID: "session-test-0001", Status: payments.PaymentStatusPending, AmountRupiah: attempt.AmountRupiah, Currency: attempt.Currency}); reason != "" {
		t.Fatalf("pending response with exact money reason = %q", reason)
	}
	if reason := validateInquiryResponse(attempt, payments.PaymentStatusResponse{Scope: payments.PaymentInquiryScopeCheckoutSession, ProviderSessionID: "session-test-0001", Status: payments.PaymentStatusPending, AmountRupiah: attempt.AmountRupiah + 1}); reason != "AMOUNT_MISMATCH" {
		t.Fatalf("pending amount mismatch reason = %q", reason)
	}
	if reason := validateInquiryResponse(attempt, payments.PaymentStatusResponse{Scope: payments.PaymentInquiryScopeCheckoutSession, ProviderSessionID: "session-test-0001", Status: payments.PaymentStatusPending, Currency: payments.Currency("USD")}); reason != "CURRENCY_MISMATCH" {
		t.Fatalf("pending currency mismatch reason = %q", reason)
	}
}

func TestNewProcessorRejectsOutboxIncompatibleRetryPolicy(t *testing.T) {
	pool := &pgxpool.Pool{}
	attempts := payments.NewRepository(pool)
	outbox := paymentoutbox.NewRepository(pool)
	adapter := &scriptedWorkerAdapter{}
	auditService := audit.NewPlatformService(audit.NewPlatformRepository())

	for _, tc := range []struct {
		name   string
		policy RetryPolicy
	}{
		{name: "initial not microsecond aligned", policy: RetryPolicy{InitialDelay: time.Second + time.Nanosecond, MaxDelay: time.Minute}},
		{name: "maximum not microsecond aligned", policy: RetryPolicy{InitialDelay: time.Second, MaxDelay: time.Minute + time.Nanosecond}},
		{name: "maximum above outbox cap", policy: RetryPolicy{InitialDelay: time.Second, MaxDelay: 24*time.Hour + time.Microsecond}},
		{name: "initial above maximum", policy: RetryPolicy{InitialDelay: time.Minute, MaxDelay: time.Second}},
		{name: "partial zero value", policy: RetryPolicy{InitialDelay: time.Second}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := NewProcessor(pool, attempts, outbox, adapter, ProcessorOptions{Audit: auditService, RetryPolicy: tc.policy}); !errors.Is(err, paymentoutbox.ErrInvalidCommand) {
				t.Fatalf("retry policy %#v error = %v; want ErrInvalidCommand", tc.policy, err)
			}
		})
	}

	if _, err := NewProcessor(pool, attempts, outbox, adapter, ProcessorOptions{
		Audit:       auditService,
		RetryPolicy: RetryPolicy{InitialDelay: time.Microsecond, MaxDelay: 24 * time.Hour},
	}); err != nil {
		t.Fatalf("maximum outbox-compatible retry policy rejected: %v", err)
	}

	for _, adapterTimeout := range []time.Duration{
		-time.Nanosecond,
		24 * time.Hour,
		time.Duration(1<<63 - 1),
	} {
		if _, err := NewProcessor(pool, attempts, outbox, adapter, ProcessorOptions{
			Audit:          auditService,
			AdapterTimeout: adapterTimeout,
		}); !errors.Is(err, paymentoutbox.ErrInvalidCommand) {
			t.Fatalf("unleasable adapter timeout %s error = %v; want ErrInvalidCommand", adapterTimeout, err)
		}
	}
	if _, err := NewProcessor(pool, attempts, outbox, adapter, ProcessorOptions{
		Audit:          auditService,
		AdapterTimeout: 24*time.Hour - time.Nanosecond,
	}); err != nil {
		t.Fatalf("maximum leasable adapter timeout rejected: %v", err)
	}
}

func TestNewProcessorRejectsTypedNilDependencies(t *testing.T) {
	pool := &pgxpool.Pool{}
	outbox := paymentoutbox.NewRepository(pool)
	auditService := audit.NewPlatformService(audit.NewPlatformRepository())
	validAttempts := payments.NewRepository(pool)
	validAdapter := &scriptedWorkerAdapter{}
	var nilAttempts *payments.Repository
	var nilAdapter *scriptedWorkerAdapter
	var nilAudit *failingAuditService

	for _, tc := range []struct {
		name     string
		attempts paymentAttemptRepository
		adapter  payments.PaymentAdapter
		audit    audit.PlatformService
	}{
		{name: "typed nil repository", attempts: nilAttempts, adapter: validAdapter, audit: auditService},
		{name: "typed nil adapter", attempts: validAttempts, adapter: nilAdapter, audit: auditService},
		{name: "typed nil audit", attempts: validAttempts, adapter: validAdapter, audit: nilAudit},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := NewProcessor(pool, tc.attempts, outbox, tc.adapter, ProcessorOptions{Audit: tc.audit}); !errors.Is(err, ErrProcessorUnavailable) {
				t.Fatalf("typed nil dependency error = %v; want ErrProcessorUnavailable", err)
			}
		})
	}
}

func TestProcessorCallTimeoutIsNilSafe(t *testing.T) {
	var unavailable *Processor
	if got := unavailable.CallTimeout(); got != 0 {
		t.Fatalf("nil processor timeout = %s; want 0", got)
	}
	processor := &Processor{adapterTimeout: 7 * time.Second}
	if got := processor.CallTimeout(); got != 7*time.Second {
		t.Fatalf("processor timeout = %s; want 7s", got)
	}
}

func TestProcessorCancelledContextFailsClosedBeforeCommandHandling(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	adapterCalls := 0
	processor := &Processor{adapter: payments.NewFakeAdapter(payments.FakeAdapterScript{
		CreatePayment: func(context.Context, payments.CreatePaymentRequest) (payments.CreatePaymentResponse, error) {
			adapterCalls++
			return payments.CreatePaymentResponse{}, nil
		},
	})}
	if err := processor.Process(ctx, paymentoutbox.Command{}); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled processor error = %v; want context.Canceled", err)
	}
	if adapterCalls != 0 {
		t.Fatalf("cancelled processor adapter calls = %d; want 0", adapterCalls)
	}
}

func TestCommandExecutionDecisionMatrix(t *testing.T) {
	now := time.Date(2026, 7, 31, 12, 0, 0, 0, time.UTC)
	attemptID := "00000000-0000-4000-8000-000000000001"
	owner := "worker:00000000-0000-4000-8000-000000000002"
	token := "00000000-0000-4000-8000-000000000003"
	activeUntil := now.Add(time.Minute)
	expiredAt := now.Add(-time.Microsecond)
	baseCommand := paymentoutbox.Command{
		CommandType:      paymentoutbox.CommandPaymentCreate,
		State:            paymentoutbox.StateLeased,
		PaymentAttemptID: &attemptID,
		LeaseOwner:       &owner,
		LeaseToken:       &token,
		LeaseExpiresAt:   &activeUntil,
	}
	sessionID := "session-decision-matrix-0001"
	requestID := "request-decision-matrix-0001"
	paymentID := "payment-decision-matrix-0001"
	unsafeID := "secret-decision-matrix-0001"

	tests := []struct {
		name     string
		command  paymentoutbox.Command
		attempt  payments.PaymentAttempt
		action   executionAction
		identity providerIdentityState
	}{
		{name: "create identity none", command: baseCommand, attempt: payments.PaymentAttempt{State: payments.AttemptStateCreated}, action: executionCallCreate, identity: providerIdentityNone},
		{name: "create known session", command: baseCommand, attempt: payments.PaymentAttempt{State: payments.AttemptStatePending, ProviderSessionID: &sessionID}, action: executionLocalCreateRecovery, identity: providerIdentitySession},
		{name: "create terminal", command: baseCommand, attempt: payments.PaymentAttempt{State: payments.AttemptStateCancelled, ProviderSessionID: &sessionID}, action: executionLocalNoop, identity: providerIdentitySession},
		{name: "create unsafe identity", command: baseCommand, attempt: payments.PaymentAttempt{State: payments.AttemptStatePending, ProviderSessionID: &unsafeID}, action: executionLocalTerminal, identity: providerIdentityInvalid},
		{
			name:    "inquiry session",
			command: withCommandType(baseCommand, paymentoutbox.CommandPaymentInquiry),
			attempt: payments.PaymentAttempt{State: payments.AttemptStatePending, ProviderSessionID: &sessionID},
			action:  executionCallInquiry, identity: providerIdentitySession,
		},
		{
			name:    "inquiry request",
			command: withCommandType(baseCommand, paymentoutbox.CommandPaymentInquiry),
			attempt: payments.PaymentAttempt{State: payments.AttemptStatePending, ProviderPaymentReqID: &requestID},
			action:  executionCallInquiry, identity: providerIdentityPaymentRequest,
		},
		{
			name:    "inquiry payment",
			command: withCommandType(baseCommand, paymentoutbox.CommandPaymentInquiry),
			attempt: payments.PaymentAttempt{State: payments.AttemptStatePending, ProviderPaymentReqID: &requestID, ProviderPaymentID: &paymentID},
			action:  executionCallInquiry, identity: providerIdentityPayment,
		},
		{
			name:    "inquiry identity none",
			command: withCommandType(baseCommand, paymentoutbox.CommandPaymentInquiry),
			attempt: payments.PaymentAttempt{State: payments.AttemptStatePending},
			action:  executionLocalRetry, identity: providerIdentityNone,
		},
		{
			name:    "inquiry payment without request",
			command: withCommandType(baseCommand, paymentoutbox.CommandPaymentInquiry),
			attempt: payments.PaymentAttempt{State: payments.AttemptStatePending, ProviderPaymentID: &paymentID},
			action:  executionLocalTerminal, identity: providerIdentityInvalid,
		},
		{
			name:    "inquiry terminal",
			command: withCommandType(baseCommand, paymentoutbox.CommandPaymentInquiry),
			attempt: payments.PaymentAttempt{State: payments.AttemptStateCaptured, ProviderPaymentReqID: &requestID, ProviderPaymentID: &paymentID},
			action:  executionLocalNoop, identity: providerIdentityPayment,
		},
		{
			name: "expired lease",
			command: func() paymentoutbox.Command {
				command := baseCommand
				command.LeaseExpiresAt = &expiredAt
				return command
			}(),
			attempt: payments.PaymentAttempt{State: payments.AttemptStateCreated},
			action:  executionRejectLease, identity: providerIdentityNone,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			decision := decideCommandExecution(tc.command, tc.attempt, now)
			if decision.Action != tc.action || decision.IdentityState != tc.identity {
				t.Fatalf("decision = %#v; want action=%q identity=%q", decision, tc.action, tc.identity)
			}
		})
	}
}

func TestTerminalAttemptDecisionNeverCallsProvider(t *testing.T) {
	now := time.Date(2026, 7, 31, 12, 0, 0, 0, time.UTC)
	attemptID := "00000000-0000-4000-8000-000000000001"
	owner := "worker:00000000-0000-4000-8000-000000000002"
	token := "00000000-0000-4000-8000-000000000003"
	activeUntil := now.Add(time.Minute)
	requestID := "request-terminal-matrix-0001"
	paymentID := "payment-terminal-matrix-0001"
	for _, commandType := range []paymentoutbox.CommandType{
		paymentoutbox.CommandPaymentCreate,
		paymentoutbox.CommandPaymentInquiry,
	} {
		for _, state := range []payments.AttemptState{
			payments.AttemptStateCaptured,
			payments.AttemptStateFailed,
			payments.AttemptStateExpired,
			payments.AttemptStateCancelled,
		} {
			t.Run(string(commandType)+"/"+string(state), func(t *testing.T) {
				command := paymentoutbox.Command{
					CommandType: commandType, State: paymentoutbox.StateLeased,
					PaymentAttemptID: &attemptID, LeaseOwner: &owner,
					LeaseToken: &token, LeaseExpiresAt: &activeUntil,
				}
				attempt := payments.PaymentAttempt{
					State: state, ProviderPaymentReqID: &requestID,
					ProviderPaymentID: &paymentID,
				}
				if decision := decideCommandExecution(command, attempt, now); decision.Action != executionLocalNoop {
					t.Fatalf("terminal decision = %#v; want LOCAL_NOOP", decision)
				}
			})
		}
	}
}

func withCommandType(command paymentoutbox.Command, commandType paymentoutbox.CommandType) paymentoutbox.Command {
	command.CommandType = commandType
	return command
}

func TestInquiryDecisionFreezesSessionToPaymentHandoff(t *testing.T) {
	attempt := payments.PaymentAttempt{
		ProviderSessionID: strptr("session-test-0001"),
		AmountRupiah:      10000,
		Currency:          payments.CurrencyIDR,
	}
	decision := decideInquiryResponse(attempt, payments.PaymentStatusResponse{
		Scope:                payments.PaymentInquiryScopeCheckoutSession,
		ProviderSessionID:    "session-test-0001",
		ProviderPaymentReqID: "payment-request-test-0001",
		Status:               payments.PaymentStatusPending,
	})
	if decision.Kind != inquiryBindIdentityAndRetry {
		t.Fatalf("handoff decision = %q; want %q", decision.Kind, inquiryBindIdentityAndRetry)
	}

	for _, response := range []payments.PaymentStatusResponse{
		{Scope: payments.PaymentInquiryScopeCheckoutSession, ProviderSessionID: "wrong-session-0002", Status: payments.PaymentStatusPending},
		{Scope: payments.PaymentInquiryScopeCheckoutSession, ProviderSessionID: "session-test-0001", ProviderPaymentID: "payment-test-0001", Status: payments.PaymentStatusPending},
		{Scope: payments.PaymentInquiryScopeCheckoutSession, ProviderSessionID: "session-test-0001", Status: payments.PaymentStatusCaptured},
	} {
		if got := decideInquiryResponse(attempt, response).Kind; got != inquiryRejectMismatch && got != inquiryRejectMalformed {
			t.Fatalf("invalid session response decision = %q", got)
		}
	}
}

func TestCreateClassificationRejectsInvalidOptionalProviderIdentity(t *testing.T) {
	attempt := payments.PaymentAttempt{
		AmountRupiah: 10000,
		Currency:     payments.CurrencyIDR,
	}
	for _, tc := range []struct {
		name      string
		requestID string
		paymentID string
	}{
		{name: "request contains NUL", requestID: "payment-request-\x00invalid"},
		{name: "request contains newline", requestID: "payment-request-\ninvalid"},
		{name: "payment contains NUL", paymentID: "payment-\x00invalid"},
		{name: "payment contains tab", paymentID: "payment-\tinvalid"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if _, ok := classifyCreateResult(attempt, payments.CreatePaymentResponse{
				ProviderSessionID:    "session-created-0001",
				ProviderPaymentReqID: tc.requestID,
				ProviderPaymentID:    tc.paymentID,
				Status:               payments.PaymentStatusPending,
				AmountRupiah:         attempt.AmountRupiah,
				Currency:             attempt.Currency,
			}); ok {
				t.Fatal("invalid optional provider identity was accepted")
			}
		})
	}
}

func TestInquiryDecisionClassifiesInvalidProviderIdentityAsMalformed(t *testing.T) {
	sessionAttempt := payments.PaymentAttempt{
		ProviderSessionID: strptr("session-created-0001"),
		AmountRupiah:      10000,
		Currency:          payments.CurrencyIDR,
	}
	paymentAttempt := payments.PaymentAttempt{
		ProviderSessionID:    strptr("session-created-0001"),
		ProviderPaymentReqID: strptr("payment-request-0001"),
		AmountRupiah:         10000,
		Currency:             payments.CurrencyIDR,
	}
	for _, tc := range []struct {
		name     string
		attempt  payments.PaymentAttempt
		response payments.PaymentStatusResponse
	}{
		{
			name:    "session response request contains NUL",
			attempt: sessionAttempt,
			response: payments.PaymentStatusResponse{
				Scope:                payments.PaymentInquiryScopeCheckoutSession,
				ProviderSessionID:    "session-created-0001",
				ProviderPaymentReqID: "payment-request-\x00invalid",
				Status:               payments.PaymentStatusPending,
			},
		},
		{
			name:    "payment response payment ID contains newline",
			attempt: paymentAttempt,
			response: payments.PaymentStatusResponse{
				Scope:                payments.PaymentInquiryScopePayment,
				ProviderPaymentReqID: "payment-request-0001",
				ProviderPaymentID:    "payment-\ninvalid",
				Status:               payments.PaymentStatusPending,
			},
		},
		{
			name:    "payment response request contains tab",
			attempt: paymentAttempt,
			response: payments.PaymentStatusResponse{
				Scope:                payments.PaymentInquiryScopePayment,
				ProviderPaymentReqID: "payment-request-\tinvalid",
				Status:               payments.PaymentStatusPending,
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			decision := decideInquiryResponse(tc.attempt, tc.response)
			if decision.Kind != inquiryRejectMalformed || decision.Reason != "MALFORMED_RESPONSE" {
				t.Fatalf("decision = %#v; want malformed response", decision)
			}
		})
	}
}

func TestInquiryDecisionRequiresPaymentIdentityAndEvidence(t *testing.T) {
	attempt := payments.PaymentAttempt{ProviderPaymentReqID: strptr("request-test-0001"), AmountRupiah: 10000, Currency: payments.CurrencyIDR}
	base := payments.PaymentStatusResponse{Scope: payments.PaymentInquiryScopePayment, ProviderPaymentReqID: "request-test-0001", Status: payments.PaymentStatusCaptured, AmountRupiah: 10000, Currency: payments.CurrencyIDR, ProviderPaymentID: "payment-test-0001"}
	if got := decideInquiryResponse(attempt, base).Kind; got != inquiryRejectMalformed {
		t.Fatalf("missing captured timestamp/hash decision = %q", got)
	}
	captured := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	base.CapturedAt = &captured
	base.PayloadHash = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	if got := decideInquiryResponse(attempt, base).Kind; got != inquiryCapture {
		t.Fatalf("valid capture decision = %q", got)
	}
}

func TestReconciliationAuditRequiresAttemptIdentity(t *testing.T) {
	processor := &Processor{}
	if err := processor.recordReconciliationTx(context.Background(), nil, paymentoutbox.Command{}, "REFERENCE_MISMATCH"); !errors.Is(err, ErrMalformedCommand) {
		t.Fatalf("missing attempt identity error = %v; want ErrMalformedCommand", err)
	}
}

func TestCommandFactsMatchBindsPayloadHashAndDeterministicKey(t *testing.T) {
	attempt := payments.PaymentAttempt{
		ID:              "00000000-0000-4000-8000-000000000001",
		BookingID:       "00000000-0000-4000-8000-000000000002",
		AttemptNo:       1,
		AmountRupiah:    10000,
		Currency:        payments.CurrencyIDR,
		RequestedMethod: payments.RequestedMethodQRIS,
		RequestHash:     "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
	}
	payload := paymentoutbox.PaymentCommandPayload{
		AttemptID:       attempt.ID,
		AmountRupiah:    attempt.AmountRupiah,
		Currency:        string(attempt.Currency),
		RequestedMethod: string(attempt.RequestedMethod),
	}
	create := paymentoutbox.Command{CommandType: paymentoutbox.CommandPaymentCreate, IdempotencyKey: paymentoutbox.DeterministicCreateKey(attempt.BookingID, attempt.AttemptNo), RequestHash: attempt.RequestHash}
	if !commandFactsMatch(create, attempt, payload) {
		t.Fatal("valid create command facts rejected")
	}
	create.RequestHash = "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
	if commandFactsMatch(create, attempt, payload) {
		t.Fatal("request hash mismatch accepted")
	}
}

func strptr(value string) *string { return &value }
