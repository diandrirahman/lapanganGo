package platformfinance

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestJournalReadPrimitivesListSummaryFiltersAndReversalLinks(t *testing.T) {
	_, _, pool := newLedgerMigrationDatabase(t, ledgerMigrationVersion)
	ctx := context.Background()
	postService, err := NewJournalService(NewJournalRepository())
	require.NoError(t, err)
	readService, err := NewJournalReadService(NewJournalReadRepository(pool))
	require.NoError(t, err)

	ownerID := insertOwnerFixture(t, pool)
	venueID, bookingID := insertReadBookingFixture(t, pool, ownerID)
	base := time.Now().UTC().Add(-24 * time.Hour).Truncate(time.Microsecond)
	source := postReadFixtureJournal(t, pool, postService, PostJournalParams{
		EventKey:       "read.source:" + uuid.NewString(),
		EventType:      "READ_SOURCE",
		BookingID:      &bookingID,
		OwnerProfileID: &ownerID,
		VenueID:        &venueID,
		EffectiveAt:    base,
		Entries: []PostJournalEntry{
			{AccountCode: "BANK_CASH", Side: JournalSideDebit, AmountRupiah: 2},
			{AccountCode: "BANK_CASH", Side: JournalSideDebit, AmountRupiah: 1},
			{AccountCode: "FUNDING_CLEARING", Side: JournalSideCredit, AmountRupiah: 3},
		},
	})
	matching := postReadFixtureJournal(t, pool, postService, PostJournalParams{
		EventKey:    "read.match:" + uuid.NewString(),
		EventType:   "READ_MATCH",
		EffectiveAt: base,
		Entries: []PostJournalEntry{
			{AccountCode: "BANK_CASH", Side: JournalSideDebit, AmountRupiah: 10},
			{AccountCode: "FUNDING_CLEARING", Side: JournalSideCredit, AmountRupiah: 10},
		},
	})
	postReadFixtureJournal(t, pool, postService, PostJournalParams{
		EventKey:    "read.old:" + uuid.NewString(),
		EventType:   "READ_OLD",
		EffectiveAt: base.Add(-time.Hour),
		Entries: []PostJournalEntry{
			{AccountCode: "PSP_CLEARING", Side: JournalSideDebit, AmountRupiah: 5},
			{AccountCode: "FUNDING_CLEARING", Side: JournalSideCredit, AmountRupiah: 5},
		},
	})
	reversal := reverseReadFixtureJournal(t, pool, postService, source.ID, base.Add(time.Hour))

	var beforeJournals, beforeEntries int
	require.NoError(t, pool.QueryRow(ctx, "SELECT COUNT(*) FROM platform_journals").Scan(&beforeJournals))
	require.NoError(t, pool.QueryRow(ctx, "SELECT COUNT(*) FROM platform_ledger_entries").Scan(&beforeEntries))

	page, err := readService.ListJournals(ctx, JournalListQuery{Limit: 100})
	require.NoError(t, err)
	assert.Equal(t, 4, page.TotalItems)
	assert.Equal(t, 1, page.TotalPages)
	assert.Len(t, page.Items, 4)
	assert.Equal(t, reversal.ID, page.Items[0].ID, "effective_at must be the primary ordering key")
	higherTieID, lowerTieID := source.ID, matching.ID
	if lowerTieID > higherTieID {
		higherTieID, lowerTieID = lowerTieID, higherTieID
	}
	assert.Equal(t, higherTieID, page.Items[1].ID, "identical effective_at must be ordered by id DESC")
	assert.Equal(t, lowerTieID, page.Items[2].ID, "identical effective_at must be ordered by id DESC")
	assert.Equal(t, source.ID, findReadItem(page.Items, source.ID).ID)
	assert.Equal(t, reversal.ID, *findReadItem(page.Items, source.ID).ReversedByJournalID)
	assert.Equal(t, source.ID, *findReadItem(page.Items, reversal.ID).ReversesJournalID)
	assert.Equal(t, "manual correction", *findReadItem(page.Items, reversal.ID).ReversalReason)

	pageOne, err := readService.ListJournals(ctx, JournalListQuery{Page: 1, Limit: 2})
	require.NoError(t, err)
	pageTwo, err := readService.ListJournals(ctx, JournalListQuery{Page: 2, Limit: 2})
	require.NoError(t, err)
	assert.Equal(t, 4, pageOne.TotalItems)
	assert.Len(t, pageOne.Items, 2)
	assert.Equal(t, 4, pageTwo.TotalItems)
	assert.Len(t, pageTwo.Items, 2)
	assert.Equal(t, page.Items[0].ID, pageOne.Items[0].ID)
	assert.Equal(t, page.Items[1].ID, pageOne.Items[1].ID)
	assert.Equal(t, page.Items[2].ID, pageTwo.Items[0].ID)
	assert.Equal(t, page.Items[3].ID, pageTwo.Items[1].ID)
	assert.NotEqual(t, pageOne.Items[0].ID, pageTwo.Items[0].ID, "pagination pages must not overlap")

	from := base
	to := base.Add(time.Hour)
	ranged, err := readService.ListJournals(ctx, JournalListQuery{EffectiveFrom: &from, EffectiveTo: &to, Limit: 100})
	require.NoError(t, err)
	assert.Equal(t, 2, ranged.TotalItems)
	assert.Contains(t, readItemIDs(ranged.Items), source.ID)
	assert.Contains(t, readItemIDs(ranged.Items), matching.ID)

	eventOnly, err := readService.ListJournals(ctx, JournalListQuery{EventType: "READ_MATCH", Limit: 100})
	require.NoError(t, err)
	require.Len(t, eventOnly.Items, 1)
	assert.Equal(t, matching.ID, eventOnly.Items[0].ID)

	ownerOnly, err := readService.ListJournals(ctx, JournalListQuery{OwnerProfileID: ownerID, Limit: 100})
	require.NoError(t, err)
	assert.Equal(t, 2, ownerOnly.TotalItems)

	venueOnly, err := readService.ListJournals(ctx, JournalListQuery{VenueID: venueID, Limit: 100})
	require.NoError(t, err)
	assert.Equal(t, 2, venueOnly.TotalItems)

	bookingOnly, err := readService.ListJournals(ctx, JournalListQuery{BookingID: bookingID, Limit: 100})
	require.NoError(t, err)
	assert.Equal(t, 2, bookingOnly.TotalItems)

	accountSummary, err := readService.GetSummary(ctx, JournalListQuery{AccountCode: "BANK_CASH"})
	require.NoError(t, err)
	assert.Equal(t, 3, accountSummary.JournalCount)
	assert.Equal(t, 1, accountSummary.ReversalCount)
	assert.Equal(t, "16", accountSummary.TotalDebitRupiah)
	assert.Equal(t, "16", accountSummary.TotalCreditRupiah)

	allSummary, err := readService.GetSummary(ctx, JournalListQuery{})
	require.NoError(t, err)
	assert.Equal(t, 4, allSummary.JournalCount)
	assert.Equal(t, 1, allSummary.ReversalCount)
	assert.Equal(t, "21", allSummary.TotalDebitRupiah)
	assert.Equal(t, "21", allSummary.TotalCreditRupiah)

	empty, err := readService.ListJournals(ctx, JournalListQuery{AccountCode: "OPEX_OTHER", Limit: 100})
	require.NoError(t, err)
	assert.NotNil(t, empty.Items)
	assert.Empty(t, empty.Items)
	assert.Zero(t, empty.TotalItems)

	var afterJournals, afterEntries int
	require.NoError(t, pool.QueryRow(ctx, "SELECT COUNT(*) FROM platform_journals").Scan(&afterJournals))
	require.NoError(t, pool.QueryRow(ctx, "SELECT COUNT(*) FROM platform_ledger_entries").Scan(&afterEntries))
	assert.Equal(t, beforeJournals, afterJournals)
	assert.Equal(t, beforeEntries, afterEntries)
}

