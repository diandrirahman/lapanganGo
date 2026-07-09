package blockedslots

import (
	"context"
	"errors"
	"strings"
	"time"

	"lapangango-api/internal/httputil"
)

var (
	ErrOwnerProfileNotFound    = errors.New("owner profile not found")
	ErrCourtNotFound           = errors.New("court not found")
	ErrBlockedSlotNotFound     = errors.New("blocked slot not found")
	ErrInvalidBlockedSlot      = errors.New("invalid blocked slot")
	ErrInvalidBlockedSlotRange = errors.New("invalid blocked slot range")
)

type Service struct {
	repository *Repository
}

func NewService(repository *Repository) *Service {
	return &Service{repository: repository}
}

func (s *Service) CreateBlockedSlot(ctx context.Context, ownerCtx httputil.OwnerContext, courtID string, req CreateBlockedSlotRequest) (BlockedSlotResponse, error) {
	court, err := s.getOwnedCourt(ctx, courtID, ownerCtx.OwnerProfileID)
	if err != nil {
		return BlockedSlotResponse{}, err
	}

	if !ownerCtx.IsOwner && !containsID(ownerCtx.AllowedVenueIDs, court.VenueID) {
		return BlockedSlotResponse{}, ErrCourtNotFound
	}

	params, err := buildBlockedSlotParams(courtID, req)
	if err != nil {
		return BlockedSlotResponse{}, err
	}

	blockedSlot, err := s.repository.Create(ctx, params)
	if err != nil {
		return BlockedSlotResponse{}, err
	}

	return toBlockedSlotResponse(blockedSlot), nil
}

func (s *Service) ListBlockedSlots(ctx context.Context, ownerCtx httputil.OwnerContext, courtID, fromValue, toValue string) ([]BlockedSlotResponse, error) {
	court, err := s.getOwnedCourt(ctx, courtID, ownerCtx.OwnerProfileID)
	if err != nil {
		return nil, err
	}

	if !ownerCtx.IsOwner && !containsID(ownerCtx.AllowedVenueIDs, court.VenueID) {
		return []BlockedSlotResponse{}, nil
	}

	from, to, err := buildListRange(fromValue, toValue)
	if err != nil {
		return nil, err
	}

	blockedSlots, err := s.repository.ListByCourtID(ctx, courtID, from, to)
	if err != nil {
		return nil, err
	}

	return toBlockedSlotResponses(blockedSlots), nil
}

func (s *Service) DeleteBlockedSlot(ctx context.Context, ownerCtx httputil.OwnerContext, blockedSlotID string) (BlockedSlotResponse, error) {
	blockedSlot, err := s.repository.FindByIDAndOwnerProfileID(ctx, blockedSlotID, ownerCtx.OwnerProfileID)
	if IsNotFound(err) {
		return BlockedSlotResponse{}, ErrBlockedSlotNotFound
	}
	if err != nil {
		return BlockedSlotResponse{}, err
	}

	if !ownerCtx.IsOwner {
		court, err := s.getOwnedCourt(ctx, blockedSlot.CourtID, ownerCtx.OwnerProfileID)
		if err != nil {
			return BlockedSlotResponse{}, err
		}
		if !containsID(ownerCtx.AllowedVenueIDs, court.VenueID) {
			return BlockedSlotResponse{}, ErrBlockedSlotNotFound
		}
	}

	blockedSlot, err = s.repository.DeleteByIDAndOwnerProfileID(ctx, blockedSlotID, ownerCtx.OwnerProfileID)
	if IsNotFound(err) {
		return BlockedSlotResponse{}, ErrBlockedSlotNotFound
	}
	if err != nil {
		return BlockedSlotResponse{}, err
	}

	return toBlockedSlotResponse(blockedSlot), nil
}



func (s *Service) getOwnedCourt(ctx context.Context, courtID, ownerProfileID string) (Court, error) {
	court, err := s.repository.FindCourtByIDAndOwnerProfileID(ctx, courtID, ownerProfileID)
	if IsNotFound(err) {
		return Court{}, ErrCourtNotFound
	}
	if err != nil {
		return Court{}, err
	}

	return court, nil
}

func buildBlockedSlotParams(courtID string, req CreateBlockedSlotRequest) (BlockedSlotParams, error) {
	startAt, err := parseRFC3339(req.StartAt)
	if err != nil {
		return BlockedSlotParams{}, ErrInvalidBlockedSlot
	}

	endAt, err := parseRFC3339(req.EndAt)
	if err != nil {
		return BlockedSlotParams{}, ErrInvalidBlockedSlot
	}

	if !endAt.After(startAt) {
		return BlockedSlotParams{}, ErrInvalidBlockedSlotRange
	}

	return BlockedSlotParams{
		CourtID: courtID,
		StartAt: startAt,
		EndAt:   endAt,
		Reason:  optionalString(req.Reason),
	}, nil
}

func buildListRange(fromValue, toValue string) (*time.Time, *time.Time, error) {
	from, err := optionalRFC3339(fromValue)
	if err != nil {
		return nil, nil, ErrInvalidBlockedSlot
	}

	to, err := optionalRFC3339(toValue)
	if err != nil {
		return nil, nil, ErrInvalidBlockedSlot
	}

	if from != nil && to != nil && !to.After(*from) {
		return nil, nil, ErrInvalidBlockedSlotRange
	}

	return from, to, nil
}

func optionalRFC3339(value string) (*time.Time, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil, nil
	}

	parsed, err := parseRFC3339(value)
	if err != nil {
		return nil, err
	}

	return &parsed, nil
}

func parseRFC3339(value string) (time.Time, error) {
	parsed, err := time.Parse(time.RFC3339, strings.TrimSpace(value))
	if err != nil {
		return time.Time{}, err
	}

	return parsed, nil
}

func optionalString(value string) *string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return nil
	}

	return &trimmed
}

func toBlockedSlotResponses(blockedSlots []BlockedSlot) []BlockedSlotResponse {
	responses := make([]BlockedSlotResponse, 0, len(blockedSlots))
	for _, blockedSlot := range blockedSlots {
		responses = append(responses, toBlockedSlotResponse(blockedSlot))
	}

	return responses
}

func toBlockedSlotResponse(blockedSlot BlockedSlot) BlockedSlotResponse {
	return BlockedSlotResponse{
		ID:        blockedSlot.ID,
		CourtID:   blockedSlot.CourtID,
		StartAt:   blockedSlot.StartAt,
		EndAt:     blockedSlot.EndAt,
		Reason:    blockedSlot.Reason,
		CreatedAt: blockedSlot.CreatedAt,
		UpdatedAt: blockedSlot.UpdatedAt,
	}
}

func containsID(ids []string, id string) bool {
	for _, val := range ids {
		if val == id {
			return true
		}
	}
	return false
}
