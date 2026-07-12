package audit

import (
	"context"
	"encoding/json"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

type DBTX interface {
	Exec(context.Context, string, ...any) (pgconn.CommandTag, error)
	Query(context.Context, string, ...any) (pgx.Rows, error)
	QueryRow(context.Context, string, ...any) pgx.Row
}

type PlatformRepository interface {
	Create(ctx context.Context, db DBTX, params CreatePlatformAuditLogParams) error
}

type platformRepository struct{}

func NewPlatformRepository() PlatformRepository {
	return &platformRepository{}
}

func (r *platformRepository) Create(ctx context.Context, db DBTX, params CreatePlatformAuditLogParams) error {
	if err := params.Validate(); err != nil {
		return err
	}

	query := `
		INSERT INTO platform_audit_logs (
		  actor_user_id,
		  actor_role,
		  action,
		  entity_type,
		  entity_id,
		  owner_profile_id,
		  venue_id,
		  correlation_id,
		  metadata,
		  ip_address,
		  user_agent
		)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9::jsonb,$10,$11);
	`

	metadataJSON, err := json.Marshal(params.Metadata)
	if err != nil {
		return err
	}

	_, err = db.Exec(ctx, query,
		params.ActorUserID,
		params.ActorRole,
		params.Action,
		params.EntityType,
		params.EntityID,
		params.OwnerProfileID,
		params.VenueID,
		params.CorrelationID,
		string(metadataJSON),
		params.IPAddress,
		params.UserAgent,
	)
	return err
}
