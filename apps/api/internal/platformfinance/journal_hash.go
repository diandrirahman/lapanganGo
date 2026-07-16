package platformfinance

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"sort"
	"strconv"
)

type canonicalJournalMetadata struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}

type canonicalJournalEntry struct {
	AccountCode    string  `json:"account_code"`
	OwnerProfileID *string `json:"owner_profile_id"`
	Side           string  `json:"side"`
	AmountRupiah   string  `json:"amount_rupiah"`
}

type canonicalJournalPayloadV1 struct {
	Version           string                     `json:"version"`
	EventKey          string                     `json:"event_key"`
	EventType         string                     `json:"event_type"`
	BookingID         *string                    `json:"booking_id"`
	OwnerProfileID    *string                    `json:"owner_profile_id"`
	VenueID           *string                    `json:"venue_id"`
	Currency          string                     `json:"currency"`
	EffectiveAt       string                     `json:"effective_at"`
	CreatedByUserID   *string                    `json:"created_by_user_id"`
	Description       *string                    `json:"description"`
	Metadata          []canonicalJournalMetadata `json:"metadata"`
	Entries           []canonicalJournalEntry    `json:"entries"`
	ReversesJournalID *string                    `json:"reverses_journal_id"`
	ReversalReason    *string                    `json:"reversal_reason"`
}

func hashJournalPayloadV1(params PostJournalParams) (string, error) {
	metadata := make([]canonicalJournalMetadata, 0, len(params.Metadata))
	for key, value := range params.Metadata {
		metadata = append(metadata, canonicalJournalMetadata{Key: key, Value: value})
	}
	sort.Slice(metadata, func(i, j int) bool { return metadata[i].Key < metadata[j].Key })

	entries := make([]canonicalJournalEntry, 0, len(params.Entries))
	for _, entry := range params.Entries {
		entries = append(entries, canonicalJournalEntry{
			AccountCode:    entry.AccountCode,
			OwnerProfileID: entry.OwnerProfileID,
			Side:           string(entry.Side),
			AmountRupiah:   strconv.FormatInt(entry.AmountRupiah, 10),
		})
	}
	sort.SliceStable(entries, func(i, j int) bool {
		leftOwner := optionalCanonicalValue(entries[i].OwnerProfileID)
		rightOwner := optionalCanonicalValue(entries[j].OwnerProfileID)
		if entries[i].AccountCode != entries[j].AccountCode {
			return entries[i].AccountCode < entries[j].AccountCode
		}
		if leftOwner != rightOwner {
			return leftOwner < rightOwner
		}
		if entries[i].Side != entries[j].Side {
			return entries[i].Side < entries[j].Side
		}
		return entries[i].AmountRupiah < entries[j].AmountRupiah
	})

	payload := canonicalJournalPayloadV1{
		Version:         JournalPayloadHashVersionV1,
		EventKey:        params.EventKey,
		EventType:       params.EventType,
		BookingID:       params.BookingID,
		OwnerProfileID:  params.OwnerProfileID,
		VenueID:         params.VenueID,
		Currency:        JournalCurrencyIDR,
		EffectiveAt:     params.EffectiveAt.UTC().Format("2006-01-02T15:04:05.000000Z"),
		CreatedByUserID: params.CreatedByUserID,
		Description:     params.Description,
		Metadata:        metadata,
		Entries:         entries,
	}

	encoded, err := json.Marshal(payload)
	if err != nil {
		return "", ErrJournalPayloadHash
	}
	sum := sha256.Sum256(encoded)
	return hex.EncodeToString(sum[:]), nil
}

func optionalCanonicalValue(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}
