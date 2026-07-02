package mabar

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

var (
	ErrOpenMatchNotFound   = errors.New("open match not found")
	ErrParticipantNotFound = errors.New("participant not found")
)

type Repository struct {
	db *pgxpool.Pool
}

func NewRepository(db *pgxpool.Pool) *Repository {
	return &Repository{db: db}
}

func (r *Repository) ExecuteTx(ctx context.Context, fn func(tx pgx.Tx) error) error {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	if err := fn(tx); err != nil {
		return err
	}

	return tx.Commit(ctx)
}

type BookingInfo struct {
	ID         string
	CustomerID string
	Status     string
	Date       time.Time
	StartTime  time.Time
	EndTime    time.Time
}

func (r *Repository) FindBookingInfo(ctx context.Context, bookingID string) (BookingInfo, error) {
	query := `
		SELECT id::text, customer_id::text, status, booking_date, start_time, end_time
		FROM bookings
		WHERE id = $1
	`
	var b BookingInfo
	err := r.db.QueryRow(ctx, query, bookingID).Scan(&b.ID, &b.CustomerID, &b.Status, &b.Date, &b.StartTime, &b.EndTime)
	if err != nil {
		return b, err
	}
	return b, nil
}

func (r *Repository) CheckOpenMatchExistsByBookingID(ctx context.Context, bookingID string) (bool, error) {
	var count int
	err := r.db.QueryRow(ctx, `SELECT COUNT(*) FROM open_matches WHERE booking_id = $1`, bookingID).Scan(&count)
	return count > 0, err
}

