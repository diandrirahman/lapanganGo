package config

import (
	"log"
	"os"
	"strconv"

	"github.com/joho/godotenv"
)

type Config struct {
	AppPort           string
	DatabaseURL       string
	JWTSecret         string
	JWTExpiresInHours int
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

	return Config{
		AppPort:           appPort,
		DatabaseURL:       databaseURL,
		JWTSecret:         jwtSecret,
		JWTExpiresInHours: jwtExpiresInHours,
	}
}
