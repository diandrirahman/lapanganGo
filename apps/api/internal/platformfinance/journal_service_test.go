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

	for _, err := range []error{ErrJournalPersistence, ErrJournalEventKeyConflict, ErrInvalidJournalReference} {
		assert.NotContains(t, err.Error(), "platform_journals")
		assert.NotContains(t, err.Error(), "SQL")
	}
}
