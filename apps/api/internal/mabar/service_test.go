package mabar_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"lapangango-api/internal/mabar"
)

type mockRepo struct {
	mabar.MabarRepository
	bookingInfo        mabar.BookingInfo
	bookingErr         error
	openMatch          mabar.OpenMatch
	openMatchErr       error
	createErr          error
	participant        mabar.Participant
	participantErr     error
	joinedCount        int
	joinedCountErr     error
	exists             bool
	existsErr          error
	joinCtx            mabar.OpenMatchJoinContext
	joinCtxErr         error
	upsertCalled       bool
	upsertStatus       string
	updateStatusCalled bool
	updatedStatus      string
}

func (m *mockRepo) FindBookingInfo(ctx context.Context, bookingID string) (mabar.BookingInfo, error) {
	return m.bookingInfo, m.bookingErr
}

func (m *mockRepo) CheckOpenMatchExistsByBookingID(ctx context.Context, bookingID string) (bool, error) {
	return m.exists, m.existsErr
}

func (m *mockRepo) CreateOpenMatch(ctx context.Context, params mabar.CreateOpenMatchParams) (mabar.OpenMatch, error) {
	return m.openMatch, m.createErr
}

func (m *mockRepo) GetOpenMatchWithDetails(ctx context.Context, id string) (mabar.OpenMatch, error) {
	return m.openMatch, m.openMatchErr
}

func (m *mockRepo) CountJoinedParticipants(ctx context.Context, id string) (int, error) {
	return m.joinedCount, m.joinedCountErr
}

func (m *mockRepo) GetOpenMatchJoinContextTx(ctx context.Context, tx pgx.Tx, openMatchID string) (mabar.OpenMatchJoinContext, error) {
	return m.joinCtx, m.joinCtxErr
}

func (m *mockRepo) CountJoinedParticipantsTx(ctx context.Context, tx pgx.Tx, openMatchID string) (int, error) {
	return m.joinedCount, m.joinedCountErr
}

func (m *mockRepo) ExecuteTx(ctx context.Context, fn func(tx pgx.Tx) error) error {
	// For testing, just execute fn directly without a real tx.
	// Since fn might call LockOpenMatch which needs a tx, we'll implement a mock for LockOpenMatch that ignores tx
	return fn(nil)
}

func (m *mockRepo) LockOpenMatch(ctx context.Context, tx pgx.Tx, id string) (mabar.OpenMatch, error) {
	return m.openMatch, m.openMatchErr
}

func (m *mockRepo) FindParticipant(ctx context.Context, openMatchID, userID string) (mabar.Participant, error) {
	return m.participant, m.participantErr
}

func (m *mockRepo) UpsertParticipant(ctx context.Context, tx pgx.Tx, openMatchID, userID, status string) error {
	m.upsertCalled = true
	m.upsertStatus = status
	return nil
}

func (m *mockRepo) UpdateOpenMatchStatus(ctx context.Context, tx pgx.Tx, openMatchID, status string) error {
	m.updateStatusCalled = true
	m.updatedStatus = status
	return nil
}

// In the real code tx.QueryRow is used. To decouple we'd need to mock it, but for our tests we'll just test CreateOpenMatch and CancelOpenMatch which don't directly call tx.QueryRow, wait, JoinOpenMatch uses tx.QueryRow.
// To fix JoinOpenMatch using tx.QueryRow directly we should abstract it.

func TestService_CreateOpenMatch_Success(t *testing.T) {
	repo := &mockRepo{
		bookingInfo: mabar.BookingInfo{
			CustomerID: "user-1",
			Status:     "PAID",
			Date:       time.Now().Add(24 * time.Hour),
			StartTime:  time.Now().Add(24 * time.Hour),
		},
		openMatch: mabar.OpenMatch{ID: "om-1", MaxPlayers: 10},
	}
	svc := mabar.NewService(repo)

	req := mabar.CreateOpenMatchRequest{
		Title:          "Test Match",
		Level:          "All Levels",
		MaxPlayers:     10,
		PricePerPlayer: 50000,
	}

	_, err := svc.CreateOpenMatch(context.Background(), "b-1", "user-1", req)
	if err != nil {
		t.Errorf("Expected no error, got %v", err)
	}
}

