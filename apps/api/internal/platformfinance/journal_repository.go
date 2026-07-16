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
	TryInsertJournal(ctx context.Context, tx pgx.Tx, journal preparedJournal) (*PostedJournal, bool, error)
	InsertEntries(ctx context.Context, tx pgx.Tx, journalID string, entries []preparedJournalEntry) ([]PostedJournalEntry, error)
	GetJournalByEventKey(ctx context.Context, tx pgx.Tx, eventKey string) (*loadedJournal, error)
	GetJournalByIDForReversal(ctx context.Context, tx pgx.Tx, journalID string) (*loadedJournal, error)
	GetReversalBySourceID(ctx context.Context, tx pgx.Tx, journalID string) (*loadedJournal, error)
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

func (r *journalRepository) TryInsertJournal(ctx context.Context, tx pgx.Tx, journal preparedJournal) (*PostedJournal, bool, error) {
	metadata, err := json.Marshal(journal.Metadata)
	if err != nil {
		return nil, false, ErrJournalPersistence
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
		ReversesJournalID:  journal.ReversesJournalID,
		ReversalReason:     journal.ReversalReason,
		CreatedByUserID:    journal.CreatedByUserID,
		Description:        journal.Description,
		Metadata:           cloneJournalMetadata(journal.Metadata),
	}

	err = tx.QueryRow(ctx, `
		INSERT INTO platform_journals (
			id, event_key, event_type, payload_hash, payload_hash_version,
			booking_id, owner_profile_id, venue_id, currency, effective_at,
			reverses_journal_id, reversal_reason, created_by_user_id, description, metadata
		) VALUES (
			$1, $2, $3, $4, $5,
			$6, $7, $8, $9, $10,
			$11, $12, $13, $14, $15
		)
		ON CONFLICT (event_key) DO NOTHING
		RETURNING posted_at, created_at
	`,
		journal.ID, journal.EventKey, journal.EventType, journal.PayloadHash, journal.PayloadHashVersion,
		journal.BookingID, journal.OwnerProfileID, journal.VenueID, journal.Currency, journal.EffectiveAt,
		journal.ReversesJournalID, journal.ReversalReason, journal.CreatedByUserID, journal.Description, metadata,
	).Scan(&posted.PostedAt, &posted.CreatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, false, nil
		}
		return nil, false, mapJournalRepositoryError(err)
	}
	return posted, true, nil
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

func (r *journalRepository) GetJournalByEventKey(ctx context.Context, tx pgx.Tx, eventKey string) (*loadedJournal, error) {
	return r.loadJournal(ctx, tx, `
		SELECT
			id, event_key, event_type, payload_hash, payload_hash_version,
			booking_id, owner_profile_id, venue_id, currency, effective_at,
			posted_at, reverses_journal_id, reversal_reason,
			created_by_user_id, description, metadata, created_at,
			(created_txid = txid_current()) AS created_in_current_tx
		FROM platform_journals
		WHERE event_key = $1
	`, eventKey)
}

func (r *journalRepository) GetJournalByIDForReversal(ctx context.Context, tx pgx.Tx, journalID string) (*loadedJournal, error) {
	loaded, err := r.loadJournalOptional(ctx, tx, `
		SELECT
			id, event_key, event_type, payload_hash, payload_hash_version,
			booking_id, owner_profile_id, venue_id, currency, effective_at,
			posted_at, reverses_journal_id, reversal_reason,
			created_by_user_id, description, metadata, created_at,
			(created_txid = txid_current()) AS created_in_current_tx
		FROM platform_journals
		WHERE id = $1
		FOR UPDATE
	`, journalID)
	if err != nil {
		return nil, err
	}
	if loaded == nil {
		return nil, ErrInvalidJournalReference
	}
	return loaded, nil
}

func (r *journalRepository) GetReversalBySourceID(ctx context.Context, tx pgx.Tx, journalID string) (*loadedJournal, error) {
	return r.loadJournalOptional(ctx, tx, `
		SELECT
			id, event_key, event_type, payload_hash, payload_hash_version,
			booking_id, owner_profile_id, venue_id, currency, effective_at,
			posted_at, reverses_journal_id, reversal_reason,
			created_by_user_id, description, metadata, created_at,
			(created_txid = txid_current()) AS created_in_current_tx
		FROM platform_journals
		WHERE reverses_journal_id = $1
	`, journalID)
}

func (r *journalRepository) loadJournal(ctx context.Context, tx pgx.Tx, query string, arg string) (*loadedJournal, error) {
	loaded, err := r.loadJournalOptional(ctx, tx, query, arg)
	if err != nil {
		return nil, err
	}
	if loaded == nil {
		return nil, ErrJournalIntegrity
	}
	return loaded, nil
}

func (r *journalRepository) loadJournalOptional(ctx context.Context, tx pgx.Tx, query string, arg string) (*loadedJournal, error) {
	loaded := &loadedJournal{}
	var metadata []byte
	err := tx.QueryRow(ctx, query, arg).Scan(
		&loaded.Journal.ID,
		&loaded.Journal.EventKey,
		&loaded.Journal.EventType,
		&loaded.Journal.PayloadHash,
		&loaded.Journal.PayloadHashVersion,
		&loaded.Journal.BookingID,
		&loaded.Journal.OwnerProfileID,
		&loaded.Journal.VenueID,
		&loaded.Journal.Currency,
		&loaded.Journal.EffectiveAt,
		&loaded.Journal.PostedAt,
		&loaded.Journal.ReversesJournalID,
		&loaded.Journal.ReversalReason,
		&loaded.Journal.CreatedByUserID,
		&loaded.Journal.Description,
		&metadata,
		&loaded.Journal.CreatedAt,
		&loaded.CreatedInCurrentTx,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, mapJournalRepositoryError(err)
	}
	if err := json.Unmarshal(metadata, &loaded.Journal.Metadata); err != nil {
		return nil, ErrJournalIntegrity
	}
	loaded.ReversesJournalID = loaded.Journal.ReversesJournalID
	loaded.ReversalReason = loaded.Journal.ReversalReason

	rows, err := tx.Query(ctx, `
		SELECT id, journal_id, account_code, owner_profile_id, side, amount_rupiah, created_at
		FROM platform_ledger_entries
		WHERE journal_id = $1
		ORDER BY account_code, owner_profile_id NULLS FIRST, side, amount_rupiah, id
	`, loaded.Journal.ID)
	if err != nil {
		return nil, mapJournalRepositoryError(err)
	}
	defer rows.Close()

	for rows.Next() {
		var entry PostedJournalEntry
		if err := rows.Scan(
			&entry.ID,
			&entry.JournalID,
			&entry.AccountCode,
			&entry.OwnerProfileID,
			&entry.Side,
			&entry.AmountRupiah,
			&entry.CreatedAt,
		); err != nil {
			return nil, mapJournalRepositoryError(err)
		}
		loaded.Journal.Entries = append(loaded.Journal.Entries, entry)
	}
	if err := rows.Err(); err != nil {
		return nil, mapJournalRepositoryError(err)
	}
	return loaded, nil
}

func mapJournalRepositoryError(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return err
	}
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrJournalIntegrity
	}

	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) {
		return ErrJournalPersistence
	}

	switch pgErr.Code {
	case "23505":
		if pgErr.ConstraintName == "platform_journals_event_key_key" || pgErr.ConstraintName == "platform_journals_reverses_journal_id_key" {
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
