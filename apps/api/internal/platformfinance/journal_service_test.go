package platformfinance

import (
	"context"
	"errors"
	"math"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func validPostJournalParams() PostJournalParams {
	return PostJournalParams{
		EventKey:    "test.journal:unit-001",
		EventType:   "TEST_JOURNAL",
		EffectiveAt: time.Date(2026, time.July, 16, 10, 11, 12, 123456789, time.FixedZone("WIB", 7*60*60)),
		Metadata: map[string]string{
			"source_type":      "unit_test",
			"source_reference": "case-001",
		},
		Entries: []PostJournalEntry{
			{AccountCode: "BANK_CASH", Side: JournalSideDebit, AmountRupiah: 1},
			{AccountCode: "FUNDING_CLEARING", Side: JournalSideCredit, AmountRupiah: 1},
		},
	}
}

func TestValidateAndNormalizeJournalAcceptsExactRp1Balance(t *testing.T) {
	params := validPostJournalParams()
	normalized, accountCodes, err := validateAndNormalizeJournal(params)
	require.NoError(t, err)
	assert.Equal(t, []string{"BANK_CASH", "FUNDING_CLEARING"}, accountCodes)
	assert.Equal(t, time.Date(2026, time.July, 16, 3, 11, 12, 123456000, time.UTC), normalized.EffectiveAt)
	assert.Equal(t, int64(1), normalized.Entries[0].AmountRupiah)

	params.Entries = []PostJournalEntry{
		{AccountCode: "BANK_CASH", Side: JournalSideDebit, AmountRupiah: 25},
		{AccountCode: "PSP_CLEARING", Side: JournalSideDebit, AmountRupiah: 75},
		{AccountCode: "FUNDING_CLEARING", Side: JournalSideCredit, AmountRupiah: 100},
	}
	_, _, err = validateAndNormalizeJournal(params)
	require.NoError(t, err)
}

func TestValidateAndNormalizeJournalRejectsMinimumAmountBalanceAndOverflow(t *testing.T) {
	tests := []struct {
		name     string
		mutate   func(*PostJournalParams)
		expected error
	}{
		{
			name: "minimum entries",
			mutate: func(params *PostJournalParams) {
				params.Entries = params.Entries[:1]
			},
			expected: ErrJournalRequiresTwoEntries,
		},
		{
			name: "zero amount",
			mutate: func(params *PostJournalParams) {
				params.Entries[0].AmountRupiah = 0
			},
			expected: ErrInvalidJournalAmount,
		},
		{
			name: "negative amount",
			mutate: func(params *PostJournalParams) {
				params.Entries[0].AmountRupiah = -1
			},
			expected: ErrInvalidJournalAmount,
		},
		{
			name: "unbalanced",
			mutate: func(params *PostJournalParams) {
				params.Entries[1].AmountRupiah = 2
			},
			expected: ErrJournalUnbalanced,
		},
		{
			name: "debit overflow",
			mutate: func(params *PostJournalParams) {
				params.Entries = []PostJournalEntry{
					{AccountCode: "BANK_CASH", Side: JournalSideDebit, AmountRupiah: math.MaxInt64},
					{AccountCode: "PSP_CLEARING", Side: JournalSideDebit, AmountRupiah: 1},
					{AccountCode: "FUNDING_CLEARING", Side: JournalSideCredit, AmountRupiah: math.MaxInt64},
					{AccountCode: "ACCOUNTS_PAYABLE", Side: JournalSideCredit, AmountRupiah: 1},
				}
			},
			expected: ErrJournalAmountOverflow,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			params := validPostJournalParams()
			tt.mutate(&params)
			_, _, err := validateAndNormalizeJournal(params)
			assert.ErrorIs(t, err, tt.expected)
		})
	}
}

