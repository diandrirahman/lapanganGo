package commercialterms

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"lapangango-api/internal/audit"
)

type Service interface {
	GetTerms(ctx context.Context, query GetTermsQuery) (PaginatedTermsResponse, error)
	Preview(ctx context.Context, req PreviewRequest) (PreviewResponse, error)
	CreateTerm(ctx context.Context, req CreateTermRequest, idempotencyKey string, adminID string, ipAddress, userAgent string) (*CommercialTerm, error)
}

type service struct {
	repo         Repository
	dbPool       *pgxpool.Pool
	auditService audit.PlatformService
}

func NewService(repo Repository, dbPool *pgxpool.Pool, auditService audit.PlatformService) Service {
	return &service{repo: repo, dbPool: dbPool, auditService: auditService}
}

func (s *service) GetTerms(ctx context.Context, query GetTermsQuery) (PaginatedTermsResponse, error) {
	return s.repo.GetTerms(ctx, query)
}

func (s *service) Preview(ctx context.Context, req PreviewRequest) (PreviewResponse, error) {
	if err := req.Validate(); err != nil {
		return PreviewResponse{}, err
	}

	bookingAmounts := []int64{100_000, 200_000, 500_000}
	var scenarios []PreviewScenario

	for _, amount := range bookingAmounts {
		commission := (amount * int64(req.CommissionBps)) / 10000
		net := amount - commission

		scenarios = append(scenarios, PreviewScenario{
			BookingAmountInt64:        amount,
			CommissionBps:             req.CommissionBps,
			ProjectedCommissionRupiah: commission,
			ProjectedOwnerNetRupiah:   net,
		})
	}

	return PreviewResponse{
		FinanceMode:      req.FinanceMode,
		CollectionMethod: req.CollectionMethod,
		Scenarios:        scenarios,
	}, nil
}

type ErrValidationError struct{ error }
type ErrForbidden struct{ error }
type ErrNotFound struct{ error }
type ErrConflict struct{ error }

