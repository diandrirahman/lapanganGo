package config

import (
	"log"
	"os"
	"strconv"

	"github.com/joho/godotenv"
)

type Config struct {
	AppPort                           string
	DatabaseURL                       string
	JWTSecret                         string
	JWTExpiresInHours                 int
	BookingPaymentTTLMinutes          int
	BookingExpirySweepIntervalSeconds int
	RedisURL                          string
	GeneralRateLimitPerMinute         int
	AuthRateLimitPerMinute            int
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

	return Config{
		AppPort:                           appPort,
		DatabaseURL:                       databaseURL,
		JWTSecret:                         jwtSecret,
		JWTExpiresInHours:                 jwtExpiresInHours,
		BookingPaymentTTLMinutes:          bookingPaymentTTLMinutes,
		BookingExpirySweepIntervalSeconds: bookingExpirySweepIntervalSeconds,
		RedisURL:                          redisURL,
		GeneralRateLimitPerMinute:         generalRateLimitPerMinute,
		AuthRateLimitPerMinute:            authRateLimitPerMinute,
	}
}