func TestValidateAndNormalizeJournalRejectsInvalidStructureAndReferences(t *testing.T) {
	tests := []struct {
		name     string
		mutate   func(*PostJournalParams)
		expected error
	}{
		{"event key format", func(p *PostJournalParams) { p.EventKey = "Invalid Key" }, ErrInvalidJournalRequest},
		{"reversal reserved", func(p *PostJournalParams) { p.EventKey = "journal.reversed:" + uuid.NewString() }, ErrInvalidJournalRequest},
		{"event type", func(p *PostJournalParams) { p.EventType = "test_journal" }, ErrInvalidJournalRequest},
		{"zero effective time", func(p *PostJournalParams) { p.EffectiveAt = time.Time{} }, ErrInvalidJournalRequest},
		{"invalid side", func(p *PostJournalParams) { p.Entries[0].Side = "DR" }, ErrInvalidJournalRequest},
		{"empty account", func(p *PostJournalParams) { p.Entries[0].AccountCode = "" }, ErrUnknownJournalAccount},
		{"booking uuid", func(p *PostJournalParams) { value := "bad"; p.BookingID = &value }, ErrInvalidJournalReference},
		{"entry owner uuid", func(p *PostJournalParams) { value := "bad"; p.Entries[0].OwnerProfileID = &value }, ErrInvalidJournalReference},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			params := validPostJournalParams()
			tt.mutate(&params)
			_, _, err := validateAndNormalizeJournal(params)
			assert.ErrorIs(t, err, tt.expected)
		})
	}
}

func TestValidateJournalMetadataAllowlist(t *testing.T) {
	valid := map[string]string{
		"source_type":         "manual",
		"source_reference":    "reference-1",
		"reason_code":         "correction",
		"calculation_version": "v1",
	}
	require.NoError(t, validateJournalMetadata(valid))
	require.NoError(t, validateJournalMetadata(nil))

	invalid := []map[string]string{
		{"unknown": "value"},
		{"source_type": ""},
		{"source_type": " padded"},
		{"source_type": strings.Repeat("a", 192)},
		{"source_type": "bearer-value"},
		{"reason_code": "containsPassword"},
	}
	for _, metadata := range invalid {
		assert.ErrorIs(t, validateJournalMetadata(metadata), ErrInvalidJournalMetadata)
	}
}

func TestValidateJournalAccountDimensionsUsesCatalogDefinitions(t *testing.T) {
	ownerID := uuid.NewString()
	params := validPostJournalParams()
	definitions := map[string]JournalAccountDefinition{
		"BANK_CASH":        {Code: "BANK_CASH", OwnerDimension: JournalOwnerDimensionForbidden},
		"FUNDING_CLEARING": {Code: "FUNDING_CLEARING", OwnerDimension: JournalOwnerDimensionForbidden},
	}
	require.NoError(t, validateJournalAccountDimensions(params, definitions))

	delete(definitions, "BANK_CASH")
	assert.ErrorIs(t, validateJournalAccountDimensions(params, definitions), ErrUnknownJournalAccount)

	params = validPostJournalParams()
	params.OwnerProfileID = &ownerID
	params.Entries[0].AccountCode = "OWNER_RECEIVABLE"
	params.Entries[0].OwnerProfileID = &ownerID
	definitions = map[string]JournalAccountDefinition{
		"OWNER_RECEIVABLE": {Code: "OWNER_RECEIVABLE", OwnerDimension: JournalOwnerDimensionRequired},
		"FUNDING_CLEARING": {Code: "FUNDING_CLEARING", OwnerDimension: JournalOwnerDimensionForbidden},
	}
	require.NoError(t, validateJournalAccountDimensions(params, definitions))

	wrongOwnerID := uuid.NewString()
	params.Entries[0].OwnerProfileID = &wrongOwnerID
	assert.ErrorIs(t, validateJournalAccountDimensions(params, definitions), ErrInvalidJournalOwnerDimension)

	params = validPostJournalParams()
	params.Entries[0].OwnerProfileID = &ownerID
	assert.ErrorIs(t, validateJournalAccountDimensions(params, map[string]JournalAccountDefinition{
		"BANK_CASH":        {Code: "BANK_CASH", OwnerDimension: JournalOwnerDimensionForbidden},
		"FUNDING_CLEARING": {Code: "FUNDING_CLEARING", OwnerDimension: JournalOwnerDimensionForbidden},
	}), ErrInvalidJournalOwnerDimension)
}

