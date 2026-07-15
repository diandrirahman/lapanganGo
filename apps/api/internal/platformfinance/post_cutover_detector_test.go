package platformfinance

import (
	"context"
	"errors"
	"math/big"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func createValidCandidate(channel BookingChannel) PostCutoverCandidate {
	cID := uuid.New()
	vID := uuid.New()
	oID := uuid.New()

	c := PostCutoverCandidate{
		ID:             uuid.New(),
		CreatedAt:      time.Now(),
		CourtID:        &cID,
		VenueID:        &vID,
		OwnerProfileID: &oID,
		OriginalPrice:  pgtype.Numeric{Int: big.NewInt(100000), Exp: 0, Valid: true},
		FinalPrice:     pgtype.Numeric{Int: big.NewInt(100000), Exp: 0, Valid: true},
		TotalPrice:     pgtype.Numeric{Int: big.NewInt(100000), Exp: 0, Valid: true},
		DiscountAmount: pgtype.Numeric{Int: big.NewInt(0), Exp: 0, Valid: true},
	}

	if channel == BookingChannelOwnerWalkIn {
		c.OfflineRowCount = 1
		c.HasOfflineRecord = true
		c.OfflineSystemPrice = pgtype.Numeric{Int: big.NewInt(100000), Exp: 0, Valid: true}
		c.OfflineFinalPrice = pgtype.Numeric{Int: big.NewInt(100000), Exp: 0, Valid: true}
	}

	return c
}

func createValidTerm(channel BookingChannel) *CommercialTerm {
	return &CommercialTerm{
		ID:               uuid.NewString(),
		Phase:            "STANDARD",
		FinanceMode:      "SIMULATION",
		CollectionMethod: "NONE",
		CommissionBps:    1000,
		ValidFrom:        time.Now().Add(-1 * time.Hour),
	}
}

func TestClassifyPostCutoverP0Candidate_ValidOnline(t *testing.T) {
	c := createValidCandidate(BookingChannelMarketplaceOnline)
	term := createValidTerm(BookingChannelMarketplaceOnline)

	res := ClassifyPostCutoverP0Candidate(c, term, nil)
	assert.Equal(t, ClassificationRepairablePolicyOnline, res.Classification)
	assert.Empty(t, res.Reason)
}

func TestClassifyPostCutoverP0Candidate_ValidWalkIn(t *testing.T) {
	c := createValidCandidate(BookingChannelOwnerWalkIn)
	term := createValidTerm(BookingChannelOwnerWalkIn)

	res := ClassifyPostCutoverP0Candidate(c, term, nil)
	assert.Equal(t, ClassificationRepairablePolicyWalkIn, res.Classification)
	assert.Empty(t, res.Reason)
}

func TestClassifyPostCutoverP0Candidate_WalkInZeroFinalPrice(t *testing.T) {
	c := createValidCandidate(BookingChannelOwnerWalkIn)
	c.OriginalPrice = pgtype.Numeric{Int: big.NewInt(0), Exp: 0, Valid: true}
	c.FinalPrice = pgtype.Numeric{Int: big.NewInt(0), Exp: 0, Valid: true}
	c.TotalPrice = pgtype.Numeric{Int: big.NewInt(0), Exp: 0, Valid: true}
	c.OfflineSystemPrice = pgtype.Numeric{Int: big.NewInt(0), Exp: 0, Valid: true}
	c.OfflineFinalPrice = pgtype.Numeric{Int: big.NewInt(0), Exp: 0, Valid: true}

	term := createValidTerm(BookingChannelOwnerWalkIn)

	res := ClassifyPostCutoverP0Candidate(c, term, nil)
	assert.Equal(t, ClassificationRepairablePolicyWalkIn, res.Classification)
}

func TestClassifyPostCutoverP0Candidate_OnlineZeroFinalPrice(t *testing.T) {
	c := createValidCandidate(BookingChannelMarketplaceOnline)
	c.OriginalPrice = pgtype.Numeric{Int: big.NewInt(0), Exp: 0, Valid: true}
	c.FinalPrice = pgtype.Numeric{Int: big.NewInt(0), Exp: 0, Valid: true}
	c.TotalPrice = pgtype.Numeric{Int: big.NewInt(0), Exp: 0, Valid: true}
	c.DiscountAmount = pgtype.Numeric{Int: big.NewInt(0), Exp: 0, Valid: true}

	term := createValidTerm(BookingChannelMarketplaceOnline)

	res := ClassifyPostCutoverP0Candidate(c, term, nil)
	assert.Equal(t, ClassificationRepairablePolicyOnline, res.Classification)
}

func TestClassifyPostCutoverP0Candidate_MissingReferences(t *testing.T) {
	term := createValidTerm(BookingChannelMarketplaceOnline)

	c1 := createValidCandidate(BookingChannelMarketplaceOnline)
	c1.CourtID = nil
	res := ClassifyPostCutoverP0Candidate(c1, term, nil)
	assert.Equal(t, ClassificationManualDecisionRequired, res.Classification)
	assert.Equal(t, ReasonMissingCourtReference, res.Reason)

	c2 := createValidCandidate(BookingChannelMarketplaceOnline)
	c2.VenueID = nil
	res = ClassifyPostCutoverP0Candidate(c2, term, nil)
	assert.Equal(t, ClassificationManualDecisionRequired, res.Classification)
	assert.Equal(t, ReasonMissingVenueReference, res.Reason)

	c3 := createValidCandidate(BookingChannelMarketplaceOnline)
	c3.OwnerProfileID = nil
	res = ClassifyPostCutoverP0Candidate(c3, term, nil)
	assert.Equal(t, ClassificationManualDecisionRequired, res.Classification)
	assert.Equal(t, ReasonMissingOwnerReference, res.Reason)
}

func TestClassifyPostCutoverP0Candidate_ResolverErrors(t *testing.T) {
	c := createValidCandidate(BookingChannelMarketplaceOnline)

	tests := []struct {
		err      error
		expected PostCutoverP0Reason
	}{
		{ErrMissingEffectiveCommercialTerm, ReasonMissingEffectiveTerm},
		{ErrDuplicateCommercialTerm, ReasonDuplicateEffectiveTerm},
		{ErrUnsupportedCommercialTermFinanceMode, ReasonUnsupportedFinanceMode},
	}

	for _, tt := range tests {
		res := ClassifyPostCutoverP0Candidate(c, nil, tt.err)
		assert.Equal(t, ClassificationManualDecisionRequired, res.Classification)
		assert.Equal(t, tt.expected, res.Reason)
	}

	operational := ClassifyPostCutoverP0Candidate(c, nil, errors.New("database unavailable"))
	assert.EqualError(t, operational.OperationalError, "database unavailable")
}

func TestClassifyPostCutoverP0Candidate_InvalidMoneyValues(t *testing.T) {
	c := createValidCandidate(BookingChannelMarketplaceOnline)
	term := createValidTerm(BookingChannelMarketplaceOnline)

	// Fractional
	c.OriginalPrice = pgtype.Numeric{Int: big.NewInt(1001), Exp: -2, Valid: true}
	res := ClassifyPostCutoverP0Candidate(c, term, nil)
	assert.Equal(t, ClassificationManualDecisionRequired, res.Classification)
	assert.Equal(t, ReasonFractionalMoneyValue, res.Reason)

	// Negative
	c.OriginalPrice = pgtype.Numeric{Int: big.NewInt(-1000), Exp: 0, Valid: true}
	res = ClassifyPostCutoverP0Candidate(c, term, nil)
	assert.Equal(t, ClassificationManualDecisionRequired, res.Classification)
	assert.Equal(t, ReasonNegativeMoneyValue, res.Reason)

	// Overflow
	c.OriginalPrice = pgtype.Numeric{Int: big.NewInt(10_000_000_000), Exp: 0, Valid: true}
	res = ClassifyPostCutoverP0Candidate(c, term, nil)
	assert.Equal(t, ClassificationManualDecisionRequired, res.Classification)
	assert.Equal(t, ReasonMoneyOverflow, res.Reason)

	// NaN
	c.OriginalPrice = pgtype.Numeric{NaN: true, Valid: true}
	res = ClassifyPostCutoverP0Candidate(c, term, nil)
	assert.Equal(t, ClassificationManualDecisionRequired, res.Classification)
	assert.Equal(t, ReasonFractionalMoneyValue, res.Reason)

	// Missing
	c.OriginalPrice = pgtype.Numeric{Valid: false}
	res = ClassifyPostCutoverP0Candidate(c, term, nil)
	assert.Equal(t, ClassificationManualDecisionRequired, res.Classification)
	assert.Equal(t, ReasonMissingMoneyValue, res.Reason)
}

func TestClassifyPostCutoverP0Candidate_WholeRupiahNumericWithNegativeExponent(t *testing.T) {
	c := createValidCandidate(BookingChannelMarketplaceOnline)
	c.OriginalPrice = pgtype.Numeric{Int: big.NewInt(10_000_000), Exp: -2, Valid: true}
	c.FinalPrice = pgtype.Numeric{Int: big.NewInt(10_000_000), Exp: -2, Valid: true}
	c.TotalPrice = pgtype.Numeric{Int: big.NewInt(10_000_000), Exp: -2, Valid: true}
	c.DiscountAmount = pgtype.Numeric{Int: big.NewInt(0), Exp: 0, Valid: true}

	res := ClassifyPostCutoverP0Candidate(c, createValidTerm(BookingChannelMarketplaceOnline), nil)
	assert.Equal(t, ClassificationRepairablePolicyOnline, res.Classification)
}

func TestClassifyPostCutoverP0Candidate_OnlinePriceRules(t *testing.T) {
	term := createValidTerm(BookingChannelMarketplaceOnline)

	// Positive adjustment (Markup)
	c := createValidCandidate(BookingChannelMarketplaceOnline)
	c.DiscountAmount = pgtype.Numeric{Int: big.NewInt(0), Exp: 0, Valid: true}
	c.FinalPrice = pgtype.Numeric{Int: big.NewInt(110000), Exp: 0, Valid: true}
	c.TotalPrice = pgtype.Numeric{Int: big.NewInt(110000), Exp: 0, Valid: true}
	res := ClassifyPostCutoverP0Candidate(c, term, nil)
	assert.Equal(t, ClassificationManualDecisionRequired, res.Classification)
	assert.Equal(t, ReasonOnlinePositiveAdjustment, res.Reason)

	// Arithmetic Mismatch
	c = createValidCandidate(BookingChannelMarketplaceOnline)
	c.DiscountAmount = pgtype.Numeric{Int: big.NewInt(10000), Exp: 0, Valid: true} // Original 100k, discount 10k, final 100k (mismatch)
	res = ClassifyPostCutoverP0Candidate(c, term, nil)
	assert.Equal(t, ClassificationManualDecisionRequired, res.Classification)
	assert.Equal(t, ReasonDiscountArithmeticMismatch, res.Reason)

	// Promo fact missing
	c = createValidCandidate(BookingChannelMarketplaceOnline)
	c.DiscountAmount = pgtype.Numeric{Int: big.NewInt(10000), Exp: 0, Valid: true}
	c.FinalPrice = pgtype.Numeric{Int: big.NewInt(90000), Exp: 0, Valid: true}
	c.TotalPrice = pgtype.Numeric{Int: big.NewInt(90000), Exp: 0, Valid: true}
	res = ClassifyPostCutoverP0Candidate(c, term, nil)
	assert.Equal(t, ClassificationManualDecisionRequired, res.Classification)
	assert.Equal(t, ReasonPromoFactMissing, res.Reason)
}

func TestClassifyPostCutoverP0Candidate_OfflinePriceRules(t *testing.T) {
	term := createValidTerm(BookingChannelOwnerWalkIn)

	// System price mismatch
	c := createValidCandidate(BookingChannelOwnerWalkIn)
	c.OfflineSystemPrice = pgtype.Numeric{Int: big.NewInt(90000), Exp: 0, Valid: true} // Original is 100k
	res := ClassifyPostCutoverP0Candidate(c, term, nil)
	assert.Equal(t, ClassificationManualDecisionRequired, res.Classification)
	assert.Equal(t, ReasonOfflineSystemPriceMismatch, res.Reason)

	// Missing Override Reason
	c = createValidCandidate(BookingChannelOwnerWalkIn)
	c.OfflineSystemPrice = pgtype.Numeric{Int: big.NewInt(100000), Exp: 0, Valid: true}
	c.FinalPrice = pgtype.Numeric{Int: big.NewInt(90000), Exp: 0, Valid: true}
	c.TotalPrice = pgtype.Numeric{Int: big.NewInt(90000), Exp: 0, Valid: true}
	c.OfflineFinalPrice = pgtype.Numeric{Int: big.NewInt(90000), Exp: 0, Valid: true}
	res = ClassifyPostCutoverP0Candidate(c, term, nil)
	assert.Equal(t, ClassificationManualDecisionRequired, res.Classification)
	assert.Equal(t, ReasonAdjustmentWithoutReason, res.Reason)

	// Valid adjustment with reason
	c.HasOverrideReason = true
	res = ClassifyPostCutoverP0Candidate(c, term, nil)
	assert.Equal(t, ClassificationRepairablePolicyWalkIn, res.Classification)
	assert.Empty(t, res.Reason)
}

func TestFetchPostCutoverP0Candidates_Validation(t *testing.T) {
	// Dummy queryer not used because validation fails first
	_, err := FetchPostCutoverP0Candidates(context.Background(), nil, PostCutoverDetectorParams{
		BatchSize: 0,
	})
	require.Error(t, err)
	assert.Equal(t, ErrPostCutoverDetectorIntegrity, err)
}
