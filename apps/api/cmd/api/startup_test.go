package main

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"lapangango-api/internal/config"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"
)

// mock pgxpool cannot be directly mocked without an interface,
// so we'll just pass nil for the pool since our test functions don't use it.

func TestStartup_InvalidConfig(t *testing.T) {
	cases := []struct {
		name      string
		cfg       config.Config
		loadErr   error
		expectErr string
	}{
		{
			name:      "Load error halts startup",
			cfg:       config.Config{},
			loadErr:   errors.New("simulated load error"),
			expectErr: "configuration_load_failed",
		},
		{
			name:      "Monetization validation error halts startup",
			cfg:       config.Config{PlatformMonetizationEnabled: true},
			loadErr:   nil,
			expectErr: "configuration_invalid",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dbOpened := 0
			migrationsRun := 0
			schemaEnsured := 0
			depsBuilt := 0
			listenerCalled := 0

			ops := StartupOperations{
				ConfigLoader: func() (config.Config, error) {
					return tc.cfg, tc.loadErr
				},
				DatabaseOpener: func(ctx context.Context, cfg config.Config) (*pgxpool.Pool, error) {
					dbOpened++
					return nil, nil
				},
				MigrationRunner: func(dbURL string) error {
					migrationsRun++
					return nil
				},
				SchemaEnsurer: func(ctx context.Context, dbPool *pgxpool.Pool) error {
					schemaEnsured++
					return nil
				},
				DependencyBuilder: func(ctx context.Context, cfg config.Config, dbPool *pgxpool.Pool) (*gin.Engine, context.CancelFunc, error) {
					depsBuilt++
					return nil, func() {}, nil
				},
				Listener: func(context.Context, config.Config, *gin.Engine) error {
					listenerCalled++
					return nil
				},
			}

			err := Bootstrap(context.Background(), ops)
			if err == nil {
				t.Fatalf("expected error containing %q, got nil", tc.expectErr)
			}
			if !strings.Contains(err.Error(), tc.expectErr) {
				t.Fatalf("expected error containing %q, got %q", tc.expectErr, err.Error())
			}

			if dbOpened != 0 || migrationsRun != 0 || schemaEnsured != 0 || depsBuilt != 0 || listenerCalled != 0 {
				t.Errorf("expected 0 side effects, got dbOpened=%d, migrationsRun=%d, schemaEnsured=%d, depsBuilt=%d, listenerCalled=%d",
					dbOpened, migrationsRun, schemaEnsured, depsBuilt, listenerCalled)
			}
		})
	}
}

func TestStartup_ValidConfigPassesThrough(t *testing.T) {
	dbOpened := 0
	migrationsRun := 0
	schemaEnsured := 0
	depsBuilt := 0
	listenerCalled := 0
	workerCancelled := 0
	listenerReturned := false

	ops := StartupOperations{
		ConfigLoader: func() (config.Config, error) {
			return config.Config{DatabaseURL: "dummy"}, nil
		},
		DatabaseOpener: func(ctx context.Context, cfg config.Config) (*pgxpool.Pool, error) {
			dbOpened++
			return nil, nil
		},
		MigrationRunner: func(dbURL string) error {
			migrationsRun++
			return nil
		},
		SchemaEnsurer: func(ctx context.Context, dbPool *pgxpool.Pool) error {
			schemaEnsured++
			return nil
		},
		DependencyBuilder: func(ctx context.Context, cfg config.Config, dbPool *pgxpool.Pool) (*gin.Engine, context.CancelFunc, error) {
			depsBuilt++
			return nil, func() {
				if !listenerReturned {
					t.Errorf("worker cancellation happened before listener returned")
				}
				workerCancelled++
			}, nil
		},
		Listener: func(context.Context, config.Config, *gin.Engine) error {
			listenerCalled++
			listenerReturned = true
			return nil
		},
	}

	err := Bootstrap(context.Background(), ops)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if dbOpened != 1 || migrationsRun != 1 || schemaEnsured != 1 || depsBuilt != 1 || listenerCalled != 1 {
		t.Errorf("expected all side effects exactly once, got dbOpened=%d, migrationsRun=%d, schemaEnsured=%d, depsBuilt=%d, listenerCalled=%d",
			dbOpened, migrationsRun, schemaEnsured, depsBuilt, listenerCalled)
	}
	if workerCancelled != 1 {
		t.Fatalf("expected exactly one worker cancellation after clean listener return, got %d", workerCancelled)
	}
}

