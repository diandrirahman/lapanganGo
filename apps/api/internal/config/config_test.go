package config

import (
	"strings"
	"testing"
)

func TestConfigLoadFrom_Booleans(t *testing.T) {
	cases := []struct {
		name        string
		envValue    string
		expectError bool
		expectBool  bool
	}{
		{"unset", "", false, false},
		{"exact false", "false", false, false},
		{"exact true", "true", false, true},
		{"uppercase true", "TRUE", true, false},
		{"uppercase false", "FALSE", true, false},
		{"numeric true", "1", true, false},
		{"numeric false", "0", true, false},
		{"whitespace true", " true ", true, false},
		{"arbitrary", "yes", true, false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			mockEnv := map[string]string{
				"DATABASE_URL":                   "postgres://user:pass@localhost/db",
				"JWT_SECRET":                     "secret",
				"PLATFORM_FINANCE_ADMIN_ENABLED": tc.envValue,
			}
			cfg, err := LoadFrom(func(k string) string { return mockEnv[k] })

			if tc.expectError {
				if err == nil {
					t.Fatalf("expected error for %q, got nil", tc.envValue)
				}
				if !strings.Contains(err.Error(), "invalid boolean configuration") {
					t.Fatalf("expected invalid boolean error, got: %v", err)
				}
			} else {
				if err != nil {
					t.Fatalf("expected no error for %q, got: %v", tc.envValue, err)
				}
				if cfg.PlatformFinanceAdminEnabled != tc.expectBool {
					t.Fatalf("expected bool %v, got %v", tc.expectBool, cfg.PlatformFinanceAdminEnabled)
				}
			}
		})
	}
}

func TestConfigLoadFrom_MissingRequired(t *testing.T) {
	mockEnv := map[string]string{
		// Missing DATABASE_URL and JWT_SECRET
	}
	_, err := LoadFrom(func(k string) string { return mockEnv[k] })
	if err == nil {
		t.Fatal("expected error for missing DATABASE_URL, got nil")
	}
	if !strings.Contains(err.Error(), "DATABASE_URL is required") {
		t.Fatalf("expected DATABASE_URL error, got: %v", err)
	}
}

func TestConfigLoadFrom_EmailDependencies(t *testing.T) {
	mockEnv := map[string]string{
		"DATABASE_URL":           "postgres://user:pass@localhost/db",
		"JWT_SECRET":             "secret",
		"EMAIL_DELIVERY_ENABLED": "true",
		// Missing SMTP_HOST
	}
	_, err := LoadFrom(func(k string) string { return mockEnv[k] })
	if err == nil {
		t.Fatal("expected error for missing SMTP_HOST, got nil")
	}
	if !strings.Contains(err.Error(), "SMTP_HOST is required when EMAIL_DELIVERY_ENABLED is true") {
		t.Fatalf("expected SMTP_HOST error, got: %v", err)
	}
}

func TestConfigLoadFrom_SanitizedErrors(t *testing.T) {
	mockEnv := map[string]string{
		"DATABASE_URL": "postgres://user:super_secret_password@localhost/db",
		"JWT_SECRET":   "secret",
		"SMTP_USE_TLS": "invalid_value",
	}
	_, err := LoadFrom(func(k string) string { return mockEnv[k] })
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	// The error should NOT contain the database URL or other secrets.
	if strings.Contains(err.Error(), "super_secret_password") {
		t.Fatal("error message contains sensitive information")
	}
}

func TestConfigValidation(t *testing.T) {
	// Monetization true should fail validation
	cfg1 := Config{PlatformMonetizationEnabled: true}
	err1 := cfg1.Validate()
	if err1 == nil {
		t.Fatal("expected error for monetization enabled, got nil")
	}
	if !strings.Contains(err1.Error(), "PLATFORM_MONETIZATION_ENABLED=true is strictly prohibited") {
		t.Fatalf("unexpected error message: %v", err1)
	}

	// Monetization false should pass
	cfg2 := Config{PlatformMonetizationEnabled: false}
	err2 := cfg2.Validate()
	if err2 != nil {
		t.Fatalf("expected no error for monetization disabled, got: %v", err2)
	}
}

func TestConfigLoadFrom_RejectsMonetizationBeforeCallersCanOpenDatabase(t *testing.T) {
	mockEnv := map[string]string{
		"DATABASE_URL":                  "postgres://user:pass@localhost/db",
		"JWT_SECRET":                    "secret",
		"PLATFORM_MONETIZATION_ENABLED": "true",
	}

	_, err := LoadFrom(func(k string) string { return mockEnv[k] })
	if err == nil {
		t.Fatal("expected Phase 4 monetization guard error")
	}
	if !strings.Contains(err.Error(), "PLATFORM_MONETIZATION_ENABLED=true is strictly prohibited") {
		t.Fatalf("unexpected error: %v", err)
	}
}
