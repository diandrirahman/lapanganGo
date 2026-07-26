package config

import (
	"errors"
	"fmt"
	"net/url"
	"os"
	"strconv"
	"strings"

	"github.com/joho/godotenv"
)

type Config struct {
	AppPort                            string
	DatabaseURL                        string
	JWTSecret                          string
	JWTExpiresInHours                  int
	BookingPaymentTTLMinutes           int
	BookingExpirySweepIntervalSeconds  int
	BookingAutoCompleteIntervalSeconds int
	RedisURL                           string
	GeneralRateLimitPerMinute          int
	AuthRateLimitPerMinute             int

	// Email Config
	EmailDeliveryEnabled bool
	SMTPHost             string
	SMTPPort             int
	SMTPUsername         string
	SMTPPassword         string
	SMTPFromName         string
	SMTPFromEmail        string
	SMTPUseTLS           bool
	FrontendBaseURL      string

	// Phase 4 Feature Flags
	PlatformMonetizationEnabled bool
	PlatformFinanceAdminEnabled bool

	// Phase 5 payment adapter configuration. Secret and token fields are
	// backend-only and must never be copied into API DTOs or logs.
	PaymentSandboxEnabled              bool
	PaymentCreateEnabled               bool
	PaymentInquiryEnabled              bool
	PaymentWebhookIngressEnabled       bool
	PaymentWebhookProcessorEnabled     bool
	PaymentRefundEnabled               bool
	PaymentShadowReconciliationEnabled bool
	PaymentIsolatedTestLedgerEnabled   bool
	PaymentProvider                    string
	PaymentProviderMode                string
	PaymentWebhookContractVersion      string
	PaymentReturnOrigin                string
	XenditSecretKey                    string
	XenditWebhookToken                 string
}

var ErrInvalidBooleanConfiguration = errors.New("invalid boolean configuration: must be exact lowercase 'true' or 'false' (or unset)")
var ErrFrontendProviderSecret = errors.New("frontend VITE_* provider secret/token configuration is prohibited")

func (c *Config) Validate() error {
	if c.PlatformMonetizationEnabled {
		return errors.New("PLATFORM_MONETIZATION_ENABLED=true is strictly prohibited during Phase 4 across all environments")
	}
	if c.PaymentProvider != "" && c.PaymentProvider != "XENDIT" {
		return errors.New("PAYMENT_PROVIDER must be exactly XENDIT")
	}
	if c.PaymentProviderMode != "" && c.PaymentProviderMode != "TEST" {
		return errors.New("PAYMENT_PROVIDER_MODE must be exactly TEST")
	}
	if c.PaymentWebhookContractVersion != "" && c.PaymentWebhookContractVersion != "DISABLED" &&
		c.PaymentWebhookContractVersion != "XENDIT_CALLBACK_TOKEN_V1_PROVISIONAL" &&
		c.PaymentWebhookContractVersion != "XENDIT_CALLBACK_TOKEN_V1_VERIFIED" {
		return errors.New("PAYMENT_WEBHOOK_CONTRACT_VERSION is unsupported")
	}
	if paymentCapabilityEnabled(c) && !c.PaymentSandboxEnabled {
		return errors.New("payment capability flags require PAYMENT_SANDBOX_ENABLED=true")
	}
	if (c.PaymentCreateEnabled || c.PaymentInquiryEnabled || c.PaymentRefundEnabled) && strings.TrimSpace(c.XenditSecretKey) == "" {
		return errors.New("XENDIT_SECRET_KEY is required for enabled payment commands")
	}
	if c.PaymentWebhookIngressEnabled && strings.TrimSpace(c.XenditWebhookToken) == "" {
		return errors.New("XENDIT_WEBHOOK_TOKEN is required when webhook ingress is enabled")
	}
	if c.PaymentWebhookProcessorEnabled && (!c.PaymentWebhookIngressEnabled || c.PaymentWebhookContractVersion != "XENDIT_CALLBACK_TOKEN_V1_VERIFIED") {
		return errors.New("webhook processor requires verified webhook ingress contract")
	}
	if c.PaymentRefundEnabled {
		return errors.New("PAYMENT_REFUND_ENABLED requires the payment facts and outbox prerequisites")
	}
	if c.PaymentIsolatedTestLedgerEnabled {
		return errors.New("PAYMENT_ISOLATED_TEST_LEDGER_ENABLED is restricted to isolated tests")
	}
	if c.PaymentReturnOrigin != "" && !isAllowlistedHTTPSOrigin(c.PaymentReturnOrigin) {
		return errors.New("PAYMENT_RETURN_ORIGIN must be an HTTPS origin without path, query, or fragment")
	}
	return nil
}

