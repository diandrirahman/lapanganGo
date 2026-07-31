package config

import (
	"errors"
	"strings"
	"testing"
)

func TestPaymentConfigDefaultsToDisabledTestMode(t *testing.T) {
	cfg, err := LoadFrom(paymentConfigEnv(nil))
	if err != nil {
		t.Fatalf("default payment config rejected: %v", err)
	}
	if cfg.PaymentSandboxEnabled || cfg.PaymentCreateEnabled || cfg.PaymentInquiryEnabled || cfg.PaymentWebhookIngressEnabled || cfg.PaymentRefundEnabled {
		t.Fatal("payment capability unexpectedly enabled by default")
	}
	if cfg.PaymentProvider != "XENDIT" || cfg.PaymentProviderMode != "TEST" || cfg.PaymentWebhookContractVersion != "DISABLED" {
		t.Fatalf("unexpected safe payment defaults: %#v", cfg)
	}
}

func TestPaymentConfigFailsClosedForCapabilityDependencies(t *testing.T) {
	tests := []struct {
		name string
		set  map[string]string
		want string
	}{
		{
			name: "create requires sandbox master flag",
			set:  map[string]string{"PAYMENT_CREATE_ENABLED": "true", "XENDIT_SECRET_KEY": "test-secret"},
			want: "PAYMENT_SANDBOX_ENABLED",
		},
		{
			name: "create requires backend secret",
			set:  map[string]string{"PAYMENT_SANDBOX_ENABLED": "true", "PAYMENT_CREATE_ENABLED": "true"},
			want: "XENDIT_SECRET_KEY",
		},
		{
			name: "create requires return origin",
			set: map[string]string{
				"PAYMENT_SANDBOX_ENABLED": "true",
				"PAYMENT_CREATE_ENABLED":  "true",
				"XENDIT_SECRET_KEY":       "test-secret",
			},
			want: "PAYMENT_RETURN_ORIGIN",
		},
		{
			name: "webhook ingress requires token",
			set:  map[string]string{"PAYMENT_SANDBOX_ENABLED": "true", "PAYMENT_WEBHOOK_INGRESS_ENABLED": "true"},
			want: "XENDIT_WEBHOOK_TOKEN",
		},
		{
			name: "processor requires verified contract",
			set: map[string]string{
				"PAYMENT_SANDBOX_ENABLED":           "true",
				"PAYMENT_WEBHOOK_INGRESS_ENABLED":   "true",
				"PAYMENT_WEBHOOK_PROCESSOR_ENABLED": "true",
				"XENDIT_WEBHOOK_TOKEN":              "test-token",
			},
			want: "verified webhook ingress contract",
		},
		{
			name: "live provider mode rejected",
			set:  map[string]string{"PAYMENT_PROVIDER_MODE": "LIVE"},
			want: "PAYMENT_PROVIDER_MODE",
		},
		{
			name: "refund waits for outbox prerequisites",
			set:  map[string]string{"PAYMENT_SANDBOX_ENABLED": "true", "PAYMENT_REFUND_ENABLED": "true", "XENDIT_SECRET_KEY": "test-secret"},
			want: "payment facts and outbox prerequisites",
		},
		{
			name: "test ledger is isolated",
			set:  map[string]string{"PAYMENT_SANDBOX_ENABLED": "true", "PAYMENT_ISOLATED_TEST_LEDGER_ENABLED": "true"},
			want: "restricted to isolated tests",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := LoadFrom(paymentConfigEnv(tc.set))
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("error = %v; want message containing %q", err, tc.want)
			}
		})
	}
}

