package admin

import (
	"context"

	"lapangango-api/internal/audit"
)

type Service interface {
	GetUsers(ctx context.Context, query UserQuery) (PaginatedResponse, error)
	GetOwners(ctx context.Context, query OwnerQuery) (PaginatedResponse, error)
	UpdateOwnerStatus(ctx context.Context, ownerProfileID string, status string, actorID string) error
	GetVenues(ctx context.Context, query VenueQuery) (PaginatedResponse, error)
	UpdateVenueStatus(ctx context.Context, venueID string, status string, actorID string) error
	GetVenueOwnerProfileID(ctx context.Context, venueID string) (string, error)
	GetAuditLogs(ctx context.Context, query AuditLogQuery) (PaginatedResponse, error)
	GetDashboardStats(ctx context.Context) (DashboardStatsResponse, error)
}

type service struct {
	repo     Repository
	auditSvc audit.Service
}

func NewService(repo Repository, auditSvc audit.Service) Service {
	return &service{repo: repo, auditSvc: auditSvc}
}

func (s *service) GetUsers(ctx context.Context, query UserQuery) (PaginatedResponse, error) {
	users, total, err := s.repo.GetUsers(ctx, query)
	if err != nil {
		return PaginatedResponse{}, err
	}

	limit := query.Limit
	if limit == 0 {
		limit = 10
	}
	page := query.Page
	if page < 1 {
		page = 1
	}

	totalPages := (total + limit - 1) / limit
	return PaginatedResponse{
		Data:       users,
		TotalItems: total,
		TotalPages: totalPages,
		Page:       page,
		Limit:      limit,
	}, nil
}

func (s *service) GetOwners(ctx context.Context, query OwnerQuery) (PaginatedResponse, error) {
	owners, total, err := s.repo.GetOwners(ctx, query)
	if err != nil {
		return PaginatedResponse{}, err
	}

	limit := query.Limit
	if limit == 0 {
		limit = 10
	}
	page := query.Page
	if page < 1 {
		page = 1
	}

	totalPages := (total + limit - 1) / limit
	return PaginatedResponse{
		Data:       owners,
		TotalItems: total,
		TotalPages: totalPages,
		Page:       page,
		Limit:      limit,
	}, nil
}

func (s *service) UpdateOwnerStatus(ctx context.Context, ownerProfileID string, status string, actorID string) error {
	err := s.repo.UpdateOwnerStatus(ctx, ownerProfileID, status)
	if err != nil {
		return err
	}

	// Log audit
	s.auditSvc.Record(ctx, audit.CreateAuditLogParams{
		OwnerProfileID: ownerProfileID, // Using it even if it's superadmin action
		ActorUserID:    actorID,
		ActorRole:      "SUPER_ADMIN",
		Action:         "UPDATE_OWNER_STATUS",
		EntityType:     "OWNER_PROFILE",
		EntityID:       &ownerProfileID,
		Metadata: map[string]interface{}{
			"new_status": status,
		},
	})

	return nil
}

func (s *service) GetVenues(ctx context.Context, query VenueQuery) (PaginatedResponse, error) {
	venues, total, err := s.repo.GetVenues(ctx, query)
	if err != nil {
		return PaginatedResponse{}, err
	}

	limit := query.Limit
	if limit == 0 {
		limit = 10
	}
	page := query.Page
	if page < 1 {
		page = 1
	}

	totalPages := (total + limit - 1) / limit
	return PaginatedResponse{
		Data:       venues,
		TotalItems: total,
		TotalPages: totalPages,
		Page:       page,
		Limit:      limit,
	}, nil
}

func (s *service) UpdateVenueStatus(ctx context.Context, venueID string, status string, actorID string) error {
	err := s.repo.UpdateVenueStatus(ctx, venueID, status)
	if err != nil {
		return err
	}

	// Look up owner profile ID for the venue to log the audit
	ownerProfileID, err := s.repo.GetVenueOwnerProfileID(ctx, venueID)
	if err == nil {
		s.auditSvc.Record(ctx, audit.CreateAuditLogParams{
			OwnerProfileID: ownerProfileID,
			ActorUserID:    actorID,
			ActorRole:      "SUPER_ADMIN",
			Action:         "UPDATE_VENUE_STATUS",
			EntityType:     "VENUE",
			EntityID:       &venueID,
			Metadata: map[string]interface{}{
				"new_status": status,
			},
		})
	}

	return nil
}

func (s *service) GetAuditLogs(ctx context.Context, query AuditLogQuery) (PaginatedResponse, error) {
	if query.Scope == "" {
		query.Scope = AuditScopeOwner
	}
	logs, total, err := s.repo.GetAuditLogs(ctx, query)
	if err != nil {
		return PaginatedResponse{}, err
	}

	limit := query.Limit
	if limit == 0 {
		limit = 20
	}
	page := query.Page
	if page < 1 {
		page = 1
	}

	totalPages := (total + limit - 1) / limit
	return PaginatedResponse{
		Data:       logs,
		TotalItems: total,
		TotalPages: totalPages,
		Page:       page,
		Limit:      limit,
	}, nil
}

func (s *service) GetDashboardStats(ctx context.Context) (DashboardStatsResponse, error) {
	return s.repo.GetDashboardStats(ctx)
}

func (s *service) GetVenueOwnerProfileID(ctx context.Context, venueID string) (string, error) {
	return s.repo.GetVenueOwnerProfileID(ctx, venueID)
}
