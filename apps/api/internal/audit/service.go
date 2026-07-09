package audit

import (
	"context"
	"log"
)

type Service interface {
	Record(ctx context.Context, params CreateAuditLogParams)
	ListOwnerLogs(ctx context.Context, ownerProfileID string, query AuditLogQuery) ([]AuditLogResponse, int, error)
}

type service struct {
	repo Repository
}

func NewService(repo Repository) Service {
	return &service{repo: repo}
}

func (s *service) Record(ctx context.Context, params CreateAuditLogParams) {
	if params.Metadata == nil {
		params.Metadata = make(map[string]any)
	}
	err := s.repo.Create(ctx, params)
	if err != nil {
		log.Printf("failed to write audit log: %v", err)
	}
}

func (s *service) ListOwnerLogs(ctx context.Context, ownerProfileID string, query AuditLogQuery) ([]AuditLogResponse, int, error) {
	return s.repo.ListByOwner(ctx, ownerProfileID, query)
}
