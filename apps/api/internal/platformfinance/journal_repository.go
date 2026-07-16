package platformfinance

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

type JournalRepository interface {
	GetAccountDefinitions(ctx context.Context, tx pgx.Tx, accountCodes []string) (map[string]JournalAccountDefinition, error)
	InsertJournal(ctx context.Context, tx pgx.Tx, journal preparedJournal) (*PostedJournal, error)
	InsertEntries(ctx context.Context, tx pgx.Tx, journalID string, entries []preparedJournalEntry) ([]PostedJournalEntry, error)
}

type journalRepository struct{}

func NewJournalRepository() JournalRepository {
	return &journalRepository{}
}

func (r *journalRepository) GetAccountDefinitions(ctx context.Context, tx pgx.Tx, accountCodes []string) (map[string]JournalAccountDefinition, error) {
	rows, err := tx.Query(ctx, `
		SELECT code, owner_dimension
		FROM platform_accounts
		WHERE code = ANY($1)
	`, accountCodes)
	if err != nil {
		return nil, mapJournalRepositoryError(err)
	}
	defer rows.Close()

	definitions := make(map[string]JournalAccountDefinition, len(accountCodes))
	for rows.Next() {
		var definition JournalAccountDefinition
		if err := rows.Scan(&definition.Code, &definition.OwnerDimension); err != nil {
			return nil, mapJournalRepositoryError(err)
		}
		definitions[definition.Code] = definition
	}
	if err := rows.Err(); err != nil {
		return nil, mapJournalRepositoryError(err)
	}
	return definitions, nil
}

func (r *journalRepository) InsertJournal(ctx context.Context, tx pgx.Tx, journal preparedJournal) (*PostedJournal, error) {
	metadata, err := json.Marshal(journal.Metadata)
	if err != nil {
		return nil, ErrJournalPersistence
	}

	posted := &PostedJournal{
		ID:                 journal.ID,
		EventKey:           journal.EventKey,
		EventType:          journal.EventType,
		PayloadHash:        journal.PayloadHash,
		PayloadHashVersion: journal.PayloadHashVersion,
		BookingID:          journal.BookingID,
		OwnerProfileID:     journal.OwnerProfileID,
		VenueID:            journal.VenueID,
		Currency:           journal.Currency,
		EffectiveAt:        journal.EffectiveAt,
		CreatedByUserID:    journal.CreatedByUserID,
		Description:        journal.Description,
		Metadata:           cloneJournalMetadata(journal.Metadata),
	}

	err = tx.QueryRow(ctx, `
		INSERT INTO platform_journals (
			id, event_key, event_type, payload_hash, payload_hash_version,
			booking_id, owner_profile_id, venue_id, currency, effective_at,
			created_by_user_id, description, metadata
		) VALUES (
			$1, $2, $3, $4, $5,
			$6, $7, $8, $9, $10,
			$11, $12, $13
		)
		RETURNING posted_at, created_at
	`,
		journal.ID, journal.EventKey, journal.EventType, journal.PayloadHash, journal.PayloadHashVersion,
		journal.BookingID, journal.OwnerProfileID, journal.VenueID, journal.Currency, journal.EffectiveAt,
		journal.CreatedByUserID, journal.Description, metadata,
	).Scan(&posted.PostedAt, &posted.CreatedAt)
	if err != nil {
		return nil, mapJournalRepositoryError(err)
	}
	return posted, nil
}

func (r *journalRepository) InsertEntries(ctx context.Context, tx pgx.Tx, journalID string, entries []preparedJournalEntry) ([]PostedJournalEntry, error) {
	posted := make([]PostedJournalEntry, 0, len(entries))
	for _, entry := range entries {
		result := PostedJournalEntry{
			ID:             entry.ID,
			JournalID:      journalID,
			AccountCode:    entry.AccountCode,
			OwnerProfileID: entry.OwnerProfileID,
			Side:           entry.Side,
			AmountRupiah:   entry.AmountRupiah,
		}
		err := tx.QueryRow(ctx, `
			INSERT INTO platform_ledger_entries (
				id, journal_id, account_code, owner_profile_id, side, amount_rupiah
			) VALUES ($1, $2, $3, $4, $5, $6)
			RETURNING created_at
		`, entry.ID, journalID, entry.AccountCode, entry.OwnerProfileID, entry.Side, entry.AmountRupiah).Scan(&result.CreatedAt)
		if err != nil {
			return nil, mapJournalRepositoryError(err)
		}
		posted = append(posted, result)
	}
	return posted, nil
}

func mapJournalRepositoryError(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return err
	}

	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) {
		return ErrJournalPersistence
	}

	switch pgErr.Code {
	case "23505":
		if pgErr.ConstraintName == "platform_journals_event_key_key" {
			return ErrJournalEventKeyConflict
		}
	case "23503":
		return ErrInvalidJournalReference
	case "22003":
		return ErrJournalAmountOverflow
	case "23514":
		if pgErr.ConstraintName == "platform_journal_balance_guard" {
			return ErrJournalUnbalanced
		}
		return ErrInvalidJournalRequest
	}
	return ErrJournalPersistence
}