func TestJournalPayloadHashV1IsCanonicalAndPreservesMultiplicity(t *testing.T) {
	params := validPostJournalParams()
	normalized, _, err := validateAndNormalizeJournal(params)
	require.NoError(t, err)
	hash, err := hashJournalPayloadV1(normalized)
	require.NoError(t, err)
	assert.Equal(t, "79b8836bf8cdb7865a5335d51a7300f308ca9f4e2338b5e820e37ec36a1176f4", hash)

	reordered := normalized
	reordered.Metadata = map[string]string{
		"source_reference": "case-001",
		"source_type":      "unit_test",
	}
	reordered.Entries = []PostJournalEntry{normalized.Entries[1], normalized.Entries[0]}
	reorderedHash, err := hashJournalPayloadV1(reordered)
	require.NoError(t, err)
	assert.Equal(t, hash, reorderedHash)

	withDuplicate := normalized
	withDuplicate.Entries = append(append([]PostJournalEntry{}, normalized.Entries...), normalized.Entries[0])
	duplicateHash, err := hashJournalPayloadV1(withDuplicate)
	require.NoError(t, err)
	assert.NotEqual(t, hash, duplicateHash)
}

func TestReplayExistingJournalRequiresExactCanonicalIntegrity(t *testing.T) {
	params := validPostJournalParams()
	normalized, _, err := validateAndNormalizeJournal(params)
	require.NoError(t, err)
	hash, err := hashJournalPayloadV1(normalized)
	require.NoError(t, err)

	journalID := uuid.NewString()
	existing := &loadedJournal{
		Journal: PostedJournal{
			ID:                 journalID,
			EventKey:           normalized.EventKey,
			EventType:          normalized.EventType,
			PayloadHash:        hash,
			PayloadHashVersion: JournalPayloadHashVersionV1,
			Currency:           JournalCurrencyIDR,
			EffectiveAt:        normalized.EffectiveAt,
			Metadata:           cloneJournalMetadata(normalized.Metadata),
			Entries: []PostedJournalEntry{
				{ID: uuid.NewString(), JournalID: journalID, AccountCode: "FUNDING_CLEARING", Side: JournalSideCredit, AmountRupiah: 1},
				{ID: uuid.NewString(), JournalID: journalID, AccountCode: "BANK_CASH", Side: JournalSideDebit, AmountRupiah: 1},
			},
		},
	}
	definitions := map[string]JournalAccountDefinition{
		"BANK_CASH":        {Code: "BANK_CASH", OwnerDimension: JournalOwnerDimensionForbidden},
		"FUNDING_CLEARING": {Code: "FUNDING_CLEARING", OwnerDimension: JournalOwnerDimensionForbidden},
	}

	replayed, err := replayExistingJournal(existing, normalized, hash, definitions)
	require.NoError(t, err)
	assert.Equal(t, journalID, replayed.ID)
	assert.Equal(t, "BANK_CASH", replayed.Entries[0].AccountCode)

	differentPayload := normalized
	differentPayload.Entries = []PostJournalEntry{
		{AccountCode: "BANK_CASH", Side: JournalSideDebit, AmountRupiah: 2},
		{AccountCode: "FUNDING_CLEARING", Side: JournalSideCredit, AmountRupiah: 2},
	}
	differentHash, err := hashJournalPayloadV1(differentPayload)
	require.NoError(t, err)
	_, err = replayExistingJournal(existing, differentPayload, differentHash, definitions)
	assert.ErrorIs(t, err, ErrJournalEventKeyConflict)

	existing.Journal.Metadata["source_type"] = "tampered"
	_, err = replayExistingJournal(existing, normalized, hash, definitions)
	assert.ErrorIs(t, err, ErrJournalIntegrity)

	existing.Journal.Metadata["source_type"] = "unit_test"
	existing.Journal.PayloadHashVersion = "UNKNOWN_VERSION"
	_, err = replayExistingJournal(existing, normalized, hash, definitions)
	assert.ErrorIs(t, err, ErrJournalIntegrity)
}