type OpenMatch struct {
	ID             string
	BookingID      string
	HostUserID     string
	HostName       string
	Title          string
	Description    string
	SportName      string
	VenueName      string
	CourtName      string
	MatchDate      time.Time
	StartTime      time.Time
	EndTime        time.Time
	Level          string
	MaxPlayers     int
	PricePerPlayer float64
	Status         string
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

type CreateOpenMatchParams struct {
	BookingID      string
	HostUserID     string
	Title          string
	Description    string
	Level          string
	MaxPlayers     int
	PricePerPlayer float64
}

func (r *Repository) CreateOpenMatch(ctx context.Context, params CreateOpenMatchParams) (OpenMatch, error) {
	query := `
		INSERT INTO open_matches (booking_id, host_user_id, title, description, level, max_players, price_per_player, status)
		VALUES ($1, $2, $3, $4, $5, $6, $7, 'OPEN')
		RETURNING id::text, booking_id::text, host_user_id::text, title, description, level, max_players, price_per_player, status, created_at, updated_at
	`
	var om OpenMatch
	err := r.db.QueryRow(ctx, query,
		params.BookingID, params.HostUserID, params.Title, params.Description, params.Level, params.MaxPlayers, params.PricePerPlayer,
	).Scan(
		&om.ID, &om.BookingID, &om.HostUserID, &om.Title, &om.Description, &om.Level, &om.MaxPlayers, &om.PricePerPlayer, &om.Status, &om.CreatedAt, &om.UpdatedAt,
	)

	var pgErr *pgconn.PgError
	if err != nil && errors.As(err, &pgErr) && pgErr.Code == "23505" {
		return OpenMatch{}, ErrMatchAlreadyExists
	}

	return om, err
}

func (r *Repository) GetOpenMatchWithDetails(ctx context.Context, id string) (OpenMatch, error) {
	query := `
		SELECT 
			om.id::text, om.booking_id::text, om.host_user_id::text, u.name as host_name,
			om.title, om.description, s.name as sport_name, v.name as venue_name, c.name as court_name,
			b.booking_date, b.start_time, b.end_time,
			om.level, om.max_players, om.price_per_player, om.status, om.created_at, om.updated_at
		FROM open_matches om
		JOIN users u ON u.id = om.host_user_id
		JOIN bookings b ON b.id = om.booking_id
		JOIN courts c ON c.id = b.court_id
		JOIN venues v ON v.id = c.venue_id
		JOIN sports s ON s.id = c.sport_id
		WHERE om.id = $1
	`
	var om OpenMatch
	err := r.db.QueryRow(ctx, query, id).Scan(
		&om.ID, &om.BookingID, &om.HostUserID, &om.HostName,
		&om.Title, &om.Description, &om.SportName, &om.VenueName, &om.CourtName,
		&om.MatchDate, &om.StartTime, &om.EndTime,
		&om.Level, &om.MaxPlayers, &om.PricePerPlayer, &om.Status, &om.CreatedAt, &om.UpdatedAt,
	)
	if err == pgx.ErrNoRows {
		return om, ErrOpenMatchNotFound
	}
	return om, err
}

type ListOpenMatchesFilter struct {
	SportID string
	City    string
	Date    string
	Level   string
	Limit   int
	Offset  int
	Now     time.Time
}

func (r *Repository) ListOpenMatches(ctx context.Context, filter ListOpenMatchesFilter) ([]OpenMatch, int, error) {
	countQuery := `
		SELECT count(*)
		FROM open_matches om
		JOIN bookings b ON b.id = om.booking_id
		JOIN courts c ON c.id = b.court_id
		JOIN venues v ON v.id = c.venue_id
		JOIN sports s ON s.id = c.sport_id
		WHERE om.status = 'OPEN'
		  AND b.status IN ('PAID', 'CONFIRMED')
		  AND (b.booking_date + b.start_time::time) > $3
		  AND ($1 = '' OR s.id::text = $1)
		  AND ($2 = '' OR v.city ILIKE '%' || $2 || '%')
		  AND ($4 = '' OR b.booking_date::text = $4)
		  AND ($5 = '' OR om.level = $5)
	`
	var total int
	if err := r.db.QueryRow(ctx, countQuery, filter.SportID, filter.City, filter.Now, filter.Date, filter.Level).Scan(&total); err != nil {
		return nil, 0, err
	}

	query := `
		SELECT 
			om.id::text, om.booking_id::text, om.host_user_id::text, u.name as host_name,
			om.title, om.description, s.name as sport_name, v.name as venue_name, c.name as court_name,
			b.booking_date, b.start_time, b.end_time,
			om.level, om.max_players, om.price_per_player, om.status, om.created_at, om.updated_at
		FROM open_matches om
		JOIN users u ON u.id = om.host_user_id
		JOIN bookings b ON b.id = om.booking_id
		JOIN courts c ON c.id = b.court_id
		JOIN venues v ON v.id = c.venue_id
		JOIN sports s ON s.id = c.sport_id
		WHERE om.status = 'OPEN'
		  AND b.status IN ('PAID', 'CONFIRMED')
		  AND (b.booking_date + b.start_time::time) > $3
		  AND ($1 = '' OR s.id::text = $1)
		  AND ($2 = '' OR v.city ILIKE '%' || $2 || '%')
		  AND ($4 = '' OR b.booking_date::text = $4)
		  AND ($5 = '' OR om.level = $5)
		ORDER BY b.booking_date ASC, b.start_time ASC
		LIMIT $6 OFFSET $7
	`

	limit := filter.Limit
	if limit <= 0 || limit > 100 {
		limit = 10
	}
	offset := filter.Offset
	if offset < 0 {
		offset = 0
	}

	rows, err := r.db.Query(ctx, query, filter.SportID, filter.City, filter.Now, filter.Date, filter.Level, limit, offset)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var matches []OpenMatch
	for rows.Next() {
		var om OpenMatch
		if err := rows.Scan(
			&om.ID, &om.BookingID, &om.HostUserID, &om.HostName,
			&om.Title, &om.Description, &om.SportName, &om.VenueName, &om.CourtName,
			&om.MatchDate, &om.StartTime, &om.EndTime,
			&om.Level, &om.MaxPlayers, &om.PricePerPlayer, &om.Status, &om.CreatedAt, &om.UpdatedAt,
		); err != nil {
			return nil, 0, err
		}
		matches = append(matches, om)
	}
	return matches, total, rows.Err()
}

func (r *Repository) CountJoinedParticipants(ctx context.Context, openMatchID string) (int, error) {
	var count int
	err := r.db.QueryRow(ctx, `SELECT COUNT(*) FROM open_match_participants WHERE open_match_id = $1 AND status = 'JOINED'`, openMatchID).Scan(&count)
	return count, err
}

type Participant struct {
	ID          string
	OpenMatchID string
	UserID      string
	Name        string
	Status      string
	JoinedAt    time.Time
}

func (r *Repository) ListParticipants(ctx context.Context, openMatchID string) ([]Participant, error) {
	query := `
		SELECT p.id::text, p.open_match_id::text, p.user_id::text, u.name, p.status, p.joined_at
		FROM open_match_participants p
		JOIN users u ON u.id = p.user_id
		WHERE p.open_match_id = $1 AND p.status = 'JOINED'
		ORDER BY p.joined_at ASC
	`
	rows, err := r.db.Query(ctx, query, openMatchID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var participants []Participant
	for rows.Next() {
		var p Participant
		if err := rows.Scan(&p.ID, &p.OpenMatchID, &p.UserID, &p.Name, &p.Status, &p.JoinedAt); err != nil {
			return nil, err
		}
		participants = append(participants, p)
	}
	return participants, rows.Err()
}

func (r *Repository) FindParticipant(ctx context.Context, openMatchID, userID string) (Participant, error) {
	query := `
		SELECT id::text, open_match_id::text, user_id::text, status, joined_at
		FROM open_match_participants
		WHERE open_match_id = $1 AND user_id = $2
	`
	var p Participant
	err := r.db.QueryRow(ctx, query, openMatchID, userID).Scan(&p.ID, &p.OpenMatchID, &p.UserID, &p.Status, &p.JoinedAt)
	if err != nil {
		if err == pgx.ErrNoRows {
			return p, ErrParticipantNotFound
		}
		return p, err
	}
	return p, nil
}

// Transaction operations
func (r *Repository) LockOpenMatch(ctx context.Context, tx pgx.Tx, id string) (OpenMatch, error) {
	query := `
		SELECT id::text, booking_id::text, host_user_id::text, max_players, status 
		FROM open_matches 
		WHERE id = $1 FOR UPDATE
	`
	var om OpenMatch
	err := tx.QueryRow(ctx, query, id).Scan(&om.ID, &om.BookingID, &om.HostUserID, &om.MaxPlayers, &om.Status)
	if err == pgx.ErrNoRows {
		return om, ErrOpenMatchNotFound
	}
	return om, err
}

type OpenMatchJoinContext struct {
	OpenMatchID   string
	BookingID     string
	HostUserID    string
	MatchDate     time.Time
	StartTime     time.Time
	MaxPlayers    int
	MatchStatus   string
	BookingStatus string
}

func (r *Repository) GetOpenMatchJoinContextTx(ctx context.Context, tx pgx.Tx, openMatchID string) (OpenMatchJoinContext, error) {
	query := `
		SELECT
			om.id::text,
			om.booking_id::text,
			om.host_user_id::text,
			b.booking_date,
			b.start_time,
			om.max_players,
			om.status,
			b.status
		FROM open_matches om
		JOIN bookings b ON b.id = om.booking_id
		WHERE om.id = $1
		FOR UPDATE OF om, b
	`
	var jc OpenMatchJoinContext
	err := tx.QueryRow(ctx, query, openMatchID).Scan(
		&jc.OpenMatchID,
		&jc.BookingID,
		&jc.HostUserID,
		&jc.MatchDate,
		&jc.StartTime,
		&jc.MaxPlayers,
		&jc.MatchStatus,
		&jc.BookingStatus,
	)
	if err == pgx.ErrNoRows {
		return jc, ErrOpenMatchNotFound
	}
	return jc, err
}

func (r *Repository) CountJoinedParticipantsTx(ctx context.Context, tx pgx.Tx, openMatchID string) (int, error) {
	var count int
	err := tx.QueryRow(ctx, `SELECT COUNT(*) FROM open_match_participants WHERE open_match_id = $1 AND status = 'JOINED'`, openMatchID).Scan(&count)
	return count, err
}

func (r *Repository) UpsertParticipant(ctx context.Context, tx pgx.Tx, openMatchID, userID, status string) error {
	query := `
		INSERT INTO open_match_participants (open_match_id, user_id, status, joined_at, updated_at)
		VALUES ($1, $2, $3, now(), now())
		ON CONFLICT (open_match_id, user_id) 
		DO UPDATE SET status = EXCLUDED.status, updated_at = now(),
		              joined_at = CASE WHEN EXCLUDED.status = 'JOINED' THEN now() ELSE open_match_participants.joined_at END,
		              cancelled_at = CASE WHEN EXCLUDED.status = 'CANCELLED' THEN now() ELSE open_match_participants.cancelled_at END
	`
	_, err := tx.Exec(ctx, query, openMatchID, userID, status)
	return err
}

func (r *Repository) UpdateOpenMatchStatus(ctx context.Context, tx pgx.Tx, openMatchID, status string) error {
	query := `UPDATE open_matches SET status = $2, updated_at = now() WHERE id = $1`
	_, err := tx.Exec(ctx, query, openMatchID, status)
	return err
}

// Non-tx
func (r *Repository) CancelOpenMatch(ctx context.Context, openMatchID string) error {
	query := `UPDATE open_matches SET status = 'CANCELLED', updated_at = now() WHERE id = $1`
	_, err := r.db.Exec(ctx, query, openMatchID)
	return err
}
