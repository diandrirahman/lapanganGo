package main

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"

	"lapangango-api/internal/platformfinance"
)

type mockReconciliationService struct {
	report *platformfinance.ReconciliationReport
	err    error
}

func (m *mockReconciliationService) Reconcile(ctx context.Context, query platformfinance.ReconciliationQuery) (*platformfinance.ReconciliationReport, error) {
	return m.report, m.err
}

func mockRunnerOperations(report *platformfinance.ReconciliationReport, serviceErr error, poolErr error) runnerOperations {
	return runnerOperations{
		openPool: func(ctx context.Context, url string) (*pgxpool.Pool, error) {
			if poolErr != nil {
				return nil, poolErr
			}
			return nil, nil // Mock pool
		},
		closePool: func(pool *pgxpool.Pool) {
		},
		pingPool: func(ctx context.Context, pool *pgxpool.Pool) error {
			return nil
		},
		buildService: func(pool *pgxpool.Pool) platformfinance.ReconciliationService {
			return &mockReconciliationService{
				report: report,
				err:    serviceErr,
			}
		},
	}
}

func TestCLIArgsValidation(t *testing.T) {
	tests := []struct {
		name     string
		args     []string
		exitCode int
		errStr   string
	}{
		{
			name:     "Missing start date",
			args:     []string{"--end-date=2026-07-19"},
			exitCode: 1,
			errStr:   "invalid_arguments",
		},
		{
			name:     "Missing end date",
			args:     []string{"--start-date=2026-07-01"},
			exitCode: 1,
			errStr:   "invalid_arguments",
		},
		{
			name:     "Missing both dates",
			args:     []string{},
			exitCode: 1,
			errStr:   "invalid_arguments",
		},
		{
			name:     "Invalid flag",
			args:     []string{"--invalid-flag"},
			exitCode: 1,
			errStr:   "invalid_arguments",
		},
		{
			name:     "Positional argument",
			args:     []string{"--start-date=2026-07-01", "--end-date=2026-07-19", "positional"},
			exitCode: 1,
			errStr:   "invalid_arguments",
		},
		{
			name:     "Invalid date format",
			args:     []string{"--start-date=01-07-2026", "--end-date=19-07-2026"},
			exitCode: 1,
			errStr:   "invalid_arguments",
		},
		{
			name:     "Start date after end date",
			args:     []string{"--start-date=2026-07-19", "--end-date=2026-07-01"},
			exitCode: 1,
			errStr:   "invalid_arguments",
		},
		{
			name:     "367-day range",
			args:     []string{"--start-date=2025-01-01", "--end-date=2026-01-03"},
			exitCode: 1,
			errStr:   "invalid_arguments",
		},
		{
			name:     "Valid 366-day maximum range",
			args:     []string{"--start-date=2024-01-01", "--end-date=2024-12-31"},
			exitCode: 1, // Exits with setup_failed because getenv gives dummy URL and openPool fails mock
			errStr:   "setup_failed",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var stdout bytes.Buffer
			var stderr bytes.Buffer

			var poolOpened bool
			ops := mockRunnerOperations(nil, nil, nil)
			ops.openPool = func(ctx context.Context, url string) (*pgxpool.Pool, error) {
				poolOpened = true
				return nil, os.ErrPermission // force setup_failed for valid dates
			}

			getenv := func(key string) string { return "postgres://dummy:dummy@localhost:5432/dummy" }
			code := run(tt.args, getenv, &stdout, &stderr, ops)

			assert.Equal(t, tt.exitCode, code)
			assert.Contains(t, stderr.String(), tt.errStr)
			assert.Empty(t, stdout.String())

			if tt.name == "Valid 366-day maximum range" {
				assert.True(t, poolOpened, "openPool should be called on valid args")
			} else {
				assert.False(t, poolOpened, "openPool should not be called on invalid args")
			}
		})
	}
}

func TestCLIDBSetupFailure(t *testing.T) {
	t.Run("Missing database URL", func(t *testing.T) {
		var stdout, stderr bytes.Buffer
		getenv := func(key string) string { return "" } // URL missing
		ops := mockRunnerOperations(nil, nil, nil)
		code := run([]string{"--start-date=2026-07-01", "--end-date=2026-07-19"}, getenv, &stdout, &stderr, ops)

		assert.Equal(t, 1, code)
		assert.Contains(t, stderr.String(), "setup_failed")
		assert.Empty(t, stdout.String())
	})

	t.Run("Pool open failure", func(t *testing.T) {
		var stdout, stderr bytes.Buffer
		getenv := func(key string) string { return "postgres://dummy" }
		ops := mockRunnerOperations(nil, nil, os.ErrPermission)
		code := run([]string{"--start-date=2026-07-01", "--end-date=2026-07-19"}, getenv, &stdout, &stderr, ops)

		assert.Equal(t, 1, code)
		assert.Contains(t, stderr.String(), "setup_failed")
		assert.Empty(t, stdout.String())
		assert.NotContains(t, stderr.String(), os.ErrPermission.Error()) // no raw error
	})

	t.Run("Pool ping failure", func(t *testing.T) {
		var stdout, stderr bytes.Buffer
		getenv := func(key string) string { return "postgres://dummy" }
		ops := mockRunnerOperations(nil, nil, nil)
		ops.pingPool = func(ctx context.Context, pool *pgxpool.Pool) error { return os.ErrPermission }
		code := run([]string{"--start-date=2026-07-01", "--end-date=2026-07-19"}, getenv, &stdout, &stderr, ops)

		assert.Equal(t, 1, code)
		assert.Contains(t, stderr.String(), "setup_failed")
		assert.Empty(t, stdout.String())
		assert.NotContains(t, stderr.String(), os.ErrPermission.Error()) // no raw error
	})
}

