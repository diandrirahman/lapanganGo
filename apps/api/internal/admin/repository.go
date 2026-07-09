package admin

import (
	"context"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Repository interface {
	GetUsers(ctx context.Context, query UserQuery) ([]UserResponse, int, error)
	GetOwners(ctx context.Context, query OwnerQuery) ([]OwnerResponse, int, error)
	UpdateOwnerStatus(ctx context.Context, ownerProfileID string, status string) error
	GetVenues(ctx context.Context, query VenueQuery) ([]VenueResponse, int, error)
	UpdateVenueStatus(ctx context.Context, venueID string, status string) error
	GetVenueOwnerProfileID(ctx context.Context, venueID string) (string, error)
	GetAuditLogs(ctx context.Context, query AuditLogQuery) ([]AuditLogResponse, int, error)
}

type repository struct {
	db *pgxpool.Pool
}

func NewRepository(db *pgxpool.Pool) Repository {
	return &repository{db: db}
}

func (r *repository) GetUsers(ctx context.Context, query UserQuery) ([]UserResponse, int, error) {
	whereClauses := []string{"1=1"}
	args := []any{}
	argID := 1

	if query.Search != "" {
		whereClauses = append(whereClauses, fmt.Sprintf("(name ILIKE $%d OR email ILIKE $%d)", argID, argID+1))
		args = append(args, "%"+query.Search+"%", "%"+query.Search+"%")
		argID += 2
	}
	if query.Role != "" {
		whereClauses = append(whereClauses, fmt.Sprintf("role = $%d", argID))
		args = append(args, query.Role)
		argID++
	}
	if query.Status != "" {
		whereClauses = append(whereClauses, fmt.Sprintf("status = $%d", argID))
		args = append(args, query.Status)
		argID++
	}

	whereClause := strings.Join(whereClauses, " AND ")
	countQuery := "SELECT count(*) FROM users WHERE " + whereClause
	var total int
	if err := r.db.QueryRow(ctx, countQuery, args...).Scan(&total); err != nil {
		return nil, 0, err
	}

	limit := query.Limit
	if limit == 0 {
		limit = 10
	}
	page := query.Page
	if page < 1 {
		page = 1
	}
	offset := (page - 1) * limit

	sqlQuery := fmt.Sprintf(`
		SELECT id::text, name, email, phone, role::text, status::text, created_at
		FROM users
		WHERE %s
		ORDER BY created_at DESC
		LIMIT $%d OFFSET $%d
	`, whereClause, argID, argID+1)

	args = append(args, limit, offset)

	rows, err := r.db.Query(ctx, sqlQuery, args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var users []UserResponse
	for rows.Next() {
		var u UserResponse
		if err := rows.Scan(&u.ID, &u.Name, &u.Email, &u.Phone, &u.Role, &u.Status, &u.CreatedAt); err != nil {
			return nil, 0, err
		}
		users = append(users, u)
	}
	if users == nil {
		users = []UserResponse{}
	}

	return users, total, nil
}

func (r *repository) GetOwners(ctx context.Context, query OwnerQuery) ([]OwnerResponse, int, error) {
	whereClauses := []string{"1=1"}
	args := []any{}
	argID := 1

	if query.Search != "" {
		whereClauses = append(whereClauses, fmt.Sprintf("(p.business_name ILIKE $%d OR u.name ILIKE $%d OR u.email ILIKE $%d)", argID, argID+1, argID+2))
		args = append(args, "%"+query.Search+"%", "%"+query.Search+"%", "%"+query.Search+"%")
		argID += 3
	}
	if query.Status != "" {
		whereClauses = append(whereClauses, fmt.Sprintf("u.status = $%d", argID))
		args = append(args, query.Status)
		argID++
	}

	whereClause := strings.Join(whereClauses, " AND ")

	countQuery := `
		SELECT count(*) 
		FROM owner_profiles p
		JOIN users u ON p.user_id = u.id
		WHERE ` + whereClause

	var total int
	if err := r.db.QueryRow(ctx, countQuery, args...).Scan(&total); err != nil {
		return nil, 0, err
	}

	limit := query.Limit
	if limit == 0 {
		limit = 10
	}
	page := query.Page
	if page < 1 {
		page = 1
	}
	offset := (page - 1) * limit

	sqlQuery := fmt.Sprintf(`
		SELECT p.id::text, p.user_id::text, p.business_name, u.status::text, p.created_at
		FROM owner_profiles p
		JOIN users u ON p.user_id = u.id
		WHERE %s
		ORDER BY p.created_at DESC
		LIMIT $%d OFFSET $%d
	`, whereClause, argID, argID+1)

	args = append(args, limit, offset)

	rows, err := r.db.Query(ctx, sqlQuery, args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var owners []OwnerResponse
	for rows.Next() {
		var o OwnerResponse
		if err := rows.Scan(&o.ID, &o.UserID, &o.BusinessName, &o.Status, &o.CreatedAt); err != nil {
			return nil, 0, err
		}
		owners = append(owners, o)
	}
	if owners == nil {
		owners = []OwnerResponse{}
	}

	return owners, total, nil
}

func (r *repository) UpdateOwnerStatus(ctx context.Context, ownerProfileID string, status string) error {
	query := `
		UPDATE users
		SET status = $1, updated_at = now()
		FROM owner_profiles
		WHERE owner_profiles.user_id = users.id AND owner_profiles.id = $2
	`
	tag, err := r.db.Exec(ctx, query, status, ownerProfileID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return pgx.ErrNoRows
	}
	return nil
}

func (r *repository) GetVenues(ctx context.Context, query VenueQuery) ([]VenueResponse, int, error) {
	whereClauses := []string{"1=1"}
	args := []any{}
	argID := 1

	if query.Search != "" {
		whereClauses = append(whereClauses, fmt.Sprintf("(name ILIKE $%d OR city ILIKE $%d)", argID, argID+1))
		args = append(args, "%"+query.Search+"%", "%"+query.Search+"%")
		argID += 2
	}
	if query.Status != "" {
		whereClauses = append(whereClauses, fmt.Sprintf("status = $%d", argID))
		args = append(args, query.Status)
		argID++
	}

	whereClause := strings.Join(whereClauses, " AND ")
	countQuery := "SELECT count(*) FROM venues WHERE " + whereClause
	var total int
	if err := r.db.QueryRow(ctx, countQuery, args...).Scan(&total); err != nil {
		return nil, 0, err
	}

	limit := query.Limit
	if limit == 0 {
		limit = 10
	}
	page := query.Page
	if page < 1 {
		page = 1
	}
	offset := (page - 1) * limit

	sqlQuery := fmt.Sprintf(`
		SELECT id::text, owner_profile_id::text, name, city, status::text, created_at
		FROM venues
		WHERE %s
		ORDER BY created_at DESC
		LIMIT $%d OFFSET $%d
	`, whereClause, argID, argID+1)

	args = append(args, limit, offset)

	rows, err := r.db.Query(ctx, sqlQuery, args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var venues []VenueResponse
	for rows.Next() {
		var v VenueResponse
		if err := rows.Scan(&v.ID, &v.OwnerID, &v.Name, &v.City, &v.Status, &v.CreatedAt); err != nil {
			return nil, 0, err
		}
		venues = append(venues, v)
	}
	if venues == nil {
		venues = []VenueResponse{}
	}

	return venues, total, nil
}

func (r *repository) UpdateVenueStatus(ctx context.Context, venueID string, status string) error {
	query := `UPDATE venues SET status = $1, updated_at = now() WHERE id = $2`
	tag, err := r.db.Exec(ctx, query, status, venueID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return pgx.ErrNoRows
	}
	return nil
}

func (r *repository) GetVenueOwnerProfileID(ctx context.Context, venueID string) (string, error) {
	query := `SELECT owner_profile_id::text FROM venues WHERE id = $1`
	var ownerProfileID string
	err := r.db.QueryRow(ctx, query, venueID).Scan(&ownerProfileID)
	if err != nil {
		return "", err
	}
	return ownerProfileID, nil
}

func (r *repository) GetAuditLogs(ctx context.Context, query AuditLogQuery) ([]AuditLogResponse, int, error) {
	whereClauses := []string{"1=1"}
	args := []any{}
	argID := 1

	if query.Action != "" {
		whereClauses = append(whereClauses, fmt.Sprintf("action = $%d", argID))
		args = append(args, query.Action)
		argID++
	}
	if query.EntityType != "" {
		whereClauses = append(whereClauses, fmt.Sprintf("entity_type = $%d", argID))
		args = append(args, query.EntityType)
		argID++
	}

	whereClause := strings.Join(whereClauses, " AND ")
	countQuery := "SELECT count(*) FROM owner_audit_logs WHERE " + whereClause
	var total int
	if err := r.db.QueryRow(ctx, countQuery, args...).Scan(&total); err != nil {
		return nil, 0, err
	}

	limit := query.Limit
	if limit == 0 {
		limit = 10
	}
	page := query.Page
	if page < 1 {
		page = 1
	}
	offset := (page - 1) * limit

	sqlQuery := fmt.Sprintf(`
		SELECT id::text, owner_profile_id::text, actor_user_id::text, actor_role, action, entity_type, entity_id::text, metadata, ip_address, user_agent, created_at
		FROM owner_audit_logs
		WHERE %s
		ORDER BY created_at DESC
		LIMIT $%d OFFSET $%d
	`, whereClause, argID, argID+1)

	args = append(args, limit, offset)

	rows, err := r.db.Query(ctx, sqlQuery, args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var logs []AuditLogResponse
	for rows.Next() {
		var l AuditLogResponse
		if err := rows.Scan(&l.ID, &l.OwnerProfileID, &l.ActorUserID, &l.ActorRole, &l.Action, &l.EntityType, &l.EntityID, &l.Metadata, &l.IPAddress, &l.UserAgent, &l.CreatedAt); err != nil {
			return nil, 0, err
		}
		logs = append(logs, l)
	}
	if logs == nil {
		logs = []AuditLogResponse{}
	}

	return logs, total, nil
}
