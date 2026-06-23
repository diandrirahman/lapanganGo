package mabar

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

var (
	ErrUnauthorized            = errors.New("unauthorized action")
	ErrBookingNotFound         = errors.New("booking not found")
	ErrBookingInvalid          = errors.New("booking is not confirmed or cancelled")
	ErrBookingPassed           = errors.New("booking time has already passed")
	ErrMatchAlreadyExists      = errors.New("open match already exists for this booking")
	ErrMatchPassed             = errors.New("match time has already passed")
	ErrMatchNotOpen            = errors.New("open match is not open")
	ErrHostCannotJoin          = errors.New("host cannot join their own match")
	ErrAlreadyJoined           = errors.New("user already joined this match")
	ErrMatchFull               = errors.New("open match is full")
	ErrNotJoined               = errors.New("user is not joined to this match")
	ErrInvalidLevel            = errors.New("invalid level")
	ErrBookingCancelled        = errors.New("booking for this open match is cancelled")
	ErrBookingNotConfirmed     = errors.New("booking for this open match is not confirmed")
	ErrInvalidTitle            = errors.New("title is required")
	ErrInvalidMaxPlayers       = errors.New("max_players must be greater than 0")
	ErrInvalidPricePerPlayer   = errors.New("price_per_player cannot be negative")
	ErrCannotLeaveClosedMatch  = errors.New("cannot leave cancelled or completed match")
	ErrCannotCancelClosedMatch = errors.New("match already cancelled or completed")
)

type MabarRepository interface {
	FindBookingInfo(ctx context.Context, bookingID string) (BookingInfo, error)
	CheckOpenMatchExistsByBookingID(ctx context.Context, bookingID string) (bool, error)
	CreateOpenMatch(ctx context.Context, params CreateOpenMatchParams) (OpenMatch, error)
	ListOpenMatches(ctx context.Context, filter ListOpenMatchesFilter) ([]OpenMatch, error)
	CountJoinedParticipants(ctx context.Context, openMatchID string) (int, error)
	GetOpenMatchWithDetails(ctx context.Context, id string) (OpenMatch, error)
	ListParticipants(ctx context.Context, openMatchID string) ([]Participant, error)
	FindParticipant(ctx context.Context, openMatchID, userID string) (Participant, error)
	ExecuteTx(ctx context.Context, fn func(tx pgx.Tx) error) error
	LockOpenMatch(ctx context.Context, tx pgx.Tx, id string) (OpenMatch, error)
	GetOpenMatchJoinContextTx(ctx context.Context, tx pgx.Tx, openMatchID string) (OpenMatchJoinContext, error)
	CountJoinedParticipantsTx(ctx context.Context, tx pgx.Tx, openMatchID string) (int, error)
	UpsertParticipant(ctx context.Context, tx pgx.Tx, openMatchID, userID, status string) error
	UpdateOpenMatchStatus(ctx context.Context, tx pgx.Tx, openMatchID, status string) error
}

type Service struct {
	repo MabarRepository
}

func NewService(repo MabarRepository) *Service {
	return &Service{repo: repo}
}

func isValidLevel(level string) bool {
	valid := map[string]bool{
		"Beginner / Fun": true,
		"Intermediate":   true,
		"Advanced":       true,
		"All Levels":     true,
	}
	return valid[level]
}

func nowJakarta() time.Time {
	loc, err := time.LoadLocation("Asia/Jakarta")
	if err != nil {
		loc = time.FixedZone("Asia/Jakarta", 7*60*60)
	}
	return time.Now().In(loc)
}

