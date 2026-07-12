package audit

import (
	"context"
)

type PlatformService interface {
	Record(ctx context.Context, db DBTX, params CreatePlatformAuditLogParams) error
}

type platformService struct {
	repo PlatformRepository
}

func NewPlatformService(repo PlatformRepository) PlatformService {
	return &platformService{repo: repo}
}

func (s *platformService) Record(ctx context.Context, db DBTX, params CreatePlatformAuditLogParams) error {
	if params.Metadata == nil {
		params.Metadata = make(map[string]any)
	}
	return s.repo.Create(ctx, db, params)
}