func (s *service) CreateTerm(ctx context.Context, req CreateTermRequest, idempotencyKey string, adminID string, ipAddress, userAgent string) (*CommercialTerm, error) {
	if err := req.Validate(); err != nil {
		return nil, ErrValidationError{err}
	}

	req.ValidFrom = req.ValidFrom.UTC().Truncate(time.Microsecond)

	scopeKey := "GLOBAL"
	if req.OwnerProfileID != nil && *req.OwnerProfileID != "" {
		scopeKey = *req.OwnerProfileID
	}

	tx, err := s.dbPool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)

	if err := s.repo.LockIdempotency(ctx, tx, idempotencyKey); err != nil {
		return nil, err
	}

	if scopeKey != "GLOBAL" {
		if err := s.repo.GetOwnerForShare(ctx, tx, scopeKey); err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return nil, ErrNotFound{errors.New("owner not found")}
			}
			return nil, err
		}
	}

	if req.FinanceMode == "LIVE" {
		metadata, _, err := s.repo.GetAuditByCorrelationID(ctx, tx, idempotencyKey, audit.ActionPlatformCommercialTermLiveRejected)
		if err != nil {
			return nil, err
		}
		if metadata == nil {
			err = s.auditService.Record(ctx, tx, audit.CreatePlatformAuditLogParams{
				ActorUserID:    &adminID,
				ActorRole:      "SUPER_ADMIN",
				Action:         audit.ActionPlatformCommercialTermLiveRejected,
				EntityType:     audit.EntityPlatformCommercialTerm,
				CorrelationID:  &idempotencyKey,
				Metadata:       map[string]any{"reason": "LIVE_NOT_ALLOWED"},
				OwnerProfileID: req.OwnerProfileID,
				IPAddress:      &ipAddress,
				UserAgent:      &userAgent,
			})
			if err != nil {
				return nil, err
			}
		}
		if err := tx.Commit(ctx); err != nil {
			return nil, err
		}
		return nil, ErrForbidden{errors.New("LIVE finance_mode is not allowed")}
	}

	_, existingEntityID, err := s.repo.GetAuditByCorrelationID(ctx, tx, idempotencyKey, audit.ActionPlatformCommercialTermCreated)
	if err != nil {
		return nil, err
	}
	if existingEntityID != "" {
		term, err := s.repo.GetTermByID(ctx, tx, existingEntityID)
		if err != nil {
			return nil, err
		}

		ownerProfileIDMatch := false
		if req.OwnerProfileID == nil {
			ownerProfileIDMatch = (term.OwnerProfileID == nil)
		} else {
			ownerProfileIDMatch = (term.OwnerProfileID != nil && *term.OwnerProfileID == *req.OwnerProfileID)
		}

		labelMatch := term.Label == req.Label
		phaseMatch := term.Phase == req.Phase
		bpsMatch := term.CommissionBps == *req.CommissionBps
		validFromMatch := term.ValidFrom.Equal(req.ValidFrom)

		if ownerProfileIDMatch && labelMatch && phaseMatch && bpsMatch && validFromMatch {
			term.Status = "SCHEDULED"
			return &term, nil
		}
		return nil, ErrConflict{errors.New("idempotency key conflict with different payload")}
	}

	ts, err := s.repo.GetTransactionTimestamp(ctx, tx)
	if err != nil {
		return nil, err
	}

	if req.ValidFrom.Before(ts) || req.ValidFrom.Equal(ts) {
		return nil, ErrValidationError{errors.New("valid_from must be strictly greater than transaction_timestamp()")}
	}

	if err := s.repo.LockScope(ctx, tx, scopeKey); err != nil {
		return nil, err
	}

	oldTerm, err := s.repo.GetOpenEndedTermByScope(ctx, tx, scopeKey)
	if err != nil {
		return nil, err
	}

	if oldTerm != nil {
		if !req.ValidFrom.After(oldTerm.ValidFrom) {
			return nil, ErrConflict{errors.New("new valid_from must be greater than existing valid_from")}
		}
		if err := s.repo.SupersedeTerm(ctx, tx, oldTerm.ID, req.ValidFrom); err != nil {
			return nil, err
		}
	}

	newTerm := &CommercialTerm{
		ID:               uuid.NewString(),
		OwnerProfileID:   req.OwnerProfileID,
		Label:            req.Label,
		Phase:            req.Phase,
		FinanceMode:      req.FinanceMode,
		CollectionMethod: req.CollectionMethod,
		CommissionBps:    *req.CommissionBps,
		ValidFrom:        req.ValidFrom,
		CreatedByUserID:  adminID,
	}

	if oldTerm != nil {
		newTerm.SupersedesID = &oldTerm.ID
	}

	if err := s.repo.CreateTerm(ctx, tx, newTerm); err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && (pgErr.Code == "23P01" || pgErr.Code == "23505") {
			return nil, ErrConflict{errors.New("commercial term overlap or conflict")}
		}
		return nil, err
	}

	if oldTerm != nil {
		err = s.auditService.Record(ctx, tx, audit.CreatePlatformAuditLogParams{
			ActorUserID:    &adminID,
			ActorRole:      "SUPER_ADMIN",
			Action:         audit.ActionPlatformCommercialTermSuperseded,
			EntityType:     audit.EntityPlatformCommercialTerm,
			EntityID:       &oldTerm.ID,
			CorrelationID:  &idempotencyKey,
			Metadata:       map[string]any{"superseded_term_id": oldTerm.ID, "new_term_id": newTerm.ID},
			OwnerProfileID: req.OwnerProfileID,
			IPAddress:      &ipAddress,
			UserAgent:      &userAgent,
		})
		if err != nil {
			return nil, err
		}
	}

	err = s.auditService.Record(ctx, tx, audit.CreatePlatformAuditLogParams{
		ActorUserID:    &adminID,
		ActorRole:      "SUPER_ADMIN",
		Action:         audit.ActionPlatformCommercialTermCreated,
		EntityType:     audit.EntityPlatformCommercialTerm,
		EntityID:       &newTerm.ID,
		CorrelationID:  &idempotencyKey,
		Metadata:       map[string]any{"commission_bps": newTerm.CommissionBps, "label": newTerm.Label, "valid_from": newTerm.ValidFrom.Format(time.RFC3339Nano), "phase": newTerm.Phase},
		OwnerProfileID: req.OwnerProfileID,
		IPAddress:      &ipAddress,
		UserAgent:      &userAgent,
	})
	if err != nil {
		return nil, err
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}

	if newTerm.ValidUntil != nil && !ts.Before(*newTerm.ValidUntil) {
		newTerm.Status = "HISTORICAL"
	} else if newTerm.ValidFrom.After(ts) {
		newTerm.Status = "SCHEDULED"
	} else {
		newTerm.Status = "CURRENT"
	}

	return newTerm, nil
}