func (s *Service) CreateOpenMatch(ctx context.Context, bookingID, userID string, req CreateOpenMatchRequest) (OpenMatchResponse, error) {
	title := strings.TrimSpace(req.Title)
	if title == "" {
		return OpenMatchResponse{}, ErrInvalidTitle
	}
	req.Title = title
	req.Description = strings.TrimSpace(req.Description)

	if !isValidLevel(req.Level) {
		return OpenMatchResponse{}, ErrInvalidLevel
	}
	if req.MaxPlayers <= 0 {
		return OpenMatchResponse{}, ErrInvalidMaxPlayers
	}
	if req.PricePerPlayer < 0 {
		return OpenMatchResponse{}, ErrInvalidPricePerPlayer
	}

	b, err := s.repo.FindBookingInfo(ctx, bookingID)
	if err != nil {
		return OpenMatchResponse{}, ErrBookingNotFound
	}

	if b.CustomerID != userID {
		return OpenMatchResponse{}, ErrUnauthorized
	}
	if b.Status != "CONFIRMED" {
		return OpenMatchResponse{}, ErrBookingInvalid
	}

	loc, err := time.LoadLocation("Asia/Jakarta")
	if err != nil {
		loc = time.FixedZone("Asia/Jakarta", 7*60*60)
	}
	matchTime := time.Date(b.Date.Year(), b.Date.Month(), b.Date.Day(), b.StartTime.Hour(), b.StartTime.Minute(), 0, 0, loc)

	if matchTime.Before(nowJakarta()) {
		return OpenMatchResponse{}, ErrBookingPassed
	}

	exists, err := s.repo.CheckOpenMatchExistsByBookingID(ctx, bookingID)
	if err != nil {
		return OpenMatchResponse{}, err
	}
	if exists {
		return OpenMatchResponse{}, ErrMatchAlreadyExists
	}

	om, err := s.repo.CreateOpenMatch(ctx, CreateOpenMatchParams{
		BookingID:      bookingID,
		HostUserID:     userID,
		Title:          req.Title,
		Description:    req.Description,
		Level:          req.Level,
		MaxPlayers:     req.MaxPlayers,
		PricePerPlayer: req.PricePerPlayer,
	})
	if err != nil {
		return OpenMatchResponse{}, err
	}

	return s.GetOpenMatchByID(ctx, om.ID)
}

func (s *Service) ListOpenMatches(ctx context.Context, filter ListOpenMatchesFilter) ([]OpenMatchResponse, error) {
	if filter.Now.IsZero() {
		filter.Now = nowJakarta()
	}
	matches, err := s.repo.ListOpenMatches(ctx, filter)
	if err != nil {
		return nil, err
	}

	var res []OpenMatchResponse
	for _, m := range matches {
		joined, err := s.repo.CountJoinedParticipants(ctx, m.ID)
		if err != nil {
			return nil, err
		}
		res = append(res, s.mapToResponse(m, joined))
	}
	return res, nil
}

func (s *Service) GetOpenMatchByID(ctx context.Context, id string) (OpenMatchResponse, error) {
	om, err := s.repo.GetOpenMatchWithDetails(ctx, id)
	if err != nil {
		return OpenMatchResponse{}, err
	}
	joined, err := s.repo.CountJoinedParticipants(ctx, id)
	if err != nil {
		return OpenMatchResponse{}, err
	}
	return s.mapToResponse(om, joined), nil
}

func (s *Service) GetOpenMatchDetail(ctx context.Context, id string) (OpenMatchDetailResponse, error) {
	omRes, err := s.GetOpenMatchByID(ctx, id)
	if err != nil {
		return OpenMatchDetailResponse{}, err
	}

	parts, err := s.repo.ListParticipants(ctx, id)
	if err != nil {
		return OpenMatchDetailResponse{}, err
	}

	var partRes []ParticipantResponse
	for _, p := range parts {
		partRes = append(partRes, ParticipantResponse{
			ID:       p.ID,
			UserID:   p.UserID,
			Name:     p.Name,
			Status:   p.Status,
			JoinedAt: p.JoinedAt,
		})
	}

	return OpenMatchDetailResponse{
		OpenMatch:    omRes,
		Participants: partRes,
	}, nil
}