func paymentCapabilityEnabled(c *Config) bool {
	return c.PaymentCreateEnabled || c.PaymentInquiryEnabled || c.PaymentWebhookIngressEnabled ||
		c.PaymentWebhookProcessorEnabled || c.PaymentRefundEnabled || c.PaymentShadowReconciliationEnabled ||
		c.PaymentIsolatedTestLedgerEnabled
}

func isAllowlistedHTTPSOrigin(value string) bool {
	parsed, err := url.Parse(value)
	return err == nil && parsed.Scheme == "https" && parsed.Host != "" && parsed.User == nil &&
		parsed.Path == "" && parsed.RawQuery == "" && parsed.Fragment == ""
}

func parseStrictBool(value string) (bool, error) {
	switch value {
	case "":
		return false, nil
	case "false":
		return false, nil
	case "true":
		return true, nil
	default:
		return false, ErrInvalidBooleanConfiguration
	}
}

// Load loads the configuration by reading .env (if it exists) and then using os.Getenv.
func Load() (Config, error) {
	_ = godotenv.Load() // ignore error, as .env is optional
	return LoadFromEnvironment(os.Getenv, os.Environ())
}

// LoadFromEnvironment applies process-wide security gates that cannot be
// expressed through getenv lookups before loading the regular configuration.
func LoadFromEnvironment(getenv func(string) string, environ []string) (Config, error) {
	if err := rejectFrontendProviderSecrets(environ); err != nil {
		return Config{}, err
	}
	return LoadFrom(getenv)
}

func rejectFrontendProviderSecrets(environ []string) error {
	for _, entry := range environ {
		name, _, found := strings.Cut(entry, "=")
		if !found {
			name = entry
		}
		upperName := strings.ToUpper(strings.TrimSpace(name))
		if !strings.HasPrefix(upperName, "VITE_") {
			continue
		}

		isProviderVariable := strings.Contains(upperName, "XENDIT") ||
			strings.Contains(upperName, "PAYMENT") ||
			strings.Contains(upperName, "PROVIDER")
		isSecretVariable := strings.Contains(upperName, "SECRET") ||
			strings.Contains(upperName, "TOKEN") ||
			strings.Contains(upperName, "PRIVATE_KEY") ||
			strings.Contains(upperName, "API_KEY")
		if isProviderVariable && isSecretVariable {
			return ErrFrontendProviderSecret
		}
	}
	return nil
}