func TestJournalMoneyContractUsesInt64AndNoFloat(t *testing.T) {
	entryType := reflect.TypeOf(PostJournalEntry{})
	amount, ok := entryType.FieldByName("AmountRupiah")
	require.True(t, ok)
	assert.Equal(t, reflect.Int64, amount.Type.Kind())

	postedType := reflect.TypeOf(PostedJournalEntry{})
	postedAmount, ok := postedType.FieldByName("AmountRupiah")
	require.True(t, ok)
	assert.Equal(t, reflect.Int64, postedAmount.Type.Kind())

	for _, typ := range []reflect.Type{entryType, postedType} {
		for index := 0; index < typ.NumField(); index++ {
			kind := typ.Field(index).Type.Kind()
			assert.NotEqual(t, reflect.Float32, kind)
			assert.NotEqual(t, reflect.Float64, kind)
		}
	}
}

func TestJournalServiceConstructorAndGenericErrorMapping(t *testing.T) {
	service, err := NewJournalService(nil)
	assert.Nil(t, service)
	assert.ErrorIs(t, err, ErrInvalidJournalService)

	service, err = NewJournalService(NewJournalRepository())
	require.NoError(t, err)
	_, err = service.PostJournal(context.Background(), nil, validPostJournalParams())
	assert.ErrorIs(t, err, ErrJournalTransactionRequired)

	assert.ErrorIs(t, mapJournalRepositoryError(errors.New("raw sql failed on platform_journals")), ErrJournalPersistence)
	assert.ErrorIs(t, mapJournalRepositoryError(&pgconn.PgError{
		Code:           "23505",
		ConstraintName: "platform_journals_event_key_key",
		Message:        "duplicate key value violates unique constraint",
	}), ErrJournalEventKeyConflict)
	assert.ErrorIs(t, mapJournalRepositoryError(&pgconn.PgError{
		Code:    "23503",
		Message: "violates foreign key constraint on platform_journals",
	}), ErrInvalidJournalReference)
	assert.ErrorIs(t, mapJournalRepositoryError(&pgconn.PgError{
		Code:           "23514",
		ConstraintName: "platform_journal_balance_guard",
		Message:        "journal does not balance",
	}), ErrJournalUnbalanced)
	assert.ErrorIs(t, normalizeJournalServiceError(ErrJournalIntegrity), ErrJournalIntegrity)

	for _, err := range []error{ErrJournalPersistence, ErrJournalEventKeyConflict, ErrJournalIntegrity, ErrInvalidJournalReference} {
		assert.NotContains(t, err.Error(), "platform_journals")
		assert.NotContains(t, err.Error(), "SQL")
	}
}

func TestNormalizeReverseJournalParamsEnforcesStableContract(t *testing.T) {
	actorID := uuid.NewString()
	journalID := uuid.NewString()
	normalized, err := normalizeReverseJournalParams(ReverseJournalParams{
		JournalID:       journalID,
		Reason:          "manual correction",
		EffectiveAt:     time.Date(2026, time.July, 16, 10, 11, 12, 123456789, time.FixedZone("WIB", 7*60*60)),
		CreatedByUserID: &actorID,
		Metadata:        map[string]string{"reason_code": "correction"},
	})
	require.NoError(t, err)
	assert.Equal(t, journalID, normalized.JournalID)
	assert.Equal(t, time.Date(2026, time.July, 16, 3, 11, 12, 123456000, time.UTC), normalized.EffectiveAt)
	assert.Equal(t, actorID, *normalized.CreatedByUserID)

	for _, params := range []ReverseJournalParams{
		{JournalID: "bad", Reason: "reason", EffectiveAt: time.Now()},
		{JournalID: journalID, Reason: "", EffectiveAt: time.Now()},
		{JournalID: journalID, Reason: " padded", EffectiveAt: time.Now()},
		{JournalID: journalID, Reason: strings.Repeat("r", 501), EffectiveAt: time.Now()},
		{JournalID: journalID, Reason: "reason", EffectiveAt: time.Time{}},
	} {
		_, err := normalizeReverseJournalParams(params)
		assert.Error(t, err)
	}
}

