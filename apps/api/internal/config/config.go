package config

import (
	"log"
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
}

func Load() Config {
	err := godotenv.Load()
	if err != nil {
		log.Println("No .env file found, using system environment")
	}

	appPort := os.Getenv("APP_PORT")
	if appPort == "" {
		appPort = "8080"
	}

	databaseURL := os.Getenv("DATABASE_URL")
	if databaseURL == "" {
		log.Fatal("DATABASE_URL is required")
	}

	jwtSecret := os.Getenv("JWT_SECRET")
	if jwtSecret == "" {
		log.Fatal("JWT_SECRET is required")
	}

	jwtExpiresInHours := 24
	if value := os.Getenv("JWT_EXPIRES_IN_HOURS"); value != "" {
		parsedValue, err := strconv.Atoi(value)
		if err != nil || parsedValue <= 0 {
			log.Fatal("JWT_EXPIRES_IN_HOURS must be a positive number")
		}
		jwtExpiresInHours = parsedValue
	}

	bookingPaymentTTLMinutes := 30
	if value := os.Getenv("BOOKING_PAYMENT_TTL_MINUTES"); value != "" {
		parsedValue, err := strconv.Atoi(value)
		if err != nil || parsedValue <= 0 {
			log.Fatal("BOOKING_PAYMENT_TTL_MINUTES must be a positive number")
		}
		bookingPaymentTTLMinutes = parsedValue
	}

	bookingExpirySweepIntervalSeconds := 60
	if value := os.Getenv("BOOKING_EXPIRY_SWEEP_INTERVAL_SECONDS"); value != "" {
		parsedValue, err := strconv.Atoi(value)
		if err != nil || parsedValue <= 0 {
			log.Fatal("BOOKING_EXPIRY_SWEEP_INTERVAL_SECONDS must be a positive number")
		}
		bookingExpirySweepIntervalSeconds = parsedValue
	}

	bookingAutoCompleteIntervalSeconds := 300
	if value := os.Getenv("BOOKING_AUTO_COMPLETE_INTERVAL_SECONDS"); value != "" {
		parsedValue, err := strconv.Atoi(value)
		if err != nil || parsedValue <= 0 {
			log.Fatal("BOOKING_AUTO_COMPLETE_INTERVAL_SECONDS must be a positive number")
		}
		bookingAutoCompleteIntervalSeconds = parsedValue
	}

	redisURL := os.Getenv("REDIS_URL")

	generalRateLimitPerMinute := 100
	if value := os.Getenv("GENERAL_RATE_LIMIT_PER_MINUTE"); value != "" {
		if parsedValue, err := strconv.Atoi(value); err == nil && parsedValue > 0 {
			generalRateLimitPerMinute = parsedValue
		}
	}

	authRateLimitPerMinute := 100
	if value := os.Getenv("AUTH_RATE_LIMIT_PER_MINUTE"); value != "" {
		if parsedValue, err := strconv.Atoi(value); err == nil && parsedValue > 0 {
			authRateLimitPerMinute = parsedValue
		}
	}

	emailDeliveryEnabled := os.Getenv("EMAIL_DELIVERY_ENABLED") == "true"
	smtpHost := os.Getenv("SMTP_HOST")
	smtpPort := 587
	if value := os.Getenv("SMTP_PORT"); value != "" {
		if parsedValue, err := strconv.Atoi(value); err == nil && parsedValue > 0 {
			smtpPort = parsedValue
		}
	}
	smtpUsername := os.Getenv("SMTP_USERNAME")
	smtpPassword := os.Getenv("SMTP_PASSWORD")
	
	smtpFromName := os.Getenv("SMTP_FROM_NAME")
	if smtpFromName == "" {
		smtpFromName = "LapangGo"
	}

	smtpFromEmail := os.Getenv("SMTP_FROM_EMAIL")
	if smtpFromEmail == "" {
		smtpFromEmail = "no-reply@lapanggo.local"
	}

	smtpUseTLS := os.Getenv("SMTP_USE_TLS") == "true"
	if os.Getenv("SMTP_USE_TLS") == "" {
		smtpUseTLS = true // Default to true if not specified, except maybe for local mailpit.
	}
	
	frontendBaseURL := os.Getenv("FRONTEND_BASE_URL")
	if frontendBaseURL == "" {
		frontendBaseURL = "http://localhost:3000"
	}

	if emailDeliveryEnabled && smtpHost == "" {
		log.Fatal("SMTP_HOST is required when EMAIL_DELIVERY_ENABLED is true")
	}

	return Config{
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
	}
}
