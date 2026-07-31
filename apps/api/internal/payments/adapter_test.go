package payments

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"
)

func TestAdapterErrorNormalizationIsRedactedAndClassified(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want AdapterErrorCode
	}{
		{name: "deadline", err: context.DeadlineExceeded, want: AdapterErrorRetryableTimeout},
		{name: "cancelled", err: context.Canceled, want: AdapterErrorRetryableProvider},
		{name: "normalized terminal", err: NewAdapterError(AdapterErrorTerminalProvider, 0), want: AdapterErrorTerminalProvider},
		{name: "unknown", err: errors.New("provider raw response with secret token"), want: AdapterErrorRetryableProvider},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := NormalizeAdapterError(tc.err)
			if got.Code() != tc.want {
				t.Fatalf("normalized code = %q; want %q", got.Code(), tc.want)
			}
			if strings.Contains(got.Error(), "secret") || strings.Contains(got.Error(), "raw response") {
				t.Fatalf("normalized error leaked provider text: %q", got.Error())
			}
		})
	}

	if !errors.Is(NewAdapterError(AdapterErrorRateLimited, time.Second), ErrRateLimited) {
		t.Fatal("rate-limited normalized error did not match sentinel")
	}
	pointerError := &AdapterError{code: AdapterErrorAuthenticationFailed}
	if got := NormalizeAdapterError(pointerError); got.Code() != AdapterErrorAuthenticationFailed {
		t.Fatalf("pointer adapter error code = %q", got.Code())
	}

	rawCode := AdapterErrorCode("provider raw response with secret token")
	if got := NormalizeAdapterError(NewAdapterError(rawCode, 0)); got.Code() != AdapterErrorMalformedResponse {
		t.Fatalf("arbitrary adapter error code = %q; want %q", got.Code(), AdapterErrorMalformedResponse)
	}
	if got := NewAdapterError(rawCode, 0).Error(); strings.Contains(got, "secret") || strings.Contains(got, "raw response") {
		t.Fatalf("arbitrary adapter error code leaked provider text: %q", got)
	}
}

func TestAdapterConfigRequiresTestModeAndRedactsSecrets(t *testing.T) {
	secret := "xnd_test_only_secret"
	token := "callback-token-test-only"
	valid := AdapterConfig{
		Provider:     ProviderXendit,
		Environment:  ProviderEnvironmentTest,
		Enabled:      true,
		EndpointURL:  "https://api.example.test",
		SecretKey:    secret,
		WebhookToken: token,
	}
	if err := valid.Validate(); err != nil {
		t.Fatalf("valid test adapter config rejected: %v", err)
	}
	redacted := fmt.Sprintf("%+v", valid.Redacted())
	if strings.Contains(redacted, secret) || strings.Contains(redacted, token) {
		t.Fatalf("redacted adapter config leaked secret or token: %s", redacted)
	}

	invalid := valid
	invalid.Environment = "LIVE"
	if !errors.Is(invalid.Validate(), ErrAdapterNotTestMode) {
		t.Fatal("live adapter environment was accepted")
	}

	invalid = valid
	invalid.EndpointURL = "http://api.example.test"
	if !errors.Is(invalid.Validate(), ErrAdapterURLInvalid) {
		t.Fatal("non-HTTPS adapter endpoint was accepted")
	}

	invalid = valid
	invalid.Enabled = true
	invalid.SecretKey = ""
	if !errors.Is(invalid.Validate(), ErrAdapterSecretMissing) {
		t.Fatal("enabled adapter without secret was accepted")
	}
}

func TestFakeAdapterImplementsEveryContractOperation(t *testing.T) {
	called := make(map[string]bool)
	fake := NewFakeAdapter(FakeAdapterScript{
		CreatePayment: func(context.Context, CreatePaymentRequest) (CreatePaymentResponse, error) {
			called["create"] = true
			return CreatePaymentResponse{Status: PaymentStatusPending}, nil
		},
		GetPaymentStatus: func(context.Context, GetPaymentStatusRequest) (PaymentStatusResponse, error) {
			called["status"] = true
			return PaymentStatusResponse{Status: PaymentStatusCaptured}, nil
		},
		VerifyWebhook: func(context.Context, VerifyWebhookRequest) (WebhookVerification, error) {
			called["verify"] = true
			return WebhookVerification{Verified: true}, nil
		},
		ParseWebhook: func(context.Context, ParseWebhookRequest) (WebhookEvent, error) {
			called["parse"] = true
			return WebhookEvent{State: WebhookEventStateCaptured}, nil
		},
		RequestRefund: func(context.Context, RefundRequest) (RefundResponse, error) {
			called["refund"] = true
			return RefundResponse{Status: RefundStatusProcessing}, nil
		},
		GetRefundStatus: func(context.Context, GetRefundStatusRequest) (RefundStatusResponse, error) {
			called["refund-status"] = true
			return RefundStatusResponse{Status: RefundStatusSucceeded}, nil
		},
	})

	ctx := context.Background()
	if _, err := fake.CreatePayment(ctx, CreatePaymentRequest{}); err != nil {
		t.Fatal(err)
	}
	if _, err := fake.GetPaymentStatus(ctx, GetPaymentStatusRequest{}); err != nil {
		t.Fatal(err)
	}
	if _, err := fake.VerifyWebhook(ctx, VerifyWebhookRequest{}); err != nil {
		t.Fatal(err)
	}
	if _, err := fake.ParseWebhook(ctx, ParseWebhookRequest{}); err != nil {
		t.Fatal(err)
	}
	if _, err := fake.RequestRefund(ctx, RefundRequest{}); err != nil {
		t.Fatal(err)
	}
	if _, err := fake.GetRefundStatus(ctx, GetRefundStatusRequest{}); err != nil {
		t.Fatal(err)
	}

	for _, operation := range []string{"create", "status", "verify", "parse", "refund", "refund-status"} {
		if !called[operation] {
			t.Errorf("fake adapter operation %q was not delegated", operation)
		}
	}

	var adapter PaymentAdapter = NewFakeAdapter(FakeAdapterScript{})
	if _, err := adapter.CreatePayment(ctx, CreatePaymentRequest{}); !errors.Is(err, ErrFakeAdapterUnscripted) {
		t.Fatalf("unscripted fake error = %v; want ErrFakeAdapterUnscripted", err)
	}
}