// LoadFrom is a pure function that loads configuration using the provided getenv function.
func LoadFrom(getenv func(string) string) (Config, error) {
	appPort := getenv("APP_PORT")
	if appPort == "" {
		appPort = "8080"
	}

	databaseURL := getenv("DATABASE_URL")
	if databaseURL == "" {
		return Config{}, errors.New("DATABASE_URL is required")
	}

	jwtSecret := getenv("JWT_SECRET")
	if jwtSecret == "" {
		return Config{}, errors.New("JWT_SECRET is required")
	}

	jwtExpiresInHours := 24
	if value := getenv("JWT_EXPIRES_IN_HOURS"); value != "" {
		parsedValue, err := strconv.Atoi(value)
		if err != nil || parsedValue <= 0 {
			return Config{}, errors.New("JWT_EXPIRES_IN_HOURS must be a positive number")
		}
		jwtExpiresInHours = parsedValue
	}

	bookingPaymentTTLMinutes := 30
	if value := getenv("BOOKING_PAYMENT_TTL_MINUTES"); value != "" {
		parsedValue, err := strconv.Atoi(value)
		if err != nil || parsedValue <= 0 {
			return Config{}, errors.New("BOOKING_PAYMENT_TTL_MINUTES must be a positive number")
		}
		bookingPaymentTTLMinutes = parsedValue
	}

	bookingExpirySweepIntervalSeconds := 60
	if value := getenv("BOOKING_EXPIRY_SWEEP_INTERVAL_SECONDS"); value != "" {
		parsedValue, err := strconv.Atoi(value)
		if err != nil || parsedValue <= 0 {
			return Config{}, errors.New("BOOKING_EXPIRY_SWEEP_INTERVAL_SECONDS must be a positive number")
		}
		bookingExpirySweepIntervalSeconds = parsedValue
	}

	bookingAutoCompleteIntervalSeconds := 300
	if value := getenv("BOOKING_AUTO_COMPLETE_INTERVAL_SECONDS"); value != "" {
		parsedValue, err := strconv.Atoi(value)
		if err != nil || parsedValue <= 0 {
			return Config{}, errors.New("BOOKING_AUTO_COMPLETE_INTERVAL_SECONDS must be a positive number")
		}
		bookingAutoCompleteIntervalSeconds = parsedValue
	}

	redisURL := getenv("REDIS_URL")

	generalRateLimitPerMinute := 100
	if value := getenv("GENERAL_RATE_LIMIT_PER_MINUTE"); value != "" {
		if parsedValue, err := strconv.Atoi(value); err == nil && parsedValue > 0 {
			generalRateLimitPerMinute = parsedValue
		}
	}

	authRateLimitPerMinute := 100
	if value := getenv("AUTH_RATE_LIMIT_PER_MINUTE"); value != "" {
		if parsedValue, err := strconv.Atoi(value); err == nil && parsedValue > 0 {
			authRateLimitPerMinute = parsedValue
		}
	}

	emailDeliveryEnabled, err := parseStrictBool(getenv("EMAIL_DELIVERY_ENABLED"))
	if err != nil {
		return Config{}, fmt.Errorf("EMAIL_DELIVERY_ENABLED %w", err)
	}

	smtpHost := getenv("SMTP_HOST")
	smtpPort := 587
	if value := getenv("SMTP_PORT"); value != "" {
		if parsedValue, err := strconv.Atoi(value); err == nil && parsedValue > 0 {
			smtpPort = parsedValue
		}
	}
	smtpUsername := getenv("SMTP_USERNAME")
	smtpPassword := getenv("SMTP_PASSWORD")

	smtpFromName := getenv("SMTP_FROM_NAME")
	if smtpFromName == "" {
		smtpFromName = "LapangGo"
	}

	smtpFromEmail := getenv("SMTP_FROM_EMAIL")
	if smtpFromEmail == "" {
		smtpFromEmail = "no-reply@lapanggo.local"
	}

	smtpUseTLS := true
	if value := getenv("SMTP_USE_TLS"); value != "" {
		parsedUseTLS, err := parseStrictBool(value)
		if err != nil {
			return Config{}, fmt.Errorf("SMTP_USE_TLS %w", err)
		}
		smtpUseTLS = parsedUseTLS
	}

	frontendBaseURL := getenv("FRONTEND_BASE_URL")
	if frontendBaseURL == "" {
		frontendBaseURL = "http://localhost:3000"
	}

	if emailDeliveryEnabled && smtpHost == "" {
		return Config{}, errors.New("SMTP_HOST is required when EMAIL_DELIVERY_ENABLED is true")
	}

	platformMonetizationEnabled, err := parseStrictBool(getenv("PLATFORM_MONETIZATION_ENABLED"))
	if err != nil {
		return Config{}, fmt.Errorf("PLATFORM_MONETIZATION_ENABLED %w", err)
	}

	platformFinanceAdminEnabled, err := parseStrictBool(getenv("PLATFORM_FINANCE_ADMIN_ENABLED"))
	if err != nil {
		return Config{}, fmt.Errorf("PLATFORM_FINANCE_ADMIN_ENABLED %w", err)
	}

	paymentSandboxEnabled, err := parseStrictBool(getenv("PAYMENT_SANDBOX_ENABLED"))
	if err != nil {
		return Config{}, fmt.Errorf("PAYMENT_SANDBOX_ENABLED %w", err)
	}
	paymentCreateEnabled, err := parseStrictBool(getenv("PAYMENT_CREATE_ENABLED"))
	if err != nil {
		return Config{}, fmt.Errorf("PAYMENT_CREATE_ENABLED %w", err)
	}
	paymentInquiryEnabled, err := parseStrictBool(getenv("PAYMENT_INQUIRY_ENABLED"))
	if err != nil {
		return Config{}, fmt.Errorf("PAYMENT_INQUIRY_ENABLED %w", err)
	}
	paymentWebhookIngressEnabled, err := parseStrictBool(getenv("PAYMENT_WEBHOOK_INGRESS_ENABLED"))
	if err != nil {
		return Config{}, fmt.Errorf("PAYMENT_WEBHOOK_INGRESS_ENABLED %w", err)
	}
	paymentWebhookProcessorEnabled, err := parseStrictBool(getenv("PAYMENT_WEBHOOK_PROCESSOR_ENABLED"))
	if err != nil {
		return Config{}, fmt.Errorf("PAYMENT_WEBHOOK_PROCESSOR_ENABLED %w", err)
	}
	paymentRefundEnabled, err := parseStrictBool(getenv("PAYMENT_REFUND_ENABLED"))
	if err != nil {
		return Config{}, fmt.Errorf("PAYMENT_REFUND_ENABLED %w", err)
	}
	paymentShadowReconciliationEnabled, err := parseStrictBool(getenv("PAYMENT_SHADOW_RECONCILIATION_ENABLED"))
	if err != nil {
		return Config{}, fmt.Errorf("PAYMENT_SHADOW_RECONCILIATION_ENABLED %w", err)
	}
	paymentIsolatedTestLedgerEnabled, err := parseStrictBool(getenv("PAYMENT_ISOLATED_TEST_LEDGER_ENABLED"))
	if err != nil {
		return Config{}, fmt.Errorf("PAYMENT_ISOLATED_TEST_LEDGER_ENABLED %w", err)
	}

	paymentProvider := getenv("PAYMENT_PROVIDER")
	if paymentProvider == "" {
		paymentProvider = "XENDIT"
	}
	paymentProviderMode := getenv("PAYMENT_PROVIDER_MODE")
	if paymentProviderMode == "" {
		paymentProviderMode = "TEST"
	}
	paymentWebhookContractVersion := getenv("PAYMENT_WEBHOOK_CONTRACT_VERSION")
	if paymentWebhookContractVersion == "" {
		paymentWebhookContractVersion = "DISABLED"
	}

	cfg := Config{
		AppPort:                            appPort,
		DatabaseURL:                        databaseURL,
		JWTSecret:                          jwtSecret,
		JWTExpiresInHours:                  jwtExpiresInHours,
		BookingPaymentTTLMinutes:           bookingPaymentTTLMinutes,
		BookingExpirySweepIntervalSeconds:  bookingExpirySweepIntervalSeconds,
		BookingAutoCompleteIntervalSeconds: bookingAutoCompleteIntervalSeconds,
		RedisURL:                           redisURL,
		GeneralRateLimitPerMinute:          generalRateLimitPerMinute,
		AuthRateLimitPerMinute:             authRateLimitPerMinute,
		EmailDeliveryEnabled:               emailDeliveryEnabled,
		SMTPHost:                           smtpHost,
		SMTPPort:                           smtpPort,
		SMTPUsername:                       smtpUsername,
		SMTPPassword:                       smtpPassword,
		SMTPFromName:                       smtpFromName,
		SMTPFromEmail:                      smtpFromEmail,
		SMTPUseTLS:                         smtpUseTLS,
		FrontendBaseURL:                    frontendBaseURL,
		PlatformMonetizationEnabled:        platformMonetizationEnabled,
		PlatformFinanceAdminEnabled:        platformFinanceAdminEnabled,
		PaymentSandboxEnabled:              paymentSandboxEnabled,
		PaymentCreateEnabled:               paymentCreateEnabled,
		PaymentInquiryEnabled:              paymentInquiryEnabled,
		PaymentWebhookIngressEnabled:       paymentWebhookIngressEnabled,
		PaymentWebhookProcessorEnabled:     paymentWebhookProcessorEnabled,
		PaymentRefundEnabled:               paymentRefundEnabled,
		PaymentShadowReconciliationEnabled: paymentShadowReconciliationEnabled,
		PaymentIsolatedTestLedgerEnabled:   paymentIsolatedTestLedgerEnabled,
		PaymentProvider:                    paymentProvider,
		PaymentProviderMode:                paymentProviderMode,
		PaymentWebhookContractVersion:      paymentWebhookContractVersion,
		PaymentReturnOrigin:                getenv("PAYMENT_RETURN_ORIGIN"),
		XenditSecretKey:                    getenv("XENDIT_SECRET_KEY"),
		XenditWebhookToken:                 getenv("XENDIT_WEBHOOK_TOKEN"),
	}

	if err := cfg.Validate(); err != nil {
		return Config{}, err
	}
	return cfg, nil
}
