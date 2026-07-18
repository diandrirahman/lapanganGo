package platformfinance

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type fakeJournalReadRepository struct {
	listData     *journalReadRepositoryData
	summaryData  *journalReadSummaryData
	listErr      error
	summaryErr   error
	listQuery    JournalListQuery
	summaryQuery JournalListQuery
}

func (f *fakeJournalReadRepository) ListJournals(_ context.Context, query JournalListQuery) (*journalReadRepositoryData, error) {
	f.listQuery = query
	return f.listData, f.listErr
}

func (f *fakeJournalReadRepository) GetSummary(_ context.Context, query JournalListQuery) (*journalReadSummaryData, error) {
	f.summaryQuery = query
	return f.summaryData, f.summaryErr
}

func TestNormalizeJournalListQueryDefaultsClampAndCanonicalizes(t *testing.T) {
	from := time.Date(2026, time.July, 16, 10, 0, 0, 123456789, time.FixedZone("WIB", 7*60*60))
	to := from.Add(time.Hour)
	ownerID := uuid.NewString()
	journalID := uuid.NewString()
	query, err := normalizeJournalListQuery(JournalListQuery{
		EffectiveFrom:  &from,
		EffectiveTo:    &to,
		EventType:      "TEST_JOURNAL",
		AccountCode:    "BANK_CASH",
		JournalID:      journalID,
		OwnerProfileID: ownerID,
		Page:           2,
		Limit:          101,
	})
	require.NoError(t, err)
	assert.Equal(t, 2, query.Page)
	assert.Equal(t, JournalReadMaxLimit, query.Limit)
	assert.Equal(t, from.UTC(), *query.EffectiveFrom)
	assert.Equal(t, to.UTC(), *query.EffectiveTo)
	assert.Equal(t, ownerID, query.OwnerProfileID)
	assert.Equal(t, journalID, query.JournalID)

	defaults, err := normalizeJournalListQuery(JournalListQuery{})
	require.NoError(t, err)
	assert.Equal(t, JournalReadDefaultPage, defaults.Page)
	assert.Equal(t, JournalReadDefaultLimit, defaults.Limit)
}

func TestNormalizeJournalListQueryRejectsInvalidFiltersAndOverflow(t *testing.T) {
	validID := uuid.NewString()
	tests := []struct {
		name  string
		query JournalListQuery
	}{
		{name: "negative page", query: JournalListQuery{Page: -1}},
		{name: "negative limit", query: JournalListQuery{Limit: -1}},
		{name: "invalid event type", query: JournalListQuery{EventType: "not-an-event"}},
		{name: "invalid account code", query: JournalListQuery{AccountCode: "bank cash"}},
		{name: "invalid owner uuid", query: JournalListQuery{OwnerProfileID: "not-a-uuid"}},
		{name: "invalid journal uuid", query: JournalListQuery{JournalID: "not-a-uuid"}},
		{name: "reversed range", query: JournalListQuery{EffectiveFrom: timePtr(time.Now().UTC()), EffectiveTo: timePtr(time.Now().UTC().Add(-time.Second))}},
		{name: "page overflow", query: JournalListQuery{Page: maxJournalReadInt(), Limit: JournalReadMaxLimit}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := normalizeJournalListQuery(tt.query)
			assert.ErrorIs(t, err, ErrInvalidJournalReadQuery)
		})
	}

	query, err := normalizeJournalListQuery(JournalListQuery{OwnerProfileID: validID})
	require.NoError(t, err)
	assert.Equal(t, validID, query.OwnerProfileID)
}

func TestBuildJournalReadCTEIncludesExactJournalFilter(t *testing.T) {
	journalID := uuid.NewString()
	cte, args, nextArg := buildJournalReadCTE(JournalListQuery{JournalID: journalID}, 3)
	assert.Contains(t, cte, "j.id = $3")
	assert.Equal(t, []any{journalID}, args)
	assert.Equal(t, 4, nextArg)
}

func TestJournalReadServiceReturnsStableEmptyAndMoneyStrings(t *testing.T) {
	repo := &fakeJournalReadRepository{
		listData:    &journalReadRepositoryData{Items: []journalReadRow{}, TotalItems: 0},
		summaryData: &journalReadSummaryData{},
	}
	service, err := NewJournalReadService(repo)
	require.NoError(t, err)

	page, err := service.ListJournals(context.Background(), JournalListQuery{})
	require.NoError(t, err)
	assert.NotNil(t, page.Items)
	assert.Empty(t, page.Items)
	assert.Equal(t, 0, page.TotalPages)
	assert.Equal(t, JournalReadDefaultPage, page.Page)
	assert.Equal(t, JournalReadDefaultLimit, page.Limit)

	summary, err := service.GetSummary(context.Background(), JournalListQuery{})
	require.NoError(t, err)
	assert.Equal(t, JournalCurrencyIDR, summary.Currency)
	assert.Equal(t, 0, summary.JournalCount)
	assert.Equal(t, 0, summary.ReversalCount)
	assert.Equal(t, "0", summary.TotalDebitRupiah)
	assert.Equal(t, "0", summary.TotalCreditRupiah)
}

func TestJournalReadServiceMapsReversalLinksAndMoneyWithoutFloats(t *testing.T) {
	row := journalReadRow{
		ID:                  uuid.NewString(),
		EventKey:            "journal.reversed:" + uuid.NewString(),
		EventType:           JournalReversalEventType,
		Currency:            JournalCurrencyIDR,
		ReversesJournalID:   stringPtr(uuid.NewString()),
		ReversalReason:      stringPtr("correction"),
		ReversedByJournalID: nil,
		EntryCount:          2,
		DebitTotalRupiah:    9223372036854775807,
		CreditTotalRupiah:   9223372036854775807,
	}
	repo := &fakeJournalReadRepository{
		listData: &journalReadRepositoryData{Items: []journalReadRow{row}, TotalItems: 1},
	}
	service, err := NewJournalReadService(repo)
	require.NoError(t, err)

	page, err := service.ListJournals(context.Background(), JournalListQuery{Page: 1, Limit: 1})
	require.NoError(t, err)
	require.Len(t, page.Items, 1)
	assert.Equal(t, "9223372036854775807", page.Items[0].DebitTotalRupiah)
	assert.Equal(t, "9223372036854775807", page.Items[0].CreditTotalRupiah)
	assert.Equal(t, row.ReversesJournalID, page.Items[0].ReversesJournalID)
	assert.Equal(t, row.ReversalReason, page.Items[0].ReversalReason)
	assert.NotContains(t, page.Items[0].DebitTotalRupiah, ".")
}

func TestJournalReadServicePreservesKnownErrors(t *testing.T) {
	repo := &fakeJournalReadRepository{listErr: ErrJournalIntegrity}
	service, err := NewJournalReadService(repo)
	require.NoError(t, err)
	_, err = service.ListJournals(context.Background(), JournalListQuery{})
	assert.ErrorIs(t, err, ErrJournalIntegrity)

	repo.listErr = errors.New("unexpected")
	_, err = service.ListJournals(context.Background(), JournalListQuery{})
	assert.ErrorIs(t, err, ErrJournalPersistence)
}

func timePtr(value time.Time) *time.Time { return &value }

func stringPtr(value string) *string { return &value }
