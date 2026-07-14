package bookings

import (
	"errors"
	"math"
)

const maxLegacyBookingRupiah int64 = 9_999_999_999

var (
	errNaNDetected              = errors.New("cannot convert NaN to exact rupiah")
	errInfinityDetected         = errors.New("cannot convert Infinity to exact rupiah")
	errNegativeValueDetected    = errors.New("cannot convert negative value to exact rupiah")
	errValueExceedsMax          = errors.New("value exceeds maximum allowed legacy booking rupiah")
	errFractionalRupiahDetected = errors.New("cannot convert fractional value to exact rupiah")
)

// exactFloat64ToRupiah securely converts a float64 value representing
// a currency amount into a whole int64 rupiah value. It ensures that the
// value is positive, has no fractional part, is finite, and falls within
// the bounds of legacy persistence max allowed value.
func exactFloat64ToRupiah(value float64) (int64, error) {
	if math.IsNaN(value) {
		return 0, errNaNDetected
	}
	if math.IsInf(value, 0) {
		return 0, errInfinityDetected
	}
	if value < 0 {
		return 0, errNegativeValueDetected
	}
	if value > float64(maxLegacyBookingRupiah) {
		return 0, errValueExceedsMax
	}
	if math.Trunc(value) != value {
		return 0, errFractionalRupiahDetected
	}

	result := int64(value)

	// Defensive round-trip check
	if float64(result) != value {
		return 0, errors.New("round-trip exact check failed")
	}

	return result, nil
}
