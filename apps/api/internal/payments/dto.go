// Package payments contains provider-neutral payment domain contracts.
//
// This package deliberately has no HTTP, database, provider SDK, or booking
// dependency. The amount parser is for validated internal/provider facts; a
// future customer create-attempt flow must source its authoritative amount
// from booking_fee_snapshots, never from a browser request.
package payments

import (
	"errors"
	"strconv"
)

var ErrInvalidPaymentInput = errors.New("invalid payment input")

type Currency string

const CurrencyIDR Currency = "IDR"

type Provider string

const ProviderXendit Provider = "XENDIT"

type ProviderEnvironment string

const ProviderEnvironmentTest ProviderEnvironment = "TEST"

type RequestedMethod string

const (
	RequestedMethodBCAVA RequestedMethod = "BCA_VA"
	RequestedMethodQRIS  RequestedMethod = "QRIS"
	RequestedMethodCard  RequestedMethod = "CARD"
)

type IntegrationMode string

const IntegrationModePaymentLink IntegrationMode = "PAYMENT_LINK"

type CaptureMethod string

const CaptureMethodAutomatic CaptureMethod = "AUTOMATIC"

type AttemptState string

const (
	AttemptStateCreated   AttemptState = "CREATED"
	AttemptStatePending   AttemptState = "PENDING"
	AttemptStateCaptured  AttemptState = "CAPTURED"
	AttemptStateFailed    AttemptState = "FAILED"
	AttemptStateExpired   AttemptState = "EXPIRED"
	AttemptStateCancelled AttemptState = "CANCELLED"
)

// PaymentAttemptInput is an internal, provider-neutral DTO. It is not an HTTP
// request model and is not persisted by this task.
type PaymentAttemptInput struct {
	Provider            Provider
	ProviderEnvironment ProviderEnvironment
	RequestedMethod     RequestedMethod
	IntegrationMode     IntegrationMode
	CaptureMethod       CaptureMethod
	State               AttemptState
	Currency            Currency
	AmountRupiah        string
	LocalReference      string
	RequestHash         string
}

// ValidatedPaymentAttempt is safe to hand to the future repository boundary.
// AmountRupiah is an integer Rupiah value only after strict parsing succeeds.
type ValidatedPaymentAttempt struct {
	Provider            Provider
	ProviderEnvironment ProviderEnvironment
	RequestedMethod     RequestedMethod
	IntegrationMode     IntegrationMode
	CaptureMethod       CaptureMethod
	State               AttemptState
	Currency            Currency
	AmountRupiah        int64
	LocalReference      string
	RequestHash         string
}

// ValidatePaymentAttemptInput checks the frozen Phase 5 values without
// trimming, case-folding, or otherwise silently normalizing caller input.
func ValidatePaymentAttemptInput(input PaymentAttemptInput) (ValidatedPaymentAttempt, error) {
	amountRupiah, err := ParsePositiveRupiah(input.AmountRupiah)
	if err != nil || !isAllowedProvider(input.Provider) ||
		!isAllowedProviderEnvironment(input.ProviderEnvironment) ||
		!isAllowedRequestedMethod(input.RequestedMethod) ||
		!isAllowedIntegrationMode(input.IntegrationMode) ||
		!isAllowedCaptureMethod(input.CaptureMethod) ||
		!isAllowedAttemptState(input.State) ||
		input.Currency != CurrencyIDR ||
		!isSafeLocalReference(input.LocalReference) ||
		!isLowerSHA256(input.RequestHash) {
		return ValidatedPaymentAttempt{}, ErrInvalidPaymentInput
	}

	return ValidatedPaymentAttempt{
		Provider:            input.Provider,
		ProviderEnvironment: input.ProviderEnvironment,
		RequestedMethod:     input.RequestedMethod,
		IntegrationMode:     input.IntegrationMode,
		CaptureMethod:       input.CaptureMethod,
		State:               input.State,
		Currency:            input.Currency,
		AmountRupiah:        amountRupiah,
		LocalReference:      input.LocalReference,
		RequestHash:         input.RequestHash,
	}, nil
}

// ParsePositiveRupiah accepts only canonical, positive base-10 integer strings
// that fit in int64. It intentionally rejects whitespace, leading zeroes,
// signs, fractions, separators, scientific notation, and non-ASCII digits.
func ParsePositiveRupiah(raw string) (int64, error) {
	if !isCanonicalPositiveDecimal(raw) {
		return 0, ErrInvalidPaymentInput
	}

	amount, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || amount <= 0 {
		return 0, ErrInvalidPaymentInput
	}
	return amount, nil
}

func isAllowedProvider(value Provider) bool {
	return value == ProviderXendit
}

func isAllowedProviderEnvironment(value ProviderEnvironment) bool {
	return value == ProviderEnvironmentTest
}

func isAllowedRequestedMethod(value RequestedMethod) bool {
	return value == RequestedMethodBCAVA || value == RequestedMethodQRIS || value == RequestedMethodCard
}

func isAllowedIntegrationMode(value IntegrationMode) bool {
	return value == IntegrationModePaymentLink
}

func isAllowedCaptureMethod(value CaptureMethod) bool {
	return value == CaptureMethodAutomatic
}

func isAllowedAttemptState(value AttemptState) bool {
	switch value {
	case AttemptStateCreated, AttemptStatePending, AttemptStateCaptured, AttemptStateFailed, AttemptStateExpired, AttemptStateCancelled:
		return true
	default:
		return false
	}
}

func isCanonicalPositiveDecimal(value string) bool {
	if value == "" || value[0] < '1' || value[0] > '9' {
		return false
	}
	for i := 1; i < len(value); i++ {
		if value[i] < '0' || value[i] > '9' {
			return false
		}
	}
	return true
}

func isSafeLocalReference(value string) bool {
	if len(value) == 0 || len(value) > 64 || !isLowerAlphaNumeric(value[0]) {
		return false
	}
	for i := 1; i < len(value); i++ {
		if !isLowerAlphaNumeric(value[i]) && value[i] != '.' && value[i] != '_' && value[i] != ':' && value[i] != '-' {
			return false
		}
	}
	return true
}

func isLowerSHA256(value string) bool {
	if len(value) != 64 {
		return false
	}
	for i := 0; i < len(value); i++ {
		if !isLowerHex(value[i]) {
			return false
		}
	}
	return true
}

func isLowerAlphaNumeric(value byte) bool {
	return (value >= 'a' && value <= 'z') || (value >= '0' && value <= '9')
}

func isLowerHex(value byte) bool {
	return (value >= '0' && value <= '9') || (value >= 'a' && value <= 'f')
}
