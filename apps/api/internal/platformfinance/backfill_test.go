package platformfinance

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// MockQueryer for unit testing
type MockBackfillQueryer struct {
	QueryFunc    func(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	QueryRowFunc func(ctx context.Context, sql string, args ...any) pgx.Row
}

func (m *MockBackfillQueryer) Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error) {
	if m.QueryFunc != nil {
		return m.QueryFunc(ctx, sql, args...)
	}
	return nil, errors.New("not implemented")
}

func (m *MockBackfillQueryer) QueryRow(ctx context.Context, sql string, args ...any) pgx.Row {
	if m.QueryRowFunc != nil {
		return m.QueryRowFunc(ctx, sql, args...)
	}
	// Return a stub row that errors out if not implemented
	return mockRow{err: errors.New("not implemented")}
}

type mockRow struct {
	err  error
	args []any
}

func (m mockRow) Scan(dest ...any) error {
	if m.err != nil {
		return m.err
	}
	for i, d := range dest {
		switch ptr := d.(type) {
		case *int:
			if len(m.args) > i {
				*ptr = m.args[i].(int)
			}
		case **time.Time:
			if len(m.args) > i {
				if t, ok := m.args[i].(time.Time); ok {
					*ptr = &t
				} else if m.args[i] == nil {
					*ptr = nil
				}
			}
		}
	}
	return nil
}

// MockRows for unit testing
type MockRows struct {
	rows [][]any
	idx  int
	err  error
}

func (m *MockRows) Next() bool {
	if m.err != nil {
		return false
	}
	if m.idx < len(m.rows) {
		m.idx++
		return true
	}
	return false
}

func (m *MockRows) Scan(dest ...any) error {
	if m.err != nil {
		return m.err
	}
	row := m.rows[m.idx-1]
	for i, d := range dest {
		switch ptr := d.(type) {
		case *uuid.UUID:
			if len(row) > i {
				*ptr = row[i].(uuid.UUID)
			}
		case *string:
			if len(row) > i {
				*ptr = row[i].(string)
			}
		}
	}
	return nil
}

func (m *MockRows) Err() error {
	return m.err
}
func (m *MockRows) Close()                                       {}
func (m *MockRows) FieldDescriptions() []pgconn.FieldDescription { return nil }
func (m *MockRows) CommandTag() pgconn.CommandTag                { return pgconn.CommandTag{} }
func (m *MockRows) Values() ([]any, error)                       { return nil, nil }
func (m *MockRows) RawValues() [][]byte                          { return nil }
func (m *MockRows) Conn() *pgx.Conn                              { return nil }

func TestLoadStoredCutover(t *testing.T) {
	ctx := context.Background()
	now := time.Now().UTC()

	t.Run("nil queryer rejected", func(t *testing.T) {
		_, err := LoadStoredCutover(ctx, nil)
		assert.ErrorIs(t, err, ErrBackfillInvalidInput)
	})

	t.Run("missing cutover", func(t *testing.T) {
		mockDB := &MockBackfillQueryer{
			QueryRowFunc: func(ctx context.Context, sql string, args ...any) pgx.Row {
				return mockRow{args: []any{0}} // count = 0
			},
		}
		_, err := LoadStoredCutover(ctx, mockDB)
		assert.ErrorIs(t, err, ErrBackfillMissingCutover)
	})

	t.Run("duplicate cutover fail-closed", func(t *testing.T) {
		mockDB := &MockBackfillQueryer{
			QueryRowFunc: func(ctx context.Context, sql string, args ...any) pgx.Row {
				return mockRow{args: []any{2}} // count = 2
			},
		}
		_, err := LoadStoredCutover(ctx, mockDB)
		assert.ErrorIs(t, err, ErrBackfillMultipleCutover)
	})

	t.Run("null cutover", func(t *testing.T) {
		var callCount int
		mockDB := &MockBackfillQueryer{
			QueryRowFunc: func(ctx context.Context, sql string, args ...any) pgx.Row {
				callCount++
				if callCount == 1 {
					return mockRow{args: []any{1}}
				}
				return mockRow{args: []any{nil}}
			},
		}
		_, err := LoadStoredCutover(ctx, mockDB)
		assert.ErrorIs(t, err, ErrBackfillMissingCutover)
		assert.Equal(t, 2, callCount)
	})

	t.Run("valid cutover", func(t *testing.T) {
		var callCount int
		mockDB := &MockBackfillQueryer{
			QueryRowFunc: func(ctx context.Context, sql string, args ...any) pgx.Row {
				callCount++
				if callCount == 1 {
					return mockRow{args: []any{1}}
				}
				return mockRow{args: []any{now}}
			},
		}
		res, err := LoadStoredCutover(ctx, mockDB)
		require.NoError(t, err)
		assert.True(t, res.Equal(now))
	})
}