func postReadFixtureJournal(t *testing.T, pool *pgxpool.Pool, service JournalService, params PostJournalParams) *PostedJournal {
	t.Helper()
	ctx := context.Background()
	tx, err := pool.Begin(ctx)
	require.NoError(t, err)
	posted, err := service.PostJournal(ctx, tx, params)
	require.NoError(t, err)
	require.NoError(t, tx.Commit(ctx))
	return posted
}

func reverseReadFixtureJournal(t *testing.T, pool *pgxpool.Pool, service JournalService, sourceID string, effectiveAt time.Time) *PostedJournal {
	t.Helper()
	ctx := context.Background()
	tx, err := pool.Begin(ctx)
	require.NoError(t, err)
	reversed, err := service.ReverseJournal(ctx, tx, ReverseJournalParams{
		JournalID:   sourceID,
		Reason:      "manual correction",
		EffectiveAt: effectiveAt,
		Metadata:    map[string]string{"reason_code": "correction"},
	})
	require.NoError(t, err)
	require.NoError(t, tx.Commit(ctx))
	return reversed
}

func insertReadBookingFixture(t *testing.T, pool *pgxpool.Pool, ownerID string) (string, string) {
	t.Helper()
	ctx := context.Background()
	tx, err := pool.Begin(ctx)
	require.NoError(t, err)

	userID := uuid.NewString()
	venueID := uuid.NewString()
	courtID := uuid.NewString()
	bookingID := uuid.NewString()
	_, err = tx.Exec(ctx, `
		INSERT INTO users (id, name, email, password_hash, role, status)
		VALUES ($1, 'Journal Read Customer', $2, 'hash', 'CUSTOMER', 'ACTIVE')
	`, userID, "journal-read-"+userID+"@example.com")
	require.NoError(t, err)
	_, err = tx.Exec(ctx, `
		INSERT INTO venues (id, owner_profile_id, name, address, city, status)
		VALUES ($1, $2, $3, 'Read Address', 'Read City', 'ACTIVE')
	`, venueID, ownerID, "Read Venue "+venueID)
	require.NoError(t, err)
	_, err = tx.Exec(ctx, `
		INSERT INTO courts (id, venue_id, sport_id, name, location_type, price_per_hour, status)
		VALUES ($1, $2, (SELECT id FROM sports ORDER BY name LIMIT 1), $3, 'INDOOR', 100, 'ACTIVE')
	`, courtID, venueID, "Read Court "+courtID)
	require.NoError(t, err)
	_, err = tx.Exec(ctx, `
		INSERT INTO bookings (id, customer_id, court_id, booking_date, start_time, end_time, total_price, status)
		VALUES ($1, $2, $3, CURRENT_DATE, '10:00', '11:00', 100, 'PAID')
	`, bookingID, userID, courtID)
	require.NoError(t, err)
	require.NoError(t, tx.Commit(ctx))
	return venueID, bookingID
}

func findReadItem(items []JournalListItem, id string) JournalListItem {
	for _, item := range items {
		if item.ID == id {
			return item
		}
	}
	return JournalListItem{}
}

func readItemIDs(items []JournalListItem) []string {
	ids := make([]string, 0, len(items))
	for _, item := range items {
		ids = append(ids, item.ID)
	}
	return ids
}