func TestWebhookEventRepresentsPaymentAndRefundFacts(t *testing.T) {
	payment := WebhookEvent{
		ProviderSessionID:    "session-test",
		ProviderPaymentReqID: "payment-request-test",
		ProviderPaymentID:    "payment-test",
		State:                WebhookEventStateCaptured,
	}
	if payment.ProviderSessionID == "" || payment.ProviderPaymentReqID == "" ||
		payment.ProviderPaymentID == "" || !payment.State.IsValid() {
		t.Fatalf("payment webhook event is incomplete: %+v", payment)
	}

	refund := WebhookEvent{
		ProviderPaymentReqID: "payment-request-test",
		ProviderPaymentID:    "payment-test",
		ProviderRefundID:     "refund-test",
		State:                WebhookEventStateProcessing,
	}
	if refund.ProviderRefundID == "" || refund.State != WebhookEventStateProcessing || !refund.State.IsValid() {
		t.Fatalf("refund webhook event is incomplete: %+v", refund)
	}
}

func TestFakeAdapterCarriesRefundRequestHash(t *testing.T) {
	const requestHash = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	var received RefundRequest
	fake := NewFakeAdapter(FakeAdapterScript{
		RequestRefund: func(_ context.Context, req RefundRequest) (RefundResponse, error) {
			received = req
			return RefundResponse{Status: RefundStatusProcessing}, nil
		},
	})

	if _, err := fake.RequestRefund(context.Background(), RefundRequest{
		IdempotencyKey: "refund:create:attempt-test:full:v1",
		RequestHash:    requestHash,
	}); err != nil {
		t.Fatal(err)
	}
	if received.RequestHash != requestHash {
		t.Fatalf("refund request hash = %q; want %q", received.RequestHash, requestHash)
	}
}

func TestFakeAdapterCarriesProviderNeutralInquiryIdentity(t *testing.T) {
	var received GetPaymentStatusRequest
	fake := NewFakeAdapter(FakeAdapterScript{
		GetPaymentStatus: func(_ context.Context, req GetPaymentStatusRequest) (PaymentStatusResponse, error) {
			received = req
			return PaymentStatusResponse{
				Scope:                PaymentInquiryScopeCheckoutSession,
				ProviderSessionID:    "session-test",
				ProviderPaymentReqID: "payment-request-test",
				Status:               PaymentStatusPending,
			}, nil
		},
	})

	_, err := fake.GetPaymentStatus(context.Background(), GetPaymentStatusRequest{
		AttemptID:            "attempt-test",
		ProviderSessionID:    "session-test",
		ProviderPaymentReqID: "payment-request-test",
		ProviderPaymentID:    "payment-test",
		IdempotencyKey:       "payment:inquiry:00000000-0000-4000-8000-000000000001",
	})
	if err != nil {
		t.Fatal(err)
	}
	if received.ProviderSessionID != "session-test" || received.ProviderPaymentReqID != "payment-request-test" ||
		received.ProviderPaymentID != "payment-test" || received.IdempotencyKey == "" {
		t.Fatalf("inquiry identity was not delegated exactly: %+v", received)
	}
}

func TestFakeAdapterScriptCanProveCreateRetryUsesSameKeyAndRequestFacts(t *testing.T) {
	var received []CreatePaymentRequest
	fake := NewFakeAdapter(FakeAdapterScript{
		CreatePayment: func(_ context.Context, req CreatePaymentRequest) (CreatePaymentResponse, error) {
			received = append(received, req)
			if len(received) == 1 {
				return CreatePaymentResponse{}, ErrRetryableTimeout
			}
			return CreatePaymentResponse{
				ProviderSessionID: "ps-661f87c614802d6c402cd82d0",
				Status:            PaymentStatusPending,
				AmountRupiah:      10000,
				Currency:          CurrencyIDR,
				CheckoutURL:       "https://checkout-staging.xendit.co/sessions/ps-661f87c614802d6c402cd82d0",
			}, nil
		},
	})
	request := CreatePaymentRequest{
		AttemptID: "00000000-0000-4000-8000-000000000001", AmountRupiah: 10000,
		Currency: CurrencyIDR, IdempotencyKey: "payment:create:00000000-0000-4000-8000-000000000002:1",
		RequestHash: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
	}
	if _, err := fake.CreatePayment(context.Background(), request); !errors.Is(err, ErrRetryableTimeout) {
		t.Fatalf("first create error = %v; want timeout", err)
	}
	if _, err := fake.CreatePayment(context.Background(), request); err != nil {
		t.Fatalf("replayed create error = %v", err)
	}
	if len(received) != 2 || received[0].IdempotencyKey != received[1].IdempotencyKey ||
		received[0].RequestHash != received[1].RequestHash || received[0].AttemptID != received[1].AttemptID {
		t.Fatalf("create retry changed immutable request facts: %+v", received)
	}
}