func (s *Service) JoinOpenMatch(ctx context.Context, openMatchID, userID string) error {
	return s.repo.ExecuteTx(ctx, func(tx pgx.Tx) error {
		om, err := s.repo.LockOpenMatch(ctx, tx, openMatchID)
		if err != nil {
			return err
		}

		if om.HostUserID == userID {
			return ErrHostCannotJoin
		}
		if om.Status != "OPEN" {
			return ErrMatchNotOpen
		}

		joinCtx, err := s.repo.GetOpenMatchJoinContextTx(ctx, tx, openMatchID)
		if err != nil {
			return err
		}

		if joinCtx.BookingStatus != "CONFIRMED" {
			return ErrBookingNotConfirmed
		}

		// check time
		loc, err := time.LoadLocation("Asia/Jakarta")
		if err != nil {
			loc = time.FixedZone("Asia/Jakarta", 7*60*60)
		}
		matchTime := time.Date(joinCtx.MatchDate.Year(), joinCtx.MatchDate.Month(), joinCtx.MatchDate.Day(), joinCtx.StartTime.Hour(), joinCtx.StartTime.Minute(), 0, 0, loc)

		if matchTime.Before(nowJakarta()) {
			return ErrMatchPassed
		}

		part, err := s.repo.FindParticipant(ctx, openMatchID, userID)
		if err == nil && part.Status == "JOINED" {
			return ErrAlreadyJoined
		}

		// Count joined
		joinedCount, err := s.repo.CountJoinedParticipantsTx(ctx, tx, openMatchID)
		if err != nil {
			return err
		}

		if joinedCount >= om.MaxPlayers {
			return ErrMatchFull
		}

		if err := s.repo.UpsertParticipant(ctx, tx, openMatchID, userID, "JOINED"); err != nil {
			return err
		}

		if joinedCount+1 >= om.MaxPlayers {
			if err := s.repo.UpdateOpenMatchStatus(ctx, tx, openMatchID, "FULL"); err != nil {
				return err
			}
		}

		return nil
	})
}

func (s *Service) LeaveOpenMatch(ctx context.Context, openMatchID, userID string) error {
	return s.repo.ExecuteTx(ctx, func(tx pgx.Tx) error {
		om, err := s.repo.LockOpenMatch(ctx, tx, openMatchID)
		if err != nil {
			return err
		}

		if om.Status == "CANCELLED" || om.Status == "COMPLETED" {
			return ErrCannotLeaveClosedMatch
		}

		part, err := s.repo.FindParticipant(ctx, openMatchID, userID)
		if err != nil || part.Status != "JOINED" {
			return ErrNotJoined
		}

		if err := s.repo.UpsertParticipant(ctx, tx, openMatchID, userID, "CANCELLED"); err != nil {
			return err
		}

		if om.Status == "FULL" {
			if err := s.repo.UpdateOpenMatchStatus(ctx, tx, openMatchID, "OPEN"); err != nil {
				return err
			}
		}

		return nil
	})
}

func (s *Service) CancelOpenMatch(ctx context.Context, openMatchID, userID string) error {
	return s.repo.ExecuteTx(ctx, func(tx pgx.Tx) error {
		om, err := s.repo.LockOpenMatch(ctx, tx, openMatchID)
		if err != nil {
			return err
		}

		if om.HostUserID != userID {
			return ErrUnauthorized
		}

		if om.Status == "CANCELLED" || om.Status == "COMPLETED" {
			return ErrCannotCancelClosedMatch
		}

		return s.repo.UpdateOpenMatchStatus(ctx, tx, openMatchID, "CANCELLED")
	})
}

func (s *Service) mapToResponse(m OpenMatch, joined int) OpenMatchResponse {
	return OpenMatchResponse{
		ID:             m.ID,
		BookingID:      m.BookingID,
		HostName:       m.HostName,
		Title:          m.Title,
		Description:    m.Description,
		SportName:      m.SportName,
		VenueName:      m.VenueName,
		CourtName:      m.CourtName,
		MatchDate:      m.MatchDate.Format("2006-01-02"),
		StartTime:      m.StartTime.Format("15:04"),
		EndTime:        m.EndTime.Format("15:04"),
		Level:          m.Level,
		MaxPlayers:     m.MaxPlayers,
		JoinedCount:    joined,
		RemainingSlots: m.MaxPlayers - joined,
		PricePerPlayer: m.PricePerPlayer,
		Status:         m.Status,
		CreatedAt:      m.CreatedAt,
		UpdatedAt:      m.UpdatedAt,
	}
}
