package audit

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"
)

type Repository interface {
	Create(ctx context.Context, params CreateAuditLogParams) error
	ListByOwner(ctx context.Context, ownerProfileID string, query AuditLogQuery) ([]AuditLogResponse, int, error)
}

type repository struct {
	db *pgxpool.Pool
}

func NewRepository(db *pgxpool.Pool) Repository {
	return &repository{db: db}
}

func (r *repository) Create(ctx context.Context, params CreateAuditLogParams) error {
	query := `
		INSERT INTO owner_audit_logs (
		  owner_profile_id,
		  actor_user_id,
		  actor_role,
		  action,
		  entity_type,
		  entity_id,
		  metadata,
		  ip_address,
		  user_agent
		)
		VALUES ($1,$2,$3,$4,$5,$6,$7::jsonb,$8,$9);
	`

	metadataJSON, err := json.Marshal(params.Metadata)
	if err != nil {
		return err
	}
	metadataValue := string(metadataJSON)

	var actorUserID *string
	if params.ActorUserID != "" {
		actorUserID = &params.ActorUserID
	}

	_, err = r.db.Exec(ctx, query,
		params.OwnerProfileID,
		actorUserID,
		params.ActorRole,
		params.Action,
		params.EntityType,
		params.EntityID,
		metadataValue,
		params.IPAddress,
		params.UserAgent,
	)
	return err
}

func (r *repository) ListByOwner(ctx context.Context, ownerProfileID string, query AuditLogQuery) ([]AuditLogResponse, int, error) {
	// Base query
	whereClauses := []string{"l.owner_profile_id = $1"}
	args := []interface{}{ownerProfileID}
	argID := 2

	if query.Action != "" {
		whereClauses = append(whereClauses, fmt.Sprintf("l.action = $%d", argID))
		args = append(args, query.Action)
		argID++
	}
	if query.EntityType != "" {
		whereClauses = append(whereClauses, fmt.Sprintf("l.entity_type = $%d", argID))
		args = append(args, query.EntityType)
		argID++
	}
	if query.ActorUserID != "" {
		whereClauses = append(whereClauses, fmt.Sprintf("l.actor_user_id = $%d", argID))
		args = append(args, query.ActorUserID)
		argID++
	}
	if query.StartDate != "" {
		whereClauses = append(whereClauses, fmt.Sprintf("l.created_at >= $%d", argID))
		args = append(args, query.StartDate)
		argID++
	}
	if query.EndDate != "" {
		whereClauses = append(whereClauses, fmt.Sprintf("l.created_at < $%d::date + interval '1 day'", argID))
		args = append(args, query.EndDate)
		argID++
	}

	whereClause := strings.Join(whereClauses, " AND ")

	countQuery := `SELECT count(*) FROM owner_audit_logs l WHERE ` + whereClause
	var total int
	err := r.db.QueryRow(ctx, countQuery, args...).Scan(&total)
	if err != nil {
		return nil, 0, err
	}

	page := query.Page
	if page < 1 {
		page = 1
	}
	limit := query.Limit
	if limit < 1 {
		limit = 20
	}
	offset := (page - 1) * limit

	sqlQuery := `
		SELECT
		  l.id::text,
		  l.actor_user_id::text,
		  u.name,
		  u.email,
		  l.actor_role,
		  l.action,
		  l.entity_type,
		  l.entity_id::text,
		  l.metadata,
		  l.ip_address,
		  l.user_agent,
		  l.created_at
		FROM owner_audit_logs l
		LEFT JOIN users u ON u.id = l.actor_user_id
		WHERE ` + whereClause + `
		ORDER BY l.created_at DESC
		LIMIT $` + fmt.Sprint(argID) + ` OFFSET $` + fmt.Sprint(argID+1)

	args = append(args, limit, offset)

	rows, err := r.db.Query(ctx, sqlQuery, args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var logs []AuditLogResponse
	for rows.Next() {
		var log AuditLogResponse
		var metadataJSON []byte
		var actorID, actorName, actorEmail *string
		var entityID *string

		err := rows.Scan(
			&log.ID,
			&actorID,
			&actorName,
			&actorEmail,
			&log.Actor.Role,
			&log.Action,
			&log.EntityType,
			&entityID,
			&metadataJSON,
			&log.IPAddress,
			&log.UserAgent,
			&log.CreatedAt,
		)
		if err != nil {
			return nil, 0, err
		}

		if actorID != nil {
			log.Actor.ID = actorID
		}
		if actorName != nil {
			log.Actor.Name = actorName
		}
		if actorEmail != nil {
			log.Actor.Email = actorEmail
		}
		if entityID != nil {
			log.EntityID = entityID
		}
		if len(metadataJSON) > 0 {
			json.Unmarshal(metadataJSON, &log.Metadata)
		} else {
			log.Metadata = make(map[string]any)
		}

		logs = append(logs, log)
	}

	if logs == nil {
		logs = []AuditLogResponse{}
	}

	return logs, total, nil
}
