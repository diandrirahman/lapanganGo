package main

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

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
			expectErr: "simulated load error",
		},
		{
			name:      "Monetization validation error halts startup",
			cfg:       config.Config{PlatformMonetizationEnabled: true},
			loadErr:   nil,
			expectErr: "strictly prohibited during Phase 4",
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
				Listener: func(cfg config.Config, engine *gin.Engine, workerCancel context.CancelFunc) error {
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
			return nil, func() {}, nil
		},
		Listener: func(cfg config.Config, engine *gin.Engine, workerCancel context.CancelFunc) error {
			listenerCalled++
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
	
	if !strings.Contains(err.Error(), "[REDACTED_DSN]") {
		t.Errorf("expected error output to contain [REDACTED_DSN], got: %v", err)
	}
}
