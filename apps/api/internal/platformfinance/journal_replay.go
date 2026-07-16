package platformfinance

import (
	"sort"

	"github.com/google/uuid"
)

func replayExistingJournal(existing *loadedJournal, requested PostJournalParams, requestedHash string, definitions map[string]JournalAccountDefinition) (*PostedJournal, error) {
	if existing == nil {
		return nil, ErrJournalIntegrity
	}
	if existing.Journal.PayloadHash != requestedHash {
		return nil, ErrJournalEventKeyConflict
	}
	if existing.Journal.PayloadHashVersion != JournalPayloadHashVersionV1 ||
		existing.ReversesJournalID != nil ||
		existing.ReversalReason != nil ||
		existing.Journal.Currency != JournalCurrencyIDR {
		return nil, ErrJournalIntegrity
	}
	if _, err := uuid.Parse(existing.Journal.ID); err != nil {
		return nil, ErrJournalIntegrity
	}
	for _, entry := range existing.Journal.Entries {
		if entry.JournalID != existing.Journal.ID {
			return nil, ErrJournalIntegrity
		}
		if _, err := uuid.Parse(entry.ID); err != nil {
			return nil, ErrJournalIntegrity
		}
	}

	storedParams := PostJournalParams{
		EventKey:        existing.Journal.EventKey,
		EventType:       existing.Journal.EventType,
		BookingID:       existing.Journal.BookingID,
		OwnerProfileID:  existing.Journal.OwnerProfileID,
		VenueID:         existing.Journal.VenueID,
		EffectiveAt:     existing.Journal.EffectiveAt,
		CreatedByUserID: existing.Journal.CreatedByUserID,
		Description:     existing.Journal.Description,
		Metadata:        cloneJournalMetadata(existing.Journal.Metadata),
		Entries:         make([]PostJournalEntry, 0, len(existing.Journal.Entries)),
	}
	for _, entry := range existing.Journal.Entries {
		storedParams.Entries = append(storedParams.Entries, PostJournalEntry{
			AccountCode:    entry.AccountCode,
			OwnerProfileID: entry.OwnerProfileID,
			Side:           entry.Side,
			AmountRupiah:   entry.AmountRupiah,
		})
	}

	normalized, _, err := validateAndNormalizeJournal(storedParams)
	if err != nil {
		return nil, ErrJournalIntegrity
	}
	if normalized.EventKey != requested.EventKey || normalized.EventType != requested.EventType {
		return nil, ErrJournalIntegrity
	}
	if err := validateJournalAccountDimensions(normalized, definitions); err != nil {
		return nil, ErrJournalIntegrity
	}
	storedHash, err := hashJournalPayloadV1(normalized)
	if err != nil || storedHash != existing.Journal.PayloadHash {
		return nil, ErrJournalIntegrity
	}

	sortPostedJournalEntries(existing.Journal.Entries)
	return &existing.Journal, nil
}

func sortPostedJournalEntries(entries []PostedJournalEntry) {
	sort.SliceStable(entries, func(i, j int) bool {
		if entries[i].AccountCode != entries[j].AccountCode {
			return entries[i].AccountCode < entries[j].AccountCode
		}
		leftOwner := optionalCanonicalValue(entries[i].OwnerProfileID)
		rightOwner := optionalCanonicalValue(entries[j].OwnerProfileID)
		if leftOwner != rightOwner {
			return leftOwner < rightOwner
		}
		if entries[i].Side != entries[j].Side {
			return entries[i].Side < entries[j].Side
		}
		if entries[i].AmountRupiah != entries[j].AmountRupiah {
			return entries[i].AmountRupiah < entries[j].AmountRupiah
		}
		return entries[i].ID < entries[j].ID
	})
}