// Package scope errWriter
type errWriter struct{}

func (errWriter) Write(p []byte) (n int, err error) {
	return 0, os.ErrPermission
}

func TestCLIExitCodes(t *testing.T) {
	getenv := func(key string) string { return "postgres://dummy" }
	asOf := time.Date(2026, 7, 19, 10, 0, 0, 0, time.UTC)

	t.Run("Clean report", func(t *testing.T) {
		var stdout, stderr bytes.Buffer
		report := &platformfinance.ReconciliationReport{
			Clean:  true,
			AsOf:   asOf,
			Status: platformfinance.ReconciliationClean,
		}
		ops := mockRunnerOperations(report, nil, nil)
		code := run([]string{"--start-date=2026-07-01", "--end-date=2026-07-19"}, getenv, &stdout, &stderr, ops)

		assert.Equal(t, 0, code)
		assert.Empty(t, stderr.String())
		assert.Contains(t, stdout.String(), `"clean":true`)

		var parsed CLIOutput
		err := json.Unmarshal(stdout.Bytes(), &parsed)
		assert.NoError(t, err)
		assert.Equal(t, "1", parsed.Version)
	})

	t.Run("Exception report", func(t *testing.T) {
		var stdout, stderr bytes.Buffer
		report := &platformfinance.ReconciliationReport{
			Clean:  false,
			AsOf:   asOf,
			Status: platformfinance.ReconciliationExceptions,
		}
		ops := mockRunnerOperations(report, nil, nil)
		code := run([]string{"--start-date=2026-07-01", "--end-date=2026-07-19"}, getenv, &stdout, &stderr, ops)

		assert.Equal(t, 1, code)
		assert.Empty(t, stderr.String())
		assert.Contains(t, stdout.String(), `"clean":false`)
	})

	t.Run("Nil report with nil error", func(t *testing.T) {
		var stdout, stderr bytes.Buffer
		ops := mockRunnerOperations(nil, nil, nil) // Service returns nil, nil
		code := run([]string{"--start-date=2026-07-01", "--end-date=2026-07-19"}, getenv, &stdout, &stderr, ops)

		assert.Equal(t, 1, code)
		assert.Contains(t, stderr.String(), "reconciliation_failed")
		assert.Empty(t, stdout.String())
	})

	t.Run("Serialization failure", func(t *testing.T) {
		var w errWriter
		var stderr bytes.Buffer

		report := &platformfinance.ReconciliationReport{
			Clean:  true,
			AsOf:   asOf,
			Status: platformfinance.ReconciliationClean,
		}
		ops := mockRunnerOperations(report, nil, nil)
		code := run([]string{"--start-date=2026-07-01", "--end-date=2026-07-19"}, getenv, w, &stderr, ops)

		assert.Equal(t, 1, code)
		assert.Contains(t, stderr.String(), "serialization_failed")
	})
}

// Package scope rawErr
type rawErr string

func (e rawErr) Error() string { return string(e) }

func TestCLISanitizedErrorOutput(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	getenv := func(key string) string { return "postgres://user:secret@host/db" }

	errWithSensitiveInfo := rawErr("sql error: duplicate key in postgres://user:secret@host/db for email user@example.com booking 30129a00-1111-1111-1111-000000000000")
	ops := mockRunnerOperations(nil, errWithSensitiveInfo, nil)

	code := run([]string{"--start-date=2026-07-01", "--end-date=2026-07-19"}, getenv, &stdout, &stderr, ops)

	assert.Equal(t, 1, code)
	assert.Contains(t, stderr.String(), "reconciliation_failed")
	assert.NotContains(t, stderr.String(), "postgres://user:secret@host/db")
	assert.NotContains(t, stderr.String(), "user@example.com")
	assert.NotContains(t, stderr.String(), "30129a00-")
	assert.NotContains(t, stderr.String(), "sql error")
	assert.Empty(t, stdout.String())
}

