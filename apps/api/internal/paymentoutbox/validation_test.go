package paymentoutbox

import (
	"errors"
	"strings"
	"testing"
	"time"
)

const validAggregateID = "11111111-1111-4111-8111-111111111111"
const validBookingID = "22222222-2222-4222-8222-222222222222"

func validEnqueueParams() EnqueueParams {
	return EnqueueParams{
		CommandType:      CommandPaymentCreate,
		AggregateType:    AggregatePaymentAttempt,
		AggregateID:      validAggregateID,
		PaymentAttemptID: validAggregateID,
		IdempotencyKey:   "payment:create:" + validBookingID + ":1",
		RequestHash:      strings.Repeat("a", 64),
		Payload: PaymentCommandPayload{
			AttemptID:       validAggregateID,
			AmountRupiah:    10000,
			Currency:        "IDR",
			RequestedMethod: "QRIS",
		},
	}
}

func TestValidateEnqueueParamsAcceptsRedactedPaymentPayload(t *testing.T) {
	payload, err := ValidateEnqueueParams(validEnqueueParams())
	if err != nil {
		t.Fatalf("valid enqueue params rejected: %v", err)
	}
	if len(payload) == 0 ||
		strings.Contains(string(payload), "secret") ||
		strings.Contains(string(payload), "card_number") ||
		strings.Contains(string(payload), "bank_account") {
		t.Fatalf("unexpected payload: %s", payload)
	}
	const expected = `{"attempt_id":"11111111-1111-4111-8111-111111111111","amount_rupiah":10000,"currency":"IDR","requested_method":"QRIS"}`
	if string(payload) != expected {
		t.Fatalf("canonical payload = %s; want %s", payload, expected)
	}
}

func TestValidateEnqueueParamsRejectsUnsafeOrInvalidValues(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*EnqueueParams)
	}{
		{name: "invalid command", mutate: func(p *EnqueueParams) { p.CommandType = "UNKNOWN" }},
		{name: "key uppercase", mutate: func(p *EnqueueParams) { p.IdempotencyKey = "PAYMENT:CREATE" }},
		{name: "arbitrary create key", mutate: func(p *EnqueueParams) { p.IdempotencyKey = "payment:create:outbox-test" }},
		{name: "secret-like key", mutate: func(p *EnqueueParams) { p.IdempotencyKey = "xnd_development_secret" }},
		{name: "hash uppercase", mutate: func(p *EnqueueParams) { p.RequestHash = strings.Repeat("A", 64) }},
		{name: "missing payload", mutate: func(p *EnqueueParams) { p.Payload = PaymentCommandPayload{} }},
		{name: "PAN disguised as attempt ID", mutate: func(p *EnqueueParams) { p.Payload.AttemptID = "4111111111111111" }},
		{name: "zero amount", mutate: func(p *EnqueueParams) { p.Payload.AmountRupiah = 0 }},
		{name: "negative amount", mutate: func(p *EnqueueParams) { p.Payload.AmountRupiah = -1 }},
		{name: "invalid currency", mutate: func(p *EnqueueParams) { p.Payload.Currency = "USD" }},
		{name: "bank account disguised as method", mutate: func(p *EnqueueParams) { p.Payload.RequestedMethod = "1234567890" }},
		{name: "invalid aggregate UUID", mutate: func(p *EnqueueParams) { p.AggregateID = "not-a-uuid"; p.PaymentAttemptID = "not-a-uuid" }},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			params := validEnqueueParams()
			tc.mutate(&params)
			if _, err := ValidateEnqueueParams(params); !errors.Is(err, ErrInvalidCommand) {
				t.Fatalf("validation error = %v; want ErrInvalidCommand", err)
			}
		})
	}
}

