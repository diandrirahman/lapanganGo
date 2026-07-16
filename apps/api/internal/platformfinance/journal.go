package platformfinance

import (
	"errors"
	"time"
)

const (
	JournalCurrencyIDR          = "IDR"
	JournalPayloadHashVersionV1 = "JOURNAL_PAYLOAD_V1"

	JournalSideDebit  JournalSide = "DEBIT"
	JournalSideCredit JournalSide = "CREDIT"

	JournalOwnerDimensionRequired  = "REQUIRED"
	JournalOwnerDimensionForbidden = "FORBIDDEN"
)

var (
	ErrInvalidJournalService        = errors.New("INVALID_JOURNAL_SERVICE")
	ErrJournalTransactionRequired   = errors.New("JOURNAL_TRANSACTION_REQUIRED")
	ErrInvalidJournalRequest        = errors.New("INVALID_JOURNAL_REQUEST")
	ErrJournalRequiresTwoEntries    = errors.New("JOURNAL_REQUIRES_TWO_ENTRIES")
	ErrInvalidJournalAmount         = errors.New("INVALID_JOURNAL_AMOUNT")
	ErrJournalAmountOverflow        = errors.New("JOURNAL_AMOUNT_OVERFLOW")
	ErrJournalUnbalanced            = errors.New("JOURNAL_UNBALANCED")
	ErrUnknownJournalAccount        = errors.New("UNKNOWN_JOURNAL_ACCOUNT")
	ErrInvalidJournalOwnerDimension = errors.New("INVALID_JOURNAL_OWNER_DIMENSION")
	ErrInvalidJournalMetadata       = errors.New("INVALID_JOURNAL_METADATA")
	ErrInvalidJournalReference      = errors.New("INVALID_JOURNAL_REFERENCE")
	ErrJournalEventKeyConflict      = errors.New("JOURNAL_EVENT_KEY_CONFLICT")
	ErrJournalIntegrity             = errors.New("JOURNAL_INTEGRITY_FAILURE")
	ErrJournalPersistence           = errors.New("JOURNAL_PERSISTENCE_FAILED")
	ErrJournalPayloadHash           = errors.New("JOURNAL_PAYLOAD_HASH_FAILED")
)

const JournalReversalEventType = "JOURNAL_REVERSED"

type JournalSide string

type PostJournalEntry struct {
	AccountCode    string
	OwnerProfileID *string
	Side           JournalSide
	AmountRupiah   int64
}

type PostJournalParams struct {
	EventKey        string
	EventType       string
	BookingID       *string
	OwnerProfileID  *string
	VenueID         *string
	EffectiveAt     time.Time
	CreatedByUserID *string
	Description     *string
	Metadata        map[string]string
	Entries         []PostJournalEntry
}

type ReverseJournalParams struct {
	JournalID       string
	Reason          string
	EffectiveAt     time.Time
	CreatedByUserID *string
	Metadata        map[string]string
}

type PostedJournalEntry struct {
	ID             string
	JournalID      string
	AccountCode    string
	OwnerProfileID *string
	Side           JournalSide
	AmountRupiah   int64
	CreatedAt      time.Time
}

type PostedJournal struct {
	ID                 string
	EventKey           string
	EventType          string
	PayloadHash        string
	PayloadHashVersion string
	BookingID          *string
	OwnerProfileID     *string
	VenueID            *string
	Currency           string
	EffectiveAt        time.Time
	PostedAt           time.Time
	ReversesJournalID  *string
	ReversalReason     *string
	CreatedByUserID    *string
	Description        *string
	Metadata           map[string]string
	CreatedAt          time.Time
	Entries            []PostedJournalEntry
}

type JournalAccountDefinition struct {
	Code           string
	OwnerDimension string
}

type preparedJournal struct {
	ID                 string
	EventKey           string
	EventType          string
	PayloadHash        string
	PayloadHashVersion string
	BookingID          *string
	OwnerProfileID     *string
	VenueID            *string
	Currency           string
	EffectiveAt        time.Time
	ReversesJournalID  *string
	ReversalReason     *string
	CreatedByUserID    *string
	Description        *string
	Metadata           map[string]string
	Entries            []preparedJournalEntry
}

type preparedJournalEntry struct {
	ID             string
	AccountCode    string
	OwnerProfileID *string
	Side           JournalSide
	AmountRupiah   int64
}

type loadedJournal struct {
	Journal            PostedJournal
	ReversesJournalID  *string
	ReversalReason     *string
	CreatedInCurrentTx bool
}
