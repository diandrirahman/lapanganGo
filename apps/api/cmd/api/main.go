package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"lapangango-api/internal/config"
	"lapangango-api/internal/database"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"
)

type StartupOperations struct {
	ConfigLoader      func() (config.Config, error)
	DatabaseOpener    func(ctx context.Context, cfg config.Config) (*pgxpool.Pool, error)
	MigrationRunner   func(dbURL string) error
	SchemaEnsurer     func(ctx context.Context, dbPool *pgxpool.Pool) error
	DependencyBuilder func(ctx context.Context, cfg config.Config, dbPool *pgxpool.Pool) (*gin.Engine, context.CancelFunc, error)
	Listener          func(cfg config.Config, engine *gin.Engine, workerCancel context.CancelFunc) error
}

func Bootstrap(ctx context.Context, ops StartupOperations) error {
	cfg, err := ops.ConfigLoader()
	if err != nil {
		return fmt.Errorf("configuration error: %w", err)
	}
	if err := cfg.Validate(); err != nil {
		return fmt.Errorf("configuration validation error: %w", err)
	}

	dbPool, err := ops.DatabaseOpener(ctx, cfg)
	if err != nil {
		return fmt.Errorf("failed to connect to PostgreSQL: %s", sanitize(err.Error(), cfg.DatabaseURL))
	}
	if dbPool != nil {
		defer dbPool.Close()
	}

	if err := ops.MigrationRunner(cfg.DatabaseURL); err != nil {
		return fmt.Errorf("migration failed: %s", sanitize(err.Error(), cfg.DatabaseURL))
	}

	if err := ops.SchemaEnsurer(ctx, dbPool); err != nil {
		return fmt.Errorf("failed to ensure booking schema: %s", sanitize(err.Error(), cfg.DatabaseURL))
	}

	r, workerCancel, err := ops.DependencyBuilder(ctx, cfg, dbPool)
	if err != nil {
		return fmt.Errorf("failed to build dependencies: %s", sanitize(err.Error(), cfg.DatabaseURL))
	}

	if err := ops.Listener(cfg, r, workerCancel); err != nil {
		return fmt.Errorf("server error: %s", sanitize(err.Error(), cfg.DatabaseURL))
	}

	return nil
}

func sanitize(msg, secret string) string {
	if secret == "" {
		return msg
	}
	return strings.ReplaceAll(msg, secret, "[REDACTED_DSN]")
}

func main() {
	ops := StartupOperations{
		ConfigLoader: config.Load,
		DatabaseOpener: func(ctx context.Context, cfg config.Config) (*pgxpool.Pool, error) {
			return database.NewPostgresPool(ctx, cfg.DatabaseURL)
		},
		MigrationRunner: func(dbURL string) error {
			log.Println("Starting database migrations...")
			err := database.RunMigrations(dbURL)
			if err == nil {
				log.Println("Database migrations completed successfully.")
			}
			return err
		},
		SchemaEnsurer: func(ctx context.Context, dbPool *pgxpool.Pool) error {
			return database.EnsureBookingSchema(ctx, dbPool)
		},
		DependencyBuilder: func(ctx context.Context, cfg config.Config, dbPool *pgxpool.Pool) (*gin.Engine, context.CancelFunc, error) {
			r, workerCancel, err := setupRouter(ctx, cfg, dbPool, true)
			return r, workerCancel, err
		},
		Listener: func(cfg config.Config, engine *gin.Engine, workerCancel context.CancelFunc) error {
			srv := &http.Server{
				Addr:    ":" + cfg.AppPort,
				Handler: engine,
			}

			go func() {
				log.Println("Server running on port", cfg.AppPort)
				if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
					log.Fatalf("Failed to run server: %v", err)
				}
			}()

			quit := make(chan os.Signal, 1)
			signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
			<-quit
			log.Println("Shutting down server...")
			workerCancel()

			ctxShutdown, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()

			if err := srv.Shutdown(ctxShutdown); err != nil {
				return fmt.Errorf("server forced to shutdown: %w", err)
			}
			log.Println("Server exiting")
			return nil
		},
	}

	ctx := context.Background()
	if err := Bootstrap(ctx, ops); err != nil {
		fmt.Fprintln(os.Stderr, "Startup failed:", err.Error())
		os.Exit(1)
	}
}