func TestValidateEnqueueParamsAcceptsReservedInquiryCommand(t *testing.T) {
	params := validEnqueueParams()
	params.CommandType = CommandPaymentInquiry
	params.IdempotencyKey = "payment:inquiry:" + validAggregateID
	if _, err := ValidateEnqueueParams(params); err != nil {
		t.Fatalf("reserved inquiry command rejected: %v", err)
	}
	if got := DeterministicInquiryKey(validAggregateID); got != params.IdempotencyKey {
		t.Fatalf("deterministic inquiry key = %q; want %q", got, params.IdempotencyKey)
	}
}

func TestValidateEnqueueParamsDefersRefundCommands(t *testing.T) {
	params := validEnqueueParams()
	params.CommandType = CommandRefundCreate
	params.AggregateType = AggregatePaymentRefund
	params.PaymentAttemptID = ""
	if _, err := ValidateEnqueueParams(params); !errors.Is(err, ErrRefundOutboxNotReady) {
		t.Fatalf("refund validation error = %v; want ErrRefundOutboxNotReady", err)
	}
}

func TestCommandTypeFilterIsAllowlistedAndDeterministic(t *testing.T) {
	filter, err := commandTypeFilter([]CommandType{CommandPaymentInquiry, CommandPaymentCreate, CommandPaymentInquiry})
	if err != nil {
		t.Fatalf("valid command filter rejected: %v", err)
	}
	if filter != "c.command_type IN ('PAYMENT_CREATE','PAYMENT_INQUIRY')" {
		t.Fatalf("filter = %q", filter)
	}
	for _, types := range [][]CommandType{{}, {CommandRefundCreate}, {"UNKNOWN"}} {
		if _, err := commandTypeFilter(types); !errors.Is(err, ErrInvalidCommand) {
			t.Fatalf("types %v error = %v; want ErrInvalidCommand", types, err)
		}
	}
}

func TestTxFinalizersNeverOpenTheirOwnTransaction(t *testing.T) {
	owner := "worker:" + validAggregateID
	if _, err := (&Repository{}).MarkRetryableTx(nil, nil, validAggregateID, owner, validAggregateID, "RETRYABLE_PROVIDER", time.Second); !errors.Is(err, ErrInvalidCommand) {
		t.Fatalf("nil retryable tx error = %v; want ErrInvalidCommand", err)
	}
	if _, err := (&Repository{}).MarkSucceededTx(nil, nil, validAggregateID, owner, validAggregateID, "sha256:"+strings.Repeat("a", 64)); !errors.Is(err, ErrInvalidCommand) {
		t.Fatalf("nil succeeded tx error = %v; want ErrInvalidCommand", err)
	}
	if _, err := (&Repository{}).MarkTerminalTx(nil, nil, validAggregateID, owner, validAggregateID, "INVALID_REQUEST"); !errors.Is(err, ErrInvalidCommand) {
		t.Fatalf("nil terminal tx error = %v; want ErrInvalidCommand", err)
	}
}