func TestInvertPostedJournalEntriesPreservesMultiplicityAndDimensions(t *testing.T) {
	ownerID := uuid.NewString()
	entries := []PostedJournalEntry{
		{AccountCode: "OWNER_PAYABLE", OwnerProfileID: &ownerID, Side: JournalSideCredit, AmountRupiah: 7},
		{AccountCode: "BANK_CASH", Side: JournalSideDebit, AmountRupiah: 4},
		{AccountCode: "BANK_CASH", Side: JournalSideDebit, AmountRupiah: 4},
	}
	inverse := invertPostedJournalEntries(entries)
	require.Len(t, inverse, len(entries))
	assert.Equal(t, JournalSideDebit, inverse[0].Side)
	assert.Equal(t, ownerID, *inverse[0].OwnerProfileID)
	assert.Equal(t, int64(7), inverse[0].AmountRupiah)
	assert.Equal(t, JournalSideCredit, inverse[1].Side)
	assert.Equal(t, JournalSideCredit, inverse[2].Side)
	assert.True(t, exactReversalEntryMultisetMatches(entries, []PostedJournalEntry{
		{AccountCode: "OWNER_PAYABLE", OwnerProfileID: &ownerID, Side: JournalSideDebit, AmountRupiah: 7},
		{AccountCode: "BANK_CASH", Side: JournalSideCredit, AmountRupiah: 4},
		{AccountCode: "BANK_CASH", Side: JournalSideCredit, AmountRupiah: 4},
	}))
}

func TestReversalPayloadHashIncludesReversalContract(t *testing.T) {
	params := validPostJournalParams()
	params.EventKey = "journal.reversed:" + uuid.NewString()
	params.EventType = JournalReversalEventType
	params.Entries = []PostJournalEntry{
		{AccountCode: "BANK_CASH", Side: JournalSideCredit, AmountRupiah: 1},
		{AccountCode: "FUNDING_CLEARING", Side: JournalSideDebit, AmountRupiah: 1},
	}
	sourceID := uuid.NewString()
	reason := "manual correction"
	hash, err := hashJournalPayloadV1WithReversal(params, &sourceID, &reason)
	require.NoError(t, err)

	changedReason := "different correction"
	changedReasonHash, err := hashJournalPayloadV1WithReversal(params, &sourceID, &changedReason)
	require.NoError(t, err)
	assert.NotEqual(t, hash, changedReasonHash)

	changedSource := uuid.NewString()
	changedSourceHash, err := hashJournalPayloadV1WithReversal(params, &changedSource, &reason)
	require.NoError(t, err)
	assert.NotEqual(t, hash, changedSourceHash)
}