func TestStartup_ErrorSanitization(t *testing.T) {
	fakeDSN := "postgres://secret_user:super_secret_password@localhost:5432/db"
	cfg := config.Config{DatabaseURL: fakeDSN}

	ops := StartupOperations{
		ConfigLoader: func() (config.Config, error) { return cfg, nil },
		DatabaseOpener: func(ctx context.Context, cfg config.Config) (*pgxpool.Pool, error) {
			return nil, fmt.Errorf("connection failed to %s because network error", cfg.DatabaseURL)
		},
	}

	err := Bootstrap(context.Background(), ops)
	if err == nil {
		t.Fatal("expected error, got nil")
	}

	if strings.Contains(err.Error(), "super_secret_password") || strings.Contains(err.Error(), fakeDSN) {
		t.Errorf("error output leaked secret DSN! Output: %v", err)
	}

	if err.Error() != "database_setup_failed" {
		t.Errorf("expected stable database setup error, got: %v", err)
	}
	for _, leaked := range []string{"super_secret_password", "secret_user", "localhost:5432", "sslmode=disable"} {
		if strings.Contains(err.Error(), leaked) {
			t.Errorf("error output leaked %q: %v", leaked, err)
		}
	}
}

func TestStartup_ErrorSanitizationDoesNotDependOnFullDSN(t *testing.T) {
	cfg := config.Config{DatabaseURL: "postgres://secret_user:super_secret_password@localhost:5432/db?sslmode=disable"}
	ops := StartupOperations{
		ConfigLoader: func() (config.Config, error) { return cfg, nil },
		DatabaseOpener: func(context.Context, config.Config) (*pgxpool.Pool, error) {
			return nil, errors.New("password authentication failed for user secret_user at localhost:5432")
		},
	}

	err := Bootstrap(context.Background(), ops)
	if err == nil || err.Error() != "database_setup_failed" {
		t.Fatalf("expected stable database setup error, got %v", err)
	}
}

func TestStartup_ListenerFailureCancelsWorkers(t *testing.T) {
	cancelled := 0
	ops := StartupOperations{
		ConfigLoader:    func() (config.Config, error) { return config.Config{DatabaseURL: "dummy"}, nil },
		DatabaseOpener:  func(context.Context, config.Config) (*pgxpool.Pool, error) { return nil, nil },
		MigrationRunner: func(string) error { return nil },
		SchemaEnsurer:   func(context.Context, *pgxpool.Pool) error { return nil },
		DependencyBuilder: func(context.Context, config.Config, *pgxpool.Pool) (*gin.Engine, context.CancelFunc, error) {
			return gin.New(), func() { cancelled++ }, nil
		},
		Listener: func(context.Context, config.Config, *gin.Engine) error {
			return errors.New("listener provider contains secret details")
		},
	}

	err := Bootstrap(context.Background(), ops)
	if err == nil || err.Error() != "server_start_failed" {
		t.Fatalf("expected stable server start error, got %v", err)
	}
	if cancelled != 1 {
		t.Fatalf("expected worker cancellation on listener failure, got %d", cancelled)
	}
}

func TestBootstrap_ContextCancellationCancelsWorkersExactlyOnce(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	listenerStarted := make(chan struct{})
	listenerReturned := atomic.Bool{}
	workerCancelled := atomic.Int32{}
	result := make(chan error, 1)

	ops := StartupOperations{
		ConfigLoader:    func() (config.Config, error) { return config.Config{DatabaseURL: "dummy"}, nil },
		DatabaseOpener:  func(context.Context, config.Config) (*pgxpool.Pool, error) { return nil, nil },
		MigrationRunner: func(string) error { return nil },
		SchemaEnsurer:   func(context.Context, *pgxpool.Pool) error { return nil },
		DependencyBuilder: func(context.Context, config.Config, *pgxpool.Pool) (*gin.Engine, context.CancelFunc, error) {
			return gin.New(), func() {
				if !listenerReturned.Load() {
					t.Errorf("worker cancellation happened before listener returned")
				}
				workerCancelled.Add(1)
			}, nil
		},
		Listener: func(ctx context.Context, _ config.Config, _ *gin.Engine) error {
			close(listenerStarted)
			<-ctx.Done()
			listenerReturned.Store(true)
			return nil
		},
	}

	go func() { result <- Bootstrap(ctx, ops) }()
	select {
	case <-listenerStarted:
	case <-time.After(time.Second):
		t.Fatal("listener did not start")
	}
	cancel()

	select {
	case err := <-result:
		if err != nil {
			t.Fatalf("expected clean context shutdown, got %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("Bootstrap did not return after context cancellation")
	}
	if got := workerCancelled.Load(); got != 1 {
		t.Fatalf("expected exactly one worker cancellation, got %d", got)
	}
}
