package schedules

import (
	"context"
	"errors"
	"strconv"
	"strings"

	"lapangango-api/internal/httputil"
)

var (
	ErrOwnerProfileNotFound   = errors.New("owner profile not found")
	ErrCourtNotFound          = errors.New("court not found")
	ErrInvalidOperatingHours  = errors.New("invalid operating hours")
	ErrDuplicateOperatingDay  = errors.New("duplicate operating day")
	ErrIncompleteOperatingSet = errors.New("operating hours must contain all 7 days")
)

type Service struct {
	repository *Repository
}

func NewService(repository *Repository) *Service {
	return &Service{repository: repository}
}

func (s *Service) GetOperatingHours(ctx context.Context, ownerCtx httputil.OwnerContext, courtID string) ([]OperatingHourResponse, error) {
	court, err := s.getOwnedCourt(ctx, courtID, ownerCtx.OwnerProfileID)
	if err != nil {
		return nil, err
	}

	if !ownerCtx.IsOwner && !containsID(ownerCtx.AllowedVenueIDs, court.VenueID) {
		return nil, ErrCourtNotFound
	}

	operatingHours, err := s.repository.ListOperatingHoursByCourtID(ctx, courtID)
	if err != nil {
		return nil, err
	}

	return toOperatingHourResponses(operatingHours), nil
}

func (s *Service) ReplaceOperatingHours(ctx context.Context, ownerCtx httputil.OwnerContext, courtID string, req ReplaceOperatingHoursRequest) ([]OperatingHourResponse, error) {
	court, err := s.getOwnedCourt(ctx, courtID, ownerCtx.OwnerProfileID)
	if err != nil {
		return nil, err
	}

	if !ownerCtx.IsOwner && !containsID(ownerCtx.AllowedVenueIDs, court.VenueID) {
		return nil, ErrCourtNotFound
	}

	params, err := buildOperatingHourParams(req)
	if err != nil {
		return nil, err
	}

	operatingHours, err := s.repository.ReplaceOperatingHours(ctx, courtID, params)
	if err != nil {
		return nil, err
	}

	return toOperatingHourResponses(operatingHours), nil
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

func buildOperatingHourParams(req ReplaceOperatingHoursRequest) ([]OperatingHourParams, error) {
	if len(req.Days) != 7 {
		return nil, ErrIncompleteOperatingSet
	}

	seen := make(map[int]bool, 7)
	params := make([]OperatingHourParams, 0, len(req.Days))
	for _, day := range req.Days {
		if day.DayOfWeek == nil || *day.DayOfWeek < 0 || *day.DayOfWeek > 6 {
			return nil, ErrInvalidOperatingHours
		}
		if seen[*day.DayOfWeek] {
			return nil, ErrDuplicateOperatingDay
		}
		seen[*day.DayOfWeek] = true

		param, err := buildOperatingHourParam(day)
		if err != nil {
			return nil, err
		}
		params = append(params, param)
	}

	if len(seen) != 7 {
		return nil, ErrIncompleteOperatingSet
	}

	return params, nil
}

func buildOperatingHourParam(req OperatingHourRequest) (OperatingHourParams, error) {
	param := OperatingHourParams{
		DayOfWeek: *req.DayOfWeek,
		IsClosed:  req.IsClosed,
	}

	if req.IsClosed {
		if hasTimeValue(req.OpenTime) || hasTimeValue(req.CloseTime) {
			return OperatingHourParams{}, ErrInvalidOperatingHours
		}
		return param, nil
	}

	openTime := optionalTimeString(req.OpenTime)
	closeTime := optionalTimeString(req.CloseTime)
	if openTime == nil || closeTime == nil {
		return OperatingHourParams{}, ErrInvalidOperatingHours
	}

	openMinutes, err := parseClockMinutes(*openTime)
	if err != nil {
		return OperatingHourParams{}, ErrInvalidOperatingHours
	}

	closeMinutes, err := parseClockMinutes(*closeTime)
	if err != nil {
		return OperatingHourParams{}, ErrInvalidOperatingHours
	}

	if closeMinutes <= openMinutes {
		return OperatingHourParams{}, ErrInvalidOperatingHours
	}

	param.OpenTime = openTime
	param.CloseTime = closeTime

	return param, nil
}

func hasTimeValue(value *string) bool {
	return value != nil && strings.TrimSpace(*value) != ""
}

func optionalTimeString(value *string) *string {
	if value == nil {
		return nil
	}

	trimmed := strings.TrimSpace(*value)
	if trimmed == "" {
		return nil
	}

	return &trimmed
}

func parseClockMinutes(value string) (int, error) {
	if len(value) != 5 || value[2] != ':' {
		return 0, ErrInvalidOperatingHours
	}

	hour, err := strconv.Atoi(value[:2])
	if err != nil {
		return 0, err
	}

	minute, err := strconv.Atoi(value[3:])
	if err != nil {
		return 0, err
	}

	if hour < 0 || hour > 23 || minute < 0 || minute > 59 {
		return 0, ErrInvalidOperatingHours
	}

	return hour*60 + minute, nil
}

func toOperatingHourResponses(operatingHours []OperatingHour) []OperatingHourResponse {
	responses := make([]OperatingHourResponse, 0, len(operatingHours))
	for _, operatingHour := range operatingHours {
		responses = append(responses, OperatingHourResponse{
			ID:        operatingHour.ID,
			CourtID:   operatingHour.CourtID,
			DayOfWeek: operatingHour.DayOfWeek,
			OpenTime:  operatingHour.OpenTime,
			CloseTime: operatingHour.CloseTime,
			IsClosed:  operatingHour.IsClosed,
			CreatedAt: operatingHour.CreatedAt,
			UpdatedAt: operatingHour.UpdatedAt,
		})
	}

	return responses
}

func containsID(ids []string, id string) bool {
	for _, val := range ids {
		if val == id {
			return true
		}
	}
	return false
}
