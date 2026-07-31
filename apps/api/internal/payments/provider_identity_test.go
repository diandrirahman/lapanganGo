package payments

import (
	"errors"
	"strings"
	"testing"
	"time"
)

func TestValidProviderIdentity(t *testing.T) {
	for _, tc := range []struct {
		name     string
		value    string
		required bool
		want     bool
	}{
		{name: "required opaque token", value: "payment-request_ABC:123/xyz", required: true, want: true},
		{name: "optional empty", required: false, want: true},
		{name: "required empty", required: true, want: false},
		{name: "leading whitespace", value: " request-123", required: true, want: false},
		{name: "embedded space", value: "request 123", required: true, want: false},
		{name: "NUL", value: "request-\x00invalid", required: true, want: false},
		{name: "newline", value: "request-\ninvalid", required: true, want: false},
		{name: "tab", value: "request-\tinvalid", required: true, want: false},
		{name: "invalid UTF-8", value: string([]byte{'r', 0xff}), required: true, want: false},
		{name: "over database limit", value: strings.Repeat("r", 192), required: true, want: false},
		{name: "secret prefix", value: "secret-demo1234", required: true, want: false},
		{name: "secret prefix case insensitive", value: "Secret_demo1234", required: true, want: false},
		{name: "api key prefix", value: "api-key-demo1234", required: true, want: false},
		{name: "authorization prefix", value: "authorization:demo1234", required: true, want: false},
		{name: "bearer prefix", value: "bearer_demo1234", required: true, want: false},
		{name: "card prefix", value: "card-demo1234", required: true, want: false},
		{name: "token prefix", value: "token_demo1234", required: true, want: false},
		{name: "sk prefix", value: "sk_test_demo1234", required: true, want: false},
		{name: "numeric account-like suffix", value: "session-4111111111111111", required: true, want: false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := ValidProviderIdentity(tc.value, tc.required); got != tc.want {
				t.Fatalf("ValidProviderIdentity(%q, %t) = %t; want %t", tc.value, tc.required, got, tc.want)
			}
		})
	}
}

func TestRepositoryProviderIdentityGuardsRejectControlCharacters(t *testing.T) {
	const attemptID = "00000000-0000-4000-8000-000000000001"
	invalidRequestID := "payment-request-\x00invalid"
	if validProviderResultIdentity(ApplyCreateProviderResultParams{
		AttemptID:            attemptID,
		Provider:             ProviderXendit,
		ProviderEnvironment:  ProviderEnvironmentTest,
		ProviderSessionID:    "session-created-0001",
		ProviderPaymentReqID: &invalidRequestID,
		ProviderStatusCode:   "PENDING",
		Status:               PaymentStatusPending,
		AmountRupiah:         10000,
		Currency:             CurrencyIDR,
	}) {
		t.Fatal("create repository guard accepted control character identity")
	}
	if validInquiryIdentityParams(ApplyInquiryIdentityParams{
		AttemptID:            attemptID,
		Provider:             ProviderXendit,
		ProviderEnvironment:  ProviderEnvironmentTest,
		Scope:                PaymentInquiryScopePayment,
		ProviderPaymentReqID: &invalidRequestID,
		ProviderStatusCode:   "PENDING",
	}) {
		t.Fatal("inquiry repository guard accepted control character identity")
	}
	capturedAt := time.Now().UTC().Add(-time.Second)
	if err := validateCaptureParams(CaptureParams{
		AttemptID:           attemptID,
		Provider:            ProviderXendit,
		ProviderEnvironment: ProviderEnvironmentTest,
		ProviderPaymentID:   "payment-\ninvalid",
		AmountRupiah:        10000,
		Currency:            CurrencyIDR,
		CapturedAt:          capturedAt,
		ObservedAt:          capturedAt.Add(time.Second),
		Authority:           "AUTHENTICATED_INQUIRY",
		SourceReference:     "payment:inquiry:00000000-0000-4000-8000-000000000001",
		PayloadHash:         strings.Repeat("a", 64),
	}); !errors.Is(err, ErrInvalidCapture) {
		t.Fatalf("capture repository guard error = %v; want ErrInvalidCapture", err)
	}
}
