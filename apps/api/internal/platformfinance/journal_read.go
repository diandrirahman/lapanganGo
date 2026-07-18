package platformfinance

import (
	"context"
	"errors"
	"regexp"
	"strconv"
	"time"

	"github.com/google/uuid"
)

const (
	JournalReadDefaultPage  = 1
	JournalReadDefaultLimit = 20
	JournalReadMaxLimit     = 100
)

var (
	ErrInvalidJournalReadService = errors.New("INVALID_JOURNAL_READ_SERVICE")
	ErrInvalidJournalReadQuery   = errors.New("INVALID_JOURNAL_READ_QUERY")
)

// JournalListQuery is the internal, read-only filter contract for platform
// journals. EffectiveFrom is inclusive and EffectiveTo is exclusive.
type JournalListQuery struct {
	EffectiveFrom  *time.Time
	EffectiveTo    *time.Time
	EventType      string
	AccountCode    string
	JournalID      string
	OwnerProfileID string
	VenueID        string
	BookingID      string
	Page           int
	Limit          int
}

// JournalReadService exposes ledger facts without exposing domain-specific
// payment, OPEX, or projection metrics.
type JournalReadService interface {
	ListJournals(ctx context.Context, query JournalListQuery) (*JournalPage, error)
	GetSummary(ctx context.Context, query JournalListQuery) (*JournalSummary, error)
}

type JournalListItem struct {
	ID                  string    `json:"id"`
	EventKey            string    `json:"event_key"`
	EventType           string    `json:"event_type"`
	BookingID           *string   `json:"booking_id"`
	OwnerProfileID      *string   `json:"owner_profile_id"`
	VenueID             *string   `json:"venue_id"`
	Currency            string    `json:"currency"`
	EffectiveAt         time.Time `json:"effective_at"`
	PostedAt            time.Time `json:"posted_at"`
	ReversesJournalID   *string   `json:"reverses_journal_id"`
	ReversalReason      *string   `json:"reversal_reason"`
	ReversedByJournalID *string   `json:"reversed_by_journal_id"`
	EntryCount          int       `json:"entry_count"`
	DebitTotalRupiah    string    `json:"debit_total_rupiah"`
	CreditTotalRupiah   string    `json:"credit_total_rupiah"`
}

type JournalPage struct {
	Items      []JournalListItem `json:"data"`
	TotalItems int               `json:"total_items"`
	TotalPages int               `json:"total_pages"`
	Page       int               `json:"page"`
	Limit      int               `json:"limit"`
}

type JournalSummary struct {
	Currency          string `json:"currency"`
	JournalCount      int    `json:"journal_count"`
	ReversalCount     int    `json:"reversal_count"`
	TotalDebitRupiah  string `json:"total_debit_rupiah"`
	TotalCreditRupiah string `json:"total_credit_rupiah"`
}

type journalReadRepositoryData struct {
	Items      []journalReadRow
	TotalItems int64
}

type journalReadRow struct {
	ID                  string
	EventKey            string
	EventType           string
	BookingID           *string
	OwnerProfileID      *string
	VenueID             *string
	Currency            string
	EffectiveAt         time.Time
	PostedAt            time.Time
	ReversesJournalID   *string
	ReversalReason      *string
	ReversedByJournalID *string
	EntryCount          int64
	DebitTotalRupiah    int64
	CreditTotalRupiah   int64
}

type journalReadSummaryData struct {
	JournalCount      int64
	ReversalCount     int64
	TotalDebitRupiah  int64
	TotalCreditRupiah int64
}

type journalReadService struct {
	repository JournalReadRepository
}

func NewJournalReadService(repository JournalReadRepository) (JournalReadService, error) {
	if repository == nil {
		return nil, ErrInvalidJournalReadService
	}
	return &journalReadService{repository: repository}, nil
}

func (s *journalReadService) ListJournals(ctx context.Context, query JournalListQuery) (*JournalPage, error) {
	normalized, err := normalizeJournalListQuery(query)
	if err != nil {
		return nil, err
	}
	data, err := s.repository.ListJournals(ctx, normalized)
	if err != nil {
		return nil, normalizeJournalReadError(err)
	}
	if data == nil {
		return nil, ErrJournalIntegrity
	}
	totalItems, err := journalReadInt(data.TotalItems)
	if err != nil {
		return nil, err
	}
	items := make([]JournalListItem, 0, len(data.Items))
	for _, row := range data.Items {
		entryCount, err := journalReadInt(row.EntryCount)
		if err != nil {
			return nil, err
		}
		items = append(items, JournalListItem{
			ID:                  row.ID,
			EventKey:            row.EventKey,
			EventType:           row.EventType,
			BookingID:           cloneOptionalString(row.BookingID),
			OwnerProfileID:      cloneOptionalString(row.OwnerProfileID),
			VenueID:             cloneOptionalString(row.VenueID),
			Currency:            row.Currency,
			EffectiveAt:         row.EffectiveAt,
			PostedAt:            row.PostedAt,
			ReversesJournalID:   cloneOptionalString(row.ReversesJournalID),
			ReversalReason:      cloneOptionalString(row.ReversalReason),
			ReversedByJournalID: cloneOptionalString(row.ReversedByJournalID),
			EntryCount:          entryCount,
			DebitTotalRupiah:    strconv.FormatInt(row.DebitTotalRupiah, 10),
			CreditTotalRupiah:   strconv.FormatInt(row.CreditTotalRupiah, 10),
		})
	}
	totalPages := 0
	if totalItems > 0 {
		totalPages = (totalItems-1)/normalized.Limit + 1
	}
	return &JournalPage{
		Items:      items,
		TotalItems: totalItems,
		TotalPages: totalPages,
		Page:       normalized.Page,
		Limit:      normalized.Limit,
	}, nil
}

