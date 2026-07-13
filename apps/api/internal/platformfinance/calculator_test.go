package platformfinance

import (
	"errors"
	"math"
	"testing"
)

func TestCalculateBookingFees(t *testing.T) {
	tests := []struct {
		name    string
		params  CalculatorParams
		want    CalculatorResult
		wantErr error
	}{
		// Positive cases
		{
			name: "Case 1: Rp200.000, 0 BPS",
			params: CalculatorParams{
				OriginalPriceRupiah: 200000,
				CommissionBps:       0,
				BookingChannel:      BookingChannelMarketplaceOnline,
			},
			want: CalculatorResult{
				FinalBookingPriceRupiah:     200000,
				CustomerChargeAmountRupiah:  200000,
				CommissionBasisAmountRupiah: 200000,
				CommissionBps:               0,
				CommissionAmountRupiah:      0,
				OwnerNetAmountRupiah:        200000,
			},
		},
		{
			name: "Case 2: Rp200.000, 500 BPS",
			params: CalculatorParams{
				OriginalPriceRupiah: 200000,
				CommissionBps:       500,
				BookingChannel:      BookingChannelMarketplaceOnline,
			},
			want: CalculatorResult{
				FinalBookingPriceRupiah:     200000,
				CustomerChargeAmountRupiah:  200000,
				CommissionBasisAmountRupiah: 200000,
				CommissionBps:               500,
				CommissionAmountRupiah:      10000,
				OwnerNetAmountRupiah:        190000,
			},
		},
		{
			name: "Case 3: Rp200.000, 700 BPS",
			params: CalculatorParams{
				OriginalPriceRupiah: 200000,
				CommissionBps:       700,
				BookingChannel:      BookingChannelMarketplaceOnline,
			},
			want: CalculatorResult{
				FinalBookingPriceRupiah:     200000,
				CustomerChargeAmountRupiah:  200000,
				CommissionBasisAmountRupiah: 200000,
				CommissionBps:               700,
				CommissionAmountRupiah:      14000,
				OwnerNetAmountRupiah:        186000,
			},
		},
		{
			name: "Case 4: Rp200.000, custom 123 BPS",
			params: CalculatorParams{
				OriginalPriceRupiah: 200000,
				CommissionBps:       123,
				BookingChannel:      BookingChannelMarketplaceOnline,
			},
			want: CalculatorResult{
				FinalBookingPriceRupiah:     200000,
				CustomerChargeAmountRupiah:  200000,
				CommissionBasisAmountRupiah: 200000,
				CommissionBps:               123,
				CommissionAmountRupiah:      2460,
				OwnerNetAmountRupiah:        197540,
			},
		},
		{
			name: "Case 5: Promo (Negative adjustment)",
			params: CalculatorParams{
				OriginalPriceRupiah:        200000,
				OwnerPriceAdjustmentRupiah: -25000,
				PriceAdjustmentReason:      "PROMO",
				CommissionBps:              700,
				BookingChannel:             BookingChannelMarketplaceOnline,
			},
			want: CalculatorResult{
				FinalBookingPriceRupiah:     175000,
				CustomerChargeAmountRupiah:  175000,
				CommissionBasisAmountRupiah: 175000,
				CommissionBps:               700,
				CommissionAmountRupiah:      12250,
				OwnerNetAmountRupiah:        162750,
			},
		},
		{
			name: "Case 6: Offline markup",
			params: CalculatorParams{
				OriginalPriceRupiah:        175000,
				OwnerPriceAdjustmentRupiah: 25000,
				PriceAdjustmentReason:      "Offline price override",
				CommissionBps:              700, // Requested
				BookingChannel:             BookingChannelOwnerWalkIn,
			},
			want: CalculatorResult{
				FinalBookingPriceRupiah:     200000,
				CustomerChargeAmountRupiah:  200000,
				CommissionBasisAmountRupiah: 200000,
				CommissionBps:               0, // Effective
				CommissionAmountRupiah:      0,
				OwnerNetAmountRupiah:        200000,
			},
		},
		{
			name: "Case 7: Exact half-up",
			params: CalculatorParams{
				OriginalPriceRupiah: 10,
				CommissionBps:       500,
				BookingChannel:      BookingChannelMarketplaceOnline,
			},
			want: CalculatorResult{
				FinalBookingPriceRupiah:     10,
				CustomerChargeAmountRupiah:  10,
				CommissionBasisAmountRupiah: 10,
				CommissionBps:               500,
				CommissionAmountRupiah:      1,
				OwnerNetAmountRupiah:        9,
			},
		},
		{
			name: "Case 8: Below half",
			params: CalculatorParams{
				OriginalPriceRupiah: 9,
				CommissionBps:       500,
				BookingChannel:      BookingChannelMarketplaceOnline,
			},
			want: CalculatorResult{
				FinalBookingPriceRupiah:     9,
				CustomerChargeAmountRupiah:  9,
				CommissionBasisAmountRupiah: 9,
				CommissionBps:               500,
				CommissionAmountRupiah:      0,
				OwnerNetAmountRupiah:        9,
			},
		},
		{
			name: "Case 9: Above half",
			params: CalculatorParams{
				OriginalPriceRupiah: 11,
				CommissionBps:       500,
				BookingChannel:      BookingChannelMarketplaceOnline,
			},
			want: CalculatorResult{
				FinalBookingPriceRupiah:     11,
				CustomerChargeAmountRupiah:  11,
				CommissionBasisAmountRupiah: 11,
				CommissionBps:               500,
				CommissionAmountRupiah:      1,
				OwnerNetAmountRupiah:        10,
			},
		},
		{
			name: "Case 10: Maximum int64",
			params: CalculatorParams{
				OriginalPriceRupiah: math.MaxInt64,
				CommissionBps:       3000,
				BookingChannel:      BookingChannelMarketplaceOnline,
			},
			want: CalculatorResult{
				FinalBookingPriceRupiah:     math.MaxInt64,
				CustomerChargeAmountRupiah:  math.MaxInt64,
				CommissionBasisAmountRupiah: math.MaxInt64,
				CommissionBps:               3000,
				CommissionAmountRupiah:      2767011611056432742,
				OwnerNetAmountRupiah:        6456360425798343065,
			},
		},
		{
			name: "Adjustment 0 tanpa reason diperbolehkan",
			params: CalculatorParams{
				OriginalPriceRupiah:        100000,
				OwnerPriceAdjustmentRupiah: 0,
				PriceAdjustmentReason:      "",
				CommissionBps:              500,
				BookingChannel:             BookingChannelMarketplaceOnline,
			},
			want: CalculatorResult{
				FinalBookingPriceRupiah:     100000,
				CustomerChargeAmountRupiah:  100000,
				CommissionBasisAmountRupiah: 100000,
				CommissionBps:               500,
				CommissionAmountRupiah:      5000,
				OwnerNetAmountRupiah:        95000,
			},
		},
		{
			name: "Final price 0 diperbolehkan",
			params: CalculatorParams{
				OriginalPriceRupiah: 0,
				CommissionBps:       500,
				BookingChannel:      BookingChannelMarketplaceOnline,
			},
			want: CalculatorResult{
				FinalBookingPriceRupiah:     0,
				CustomerChargeAmountRupiah:  0,
				CommissionBasisAmountRupiah: 0,
				CommissionBps:               500,
				CommissionAmountRupiah:      0,
				OwnerNetAmountRupiah:        0,
			},
		},
		{
			name: "Final price 0 via promo (Original=100, Adjustment=-100)",
			params: CalculatorParams{
				OriginalPriceRupiah:        100,
				OwnerPriceAdjustmentRupiah: -100,
				PriceAdjustmentReason:      "100% Promo",
				CommissionBps:              500,
				BookingChannel:             BookingChannelMarketplaceOnline,
			},
			want: CalculatorResult{
				FinalBookingPriceRupiah:     0,
				CustomerChargeAmountRupiah:  0,
				CommissionBasisAmountRupiah: 0,
				CommissionBps:               500,
				CommissionAmountRupiah:      0,
				OwnerNetAmountRupiah:        0,
			},
		},
		// Negative cases
		{
			name: "Case 11: Addition overflow",
			params: CalculatorParams{
				OriginalPriceRupiah:        math.MaxInt64,
				OwnerPriceAdjustmentRupiah: 1,
				PriceAdjustmentReason:      "overflow",
				CommissionBps:              3000,
				BookingChannel:             BookingChannelMarketplaceOnline,
			},
			wantErr: ErrOverflowDetected,
		},
		{
			name: "Case 12: Negative final",
			params: CalculatorParams{
				OriginalPriceRupiah:        100,
				OwnerPriceAdjustmentRupiah: -101,
				PriceAdjustmentReason:      "terlalu besar diskon",
				CommissionBps:              500,
				BookingChannel:             BookingChannelMarketplaceOnline,
			},
			wantErr: ErrInvalidFinalBookingPrice,
		},
		{
			name: "Original price negatif",
			params: CalculatorParams{
				OriginalPriceRupiah: -1,
				CommissionBps:       500,
				BookingChannel:      BookingChannelMarketplaceOnline,
			},
			wantErr: ErrInvalidOriginalPrice,
		},
		{
			name: "BPS -1",
			params: CalculatorParams{
				OriginalPriceRupiah: 100000,
				CommissionBps:       -1,
				BookingChannel:      BookingChannelMarketplaceOnline,
			},
			wantErr: ErrInvalidCommissionBps,
		},
		{
			name: "BPS 3001",
			params: CalculatorParams{
				OriginalPriceRupiah: 100000,
				CommissionBps:       3001,
				BookingChannel:      BookingChannelMarketplaceOnline,
			},
			wantErr: ErrInvalidCommissionBps,
		},
		{
			name: "Unknown booking channel",
			params: CalculatorParams{
				OriginalPriceRupiah: 100000,
				CommissionBps:       500,
				BookingChannel:      "SOME_RANDOM_CHANNEL",
			},
			wantErr: ErrInvalidBookingChannel,
		},
		{
			name: "Empty booking channel",
			params: CalculatorParams{
				OriginalPriceRupiah: 100000,
				CommissionBps:       500,
				BookingChannel:      "",
			},
			wantErr: ErrInvalidBookingChannel,
		},
		{
			name: "Customer service fee 1",
			params: CalculatorParams{
				OriginalPriceRupiah:      100000,
				CommissionBps:            500,
				BookingChannel:           BookingChannelMarketplaceOnline,
				CustomerServiceFeeRupiah: 1,
			},
			wantErr: ErrInvalidCustomerServiceFee,
		},
		{
			name: "Customer service fee -1",
			params: CalculatorParams{
				OriginalPriceRupiah:      100000,
				CommissionBps:            500,
				BookingChannel:           BookingChannelMarketplaceOnline,
				CustomerServiceFeeRupiah: -1,
			},
			wantErr: ErrInvalidCustomerServiceFee,
		},
		{
			name: "Positive adjustment tanpa reason",
			params: CalculatorParams{
				OriginalPriceRupiah:        100000,
				OwnerPriceAdjustmentRupiah: 1000,
				PriceAdjustmentReason:      "",
				CommissionBps:              500,
				BookingChannel:             BookingChannelMarketplaceOnline,
			},
			wantErr: ErrPriceAdjustmentReasonRequired,
		},
		{
			name: "Negative adjustment tanpa reason",
			params: CalculatorParams{
				OriginalPriceRupiah:        100000,
				OwnerPriceAdjustmentRupiah: -1000,
				PriceAdjustmentReason:      "",
				CommissionBps:              500,
				BookingChannel:             BookingChannelMarketplaceOnline,
			},
			wantErr: ErrPriceAdjustmentReasonRequired,
		},
		{
			name: "Adjustment dengan reason whitespace",
			params: CalculatorParams{
				OriginalPriceRupiah:        100000,
				OwnerPriceAdjustmentRupiah: -1000,
				PriceAdjustmentReason:      "   \t  ",
				CommissionBps:              500,
				BookingChannel:             BookingChannelMarketplaceOnline,
			},
			wantErr: ErrPriceAdjustmentReasonRequired,
		},
		{
			name: "OWNER_WALK_IN dengan requested BPS 3001 tetap ditolak sebelum effective BPS dipaksa 0",
			params: CalculatorParams{
				OriginalPriceRupiah: 100000,
				CommissionBps:       3001,
				BookingChannel:      BookingChannelOwnerWalkIn,
			},
			wantErr: ErrInvalidCommissionBps,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := CalculateBookingFees(tc.params)

			// Assertion untuk Error
			if tc.wantErr != nil {
				if !errors.Is(err, tc.wantErr) {
					t.Fatalf("expected error %v, got %v", tc.wantErr, err)
				}
				// Pastikan result tidak berisi partial success
				var emptyResult CalculatorResult
				if got != emptyResult {
					t.Fatalf("expected empty result on error, got %+v", got)
				}
				return
			}

			// Assertion Success
			if err != nil {
				t.Fatalf("expected no error, got %v", err)
			}

			if got != tc.want {
				t.Errorf("\nexpected:\n%+v\ngot:\n%+v\n", tc.want, got)
			}
		})
	}

	t.Run("Function dipanggil berulang dengan input sama menghasilkan output sama", func(t *testing.T) {
		params := CalculatorParams{
			OriginalPriceRupiah: 200000,
			CommissionBps:       700,
			BookingChannel:      BookingChannelMarketplaceOnline,
		}
		expected := CalculatorResult{
			FinalBookingPriceRupiah:     200000,
			CustomerChargeAmountRupiah:  200000,
			CommissionBasisAmountRupiah: 200000,
			CommissionBps:               700,
			CommissionAmountRupiah:      14000,
			OwnerNetAmountRupiah:        186000,
		}

		for i := 0; i < 5; i++ {
			got, err := CalculateBookingFees(params)
			if err != nil {
				t.Fatalf("iter %d: expected no error, got %v", i, err)
			}
			if got != expected {
				t.Errorf("iter %d: expected %+v, got %+v", i, expected, got)
			}
		}
	})
}

