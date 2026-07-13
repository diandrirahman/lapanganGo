package platformfinance

import (
	"errors"
	"math"
	"strings"
)

// A. BOOKING CHANNEL TYPE
type BookingChannel string

const (
	BookingChannelMarketplaceOnline BookingChannel = "MARKETPLACE_ONLINE"
	BookingChannelOwnerWalkIn       BookingChannel = "OWNER_WALK_IN"
)

// ERROR CONTRACT
var (
	ErrInvalidOriginalPrice          = errors.New("invalid original price")
	ErrInvalidFinalBookingPrice      = errors.New("invalid final booking price")
	ErrInvalidCommissionBps          = errors.New("invalid commission BPS")
	ErrInvalidBookingChannel         = errors.New("invalid booking channel")
	ErrInvalidCustomerServiceFee     = errors.New("invalid customer service fee")
	ErrPriceAdjustmentReasonRequired = errors.New("price adjustment reason required")
)

// B. CALCULATOR PARAMS
type CalculatorParams struct {
	OriginalPriceRupiah        int64
	OwnerPriceAdjustmentRupiah int64
	PriceAdjustmentReason      string
	CommissionBps              int
	BookingChannel             BookingChannel
	CustomerServiceFeeRupiah   int64
}

// C. CALCULATOR RESULT
type CalculatorResult struct {
	FinalBookingPriceRupiah     int64
	CustomerChargeAmountRupiah  int64
	CommissionBasisAmountRupiah int64
	CommissionBps               int
	CommissionAmountRupiah      int64
	OwnerNetAmountRupiah        int64
}

// ARITHMETIC LOGIC (Helpers)
func checkedAddInt64(a, b int64) (int64, error) {
	if b > 0 && a > math.MaxInt64-b {
		return 0, ErrOverflowDetected
	}
	if b < 0 && a < math.MinInt64-b {
		return 0, ErrOverflowDetected
	}
	return a + b, nil
}

func checkedSubInt64(a, b int64) (int64, error) {
	// a - b is safe if we can safely negate b and add.
	// However, -math.MinInt64 overflows.
	if b == math.MinInt64 {
		// a - MinInt64 => a + (MaxInt64 + 1)
		// This will overflow unless a < 0.
		if a >= 0 {
			return 0, ErrOverflowDetected
		}
		// a is < 0, safe to subtract
		return a - b, nil
	}
	return checkedAddInt64(a, -b)
}

func checkedMulNonNegativeInt64(a, b int64) (int64, error) {
	if a < 0 || b < 0 {
		// Contract states non-negative operands
		return 0, errors.New("negative operand in multiplication")
	}
	if a == 0 || b == 0 {
		return 0, nil
	}
	if a > math.MaxInt64/b {
		return 0, ErrOverflowDetected
	}
	return a * b, nil
}

// D. FUNCTION SIGNATURE
func CalculateBookingFees(params CalculatorParams) (CalculatorResult, error) {
	// 1. Validasi OriginalPriceRupiah >= 0
	if params.OriginalPriceRupiah < 0 {
		return CalculatorResult{}, ErrInvalidOriginalPrice
	}

	// 2. Validasi CommissionBps berada pada 0..3000
	if params.CommissionBps < 0 || params.CommissionBps > 3000 {
		return CalculatorResult{}, ErrInvalidCommissionBps
	}

	// 3. Validasi BookingChannel hanya salah satu constant allowlist
	if params.BookingChannel != BookingChannelMarketplaceOnline && params.BookingChannel != BookingChannelOwnerWalkIn {
		return CalculatorResult{}, ErrInvalidBookingChannel
	}

	// 4. Validasi CustomerServiceFeeRupiah == 0
	if params.CustomerServiceFeeRupiah != 0 {
		return CalculatorResult{}, ErrInvalidCustomerServiceFee
	}

	// 5. Jika adjustment nonzero, strings.TrimSpace(PriceAdjustmentReason) wajib tidak kosong
	if params.OwnerPriceAdjustmentRupiah != 0 {
		if strings.TrimSpace(params.PriceAdjustmentReason) == "" {
			return CalculatorResult{}, ErrPriceAdjustmentReasonRequired
		}
	}

	// 6. Hitung final booking price dengan checked addition
	finalPrice, err := checkedAddInt64(params.OriginalPriceRupiah, params.OwnerPriceAdjustmentRupiah)
	if err != nil {
		return CalculatorResult{}, err
	}

	// 7. Tolak final booking price jika hasilnya negatif
	if finalPrice < 0 {
		return CalculatorResult{}, ErrInvalidFinalBookingPrice
	}

	// 8. Tentukan effective commission BPS
	effectiveBps := params.CommissionBps
	if params.BookingChannel == BookingChannelOwnerWalkIn {
		effectiveBps = 0
	}

	// 9. Hitung commission basis
	commissionBasis := finalPrice

	// 10. Hitung commission amount menggunakan quotient/remainder algorithm
	const divisor int64 = 10_000

	whole := commissionBasis / divisor
	remainderBasis := commissionBasis % divisor
	bps64 := int64(effectiveBps)

	wholeProduct, err := checkedMulNonNegativeInt64(whole, bps64)
	if err != nil {
		return CalculatorResult{}, err
	}

	remainderProduct, err := checkedMulNonNegativeInt64(remainderBasis, bps64)
	if err != nil {
		return CalculatorResult{}, err
	}

	partial := remainderProduct / divisor
	roundingRemainder := remainderProduct % divisor

	if roundingRemainder >= 5_000 {
		partial, err = checkedAddInt64(partial, 1)
		if err != nil {
			return CalculatorResult{}, err
		}
	}

	commissionAmount, err := checkedAddInt64(wholeProduct, partial)
	if err != nil {
		return CalculatorResult{}, err
	}

	// 11. Hitung customer charge dengan checked addition
	customerCharge, err := checkedAddInt64(finalPrice, params.CustomerServiceFeeRupiah)
	if err != nil {
		return CalculatorResult{}, err
	}

	// 12. Hitung owner net dengan checked subtraction
	ownerNet, err := checkedSubInt64(commissionBasis, commissionAmount)
	if err != nil {
		return CalculatorResult{}, err
	}

	if ownerNet < 0 {
		// Defensive check although commissionAmount should never exceed commissionBasis based on math
		return CalculatorResult{}, errors.New("owner net amount cannot be negative")
	}

	// 13. Return result
	return CalculatorResult{
		FinalBookingPriceRupiah:     finalPrice,
		CustomerChargeAmountRupiah:  customerCharge,
		CommissionBasisAmountRupiah: commissionBasis,
		CommissionBps:               effectiveBps,
		CommissionAmountRupiah:      commissionAmount,
		OwnerNetAmountRupiah:        ownerNet,
	}, nil
}
