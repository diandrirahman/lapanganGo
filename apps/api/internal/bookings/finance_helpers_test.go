package bookings

import (
	"math"
	"testing"
)

func TestExactFloat64ToRupiah(t *testing.T) {
	tests := []struct {
		name    string
		value   float64
		wantErr bool
		wantVal int64
	}{
		{"0", 0, false, 0},
		{"nilai bulat normal", 100000, false, 100000},
		{"0.5", 0.5, true, 0},
		{"100000.01", 100000.01, true, 0},
		{"negative", -10000, true, 0},
		{"NaN", math.NaN(), true, 0},
		{"+Inf", math.Inf(1), true, 0},
		{"-Inf", math.Inf(-1), true, 0},
		{"exact maximum", float64(maxLegacyBookingRupiah), false, maxLegacyBookingRupiah},
		{"nilai di atas maximum", float64(maxLegacyBookingRupiah) + 1, true, 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := exactFloat64ToRupiah(tt.value)
			if (err != nil) != tt.wantErr {
				t.Errorf("exactFloat64ToRupiah() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got != tt.wantVal {
				t.Errorf("exactFloat64ToRupiah() = %v, want %v", got, tt.wantVal)
			}
		})
	}
}
