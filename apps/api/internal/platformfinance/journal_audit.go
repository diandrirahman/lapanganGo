package platformfinance

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"lapangango-api/internal/audit"
)

var (
	ErrInvalidJournalAuditService       = errors.New("INVALID_JOURNAL_AUDIT_SERVICE")
	ErrInvalidJournalAuditContext       = errors.New("INVALID_JOURNAL_AUDIT_CONTEXT")
	ErrPlatformFinanceAuditConflict     = errors.New("PLATFORM_FINANCE_AUDIT_CONFLICT")
	ErrPlatformFinanceLiveWriteRejected = errors.New("PLATFORM_FINANCE_LIVE_WRITE_REJECTED")
)

// JournalAuditContext is the request actor context captured with an audited
// journal mutation. The journal's owner and venue dimensions remain the source
// of truth and are copied to the audit event by the coordinator.
type JournalAuditContext struct {
	ActorUserID *string
	ActorRole   string
	IPAddress   *string
	UserAgent   *string
}

// AuditedJournalService owns the transaction boundary for finance mutations
// that must have a matching platform audit event. It intentionally exposes
// reversal only in Phase 3A; domain posting primitives remain internal.
type AuditedJournalService struct {
	db           JournalTransactionBeginner
	journal      JournalService
	auditService audit.PlatformService
}

type JournalTransactionBeginner interface {
	Begin(context.Context) (pgx.Tx, error)
}

func NewAuditedJournalService(pool JournalTransactionBeginner, journal JournalService, auditService audit.PlatformService) (*AuditedJournalService, error) {
	if pool == nil || journal == nil || auditService == nil {
		return nil, ErrInvalidJournalAuditService
	}
	return &AuditedJournalService{db: pool, journal: journal, auditService: auditService}, nil
}

