package platformfinance

import (
	"context"
	"errors"
	"math"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

var (
	journalEventKeyPattern          = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]*\.[a-z0-9][a-z0-9_-]*:[a-z0-9][a-z0-9._-]*(:[a-z0-9][a-z0-9._-]*)?$`)
	journalEventTypePattern         = regexp.MustCompile(`^[A-Z][A-Z0-9_]{0,79}$`)
	journalSensitiveMetadataPattern = regexp.MustCompile(`(?i)(secret|token|password|authorization|credential|payload|pii|bearer)`)
	journalMetadataAllowlist        = map[string]struct{}{
		"source_type":         {},
		"source_reference":    {},
		"reason_code":         {},
		"calculation_version": {},
	}
)

type JournalService interface {
	PostJournal(ctx context.Context, tx pgx.Tx, params PostJournalParams) (*PostedJournal, error)
}

type journalService struct {
	repository JournalRepository
}

func NewJournalService(repository JournalRepository) (JournalService, error) {
	if repository == nil {
		return nil, ErrInvalidJournalService
	}
	return &journalService{repository: repository}, nil
}

func (s *journalService) PostJournal(ctx context.Context, tx pgx.Tx, params PostJournalParams) (*PostedJournal, error) {
	normalized, accountCodes, err := validateAndNormalizeJournal(params)
	if err != nil {
		return nil, err
	}
	if tx == nil {
		return nil, ErrJournalTransactionRequired
	}

	definitions, err := s.repository.GetAccountDefinitions(ctx, tx, accountCodes)
	if err != nil {
		return nil, normalizeJournalServiceError(err)
	}
	if err := validateJournalAccountDimensions(normalized, definitions); err != nil {
		return nil, err
	}

	payloadHash, err := hashJournalPayloadV1(normalized)
	if err != nil {
		return nil, normalizeJournalServiceError(err)
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

	posted, err := s.repository.InsertJournal(ctx, tx, prepared)
	if err != nil {
		return nil, normalizeJournalServiceError(err)
	}
	entries, err := s.repository.InsertEntries(ctx, tx, prepared.ID, prepared.Entries)
	if err != nil {
		return nil, normalizeJournalServiceError(err)
	}
	posted.Entries = entries
	return posted, nil
}

func validateAndNormalizeJournal(params PostJournalParams) (PostJournalParams, []string, error) {
	if len(params.EventKey) == 0 || len(params.EventKey) > 191 || !journalEventKeyPattern.MatchString(params.EventKey) || strings.HasPrefix(params.EventKey, "journal.reversed:") {
		return PostJournalParams{}, nil, ErrInvalidJournalRequest
	}
	if !journalEventTypePattern.MatchString(params.EventType) {
		return PostJournalParams{}, nil, ErrInvalidJournalRequest
	}
	if params.EffectiveAt.IsZero() {
		return PostJournalParams{}, nil, ErrInvalidJournalRequest
	}
	if len(params.Entries) < 2 {
		return PostJournalParams{}, nil, ErrJournalRequiresTwoEntries
	}
	if err := validateJournalMetadata(params.Metadata); err != nil {
		return PostJournalParams{}, nil, err
	}

	normalized := params
	var err error
	if normalized.BookingID, err = canonicalOptionalUUID(params.BookingID); err != nil {
		return PostJournalParams{}, nil, ErrInvalidJournalReference
	}
	if normalized.OwnerProfileID, err = canonicalOptionalUUID(params.OwnerProfileID); err != nil {
		return PostJournalParams{}, nil, ErrInvalidJournalReference
	}
	if normalized.VenueID, err = canonicalOptionalUUID(params.VenueID); err != nil {
		return PostJournalParams{}, nil, ErrInvalidJournalReference
	}
	if normalized.CreatedByUserID, err = canonicalOptionalUUID(params.CreatedByUserID); err != nil {
		return PostJournalParams{}, nil, ErrInvalidJournalReference
	}
	normalized.EffectiveAt = params.EffectiveAt.UTC().Truncate(time.Microsecond)
	normalized.Metadata = cloneJournalMetadata(params.Metadata)
	normalized.Entries = make([]PostJournalEntry, 0, len(params.Entries))

	accountSet := make(map[string]struct{}, len(params.Entries))
	var debitTotal, creditTotal int64
	for _, entry := range params.Entries {
		if len(entry.AccountCode) == 0 || len(entry.AccountCode) > 80 {
			return PostJournalParams{}, nil, ErrUnknownJournalAccount
		}
		if entry.Side != JournalSideDebit && entry.Side != JournalSideCredit {
			return PostJournalParams{}, nil, ErrInvalidJournalRequest
		}
		if entry.AmountRupiah <= 0 {
			return PostJournalParams{}, nil, ErrInvalidJournalAmount
		}

		entry.OwnerProfileID, err = canonicalOptionalUUID(entry.OwnerProfileID)
		if err != nil {
			return PostJournalParams{}, nil, ErrInvalidJournalReference
		}
		if entry.Side == JournalSideDebit {
			debitTotal, err = checkedJournalAdd(debitTotal, entry.AmountRupiah)
		} else {
			creditTotal, err = checkedJournalAdd(creditTotal, entry.AmountRupiah)
		}
		if err != nil {
			return PostJournalParams{}, nil, err
		}
		normalized.Entries = append(normalized.Entries, entry)
		accountSet[entry.AccountCode] = struct{}{}
	}
	if debitTotal != creditTotal {
		return PostJournalParams{}, nil, ErrJournalUnbalanced
	}

	accountCodes := make([]string, 0, len(accountSet))
	for code := range accountSet {
		accountCodes = append(accountCodes, code)
	}
	sort.Strings(accountCodes)
	return normalized, accountCodes, nil
}

func validateJournalAccountDimensions(params PostJournalParams, definitions map[string]JournalAccountDefinition) error {
	for _, entry := range params.Entries {
		definition, ok := definitions[entry.AccountCode]
		if !ok {
			return ErrUnknownJournalAccount
		}
		switch definition.OwnerDimension {
		case JournalOwnerDimensionRequired:
			if entry.OwnerProfileID == nil || params.OwnerProfileID == nil || *entry.OwnerProfileID != *params.OwnerProfileID {
				return ErrInvalidJournalOwnerDimension
			}
		case JournalOwnerDimensionForbidden:
			if entry.OwnerProfileID != nil {
				return ErrInvalidJournalOwnerDimension
			}
		default:
			return ErrInvalidJournalOwnerDimension
		}
	}
	return nil
}

func validateJournalMetadata(metadata map[string]string) error {
	for key, value := range metadata {
		if _, ok := journalMetadataAllowlist[key]; !ok {
			return ErrInvalidJournalMetadata
		}
		if value == "" || strings.TrimSpace(value) != value || len(value) > 191 || journalSensitiveMetadataPattern.MatchString(value) {
			return ErrInvalidJournalMetadata
		}
	}
	return nil
}

func checkedJournalAdd(total, amount int64) (int64, error) {
	if amount <= 0 {
		return 0, ErrInvalidJournalAmount
	}
	if total > math.MaxInt64-amount {
		return 0, ErrJournalAmountOverflow
	}
	return total + amount, nil
}

func canonicalOptionalUUID(value *string) (*string, error) {
	if value == nil {
		return nil, nil
	}
	parsed, err := uuid.Parse(*value)
	if err != nil {
		return nil, err
	}
	canonical := parsed.String()
	return &canonical, nil
}

func cloneJournalMetadata(metadata map[string]string) map[string]string {
	cloned := make(map[string]string, len(metadata))
	for key, value := range metadata {
		cloned[key] = value
	}
	return cloned
}

func normalizeJournalServiceError(err error) error {
	known := []error{
		ErrInvalidJournalRequest,
		ErrJournalRequiresTwoEntries,
		ErrInvalidJournalAmount,
		ErrJournalAmountOverflow,
		ErrJournalUnbalanced,
		ErrUnknownJournalAccount,
		ErrInvalidJournalOwnerDimension,
		ErrInvalidJournalMetadata,
		ErrInvalidJournalReference,
		ErrJournalEventKeyConflict,
		ErrJournalPayloadHash,
		ErrJournalPersistence,
		context.Canceled,
		context.DeadlineExceeded,
	}
	for _, candidate := range known {
		if errors.Is(err, candidate) {
			return candidate
		}
	}
	return ErrJournalPersistence
}