func TestPaymentConfigAcceptsVerifiedSandboxContract(t *testing.T) {
	env := paymentConfigEnv(map[string]string{
		"PAYMENT_SANDBOX_ENABLED":           "true",
		"PAYMENT_CREATE_ENABLED":            "true",
		"PAYMENT_WEBHOOK_INGRESS_ENABLED":   "true",
		"PAYMENT_WEBHOOK_PROCESSOR_ENABLED": "true",
		"PAYMENT_WEBHOOK_CONTRACT_VERSION":  "XENDIT_CALLBACK_TOKEN_V1_VERIFIED",
		"PAYMENT_RETURN_ORIGIN":             "https://demo.example.test",
		"XENDIT_SECRET_KEY":                 "backend-test-secret",
		"XENDIT_WEBHOOK_TOKEN":              "backend-test-token",
	})
	cfg, err := LoadFrom(env)
	if err != nil {
		t.Fatalf("valid sandbox config rejected: %v", err)
	}
	if !cfg.PaymentCreateEnabled || cfg.PaymentProviderMode != "TEST" {
		t.Fatalf("unexpected accepted payment config: %#v", cfg)
	}
}

func TestPaymentConfigRejectsNonOriginReturnURL(t *testing.T) {
	_, err := LoadFrom(paymentConfigEnv(map[string]string{
		"PAYMENT_RETURN_ORIGIN": "https://demo.example.test/callback",
	}))
	if err == nil || !strings.Contains(err.Error(), "PAYMENT_RETURN_ORIGIN") {
		t.Fatalf("return URL config error = %v", err)
	}
}

func TestPaymentConfigReturnOriginMatchesDatabaseContract(t *testing.T) {
	accepted := []string{
		"https://demo.example.test",
		"https://DEMO.EXAMPLE.TEST:443",
		"https://127.0.0.1:3000",
	}
	for _, origin := range accepted {
		t.Run("accept "+origin, func(t *testing.T) {
			if _, err := LoadFrom(paymentConfigEnv(map[string]string{"PAYMENT_RETURN_ORIGIN": origin})); err != nil {
				t.Fatalf("supported return origin rejected: %v", err)
			}
		})
	}

	rejected := []string{
		"https://[::1]:3000",
		"https://demo.example.test:0",
		"https://demo.example.test:65536",
		"https://-",
		"https://..example",
		"https://999.999.999.999",
		"https://demo_example.test",
		"https://démø.example.test",
	}
	for _, origin := range rejected {
		t.Run("reject "+origin, func(t *testing.T) {
			if _, err := LoadFrom(paymentConfigEnv(map[string]string{"PAYMENT_RETURN_ORIGIN": origin})); err == nil {
				t.Fatalf("unsupported return origin accepted: %q", origin)
			}
		})
	}
}

func TestPaymentConfigRejectsFrontendProviderSecrets(t *testing.T) {
	tests := []string{
		"VITE_XENDIT_SECRET_KEY=",
		"VITE_XENDIT_WEBHOOK_TOKEN=frontend-token",
		"VITE_PAYMENT_PROVIDER_API_KEY=frontend-key",
	}
	for _, entry := range tests {
		t.Run(strings.SplitN(entry, "=", 2)[0], func(t *testing.T) {
			_, err := LoadFromEnvironment(paymentConfigEnv(nil), []string{entry})
			if !errors.Is(err, ErrFrontendProviderSecret) {
				t.Fatalf("frontend provider secret error = %v; want ErrFrontendProviderSecret", err)
			}
		})
	}
}

func TestPaymentConfigAllowsNonSecretViteConfiguration(t *testing.T) {
	cfg, err := LoadFromEnvironment(paymentConfigEnv(nil), []string{
		"VITE_API_URL=https://demo.example.test",
		"VITE_XENDIT_PUBLIC_KEY=xnd_public_test",
	})
	if err != nil {
		t.Fatalf("safe frontend configuration rejected: %v", err)
	}
	if cfg.PaymentProviderMode != "TEST" {
		t.Fatalf("payment provider mode = %q; want TEST", cfg.PaymentProviderMode)
	}
}

func paymentConfigEnv(overrides map[string]string) func(string) string {
	base := map[string]string{
		"DATABASE_URL": "postgres://user:pass@localhost/db",
		"JWT_SECRET":   "test-jwt-secret",
	}
	for key, value := range overrides {
		base[key] = value
	}
	return func(key string) string { return base[key] }
}