func TestFetchLegacyBackfillCandidates(t *testing.T) {
	ctx := context.Background()
	now := time.Now().UTC()
	cursor := uuid.New()

	t.Run("nil queryer rejected", func(t *testing.T) {
		_, err := FetchLegacyBackfillCandidates(ctx, nil, now, 10, nil)
		assert.ErrorIs(t, err, ErrBackfillInvalidInput)
	})

	t.Run("zero cutover rejected", func(t *testing.T) {
		mockDB := &MockBackfillQueryer{}
		_, err := FetchLegacyBackfillCandidates(ctx, mockDB, time.Time{}, 10, nil)
		assert.ErrorIs(t, err, ErrBackfillInvalidInput)
	})

	t.Run("batch size rejected", func(t *testing.T) {
		mockDB := &MockBackfillQueryer{}
		_, err := FetchLegacyBackfillCandidates(ctx, mockDB, now, 0, nil)
		assert.ErrorIs(t, err, ErrBackfillInvalidInput)
		_, err = FetchLegacyBackfillCandidates(ctx, mockDB, now, -1, nil)
		assert.ErrorIs(t, err, ErrBackfillInvalidInput)
		_, err = FetchLegacyBackfillCandidates(ctx, mockDB, now, 1001, nil)
		assert.ErrorIs(t, err, ErrBackfillInvalidInput)
	})

	t.Run("empty result pagination", func(t *testing.T) {
		mockDB := &MockBackfillQueryer{
			QueryFunc: func(ctx context.Context, sql string, args ...any) (pgx.Rows, error) {
				return &MockRows{}, nil
			},
		}
		batch, err := FetchLegacyBackfillCandidates(ctx, mockDB, now, 10, nil)
		require.NoError(t, err)
		assert.Equal(t, 0, batch.Count)
		assert.False(t, batch.HasMore)
		assert.Nil(t, batch.NextCursor)
	})

	t.Run("less than batch size pagination", func(t *testing.T) {
		mockDB := &MockBackfillQueryer{
			QueryFunc: func(ctx context.Context, sql string, args ...any) (pgx.Rows, error) {
				return &MockRows{
					rows: [][]any{
						{uuid.New(), "MARKETPLACE_ONLINE"},
					},
				}, nil
			},
		}
		batch, err := FetchLegacyBackfillCandidates(ctx, mockDB, now, 10, nil)
		require.NoError(t, err)
		assert.Equal(t, 1, batch.Count)
		assert.Equal(t, 1, batch.OnlineCount)
		assert.Equal(t, 0, batch.OfflineCount)
		assert.False(t, batch.HasMore)
		assert.NotNil(t, batch.NextCursor)
	})

	t.Run("exact batch size pagination", func(t *testing.T) {
		mockDB := &MockBackfillQueryer{
			QueryFunc: func(ctx context.Context, sql string, args ...any) (pgx.Rows, error) {
				return &MockRows{
					rows: [][]any{
						{uuid.New(), "MARKETPLACE_ONLINE"},
						{uuid.New(), "OWNER_WALK_IN"},
					},
				}, nil
			},
		}
		batch, err := FetchLegacyBackfillCandidates(ctx, mockDB, now, 2, nil)
		require.NoError(t, err)
		assert.Equal(t, 2, batch.Count)
		assert.False(t, batch.HasMore)
	})

	t.Run("batch size + 1 pagination", func(t *testing.T) {
		lastID := uuid.New()
		lookaheadID := uuid.New()
		mockDB := &MockBackfillQueryer{
			QueryFunc: func(ctx context.Context, sql string, args ...any) (pgx.Rows, error) {
				return &MockRows{
					rows: [][]any{
						{uuid.New(), "MARKETPLACE_ONLINE"},
						{lastID, "OWNER_WALK_IN"},
						{lookaheadID, "MARKETPLACE_ONLINE"},
					},
				}, nil
			},
		}
		batch, err := FetchLegacyBackfillCandidates(ctx, mockDB, now, 2, nil)
		require.NoError(t, err)
		assert.Equal(t, 2, batch.Count) // lookahead row not counted
		assert.Equal(t, 1, batch.OnlineCount)
		assert.Equal(t, 1, batch.OfflineCount)
		assert.True(t, batch.HasMore)
		require.NotNil(t, batch.NextCursor)
		assert.Equal(t, lastID, *batch.NextCursor) // NextCursor must be from the last counted row
	})

	t.Run("unknown channel rejected", func(t *testing.T) {
		mockDB := &MockBackfillQueryer{
			QueryFunc: func(ctx context.Context, sql string, args ...any) (pgx.Rows, error) {
				return &MockRows{
					rows: [][]any{
						{uuid.New(), "UNKNOWN_CHANNEL"},
					},
				}, nil
			},
		}
		_, err := FetchLegacyBackfillCandidates(ctx, mockDB, now, 10, nil)
		assert.ErrorContains(t, err, "unknown channel value: UNKNOWN_CHANNEL")
	})

	t.Run("cursor does not advance integrity failure", func(t *testing.T) {
		// Test equal cursor
		mockDBEqual := &MockBackfillQueryer{
			QueryFunc: func(ctx context.Context, sql string, args ...any) (pgx.Rows, error) {
				return &MockRows{
					rows: [][]any{
						{cursor, "MARKETPLACE_ONLINE"}, // cursor returned is the same as the one passed
					},
				}, nil
			},
		}
		_, err := FetchLegacyBackfillCandidates(ctx, mockDBEqual, now, 10, &cursor)
		assert.ErrorIs(t, err, ErrBackfillIntegrity)

		// Test smaller cursor
		after := uuid.MustParse("00000000-0000-4000-8000-000000000002")
		smaller := uuid.MustParse("00000000-0000-4000-8000-000000000001")

		mockDBSmaller := &MockBackfillQueryer{
			QueryFunc: func(ctx context.Context, sql string, args ...any) (pgx.Rows, error) {
				return &MockRows{
					rows: [][]any{
						{smaller, "MARKETPLACE_ONLINE"},
					},
				}, nil
			},
		}
		_, err2 := FetchLegacyBackfillCandidates(ctx, mockDBSmaller, now, 10, &after)
		assert.ErrorIs(t, err2, ErrBackfillIntegrity)
	})

	t.Run("rows.Err is checked", func(t *testing.T) {
		mockDB := &MockBackfillQueryer{
			QueryFunc: func(ctx context.Context, sql string, args ...any) (pgx.Rows, error) {
				return &MockRows{
					err: errors.New("some db error"),
				}, nil
			},
		}
		_, err := FetchLegacyBackfillCandidates(ctx, mockDB, now, 10, nil)
		assert.ErrorContains(t, err, "row iteration error: some db error")
	})
}