func TestValidateLeaseAndErrorInputs(t *testing.T) {
	for _, owner := range []string{
		"worker with spaces",
		"worker-unique-1",
		"worker:xnd_development_secret",
		"worker:4111111111111111",
		"worker:" + strings.Repeat("a", 192),
	} {
		if validateLeaseOwner(owner) {
			t.Fatalf("invalid lease owner %q was accepted", owner)
		}
	}
	if !validateLeaseOwner("worker:"+validAggregateID) ||
		!validateLeaseDuration(time.Microsecond) ||
		!validateLeaseDuration(time.Minute) ||
		!validateLeaseDuration(24*time.Hour) ||
		!ValidateLeaseDuration(time.Minute) {
		t.Fatal("valid lease input was rejected")
	}
	if validateLeaseDuration(-time.Nanosecond) ||
		validateLeaseDuration(0) ||
		validateLeaseDuration(time.Nanosecond) ||
		validateLeaseDuration(time.Microsecond-time.Nanosecond) ||
		validateLeaseDuration(time.Microsecond+time.Nanosecond) ||
		validateLeaseDuration(24*time.Hour-time.Nanosecond) ||
		validateLeaseDuration(24*time.Hour+time.Microsecond) ||
		ValidateLeaseDuration(time.Second+time.Nanosecond) {
		t.Fatal("invalid lease duration was accepted")
	}
	if !validateRetryDelay(0) || !validateRetryDelay(time.Microsecond) ||
		!validateRetryDelay(time.Minute) || !validateRetryDelay(24*time.Hour) ||
		!ValidateRetryDelay(24*time.Hour) {
		t.Fatal("valid retry delay was rejected")
	}
	if validateRetryDelay(-time.Nanosecond) ||
		validateRetryDelay(time.Nanosecond) ||
		validateRetryDelay(24*time.Hour-time.Nanosecond) ||
		validateRetryDelay(24*time.Hour+time.Nanosecond) ||
		ValidateRetryDelay(time.Minute+time.Nanosecond) {
		t.Fatal("invalid retry delay was accepted")
	}
	if !validateLeaseToken(validAggregateID) || validateLeaseToken("not-a-token") {
		t.Fatal("lease token validation mismatch")
	}
	for _, code := range []string{"RETRYABLE_TIMEOUT", "RETRYABLE_PROVIDER", "RATE_LIMITED", "MALFORMED_RESPONSE"} {
		if !validateRetryableErrorCode(code) {
			t.Fatalf("retryable error %q was rejected", code)
		}
	}
	for _, code := range []string{
		"AUTHENTICATION_FAILED", "INVALID_REQUEST", "IDEMPOTENCY_CONFLICT",
		"REFERENCE_MISMATCH", "AMOUNT_MISMATCH", "CURRENCY_MISMATCH",
		"TERMINAL_PROVIDER",
	} {
		if !validateTerminalErrorCode(code) {
			t.Fatalf("terminal error %q was rejected", code)
		}
	}
	if validateRetryableErrorCode("AUTHENTICATION_FAILED") ||
		validateTerminalErrorCode("RETRYABLE_TIMEOUT") ||
		validateTerminalErrorCode("MALFORMED_RESPONSE") ||
		validateRetryableErrorCode("provider raw response") ||
		validateTerminalErrorCode("provider raw response") {
		t.Fatal("error category boundary accepted an invalid code")
	}
	for _, rawReference := range []string{
		"ps_1234abcd",
		"ps-661f87c614802d6c402cd82d",
		"pr-8877c08a-740d-4153-9816-3d744ed197a5",
		"py-cc3938dc-c2a5-43c4-89d7-7570793348c2",
		"pr-90392f42-d98a-49ef-a7f3-abcezas127",
		"pr-123e4567-e89b-12d3-a456-426614174000",
		"123e4567-e89b-12d3-a456-426614174000",
	} {
		digest, err := DigestProviderReference(rawReference)
		if err != nil || !validateProviderReference(digest) || strings.Contains(digest, rawReference) {
			t.Fatalf("provider reference digest for %q = %q, %v", rawReference, digest, err)
		}
	}
	for _, reference := range []string{
		"4111111111111111",
		"1234567890",
		"ref-4111-1111-1111-1111",
		"ref_1234567890",
		"sk_test_abc123",
		"xnd_development_secret",
		"provider-token-value",
		"https://provider.example/reference",
		"provider_ref_4111111111111111",
		"sha256:" + strings.Repeat("a", 63),
		"sha256:" + strings.Repeat("A", 64),
	} {
		if validateProviderReference(reference) {
			t.Fatalf("unsafe provider reference %q was accepted", reference)
		}
	}
	for _, rawReference := range []string{
		"",
		"provider-reference",
		" provider-reference",
		"provider-reference ",
		"provider\nreference",
		"4111111111111111",
		"ref-4111-1111-1111-1111",
		"ref_1234567890",
		"sk_test_abc123",
		"https://provider.example/reference",
		"xnd_development_secret",
		strings.Repeat("a", 192),
	} {
		if digest, err := DigestProviderReference(rawReference); !errors.Is(err, ErrInvalidCommand) || digest != "" {
			t.Fatalf("unsafe raw provider reference %q digest = %q, %v", rawReference, digest, err)
		}
	}
}
