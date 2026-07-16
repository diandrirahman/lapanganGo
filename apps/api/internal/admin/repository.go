package admin

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"lapangango-api/internal/audit"

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
	GetDashboardStats(ctx context.Context) (DashboardStatsResponse, error)
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
	if query.OwnerProfileID != "" {
		whereClauses = append(whereClauses, fmt.Sprintf("owner_profile_id = $%d", argID))
		args = append(args, query.OwnerProfileID)
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
	limit := query.Limit
	if limit == 0 {
		limit = 20
	}
	page := query.Page
	if page < 1 {
		page = 1
	}
	offset := (page - 1) * limit

	branches := make([]string, 0, 2)
	args := make([]any, 0, 6)
	if query.Scope == "OWNER" || query.Scope == "" {
		branch, branchArgs := buildAuditBranch("owner_audit_logs", "OWNER", query, len(args)+1)
		branches = append(branches, branch)
		args = append(args, branchArgs...)
	}
	if query.Scope == "PLATFORM" {
		branch, branchArgs := buildAuditBranch("platform_audit_logs", "PLATFORM", query, len(args)+1)
		branches = append(branches, branch)
		args = append(args, branchArgs...)
	}
	if query.Scope == "ALL" {
		branch, branchArgs := buildAuditBranch("owner_audit_logs", "OWNER", query, len(args)+1)
		branches = append(branches, branch)
		args = append(args, branchArgs...)
		branch, branchArgs = buildAuditBranch("platform_audit_logs", "PLATFORM", query, len(args)+1)
		branches = append(branches, branch)
		args = append(args, branchArgs...)
	}
	if len(branches) == 0 {
		return []AuditLogResponse{}, 0, fmt.Errorf("invalid audit scope")
	}

	limitArg := len(args) + 1
	offsetArg := len(args) + 2
	sqlQuery := fmt.Sprintf(`
		WITH audit_rows AS (
			%s
		), counted AS (
			SELECT *, count(*) OVER() AS total_items
			FROM audit_rows
		)
		SELECT id, scope, owner_profile_id, actor_user_id, actor_role, action,
		       entity_type, entity_id, venue_id, metadata, ip_address, user_agent,
		       created_at, total_items
		FROM counted
		ORDER BY created_at DESC, id DESC
		LIMIT $%d OFFSET $%d
	`, strings.Join(branches, "\nUNION ALL\n"), limitArg, offsetArg)
	args = append(args, limit, offset)

	rows, err := r.db.Query(ctx, sqlQuery, args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	logs := make([]AuditLogResponse, 0)
	total := 0
	for rows.Next() {
		var l AuditLogResponse
		var metadataJSON string
		var rowTotal int
		if err := rows.Scan(&l.ID, &l.Scope, &l.OwnerProfileID, &l.ActorUserID, &l.ActorRole, &l.Action, &l.EntityType, &l.EntityID, &l.VenueID, &metadataJSON, &l.IPAddress, &l.UserAgent, &l.CreatedAt, &rowTotal); err != nil {
			return nil, 0, err
		}
		if rowTotal > total {
			total = rowTotal
		}
		metadata := make(map[string]any)
		if err := json.Unmarshal([]byte(metadataJSON), &metadata); err != nil || metadata == nil {
			metadata = make(map[string]any)
		}
		if l.Scope == "PLATFORM" {
			metadata = audit.SanitizePlatformAuditMetadata(l.Action, metadata)
		} else {
			metadata = audit.SanitizeAuditMetadata(metadata)
		}
		l.Metadata = metadata
		logs = append(logs, l)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, err
	}

	return logs, total, nil
}

func buildAuditBranch(table, scope string, query AuditLogQuery, firstArg int) (string, []any) {
	where := []string{"1=1"}
	args := make([]any, 0, 2)
	argID := firstArg
	if query.Action != "" {
		where = append(where, fmt.Sprintf("action = $%d", argID))
		args = append(args, query.Action)
		argID++
	}
	if query.EntityType != "" {
		where = append(where, fmt.Sprintf("entity_type = $%d", argID))
		args = append(args, query.EntityType)
		argID++
	}

	venueColumn := "NULL::text"
	ipColumn := "NULL::text"
	userAgentColumn := "NULL::text"
	if table == "owner_audit_logs" {
		ipColumn = "ip_address"
		userAgentColumn = "user_agent"
	} else {
		venueColumn = "venue_id::text"
	}

	branch := fmt.Sprintf(`
		SELECT id::text,
		       '%s'::text AS scope,
		       owner_profile_id::text,
		       actor_user_id::text,
		       actor_role,
		       action,
		       entity_type,
		       entity_id::text,
		       %s AS venue_id,
		       metadata::text,
		       %s AS ip_address,
		       %s AS user_agent,
		       created_at
		FROM %s
		WHERE %s`, scope, venueColumn, ipColumn, userAgentColumn, table, strings.Join(where, " AND "))
	return branch, args
}

func (r *repository) GetDashboardStats(ctx context.Context) (DashboardStatsResponse, error) {
	var stats DashboardStatsResponse

	query := `
		SELECT
			(SELECT count(*) FROM users) as total_users,
			(SELECT count(*) FROM owner_profiles) as total_owners,
			(SELECT count(*) FROM venues) as total_venues,
			(SELECT count(*) FROM bookings) as total_bookings
	`
	err := r.db.QueryRow(ctx, query).Scan(
		&stats.TotalUsers,
		&stats.TotalOwners,
		&stats.TotalVenues,
		&stats.TotalBookings,
	)
	if err != nil {
		return stats, err
	}
	return stats, nil
}