// ReverseJournal reverses a journal and records its audit marker atomically.
// A deterministic correlation id makes retries replay the same audit marker;
// the source journal row lock in JournalService serializes concurrent retries.
func (s *AuditedJournalService) ReverseJournal(ctx context.Context, params ReverseJournalParams, actor JournalAuditContext) (*PostedJournal, error) {
	if err := validateJournalAuditContext(actor); err != nil {
		return nil, err
	}
	if params.CreatedByUserID != nil && actor.ActorUserID == nil {
		return nil, ErrInvalidJournalAuditContext
	}
	if params.CreatedByUserID != nil && actor.ActorUserID != nil {
		createdBy, createdByErr := uuid.Parse(*params.CreatedByUserID)
		actorID, actorErr := uuid.Parse(*actor.ActorUserID)
		if createdByErr != nil || actorErr != nil || createdBy != actorID {
			return nil, ErrInvalidJournalAuditContext
		}
	}

	sourceID, err := uuid.Parse(params.JournalID)
	if err != nil {
		return nil, ErrInvalidJournalReference
	}
	correlationID := "journal.reversed:" + sourceID.String()

	tx, err := s.db.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)

	reversal, err := s.journal.ReverseJournal(ctx, tx, params)
	if err != nil {
		return nil, err
	}

	marker, err := findFinanceAuditMarker(ctx, tx, audit.ActionPlatformFinanceJournalReversed, correlationID)
	if err != nil {
		return nil, err
	}
	if marker != nil {
		if err := validateReversalAuditMarker(marker, reversal, sourceID.String(), correlationID); err != nil {
			return nil, err
		}
	} else {
		entityID := reversal.ID
		sourceJournalID := sourceID.String()
		effectiveAt := reversal.EffectiveAt.UTC().Format(time.RFC3339Nano)
		correlation := correlationID
		err = s.auditService.Record(ctx, tx, audit.CreatePlatformAuditLogParams{
			ActorUserID:    actor.ActorUserID,
			ActorRole:      actor.ActorRole,
			Action:         audit.ActionPlatformFinanceJournalReversed,
			EntityType:     audit.EntityPlatformFinanceJournal,
			EntityID:       &entityID,
			OwnerProfileID: reversal.OwnerProfileID,
			VenueID:        reversal.VenueID,
			CorrelationID:  &correlation,
			Metadata: map[string]any{
				"source_journal_id": sourceJournalID,
				"effective_at":      effectiveAt,
			},
			IPAddress: actor.IPAddress,
			UserAgent: actor.UserAgent,
		})
		if err != nil {
			return nil, err
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return reversal, nil
}

// LiveWriteAttempt describes a premature write that must be rejected and
// audited without invoking any journal or domain write primitive.
type LiveWriteAttempt struct {
	ActorUserID        *string
	ActorRole          string
	OwnerProfileID     *string
	VenueID            *string
	CorrelationID      string
	RequestFingerprint string
	WriteKind          string
	IPAddress          *string
	UserAgent          *string
}

// LiveWriteGuard records a deterministic rejection marker and returns the
// sentinel error. There is deliberately no success path in Phase 3A.
type LiveWriteGuard struct {
	db           JournalTransactionBeginner
	auditService audit.PlatformService
}

func NewLiveWriteGuard(pool JournalTransactionBeginner, auditService audit.PlatformService) (*LiveWriteGuard, error) {
	if pool == nil || auditService == nil {
		return nil, ErrInvalidJournalAuditService
	}
	return &LiveWriteGuard{db: pool, auditService: auditService}, nil
}

func (g *LiveWriteGuard) RejectPrematureLiveWrite(ctx context.Context, attempt LiveWriteAttempt) error {
	if err := validateLiveWriteAttempt(attempt); err != nil {
		return err
	}

	tx, err := g.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	// platform_audit_logs has no uniqueness constraint by design. Serialize
	// marker creation at the transaction level so identical retries cannot
	// produce duplicate rejection events.
	if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1, 0))`, attempt.CorrelationID); err != nil {
		return err
	}
	marker, err := findFinanceAuditMarker(ctx, tx, audit.ActionPlatformFinanceLiveWriteRejected, attempt.CorrelationID)
	if err != nil {
		return err
	}
	if marker != nil {
		if err := validateLiveRejectionMarker(marker, attempt); err != nil {
			return err
		}
	} else {
		correlation := attempt.CorrelationID
		err = g.auditService.Record(ctx, tx, audit.CreatePlatformAuditLogParams{
			ActorUserID:    attempt.ActorUserID,
			ActorRole:      attempt.ActorRole,
			Action:         audit.ActionPlatformFinanceLiveWriteRejected,
			EntityType:     audit.EntityPlatformFinanceJournal,
			OwnerProfileID: attempt.OwnerProfileID,
			VenueID:        attempt.VenueID,
			CorrelationID:  &correlation,
			Metadata: map[string]any{
				"reason":              "LIVE_NOT_ALLOWED",
				"write_kind":          attempt.WriteKind,
				"request_fingerprint": attempt.RequestFingerprint,
			},
			IPAddress: attempt.IPAddress,
			UserAgent: attempt.UserAgent,
		})
		if err != nil {
			return err
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return err
	}
	return ErrPlatformFinanceLiveWriteRejected
}

func validateJournalAuditContext(actor JournalAuditContext) error {
	if actor.ActorRole == "" || actor.ActorRole != strings.TrimSpace(actor.ActorRole) {
		return ErrInvalidJournalAuditContext
	}
	if actor.ActorUserID != nil {
		if _, err := uuid.Parse(*actor.ActorUserID); err != nil {
			return ErrInvalidJournalAuditContext
		}
	}
	return nil
}

func validateLiveWriteAttempt(attempt LiveWriteAttempt) error {
	if err := validateJournalAuditContext(JournalAuditContext{ActorUserID: attempt.ActorUserID, ActorRole: attempt.ActorRole}); err != nil {
		return err
	}
	if strings.TrimSpace(attempt.CorrelationID) == "" || attempt.CorrelationID != strings.TrimSpace(attempt.CorrelationID) {
		return ErrInvalidJournalAuditContext
	}
	if attempt.OwnerProfileID != nil {
		if _, err := uuid.Parse(*attempt.OwnerProfileID); err != nil {
			return ErrInvalidJournalAuditContext
		}
	}
	if attempt.VenueID != nil {
		if _, err := uuid.Parse(*attempt.VenueID); err != nil {
			return ErrInvalidJournalAuditContext
		}
	}
	if len(attempt.RequestFingerprint) != 64 {
		return ErrInvalidJournalAuditContext
	}
	for _, char := range attempt.RequestFingerprint {
		if !((char >= '0' && char <= '9') || (char >= 'a' && char <= 'f') || (char >= 'A' && char <= 'F')) {
			return ErrInvalidJournalAuditContext
		}
	}
	if !audit.IsAllowedPlatformFinanceWriteKind(attempt.WriteKind) {
		return ErrInvalidJournalAuditContext
	}
	return nil
}

type financeAuditMarker struct {
	ID             string
	Action         string
	EntityType     string
	EntityID       *string
	OwnerProfileID *string
	VenueID        *string
	Metadata       map[string]any
}

func findFinanceAuditMarker(ctx context.Context, db audit.DBTX, action, correlationID string) (*financeAuditMarker, error) {
	marker := &financeAuditMarker{}
	var metadataJSON string
	var markerCount int
	err := db.QueryRow(ctx, `
		SELECT id::text, action, entity_type, entity_id::text,
		       owner_profile_id::text, venue_id::text, metadata::text,
		       COUNT(*) OVER ()
		FROM platform_audit_logs
		WHERE action = $1 AND correlation_id = $2
		ORDER BY created_at DESC, id DESC
		LIMIT 1
	`, action, correlationID).Scan(
		&marker.ID, &marker.Action, &marker.EntityType, &marker.EntityID,
		&marker.OwnerProfileID, &marker.VenueID, &metadataJSON, &markerCount,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if markerCount != 1 {
		return nil, ErrJournalIntegrity
	}
	if err := json.Unmarshal([]byte(metadataJSON), &marker.Metadata); err != nil || marker.Metadata == nil {
		return nil, ErrJournalIntegrity
	}
	return marker, nil
}

func validateReversalAuditMarker(marker *financeAuditMarker, reversal *PostedJournal, sourceID, correlationID string) error {
	if marker.Action != audit.ActionPlatformFinanceJournalReversed || marker.EntityType != audit.EntityPlatformFinanceJournal || marker.EntityID == nil || *marker.EntityID != reversal.ID {
		return ErrPlatformFinanceAuditConflict
	}
	if marker.OwnerProfileID == nil || reversal.OwnerProfileID == nil {
		if marker.OwnerProfileID != nil || reversal.OwnerProfileID != nil {
			return ErrPlatformFinanceAuditConflict
		}
	} else if *marker.OwnerProfileID != *reversal.OwnerProfileID {
		return ErrPlatformFinanceAuditConflict
	}
	if marker.VenueID == nil || reversal.VenueID == nil {
		if marker.VenueID != nil || reversal.VenueID != nil {
			return ErrPlatformFinanceAuditConflict
		}
	} else if *marker.VenueID != *reversal.VenueID {
		return ErrPlatformFinanceAuditConflict
	}
	if marker.Metadata["source_journal_id"] != sourceID || marker.Metadata["effective_at"] != reversal.EffectiveAt.UTC().Format(time.RFC3339Nano) || correlationID != reversal.EventKey {
		return ErrPlatformFinanceAuditConflict
	}
	return nil
}

func validateLiveRejectionMarker(marker *financeAuditMarker, attempt LiveWriteAttempt) error {
	if marker.Action != audit.ActionPlatformFinanceLiveWriteRejected || marker.EntityType != audit.EntityPlatformFinanceJournal || marker.EntityID != nil {
		return ErrPlatformFinanceAuditConflict
	}
	if !sameOptionalAuditUUID(marker.OwnerProfileID, attempt.OwnerProfileID) || !sameOptionalAuditUUID(marker.VenueID, attempt.VenueID) {
		return ErrPlatformFinanceAuditConflict
	}
	if marker.Metadata["reason"] != "LIVE_NOT_ALLOWED" || marker.Metadata["write_kind"] != attempt.WriteKind || marker.Metadata["request_fingerprint"] != attempt.RequestFingerprint {
		return ErrPlatformFinanceAuditConflict
	}
	return nil
}

func sameOptionalAuditUUID(stored, requested *string) bool {
	if stored == nil || requested == nil {
		return stored == nil && requested == nil
	}
	storedID, storedErr := uuid.Parse(*stored)
	requestedID, requestedErr := uuid.Parse(*requested)
	return storedErr == nil && requestedErr == nil && storedID == requestedID
}