func (s *journalReadService) GetSummary(ctx context.Context, query JournalListQuery) (*JournalSummary, error) {
	normalized, err := normalizeJournalListQuery(query)
	if err != nil {
		return nil, err
	}
	data, err := s.repository.GetSummary(ctx, normalized)
	if err != nil {
		return nil, normalizeJournalReadError(err)
	}
	if data == nil {
		return nil, ErrJournalIntegrity
	}
	journalCount, err := journalReadInt(data.JournalCount)
	if err != nil {
		return nil, err
	}
	reversalCount, err := journalReadInt(data.ReversalCount)
	if err != nil {
		return nil, err
	}
	return &JournalSummary{
		Currency:          JournalCurrencyIDR,
		JournalCount:      journalCount,
		ReversalCount:     reversalCount,
		TotalDebitRupiah:  strconv.FormatInt(data.TotalDebitRupiah, 10),
		TotalCreditRupiah: strconv.FormatInt(data.TotalCreditRupiah, 10),
	}, nil
}

func normalizeJournalListQuery(query JournalListQuery) (JournalListQuery, error) {
	if query.Page == 0 {
		query.Page = JournalReadDefaultPage
	}
	if query.Limit == 0 {
		query.Limit = JournalReadDefaultLimit
	}
	if query.Page < 1 || query.Limit < 1 {
		return JournalListQuery{}, ErrInvalidJournalReadQuery
	}
	if query.Limit > JournalReadMaxLimit {
		query.Limit = JournalReadMaxLimit
	}
	if query.Limit > 1 {
		if query.Page > (maxJournalReadInt()/query.Limit)+1 {
			return JournalListQuery{}, ErrInvalidJournalReadQuery
		}
	}
	if query.EffectiveFrom != nil {
		if query.EffectiveFrom.IsZero() {
			return JournalListQuery{}, ErrInvalidJournalReadQuery
		}
		value := query.EffectiveFrom.UTC()
		query.EffectiveFrom = &value
	}
	if query.EffectiveTo != nil {
		if query.EffectiveTo.IsZero() {
			return JournalListQuery{}, ErrInvalidJournalReadQuery
		}
		value := query.EffectiveTo.UTC()
		query.EffectiveTo = &value
	}
	if query.EffectiveFrom != nil && query.EffectiveTo != nil && !query.EffectiveFrom.Before(*query.EffectiveTo) {
		return JournalListQuery{}, ErrInvalidJournalReadQuery
	}
	if query.EventType != "" && !journalEventTypePattern.MatchString(query.EventType) {
		return JournalListQuery{}, ErrInvalidJournalReadQuery
	}
	if query.AccountCode != "" && !journalReadAccountCodePattern.MatchString(query.AccountCode) {
		return JournalListQuery{}, ErrInvalidJournalReadQuery
	}
	for _, value := range []string{query.JournalID, query.OwnerProfileID, query.VenueID, query.BookingID} {
		if value == "" {
			continue
		}
		if _, err := parseJournalReadUUID(value); err != nil {
			return JournalListQuery{}, ErrInvalidJournalReadQuery
		}
	}
	if query.JournalID != "" {
		query.JournalID = canonicalJournalReadUUID(query.JournalID)
	}
	if query.OwnerProfileID != "" {
		query.OwnerProfileID = canonicalJournalReadUUID(query.OwnerProfileID)
	}
	if query.VenueID != "" {
		query.VenueID = canonicalJournalReadUUID(query.VenueID)
	}
	if query.BookingID != "" {
		query.BookingID = canonicalJournalReadUUID(query.BookingID)
	}
	return query, nil
}

var journalReadAccountCodePattern = regexp.MustCompile(`^[A-Z][A-Z0-9_]{0,79}$`)

func parseJournalReadUUID(value string) (uuid.UUID, error) {
	return uuid.Parse(value)
}

func canonicalJournalReadUUID(value string) string {
	parsed, err := uuid.Parse(value)
	if err != nil {
		return ""
	}
	return parsed.String()
}

func cloneOptionalString(value *string) *string {
	if value == nil {
		return nil
	}
	cloned := *value
	return &cloned
}

func journalReadInt(value int64) (int, error) {
	if value < 0 || value > int64(maxJournalReadInt()) {
		return 0, ErrJournalIntegrity
	}
	return int(value), nil
}

func maxJournalReadInt() int {
	return int(^uint(0) >> 1)
}

func normalizeJournalReadError(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, ErrInvalidJournalReadQuery) || errors.Is(err, ErrJournalIntegrity) || errors.Is(err, ErrJournalAmountOverflow) || errors.Is(err, ErrJournalPersistence) || errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return err
	}
	return mapJournalRepositoryError(err)
}
