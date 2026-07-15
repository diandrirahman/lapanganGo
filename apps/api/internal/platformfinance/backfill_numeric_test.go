package platformfinance

import (
	"math/big"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseNumericExact(t *testing.T) {
	tests := []struct {
		name    string
		input   pgtype.Numeric
		want    int64
		wantErr bool
	}{
		{
			name:  "100000",
			input: pgtype.Numeric{Valid: true, Int: big.NewInt(100000), Exp: 0, InfinityModifier: pgtype.Finite},
			want:  100000,
		},
		{
			name:  "100000.00",
			input: pgtype.Numeric{Valid: true, Int: big.NewInt(10000000), Exp: -2, InfinityModifier: pgtype.Finite},
			want:  100000,
		},
		{
			name:  "0",
			input: pgtype.Numeric{Valid: true, Int: big.NewInt(0), Exp: 0, InfinityModifier: pgtype.Finite},
			want:  0,
		},
		{
			name:  "0.00",
			input: pgtype.Numeric{Valid: true, Int: big.NewInt(0), Exp: -2, InfinityModifier: pgtype.Finite},
			want:  0,
		},
		{
			name:    "100000.50",
			input:   pgtype.Numeric{Valid: true, Int: big.NewInt(10000050), Exp: -2, InfinityModifier: pgtype.Finite},
			wantErr: true,
		},
		{
			name:    "-1",
			input:   pgtype.Numeric{Valid: true, Int: big.NewInt(-1), Exp: 0, InfinityModifier: pgtype.Finite},
			wantErr: true,
		},
		{
			name:  "9999999999",
			input: pgtype.Numeric{Valid: true, Int: big.NewInt(9999999999), Exp: 0, InfinityModifier: pgtype.Finite},
			want:  9999999999,
		},
		{
			name:    "10000000000",
			input:   pgtype.Numeric{Valid: true, Int: big.NewInt(10000000000), Exp: 0, InfinityModifier: pgtype.Finite},
			wantErr: true, // out of bounds
		},
		{
			name:    "NaN",
			input:   pgtype.Numeric{Valid: true, NaN: true},
			wantErr: true,
		},
		{
			name:    "positive Infinity",
			input:   pgtype.Numeric{Valid: true, InfinityModifier: pgtype.Infinity},
			wantErr: true,
		},
		{
			name:    "negative Infinity",
			input:   pgtype.Numeric{Valid: true, InfinityModifier: pgtype.NegativeInfinity},
			wantErr: true,
		},
		{
			name:    "nil internal numeric value",
			input:   pgtype.Numeric{Valid: true, Int: nil, Exp: 0, InfinityModifier: pgtype.Finite},
			wantErr: true,
		},
		{
			name:  "positive exponent exact accepted",
			input: pgtype.Numeric{Valid: true, Int: big.NewInt(100), Exp: 1, InfinityModifier: pgtype.Finite},
			want:  1000,
		},
		{
			name:  "positive exponent maximum boundary",
			input: pgtype.Numeric{Valid: true, Int: big.NewInt(999999999), Exp: 1, InfinityModifier: pgtype.Finite},
			want:  9999999990,
		},
		{
			name:    "positive exponent overflow rejected",
			input:   pgtype.Numeric{Valid: true, Int: big.NewInt(1000000000), Exp: 1, InfinityModifier: pgtype.Finite},
			wantErr: true, // we rejected positive exponent explicitly if out of bounds
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseNumericExact(tt.input)
			if tt.wantErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
				assert.Equal(t, tt.want, got)
			}
		})
	}
}
