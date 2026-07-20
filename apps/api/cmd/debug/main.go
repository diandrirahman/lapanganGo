package main

import (
	"context"
	"fmt"
	"log"

	"golang.org/x/crypto/bcrypt"
	"lapangango-api/internal/config"
	"lapangango-api/internal/database"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatal("invalid application configuration")
	}
	ctx := context.Background()

	dbPool, err := database.NewPostgresPool(ctx, cfg.DatabaseURL)
	if err != nil {
		log.Fatal(err)
	}
	defer dbPool.Close()

	hash, _ := bcrypt.GenerateFromPassword([]byte("password123"), bcrypt.DefaultCost)

	res, err := dbPool.Exec(ctx, "UPDATE users SET password_hash = $1 WHERE email = 'rahman.diandri@gmail.com'", string(hash))
	if err != nil {
		log.Fatal(err)
	}

	fmt.Printf("Rows affected: %d\n", res.RowsAffected())
}
