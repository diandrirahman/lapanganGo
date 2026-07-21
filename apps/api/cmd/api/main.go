package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
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
	Listener          func(ctx context.Context, cfg config.Config, engine *gin.Engine) error
}

type httpServer interface {
	ListenAndServe() error
	Shutdown(context.Context) error
	Close() error
}

const serverShutdownTimeout = 5 * time.Second

// runHTTPServer owns only the HTTP server lifecycle. Worker cancellation is
// deliberately owned by Bootstrap so every post-dependency failure path is
// cancelled exactly once and can be tested without process exit.
func runHTTPServer(ctx context.Context, srv httpServer) error {
	serveErr := make(chan error, 1)
	go func() {
		serveErr <- srv.ListenAndServe()
	}()

	select {
	case err := <-serveErr:
		if errors.Is(err, http.ErrServerClosed) {
			if ctx.Err() != nil {
				return nil
			}
			return errors.New("server_closed_unexpectedly")
		}
		return err
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), serverShutdownTimeout)
		defer cancel()
		if err := srv.Shutdown(shutdownCtx); err != nil {
			_ = srv.Close()
			select {
			case <-serveErr:
			case <-time.After(serverShutdownTimeout):
			}
			return err
		}

		select {
		case err := <-serveErr:
			if errors.Is(err, http.ErrServerClosed) {
				return nil
			}
			return err
		case <-shutdownCtx.Done():
			_ = srv.Close()
			return errors.New("server_shutdown_timeout")
		}
	}
}

func Bootstrap(ctx context.Context, ops StartupOperations) error {
	cfg, err := ops.ConfigLoader()
	if err != nil {
		return errors.New("configuration_load_failed")
	}
	if err := cfg.Validate(); err != nil {
		return errors.New("configuration_invalid")
	}

	dbPool, err := ops.DatabaseOpener(ctx, cfg)
	if err != nil {
		return errors.New("database_setup_failed")
	}
	if dbPool != nil {
		defer dbPool.Close()
	}

	if err := ops.MigrationRunner(cfg.DatabaseURL); err != nil {
		return errors.New("migration_failed")
	}

	if err := ops.SchemaEnsurer(ctx, dbPool); err != nil {
		return errors.New("schema_setup_failed")
	}

	r, workerCancel, err := ops.DependencyBuilder(ctx, cfg, dbPool)
	if err != nil {
		return errors.New("dependency_setup_failed")
	}
	defer workerCancel()

	if err := ops.Listener(ctx, cfg, r); err != nil {
		return errors.New("server_start_failed")
	}

	return nil
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
		Listener: func(ctx context.Context, cfg config.Config, engine *gin.Engine) error {
			srv := &http.Server{
				Addr:    ":" + cfg.AppPort,
				Handler: engine,
			}
			log.Println("Server running on port", cfg.AppPort)
			return runHTTPServer(ctx, srv)
		},
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err := Bootstrap(ctx, ops); err != nil {
		fmt.Fprintln(os.Stderr, "Startup failed:", err.Error())
		os.Exit(1)
	}
}