func TestCLIOutputOmitsSensitiveReason(t *testing.T) {
	var stdout, stderr bytes.Buffer
	getenv := func(key string) string { return "postgres://dummy" }

	asOf := time.Date(2026, 7, 19, 10, 0, 0, 0, time.UTC)
	report := &platformfinance.ReconciliationReport{
		Clean: false,
		AsOf:  asOf,
		Checks: []platformfinance.ReconciliationCheckResult{
			{
				Code: platformfinance.ReconciliationCheckOnline,
				Exceptions: []platformfinance.ReconciliationException{
					{
						Metric: "test",
						Reason: "postgres://secret, user@example.com, UUID",
					},
				},
			},
		},
	}

	ops := mockRunnerOperations(report, nil, nil)
	run([]string{"--start-date=2026-07-01", "--end-date=2026-07-19"}, getenv, &stdout, &stderr, ops)

	assert.NotContains(t, stdout.String(), "postgres://secret")
	assert.NotContains(t, stdout.String(), "user@example.com")
}

func TestCLIDeterministicJSONSerialization(t *testing.T) {
	var stdout1, stderr1 bytes.Buffer
	var stdout2, stderr2 bytes.Buffer
	getenv := func(key string) string { return "postgres://dummy" }

	asOf := time.Date(2026, 7, 19, 10, 0, 0, 0, time.UTC)

	c1 := platformfinance.ReconciliationCheckResult{
		Code:   platformfinance.ReconciliationCheckOnline,
		Status: platformfinance.ReconciliationPass,
		Exceptions: []platformfinance.ReconciliationException{
			{BucketDate: "2026-07-10", Metric: "B", DifferenceCount: 1, DifferenceRupiah: 100},
			{BucketDate: "2026-07-09", Metric: "A", DifferenceCount: 1, DifferenceRupiah: 100},
			{BucketDate: "2026-07-10", Metric: "A", DifferenceCount: 1, DifferenceRupiah: 100},
		},
	}
	c2 := platformfinance.ReconciliationCheckResult{
		Code:   platformfinance.ReconciliationCheckRefund,
		Status: platformfinance.ReconciliationFail,
	}

	// report1 has checks and exceptions in one order
	report1 := &platformfinance.ReconciliationReport{
		Period:   platformfinance.Period{StartDate: "2026-07-01", EndDate: "2026-07-19"},
		Timezone: "Asia/Jakarta",
		AsOf:     asOf,
		Status:   platformfinance.ReconciliationExceptions,
		Clean:    false,
		Checks:   []platformfinance.ReconciliationCheckResult{c1, c2},
	}

	// c1_alt has identical data to c1 but different exception order
	c1_alt := platformfinance.ReconciliationCheckResult{
		Code:   platformfinance.ReconciliationCheckOnline,
		Status: platformfinance.ReconciliationPass,
		Exceptions: []platformfinance.ReconciliationException{
			{BucketDate: "2026-07-10", Metric: "A", DifferenceCount: 1, DifferenceRupiah: 100},
			{BucketDate: "2026-07-10", Metric: "B", DifferenceCount: 1, DifferenceRupiah: 100},
			{BucketDate: "2026-07-09", Metric: "A", DifferenceCount: 1, DifferenceRupiah: 100},
		},
	}
	// report2 has checks reversed and exceptions reordered
	report2 := &platformfinance.ReconciliationReport{
		Period:   platformfinance.Period{StartDate: "2026-07-01", EndDate: "2026-07-19"},
		Timezone: "Asia/Jakarta",
		AsOf:     asOf,
		Status:   platformfinance.ReconciliationExceptions,
		Clean:    false,
		Checks:   []platformfinance.ReconciliationCheckResult{c2, c1_alt},
	}

	// Run with report1
	ops1 := mockRunnerOperations(report1, nil, nil)
	code1 := run([]string{"--start-date=2026-07-01", "--end-date=2026-07-19"}, getenv, &stdout1, &stderr1, ops1)

	// Run with report2
	ops2 := mockRunnerOperations(report2, nil, nil)
	code2 := run([]string{"--start-date=2026-07-01", "--end-date=2026-07-19"}, getenv, &stdout2, &stderr2, ops2)

	assert.Equal(t, 1, code1)
	assert.Equal(t, 1, code2)

	out1 := stdout1.String()
	out2 := stdout2.String()
	assert.Equal(t, out1, out2, "JSON bytes must be completely deterministic regardless of input report order")

	// Verify it's exactly one JSON object
	var parsed map[string]interface{}
	err := json.Unmarshal(stdout1.Bytes(), &parsed)
	assert.NoError(t, err)
	assert.Equal(t, "1", parsed["version"])

	reportMap := parsed["report"].(map[string]interface{})
	assert.Equal(t, "2026-07-19T10:00:00Z", reportMap["as_of"]) // Must be RFC3339 UTC
}
