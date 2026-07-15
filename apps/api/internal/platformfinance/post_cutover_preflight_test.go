package platformfinance

import (
	"context"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/stretchr/testify/require"
)

type detectorPreflightQueryer struct {
	now       time.Time
	cutoverAt time.Time
	createdAt time.Time
	triggerOK bool
}

func (q detectorPreflightQueryer) Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error) {
	return nil, ErrPostCutoverDetectorIntegrity
}

func (q detectorPreflightQueryer) QueryRow(ctx context.Context, sql string, args ...any) pgx.Row {
	switch {
	case strings.Contains(sql, "SHOW transaction_read_only"):
		return detectorPreflightRow{values: []any{"on"}}
	case strings.Contains(sql, "SHOW transaction_isolation"):
		return detectorPreflightRow{values: []any{"repeatable read"}}
	case strings.Contains(sql, "SELECT version, dirty"):
		return detectorPreflightRow{values: []any{21, false}}
	case strings.Contains(sql, "SELECT COUNT(*) FROM platform_finance_cutovers"):
		return detectorPreflightRow{values: []any{1}}
	case strings.Contains(sql, "FROM platform_finance_cutovers"):
		return detectorPreflightRow{values: []any{q.cutoverAt, BookingFeeCalculationVersionV1, "test-release", uuid.New(), q.createdAt}}
	case strings.Contains(sql, "SELECT clock_timestamp()"):
		return detectorPreflightRow{values: []any{q.now}}
	case strings.Contains(sql, "FROM pg_trigger"):
		return detectorPreflightRow{values: []any{q.triggerOK}}
	default:
		return detectorPreflightRow{err: ErrPostCutoverDetectorIntegrity}
	}
}

type detectorPreflightRow struct {
	values []any
	err    error
}

func (r detectorPreflightRow) Scan(dest ...any) error {
	if r.err != nil {
		return r.err
	}
	if len(dest) != len(r.values) {
		return ErrPostCutoverDetectorIntegrity
	}
	for i, target := range dest {
		value := reflect.ValueOf(target)
		if value.Kind() != reflect.Pointer || value.IsNil() {
			return ErrPostCutoverDetectorIntegrity
		}
		source := reflect.ValueOf(r.values[i])
		if !source.IsValid() || !source.Type().AssignableTo(value.Elem().Type()) {
			return ErrPostCutoverDetectorIntegrity
		}
		value.Elem().Set(source)
	}
	return nil
}

func TestLoadPostCutoverDetectorPreflightRejectsInvalidTimestampOrdering(t *testing.T) {
	now := time.Date(2026, 7, 15, 12, 0, 0, 0, time.UTC)
	tests := []struct {
		name      string
		cutoverAt time.Time
		createdAt time.Time
	}{
		{
			name:      "future cutover",
			cutoverAt: now.Add(time.Hour),
			createdAt: now,
		},
		{
			name:      "created before cutover",
			cutoverAt: now.Add(-time.Hour),
			createdAt: now.Add(-2 * time.Hour),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := LoadPostCutoverDetectorPreflight(context.Background(), detectorPreflightQueryer{
				now:       now,
				cutoverAt: tt.cutoverAt,
				createdAt: tt.createdAt,
				triggerOK: true,
			})
			require.ErrorIs(t, err, ErrPostCutoverDetectorIntegrity)
		})
	}
}

func TestLoadPostCutoverDetectorPreflightAcceptsValidTimestampOrdering(t *testing.T) {
	now := time.Date(2026, 7, 15, 12, 0, 0, 0, time.UTC)
	cutoverAt := now.Add(-time.Hour)

	preflight, err := LoadPostCutoverDetectorPreflight(context.Background(), detectorPreflightQueryer{
		now:       now,
		cutoverAt: cutoverAt,
		createdAt: now,
		triggerOK: true,
	})

	require.NoError(t, err)
	require.Equal(t, cutoverAt, preflight.CutoverAt)
}
