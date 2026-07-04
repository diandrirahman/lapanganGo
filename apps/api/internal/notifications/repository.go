package notifications

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Notification struct {
	ID         string
	UserID     string
	Type       string
	Title      string
	Message    string
	EntityType *string
	EntityID   *string
	ReadAt     *time.Time
	CreatedAt  time.Time
}

type CreateNotificationParams struct {
	UserID     string
	Type       string
	Title      string
	Message    string
	EntityType *string
	EntityID   *string
}

type Repository interface {
	Create(ctx context.Context, params CreateNotificationParams) error
	ListByUser(ctx context.Context, userID string, limit, offset int) ([]Notification, int, error)
	UnreadCount(ctx context.Context, userID string) (int, error)
	MarkRead(ctx context.Context, userID string, notificationID string) error
	MarkAllRead(ctx context.Context, userID string) error
}

type repository struct {
	db *pgxpool.Pool
}

func NewRepository(db *pgxpool.Pool) Repository {
	return &repository{db: db}
}

func (r *repository) Create(ctx context.Context, params CreateNotificationParams) error {
	query := `
		INSERT INTO notifications (user_id, type, title, message, entity_type, entity_id)
		VALUES ($1, $2, $3, $4, $5, $6)
		ON CONFLICT DO NOTHING
	`
	_, err := r.db.Exec(ctx, query, params.UserID, params.Type, params.Title, params.Message, params.EntityType, params.EntityID)
	return err
}

func (r *repository) ListByUser(ctx context.Context, userID string, limit, offset int) ([]Notification, int, error) {
	var total int
	err := r.db.QueryRow(ctx, "SELECT COUNT(*) FROM notifications WHERE user_id = $1", userID).Scan(&total)
	if err != nil {
		return nil, 0, err
	}

	query := `
		SELECT id, user_id, type, title, message, entity_type, entity_id, read_at, created_at
		FROM notifications
		WHERE user_id = $1
		ORDER BY created_at DESC
		LIMIT $2 OFFSET $3
	`
	rows, err := r.db.Query(ctx, query, userID, limit, offset)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var notifications []Notification
	for rows.Next() {
		var n Notification
		if err := rows.Scan(&n.ID, &n.UserID, &n.Type, &n.Title, &n.Message, &n.EntityType, &n.EntityID, &n.ReadAt, &n.CreatedAt); err != nil {
			return nil, 0, err
		}
		notifications = append(notifications, n)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, err
	}

	return notifications, total, nil
}

func (r *repository) UnreadCount(ctx context.Context, userID string) (int, error) {
	var count int
	err := r.db.QueryRow(ctx, "SELECT COUNT(*) FROM notifications WHERE user_id = $1 AND read_at IS NULL", userID).Scan(&count)
	return count, err
}

func (r *repository) MarkRead(ctx context.Context, userID string, notificationID string) error {
	query := `
		UPDATE notifications
		SET read_at = NOW()
		WHERE id = $1 AND user_id = $2 AND read_at IS NULL
	`
	tag, err := r.db.Exec(ctx, query, notificationID, userID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return pgx.ErrNoRows
	}
	return nil
}

func (r *repository) MarkAllRead(ctx context.Context, userID string) error {
	query := `
		UPDATE notifications
		SET read_at = NOW()
		WHERE user_id = $1 AND read_at IS NULL
	`
	_, err := r.db.Exec(ctx, query, userID)
	return err
}
