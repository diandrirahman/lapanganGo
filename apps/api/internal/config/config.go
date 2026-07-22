package config

import (
	"errors"
	"fmt"
	"os"
	"strconv"

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
}

var ErrInvalidBooleanConfiguration = errors.New("invalid boolean configuration: must be exact lowercase 'true' or 'false' (or unset)")

func (c *Config) Validate() error {
	if c.PlatformMonetizationEnabled {
		return errors.New("PLATFORM_MONETIZATION_ENABLED=true is strictly prohibited during Phase 4 across all environments")
	}
	return nil
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
	return LoadFrom(os.Getenv)
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
	}

	if err := cfg.Validate(); err != nil {
		return Config{}, err
	}
	return cfg, nil
}
