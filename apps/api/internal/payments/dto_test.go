package payments

import (
	"errors"
	"strings"
	"testing"
)

const validPaymentHash = "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"

func TestParsePositiveRupiah(t *testing.T) {
	tests := []struct {
		name  string
		raw   string
		want  int64
		valid bool
	}{
		{name: "one rupiah", raw: "1", want: 1, valid: true},
		{name: "normal rupiah", raw: "100000", want: 100000, valid: true},
		{name: "int64 maximum", raw: "9223372036854775807", want: 9223372036854775807, valid: true},
		{name: "empty", raw: ""},
		{name: "zero", raw: "0"},
		{name: "leading zero", raw: "01"},
		{name: "negative", raw: "-1"},
		{name: "plus sign", raw: "+1"},
		{name: "fraction", raw: "10000.50"},
		{name: "scientific", raw: "1e4"},
		{name: "comma separator", raw: "10,000"},
		{name: "dot separator", raw: "10.000"},
		{name: "leading whitespace", raw: " 10000"},
		{name: "trailing whitespace", raw: "10000 "},
		{name: "newline", raw: "10000\n"},
		{name: "unicode digit", raw: "١٠٠"},
		{name: "int64 overflow", raw: "9223372036854775808"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := ParsePositiveRupiah(tc.raw)
			if tc.valid {
				if err != nil || got != tc.want {
					t.Fatalf("ParsePositiveRupiah(%q) = %d, %v; want %d, nil", tc.raw, got, err, tc.want)
				}
				return
			}
			if !errors.Is(err, ErrInvalidPaymentInput) || err != ErrInvalidPaymentInput {
				t.Fatalf("ParsePositiveRupiah(%q) returned unsafe error %v", tc.raw, err)
			}
			if got != 0 {
				t.Fatalf("ParsePositiveRupiah(%q) = %d; want 0", tc.raw, got)
			}
		})
	}
}

func TestValidatePaymentAttemptInput_Valid(t *testing.T) {
	input := validPaymentAttemptInput()
	got, err := ValidatePaymentAttemptInput(input)
	if err != nil {
		t.Fatalf("ValidatePaymentAttemptInput() error = %v", err)
	}
	if got.AmountRupiah != 100000 || got.Currency != CurrencyIDR || got.RequestedMethod != RequestedMethodQRIS {
		t.Fatalf("unexpected validated DTO: %#v", got)
	}
}

func TestValidatePaymentAttemptInput_RejectsUnsafeInputWithGenericError(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*PaymentAttemptInput)
	}{
		{name: "provider", mutate: func(input *PaymentAttemptInput) { input.Provider = "OTHER" }},
		{name: "live environment", mutate: func(input *PaymentAttemptInput) { input.ProviderEnvironment = "LIVE" }},
		{name: "lowercase rail", mutate: func(input *PaymentAttemptInput) { input.RequestedMethod = "qris" }},
		{name: "unsupported rail", mutate: func(input *PaymentAttemptInput) { input.RequestedMethod = "DANA" }},
		{name: "unsupported mode", mutate: func(input *PaymentAttemptInput) { input.IntegrationMode = "DIRECT" }},
		{name: "unsupported capture method", mutate: func(input *PaymentAttemptInput) { input.CaptureMethod = "MANUAL" }},
		{name: "unsupported state", mutate: func(input *PaymentAttemptInput) { input.State = "REFUNDED" }},
		{name: "invalid currency", mutate: func(input *PaymentAttemptInput) { input.Currency = "USD" }},
		{name: "fraction amount", mutate: func(input *PaymentAttemptInput) { input.AmountRupiah = "100.01" }},
		{name: "reference with PII-like email delimiter", mutate: func(input *PaymentAttemptInput) { input.LocalReference = "payment:user@example.test" }},
		{name: "uppercase reference", mutate: func(input *PaymentAttemptInput) { input.LocalReference = "Payment:create:one" }},
		{name: "long reference", mutate: func(input *PaymentAttemptInput) { input.LocalReference = strings.Repeat("a", 65) }},
		{name: "uppercase hash", mutate: func(input *PaymentAttemptInput) { input.RequestHash = strings.ToUpper(validPaymentHash) }},
		{name: "short hash", mutate: func(input *PaymentAttemptInput) { input.RequestHash = "0123" }},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			input := validPaymentAttemptInput()
			tc.mutate(&input)
			got, err := ValidatePaymentAttemptInput(input)
			if !errors.Is(err, ErrInvalidPaymentInput) || err != ErrInvalidPaymentInput {
				t.Fatalf("ValidatePaymentAttemptInput() returned unsafe error %v", err)
			}
			if got != (ValidatedPaymentAttempt{}) {
				t.Fatalf("invalid input returned partially validated data: %#v", got)
			}
		})
	}
}

func TestValidatePaymentAttemptInput_AllFrozenAttemptStates(t *testing.T) {
	for _, state := range []AttemptState{
		AttemptStateCreated,
		AttemptStatePending,
		AttemptStateCaptured,
		AttemptStateFailed,
		AttemptStateExpired,
		AttemptStateCancelled,
	} {
		t.Run(string(state), func(t *testing.T) {
			input := validPaymentAttemptInput()
			input.State = state
			if _, err := ValidatePaymentAttemptInput(input); err != nil {
				t.Fatalf("state %q rejected: %v", state, err)
			}
		})
	}
}

func TestValidatePaymentAttemptInput_AllAllowedRequestedMethods(t *testing.T) {
	for _, method := range []RequestedMethod{
		RequestedMethodBCAVA,
		RequestedMethodQRIS,
		RequestedMethodCard,
	} {
		t.Run(string(method), func(t *testing.T) {
			input := validPaymentAttemptInput()
			input.RequestedMethod = method
			if _, err := ValidatePaymentAttemptInput(input); err != nil {
				t.Fatalf("method %q rejected: %v", method, err)
			}
		})
	}
}

func validPaymentAttemptInput() PaymentAttemptInput {
	return PaymentAttemptInput{
		Provider:            ProviderXendit,
		ProviderEnvironment: ProviderEnvironmentTest,
		RequestedMethod:     RequestedMethodQRIS,
		IntegrationMode:     IntegrationModePaymentLink,
		CaptureMethod:       CaptureMethodAutomatic,
		State:               AttemptStateCreated,
		Currency:            CurrencyIDR,
		AmountRupiah:        "100000",
		LocalReference:      "payment:create:0123456789abcdef",
		RequestHash:         validPaymentHash,
	}
}