func TestReplayExistingReversalRequiresExactCanonicalIntegrity(t *testing.T) {
	sourceID := uuid.NewString()
	sourceParams := validPostJournalParams()
	sourceParams.EventKey = "test.source:" + sourceID
	sourceNormalized, _, err := validateAndNormalizeJournal(sourceParams)
	require.NoError(t, err)
	sourceHash, err := hashJournalPayloadV1(sourceNormalized)
	require.NoError(t, err)
	source := &loadedJournal{Journal: PostedJournal{
		ID:                 sourceID,
		EventKey:           sourceNormalized.EventKey,
		EventType:          sourceNormalized.EventType,
		PayloadHash:        sourceHash,
		PayloadHashVersion: JournalPayloadHashVersionV1,
		Currency:           JournalCurrencyIDR,
		EffectiveAt:        sourceNormalized.EffectiveAt,
		PostedAt:           sourceNormalized.EffectiveAt.Add(time.Minute),
		Metadata:           cloneJournalMetadata(sourceNormalized.Metadata),
		Entries: []PostedJournalEntry{
			{ID: uuid.NewString(), JournalID: sourceID, AccountCode: "BANK_CASH", Side: JournalSideDebit, AmountRupiah: 1},
			{ID: uuid.NewString(), JournalID: sourceID, AccountCode: "FUNDING_CLEARING", Side: JournalSideCredit, AmountRupiah: 1},
		},
	}}
	definitions := map[string]JournalAccountDefinition{
		"BANK_CASH":        {Code: "BANK_CASH", OwnerDimension: JournalOwnerDimensionForbidden},
		"FUNDING_CLEARING": {Code: "FUNDING_CLEARING", OwnerDimension: JournalOwnerDimensionForbidden},
	}
	reversalReason := "manual correction"
	reversalParams := PostJournalParams{
		EventKey:    "journal.reversed:" + sourceID,
		EventType:   JournalReversalEventType,
		EffectiveAt: source.Journal.EffectiveAt.Add(time.Minute),
		Metadata:    map[string]string{"reason_code": "correction"},
		Entries: []PostJournalEntry{
			{AccountCode: "BANK_CASH", Side: JournalSideCredit, AmountRupiah: 1},
			{AccountCode: "FUNDING_CLEARING", Side: JournalSideDebit, AmountRupiah: 1},
		},
	}
	reversalNormalized, _, err := validateAndNormalizeReversalJournal(reversalParams)
	require.NoError(t, err)
	reversalID := uuid.NewString()
	reversalHash, err := hashJournalPayloadV1WithReversal(reversalNormalized, &sourceID, &reversalReason)
	require.NoError(t, err)
	existing := &loadedJournal{Journal: PostedJournal{
		ID:                 reversalID,
		EventKey:           reversalNormalized.EventKey,
		EventType:          JournalReversalEventType,
		PayloadHash:        reversalHash,
		PayloadHashVersion: JournalPayloadHashVersionV1,
		Currency:           JournalCurrencyIDR,
		EffectiveAt:        reversalNormalized.EffectiveAt,
		PostedAt:           reversalNormalized.EffectiveAt.Add(time.Minute),
		ReversesJournalID:  &sourceID,
		ReversalReason:     &reversalReason,
		Metadata:           cloneJournalMetadata(reversalNormalized.Metadata),
		Entries: []PostedJournalEntry{
			{ID: uuid.NewString(), JournalID: reversalID, AccountCode: "FUNDING_CLEARING", Side: JournalSideDebit, AmountRupiah: 1},
			{ID: uuid.NewString(), JournalID: reversalID, AccountCode: "BANK_CASH", Side: JournalSideCredit, AmountRupiah: 1},
		},
	}}
	existing.ReversesJournalID = &sourceID
	existing.ReversalReason = &reversalReason

	replayed, err := replayExistingReversal(existing, source, reversalNormalized, reversalHash, definitions)
	require.NoError(t, err)
	assert.Equal(t, reversalID, replayed.ID)

	different := reversalNormalized
	different.Metadata = map[string]string{"reason_code": "different"}
	differentHash, err := hashJournalPayloadV1WithReversal(different, &sourceID, &reversalReason)
	require.NoError(t, err)
	_, err = replayExistingReversal(existing, source, different, differentHash, definitions)
	assert.ErrorIs(t, err, ErrJournalEventKeyConflict)

	existing.Journal.Entries[0].AmountRupiah = 2
	_, err = replayExistingReversal(existing, source, reversalNormalized, reversalHash, definitions)
	assert.ErrorIs(t, err, ErrJournalIntegrity)
}