func TestService_CreateOpenMatch_CancelledBooking(t *testing.T) {
	repo := &mockRepo{
		bookingInfo: mabar.BookingInfo{
			CustomerID: "user-1",
			Status:     "CANCELLED",
		},
	}
	svc := mabar.NewService(repo)

	req := mabar.CreateOpenMatchRequest{Title: "Valid", Level: "All Levels", MaxPlayers: 10, PricePerPlayer: 0}
	_, err := svc.CreateOpenMatch(context.Background(), "b-1", "user-1", req)
	if err != mabar.ErrBookingInvalid {
		t.Errorf("Expected ErrBookingInvalid, got %v", err)
	}
}

func TestService_CreateOpenMatch_NotOwner(t *testing.T) {
	repo := &mockRepo{
		bookingInfo: mabar.BookingInfo{
			CustomerID: "user-2", // different
			Status:     "CONFIRMED",
		},
	}
	svc := mabar.NewService(repo)

	req := mabar.CreateOpenMatchRequest{Title: "Valid", Level: "All Levels", MaxPlayers: 10, PricePerPlayer: 0}
	_, err := svc.CreateOpenMatch(context.Background(), "b-1", "user-1", req)
	if err != mabar.ErrUnauthorized {
		t.Errorf("Expected ErrUnauthorized, got %v", err)
	}
}

func TestService_CancelOpenMatch_Success(t *testing.T) {
	repo := &mockRepo{
		openMatch: mabar.OpenMatch{
			HostUserID: "host-1",
			Status:     "OPEN",
		},
	}
	svc := mabar.NewService(repo)

	err := svc.CancelOpenMatch(context.Background(), "om-1", "host-1")
	if err != nil {
		t.Errorf("Expected no error, got %v", err)
	}
}

func TestService_CancelOpenMatch_Unauthorized(t *testing.T) {
	repo := &mockRepo{
		openMatch: mabar.OpenMatch{
			HostUserID: "host-1",
			Status:     "OPEN",
		},
	}
	svc := mabar.NewService(repo)

	err := svc.CancelOpenMatch(context.Background(), "om-1", "user-1")
	if err != mabar.ErrUnauthorized {
		t.Errorf("Expected ErrUnauthorized, got %v", err)
	}
}

func TestService_CreateOpenMatch_DuplicateConflict(t *testing.T) {
	repo := &mockRepo{
		bookingInfo: mabar.BookingInfo{
			CustomerID: "user-1",
			Status:     "CONFIRMED",
			Date:       time.Now().Add(24 * time.Hour),
			StartTime:  time.Now().Add(24 * time.Hour),
		},
		exists: true, // mock duplicate
	}
	svc := mabar.NewService(repo)

	req := mabar.CreateOpenMatchRequest{Title: "Valid Title", Level: "All Levels", MaxPlayers: 10, PricePerPlayer: 0}
	_, err := svc.CreateOpenMatch(context.Background(), "b-1", "user-1", req)
	if err != mabar.ErrMatchAlreadyExists {
		t.Errorf("Expected ErrMatchAlreadyExists, got %v", err)
	}
}

func TestService_CreateOpenMatch_UniqueViolation(t *testing.T) {
	repo := &mockRepo{
		bookingInfo: mabar.BookingInfo{
			CustomerID: "user-1",
			Status:     "CONFIRMED",
			Date:       time.Now().Add(24 * time.Hour),
			StartTime:  time.Now().Add(24 * time.Hour),
		},
		exists:    false,                       // Passes first check
		createErr: mabar.ErrMatchAlreadyExists, // But fails on insert due to race condition
	}
	svc := mabar.NewService(repo)

	req := mabar.CreateOpenMatchRequest{Title: "Valid Title", Level: "All Levels", MaxPlayers: 10, PricePerPlayer: 0}
	_, err := svc.CreateOpenMatch(context.Background(), "b-1", "user-1", req)
	if err != mabar.ErrMatchAlreadyExists {
		t.Errorf("Expected ErrMatchAlreadyExists, got %v", err)
	}
}

func TestService_JoinOpenMatch_Success(t *testing.T) {
	repo := &mockRepo{
		openMatch: mabar.OpenMatch{
			HostUserID: "host-1",
			Status:     "OPEN",
			MaxPlayers: 10,
		},
		joinCtx: mabar.OpenMatchJoinContext{
			BookingStatus: "PAID",
			MatchStatus:   "OPEN",
			MatchDate:     time.Now().Add(24 * time.Hour),
			StartTime:     time.Now().Add(24 * time.Hour),
		},
		participantErr: mabar.ErrParticipantNotFound, // User hasn't joined
		joinedCount:    5,
	}
	svc := mabar.NewService(repo)

	err := svc.JoinOpenMatch(context.Background(), "om-1", "user-1")
	if err != nil {
		t.Errorf("Expected no error, got %v", err)
	}
	if !repo.upsertCalled || repo.upsertStatus != "JOINED" {
		t.Errorf("Expected UpsertParticipant JOINED")
	}
	if repo.updateStatusCalled {
		t.Errorf("Expected UpdateOpenMatchStatus NOT to be called since not full")
	}
}