// Internal tests to cover helper functions to get 100% coverage
func TestCheckedAddInt64(t *testing.T) {
	_, err := checkedAddInt64(math.MaxInt64, 1)
	if !errors.Is(err, ErrOverflowDetected) {
		t.Errorf("expected ErrOverflowDetected, got %v", err)
	}

	_, err = checkedAddInt64(math.MinInt64, -1)
	if !errors.Is(err, ErrOverflowDetected) {
		t.Errorf("expected ErrOverflowDetected, got %v", err)
	}
}

func TestCheckedSubInt64(t *testing.T) {
	_, err := checkedSubInt64(math.MinInt64, 1)
	if !errors.Is(err, ErrOverflowDetected) {
		t.Errorf("expected ErrOverflowDetected, got %v", err)
	}

	// Subtraction with MinInt64 as b
	// a - (-9223372036854775808) -> a + 9223372036854775808
	// if a >= 0 it overflows
	_, err = checkedSubInt64(0, math.MinInt64)
	if !errors.Is(err, ErrOverflowDetected) {
		t.Errorf("expected ErrOverflowDetected for 0 - MinInt64, got %v", err)
	}

	val, err := checkedSubInt64(-1, math.MinInt64)
	if err != nil {
		t.Errorf("expected success for -1 - MinInt64, got %v", err)
	}
	if val != math.MaxInt64 {
		t.Errorf("expected %d, got %d", math.MaxInt64, val)
	}
}

func TestCheckedMulNonNegativeInt64(t *testing.T) {
	// Trigger negative check
	_, err := checkedMulNonNegativeInt64(-1, 5)
	if err == nil || err.Error() != "negative operand in multiplication" {
		t.Errorf("expected negative operand error, got %v", err)
	}
	_, err = checkedMulNonNegativeInt64(5, -1)
	if err == nil || err.Error() != "negative operand in multiplication" {
		t.Errorf("expected negative operand error, got %v", err)
	}

	// Trigger overflow check
	_, err = checkedMulNonNegativeInt64(math.MaxInt64/2, 3)
	if !errors.Is(err, ErrOverflowDetected) {
		t.Errorf("expected ErrOverflowDetected, got %v", err)
	}

	// Ensure 0 works
	val, err := checkedMulNonNegativeInt64(0, 100)
	if err != nil || val != 0 {
		t.Errorf("expected 0, nil for 0 * 100")
	}
	val, err = checkedMulNonNegativeInt64(100, 0)
	if err != nil || val != 0 {
		t.Errorf("expected 0, nil for 100 * 0")
	}
}
