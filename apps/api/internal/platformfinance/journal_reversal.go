package platformfinance

import (
	"context"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

func (s *journalService) ReverseJournal(ctx context.Context, tx pgx.Tx, params ReverseJournalParams) (*PostedJournal, error) {
	normalizedReverse, err := normalizeReverseJournalParams(params)
	if err != nil {
		return nil, err
	}
	if tx == nil {
		return nil, ErrJournalTransactionRequired
	}

	source, err := s.repository.GetJournalByIDForReversal(ctx, tx, normalizedReverse.JournalID)
	if err != nil {
		return nil, normalizeJournalServiceError(err)
	}
	if source.CreatedInCurrentTx || source.Journal.ReversesJournalID != nil || source.ReversesJournalID != nil {
		return nil, ErrInvalidJournalRequest
	}

	accountCodes := journalAccountCodesFromPostedEntries(source.Journal.Entries)
	definitions, err := s.repository.GetAccountDefinitions(ctx, tx, accountCodes)
	if err != nil {
		return nil, normalizeJournalServiceError(err)
	}
	sourceParams := postJournalParamsFromPostedJournal(source.Journal)
	if _, err := replayExistingJournal(source, sourceParams, source.Journal.PayloadHash, definitions); err != nil {
		return nil, normalizeJournalServiceError(err)
	}

	reversalParams := PostJournalParams{
		EventKey:        "journal.reversed:" + source.Journal.ID,
		EventType:       JournalReversalEventType,
		BookingID:       source.Journal.BookingID,
		OwnerProfileID:  source.Journal.OwnerProfileID,
		VenueID:         source.Journal.VenueID,
		EffectiveAt:     normalizedReverse.EffectiveAt,
		CreatedByUserID: normalizedReverse.CreatedByUserID,
		Metadata:        cloneJournalMetadata(normalizedReverse.Metadata),
		Entries:         invertPostedJournalEntries(source.Journal.Entries),
	}
	normalized, _, err := validateAndNormalizeReversalJournal(reversalParams)
	if err != nil {
		return nil, err
	}
	if err := validateJournalAccountDimensions(normalized, definitions); err != nil {
		return nil, err
	}

	sourceID := source.Journal.ID
	payloadHash, err := hashJournalPayloadV1WithReversal(normalized, &sourceID, &normalizedReverse.Reason)
	if err != nil {
		return nil, normalizeJournalServiceError(err)
	}

	existing, err := s.repository.GetReversalBySourceID(ctx, tx, sourceID)
	if err != nil {
		return nil, normalizeJournalServiceError(err)
	}
	if existing != nil {
		return replayExistingReversal(existing, source, normalized, payloadHash, definitions)
	}

	prepared := preparedJournal{
		ID:                 uuid.NewString(),
		EventKey:           normalized.EventKey,
		EventType:          normalized.EventType,
		PayloadHash:        payloadHash,
		PayloadHashVersion: JournalPayloadHashVersionV1,
		BookingID:          normalized.BookingID,
		OwnerProfileID:     normalized.OwnerProfileID,
		VenueID:            normalized.VenueID,
		Currency:           JournalCurrencyIDR,
		EffectiveAt:        normalized.EffectiveAt,
		ReversesJournalID:  &sourceID,
		ReversalReason:     &normalizedReverse.Reason,
		CreatedByUserID:    normalized.CreatedByUserID,
		Description:        normalized.Description,
		Metadata:           cloneJournalMetadata(normalized.Metadata),
		Entries:            make([]preparedJournalEntry, 0, len(normalized.Entries)),
	}
	for _, entry := range normalized.Entries {
		prepared.Entries = append(prepared.Entries, preparedJournalEntry{
			ID:             uuid.NewString(),
			AccountCode:    entry.AccountCode,
			OwnerProfileID: entry.OwnerProfileID,
			Side:           entry.Side,
			AmountRupiah:   entry.AmountRupiah,
		})
	}

	posted, inserted, err := s.repository.TryInsertJournal(ctx, tx, prepared)
	if err != nil {
		return nil, normalizeJournalServiceError(err)
	}
	if !inserted {
		existing, err := s.repository.GetReversalBySourceID(ctx, tx, sourceID)
		if err != nil {
			return nil, normalizeJournalServiceError(err)
		}
		if existing == nil {
			return nil, ErrJournalIntegrity
		}
		return replayExistingReversal(existing, source, normalized, payloadHash, definitions)
	}

	entries, err := s.repository.InsertEntries(ctx, tx, prepared.ID, prepared.Entries)
	if err != nil {
		return nil, normalizeJournalServiceError(err)
	}
	sortPostedJournalEntries(entries)
	posted.Entries = entries
	return posted, nil
}

func normalizeReverseJournalParams(params ReverseJournalParams) (ReverseJournalParams, error) {
	parsedJournalID, err := uuid.Parse(params.JournalID)
	if err != nil {
		return ReverseJournalParams{}, ErrInvalidJournalReference
	}
	if params.EffectiveAt.IsZero() {
		return ReverseJournalParams{}, ErrInvalidJournalRequest
	}
	if params.Reason == "" || strings.TrimSpace(params.Reason) != params.Reason || len([]byte(params.Reason)) > 500 {
		return ReverseJournalParams{}, ErrInvalidJournalRequest
	}
	if err := validateJournalMetadata(params.Metadata); err != nil {
		return ReverseJournalParams{}, err
	}
	createdBy, err := canonicalOptionalUUID(params.CreatedByUserID)
	if err != nil {
		return ReverseJournalParams{}, ErrInvalidJournalReference
	}
	return ReverseJournalParams{
		JournalID:       parsedJournalID.String(),
		Reason:          params.Reason,
		EffectiveAt:     params.EffectiveAt.UTC().Truncate(time.Microsecond),
		CreatedByUserID: createdBy,
		Metadata:        cloneJournalMetadata(params.Metadata),
	}, nil
}

func postJournalParamsFromPostedJournal(journal PostedJournal) PostJournalParams {
	params := PostJournalParams{
		EventKey:        journal.EventKey,
		EventType:       journal.EventType,
		BookingID:       journal.BookingID,
		OwnerProfileID:  journal.OwnerProfileID,
		VenueID:         journal.VenueID,
		EffectiveAt:     journal.EffectiveAt,
		CreatedByUserID: journal.CreatedByUserID,
		Description:     journal.Description,
		Metadata:        cloneJournalMetadata(journal.Metadata),
		Entries:         make([]PostJournalEntry, 0, len(journal.Entries)),
	}
	for _, entry := range journal.Entries {
		params.Entries = append(params.Entries, PostJournalEntry{
			AccountCode:    entry.AccountCode,
			OwnerProfileID: entry.OwnerProfileID,
			Side:           entry.Side,
			AmountRupiah:   entry.AmountRupiah,
		})
	}
	return params
}

func invertPostedJournalEntries(entries []PostedJournalEntry) []PostJournalEntry {
	inverse := make([]PostJournalEntry, 0, len(entries))
	for _, entry := range entries {
		inverse = append(inverse, PostJournalEntry{
			AccountCode:    entry.AccountCode,
			OwnerProfileID: entry.OwnerProfileID,
			Side:           invertJournalSide(entry.Side),
			AmountRupiah:   entry.AmountRupiah,
		})
	}
	return inverse
}

func invertJournalSide(side JournalSide) JournalSide {
	if side == JournalSideDebit {
		return JournalSideCredit
	}
	return JournalSideDebit
}

func journalAccountCodesFromPostedEntries(entries []PostedJournalEntry) []string {
	seen := make(map[string]struct{}, len(entries))
	for _, entry := range entries {
		seen[entry.AccountCode] = struct{}{}
	}
	codes := make([]string, 0, len(seen))
	for code := range seen {
		codes = append(codes, code)
	}
	sort.Strings(codes)
	return codes
}

func replayExistingReversal(existing, source *loadedJournal, requested PostJournalParams, requestedHash string, definitions map[string]JournalAccountDefinition) (*PostedJournal, error) {
	if existing == nil || source == nil {
		return nil, ErrJournalIntegrity
	}
	if existing.Journal.PayloadHash != requestedHash {
		return nil, ErrJournalEventKeyConflict
	}
	if existing.Journal.PayloadHashVersion != JournalPayloadHashVersionV1 || existing.Journal.Currency != JournalCurrencyIDR || existing.Journal.PostedAt.IsZero() || existing.Journal.PostedAt.Before(existing.Journal.EffectiveAt) {
		return nil, ErrJournalIntegrity
	}
	sourceID := source.Journal.ID
	if !sameJournalValue(existing.Journal.ReversesJournalID, &sourceID) || existing.Journal.EventKey != "journal.reversed:"+sourceID || existing.Journal.EventType != JournalReversalEventType || existing.Journal.Description != nil {
		return nil, ErrJournalIntegrity
	}
	if !sameJournalValue(existing.Journal.BookingID, source.Journal.BookingID) || !sameJournalValue(existing.Journal.OwnerProfileID, source.Journal.OwnerProfileID) || !sameJournalValue(existing.Journal.VenueID, source.Journal.VenueID) {
		return nil, ErrJournalIntegrity
	}
	if existing.Journal.ReversalReason == nil {
		return nil, ErrJournalIntegrity
	}
	if existing.Journal.ReversalReason == nil || strings.TrimSpace(*existing.Journal.ReversalReason) != *existing.Journal.ReversalReason || len([]byte(*existing.Journal.ReversalReason)) > 500 {
		return nil, ErrJournalIntegrity
	}

	storedParams := postJournalParamsFromPostedJournal(existing.Journal)
	normalized, _, err := validateAndNormalizeReversalJournal(storedParams)
	if err != nil {
		return nil, ErrJournalIntegrity
	}
	if err := validateJournalAccountDimensions(normalized, definitions); err != nil {
		return nil, ErrJournalIntegrity
	}
	storedHash, err := hashJournalPayloadV1WithReversal(normalized, &sourceID, existing.Journal.ReversalReason)
	if err != nil || storedHash != existing.Journal.PayloadHash {
		return nil, ErrJournalIntegrity
	}
	if !exactReversalEntryMultisetMatches(source.Journal.Entries, existing.Journal.Entries) {
		return nil, ErrJournalIntegrity
	}
	if normalized.EventKey != requested.EventKey || normalized.EventType != requested.EventType {
		return nil, ErrJournalIntegrity
	}
	sortPostedJournalEntries(existing.Journal.Entries)
	return &existing.Journal, nil
}

func sameJournalValue(left, right *string) bool {
	if left == nil || right == nil {
		return left == nil && right == nil
	}
	return *left == *right
}

type journalEntryTuple struct {
	AccountCode    string
	OwnerProfileID string
	HasOwner       bool
	Side           JournalSide
	AmountRupiah   int64
}

func exactReversalEntryMultisetMatches(source, reversal []PostedJournalEntry) bool {
	expected := make(map[journalEntryTuple]int, len(source))
	for _, entry := range source {
		key := journalEntryTuple{
			AccountCode:  entry.AccountCode,
			Side:         invertJournalSide(entry.Side),
			AmountRupiah: entry.AmountRupiah,
		}
		if entry.OwnerProfileID != nil {
			key.OwnerProfileID = *entry.OwnerProfileID
			key.HasOwner = true
		}
		expected[key]++
	}
	for _, entry := range reversal {
		key := journalEntryTuple{
			AccountCode:  entry.AccountCode,
			Side:         entry.Side,
			AmountRupiah: entry.AmountRupiah,
		}
		if entry.OwnerProfileID != nil {
			key.OwnerProfileID = *entry.OwnerProfileID
			key.HasOwner = true
		}
		expected[key]--
	}
	for _, count := range expected {
		if count != 0 {
			return false
		}
	}
	return true
}