func TestService_JoinOpenMatch_LastSlotMarksFull(t *testing.T) {
	repo := &mockRepo{
		openMatch: mabar.OpenMatch{
			HostUserID: "host-1",
			Status:     "OPEN",
			MaxPlayers: 2,
		},
		joinCtx: mabar.OpenMatchJoinContext{
			BookingStatus: "CONFIRMED",
			MatchStatus:   "OPEN",
			MatchDate:     time.Now().Add(24 * time.Hour),
			StartTime:     time.Now().Add(24 * time.Hour),
		},
		participantErr: mabar.ErrParticipantNotFound, // User hasn't joined
		joinedCount:    1,                            // 1/2 joined. This join makes it 2
	}
	svc := mabar.NewService(repo)

	err := svc.JoinOpenMatch(context.Background(), "om-1", "user-1")
	if err != nil {
		t.Errorf("Expected no error, got %v", err)
	}
	if !repo.upsertCalled || repo.upsertStatus != "JOINED" {
		t.Errorf("Expected UpsertParticipant JOINED")
	}
	if !repo.updateStatusCalled || repo.updatedStatus != "FULL" {
		t.Errorf("Expected UpdateOpenMatchStatus FULL")
	}
}

func TestService_JoinOpenMatch_AlreadyJoined(t *testing.T) {
	repo := &mockRepo{
		openMatch: mabar.OpenMatch{
			HostUserID: "host-1",
			Status:     "OPEN",
		},
		joinCtx: mabar.OpenMatchJoinContext{
			BookingStatus: "CONFIRMED",
			MatchStatus:   "OPEN",
			MatchDate:     time.Now().Add(24 * time.Hour),
			StartTime:     time.Now().Add(24 * time.Hour),
		},
		participant: mabar.Participant{Status: "JOINED"},
	}
	svc := mabar.NewService(repo)

	err := svc.JoinOpenMatch(context.Background(), "om-1", "user-1")
	if err != mabar.ErrAlreadyJoined {
		t.Errorf("Expected ErrAlreadyJoined, got %v", err)
	}
}

func TestService_JoinOpenMatch_Full(t *testing.T) {
	repo := &mockRepo{
		openMatch: mabar.OpenMatch{
			HostUserID: "host-1",
			Status:     "OPEN",
			MaxPlayers: 2,
		},
		joinCtx: mabar.OpenMatchJoinContext{
			BookingStatus: "CONFIRMED",
			MatchStatus:   "OPEN",
			MatchDate:     time.Now().Add(24 * time.Hour),
			StartTime:     time.Now().Add(24 * time.Hour),
		},
		participantErr: mabar.ErrParticipantNotFound,
		joinedCount:    2, // Already full
	}
	svc := mabar.NewService(repo)

	err := svc.JoinOpenMatch(context.Background(), "om-1", "user-1")
	if err != mabar.ErrMatchFull {
		t.Errorf("Expected ErrMatchFull, got %v", err)
	}
}

func TestService_JoinOpenMatch_CountErrorDoesNotUpsert(t *testing.T) {
	repo := &mockRepo{
		openMatch: mabar.OpenMatch{
			HostUserID: "host-1",
			Status:     "OPEN",
		},
		joinCtx: mabar.OpenMatchJoinContext{
			BookingStatus: "CONFIRMED",
			MatchStatus:   "OPEN",
			MatchDate:     time.Now().Add(24 * time.Hour),
			StartTime:     time.Now().Add(24 * time.Hour),
		},
		participantErr: mabar.ErrParticipantNotFound,
		joinedCountErr: errors.New("db count error"),
	}
	svc := mabar.NewService(repo)

	err := svc.JoinOpenMatch(context.Background(), "om-1", "user-1")
	if err == nil {
		t.Errorf("Expected count error, got nil")
	}
	if repo.upsertCalled {
		t.Errorf("Expected UpsertParticipant NOT to be called")
	}
}

func TestService_JoinOpenMatch_BookingNotConfirmed(t *testing.T) {
	repo := &mockRepo{
		openMatch: mabar.OpenMatch{
			HostUserID: "host-1",
			Status:     "OPEN",
		},
		joinCtx: mabar.OpenMatchJoinContext{
			BookingStatus: "WAITING_VERIFICATION", // Or any non-PAID/non-CONFIRMED status
		},
	}
	svc := mabar.NewService(repo)

	err := svc.JoinOpenMatch(context.Background(), "om-1", "user-1")
	if err != mabar.ErrBookingNotConfirmed {
		t.Errorf("Expected ErrBookingNotConfirmed, got %v", err)
	}
}

func TestService_LeaveOpenMatch_FromFullMarksOpen(t *testing.T) {
	repo := &mockRepo{
		openMatch: mabar.OpenMatch{
			HostUserID: "host-1",
			Status:     "FULL",
		},
		participant: mabar.Participant{
			Status: "JOINED",
		},
	}
	svc := mabar.NewService(repo)

	err := svc.LeaveOpenMatch(context.Background(), "om-1", "user-1")
	if err != nil {
		t.Errorf("Expected no error, got %v", err)
	}
	if !repo.upsertCalled || repo.upsertStatus != "CANCELLED" {
		t.Errorf("Expected UpsertParticipant to be called with CANCELLED")
	}
	if !repo.updateStatusCalled || repo.updatedStatus != "OPEN" {
		t.Errorf("Expected UpdateOpenMatchStatus to be called with OPEN")
	}
}

func TestService_LeaveOpenMatch_NotJoined(t *testing.T) {
	repo := &mockRepo{
		openMatch: mabar.OpenMatch{
			HostUserID: "host-1",
			Status:     "OPEN",
		},
		participantErr: mabar.ErrParticipantNotFound,
	}
	svc := mabar.NewService(repo)

	err := svc.LeaveOpenMatch(context.Background(), "om-1", "user-1")
	if err != mabar.ErrNotJoined {
		t.Errorf("Expected ErrNotJoined, got %v", err)
	}
}

func TestService_CreateOpenMatch_InvalidTitle(t *testing.T) {
	repo := &mockRepo{}
	svc := mabar.NewService(repo)
	req := mabar.CreateOpenMatchRequest{Title: "   ", Level: "All Levels", MaxPlayers: 10, PricePerPlayer: 0}
	_, err := svc.CreateOpenMatch(context.Background(), "b-1", "user-1", req)
	if err != mabar.ErrInvalidTitle {
		t.Errorf("Expected ErrInvalidTitle, got %v", err)
	}
}

func TestService_CreateOpenMatch_InvalidLevel(t *testing.T) {
	repo := &mockRepo{}
	svc := mabar.NewService(repo)
	req := mabar.CreateOpenMatchRequest{Title: "Valid", Level: "Invalid", MaxPlayers: 10, PricePerPlayer: 0}
	_, err := svc.CreateOpenMatch(context.Background(), "b-1", "user-1", req)
	if err != mabar.ErrInvalidLevel {
		t.Errorf("Expected ErrInvalidLevel, got %v", err)
	}
}

func TestService_CreateOpenMatch_InvalidMaxPlayers(t *testing.T) {
	repo := &mockRepo{}
	svc := mabar.NewService(repo)
	req := mabar.CreateOpenMatchRequest{Title: "Valid", Level: "All Levels", MaxPlayers: 0, PricePerPlayer: 0}
	_, err := svc.CreateOpenMatch(context.Background(), "b-1", "user-1", req)
	if err != mabar.ErrInvalidMaxPlayers {
		t.Errorf("Expected ErrInvalidMaxPlayers, got %v", err)
	}
}

func TestService_CreateOpenMatch_InvalidPricePerPlayer(t *testing.T) {
	repo := &mockRepo{}
	svc := mabar.NewService(repo)
	req := mabar.CreateOpenMatchRequest{Title: "Valid", Level: "All Levels", MaxPlayers: 10, PricePerPlayer: -100}
	_, err := svc.CreateOpenMatch(context.Background(), "b-1", "user-1", req)
	if err != mabar.ErrInvalidPricePerPlayer {
		t.Errorf("Expected ErrInvalidPricePerPlayer, got %v", err)
	}
}

func TestService_CancelOpenMatch_ClosedMatch(t *testing.T) {
	repo := &mockRepo{
		openMatch: mabar.OpenMatch{
			HostUserID: "host-1",
			Status:     "COMPLETED",
		},
	}
	svc := mabar.NewService(repo)
	err := svc.CancelOpenMatch(context.Background(), "om-1", "host-1")
	if err != mabar.ErrCannotCancelClosedMatch {
		t.Errorf("Expected ErrCannotCancelClosedMatch, got %v", err)
	}
}
