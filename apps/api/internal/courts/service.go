package courts

import (
	"context"
	"errors"
	"strings"

	"lapangango-api/internal/httputil"
)

var (
	ErrOwnerProfileNotFound = errors.New("owner profile not found")
	ErrVenueNotFound        = errors.New("venue not found")
	ErrCourtNotFound        = errors.New("court not found")
	ErrCourtAlreadyExists   = errors.New("court already exists")
	ErrSportNotFound        = errors.New("sport not found")
	ErrInvalidCourtPayload  = errors.New("invalid court payload")
	ErrInvalidCourtStatus   = errors.New("invalid court status")
)

type Service struct {
	repository *Repository
}

func NewService(repository *Repository) *Service {
	return &Service{repository: repository}
}

func (s *Service) CreateCourt(ctx context.Context, ownerCtx httputil.OwnerContext, venueID string, req CreateCourtRequest) (CourtResponse, error) {
	if !ownerCtx.IsOwner && !containsID(ownerCtx.AllowedVenueIDs, venueID) {
		return CourtResponse{}, ErrVenueNotFound
	}

	if _, err := s.getOwnedVenue(ctx, venueID, ownerCtx.OwnerProfileID); err != nil {
		return CourtResponse{}, err
	}

	if _, err := s.getSport(ctx, req.SportID); err != nil {
		return CourtResponse{}, err
	}

	params, err := buildCreateCourtParams(venueID, req)
	if err != nil {
		return CourtResponse{}, err
	}

	court, err := s.repository.Create(ctx, params)
	if IsUniqueViolation(err) {
		return CourtResponse{}, ErrCourtAlreadyExists
	}
	if err != nil {
		return CourtResponse{}, err
	}

	return toCourtResponse(court), nil
}

func (s *Service) ListCourts(ctx context.Context, ownerCtx httputil.OwnerContext, venueID string) ([]CourtResponse, error) {
	if !ownerCtx.IsOwner && !containsID(ownerCtx.AllowedVenueIDs, venueID) {
		return []CourtResponse{}, nil
	}

	if _, err := s.getOwnedVenue(ctx, venueID, ownerCtx.OwnerProfileID); err != nil {
		return nil, err
	}

	courts, err := s.repository.ListByVenueIDAndOwnerProfileID(ctx, venueID, ownerCtx.OwnerProfileID)
	if err != nil {
		return nil, err
	}

	responses := make([]CourtResponse, 0, len(courts))
	for _, court := range courts {
		responses = append(responses, toCourtResponse(court))
	}

	return responses, nil
}

func (s *Service) GetCourt(ctx context.Context, ownerCtx httputil.OwnerContext, courtID string) (CourtResponse, error) {

	court, err := s.repository.FindByIDAndOwnerProfileID(ctx, courtID, ownerCtx.OwnerProfileID)
	if IsNotFound(err) {
		return CourtResponse{}, ErrCourtNotFound
	}
	if err != nil {
		return CourtResponse{}, err
	}

	if !ownerCtx.IsOwner && !containsID(ownerCtx.AllowedVenueIDs, court.VenueID) {
		return CourtResponse{}, ErrCourtNotFound
	}

	return toCourtResponse(court), nil
}

func (s *Service) UpdateCourt(ctx context.Context, ownerCtx httputil.OwnerContext, courtID string, req UpdateCourtRequest) (CourtResponse, error) {

	if _, err := s.getSport(ctx, req.SportID); err != nil {
		return CourtResponse{}, err
	}

	params, err := buildUpdateCourtParams(req)
	if err != nil {
		return CourtResponse{}, err
	}

	court, err := s.repository.UpdateByIDAndOwnerProfileID(ctx, courtID, ownerCtx.OwnerProfileID, params)
	if IsNotFound(err) {
		return CourtResponse{}, ErrCourtNotFound
	}
	if IsUniqueViolation(err) {
		return CourtResponse{}, ErrCourtAlreadyExists
	}
	if err != nil {
		return CourtResponse{}, err
	}

	if !ownerCtx.IsOwner && !containsID(ownerCtx.AllowedVenueIDs, court.VenueID) {
		return CourtResponse{}, ErrCourtNotFound
	}

	return toCourtResponse(court), nil
}

func (s *Service) UpdateCourtStatus(ctx context.Context, ownerCtx httputil.OwnerContext, courtID, status string) (CourtResponse, error) {

	status = strings.TrimSpace(status)
	if !isWritableCourtStatus(status) {
		return CourtResponse{}, ErrInvalidCourtStatus
	}

	court, err := s.repository.UpdateStatusByIDAndOwnerProfileID(ctx, courtID, ownerCtx.OwnerProfileID, status)
	if IsNotFound(err) {
		return CourtResponse{}, ErrCourtNotFound
	}
	if err != nil {
		return CourtResponse{}, err
	}

	if !ownerCtx.IsOwner && !containsID(ownerCtx.AllowedVenueIDs, court.VenueID) {
		return CourtResponse{}, ErrCourtNotFound
	}

	return toCourtResponse(court), nil
}



func (s *Service) getOwnedVenue(ctx context.Context, venueID, ownerProfileID string) (Venue, error) {
	venue, err := s.repository.FindVenueByIDAndOwnerProfileID(ctx, venueID, ownerProfileID)
	if IsNotFound(err) {
		return Venue{}, ErrVenueNotFound
	}
	if err != nil {
		return Venue{}, err
	}

	return venue, nil
}

func (s *Service) getSport(ctx context.Context, sportID string) (Sport, error) {
	sport, err := s.repository.FindSportByID(ctx, strings.TrimSpace(sportID))
	if IsNotFound(err) {
		return Sport{}, ErrSportNotFound
	}
	if err != nil {
		return Sport{}, err
	}

	return sport, nil
}

func buildCreateCourtParams(venueID string, req CreateCourtRequest) (CourtParams, error) {
	if req.PricePerHour == nil {
		return CourtParams{}, ErrInvalidCourtPayload
	}

	params := CourtParams{
		VenueID:      venueID,
		SportID:      strings.TrimSpace(req.SportID),
		Name:         strings.TrimSpace(req.Name),
		Description:  optionalString(req.Description),
		LocationType: strings.TrimSpace(req.LocationType),
		SurfaceType:  optionalString(req.SurfaceType),
		PricePerHour: *req.PricePerHour,
	}

	if !isValidCourtParams(params) {
		return CourtParams{}, ErrInvalidCourtPayload
	}

	return params, nil
}

func buildUpdateCourtParams(req UpdateCourtRequest) (CourtParams, error) {
	if req.PricePerHour == nil {
		return CourtParams{}, ErrInvalidCourtPayload
	}

	params := CourtParams{
		SportID:      strings.TrimSpace(req.SportID),
		Name:         strings.TrimSpace(req.Name),
		Description:  optionalString(req.Description),
		LocationType: strings.TrimSpace(req.LocationType),
		SurfaceType:  optionalString(req.SurfaceType),
		PricePerHour: *req.PricePerHour,
	}

	if !isValidCourtParams(params) {
		return CourtParams{}, ErrInvalidCourtPayload
	}

	return params, nil
}

func isValidCourtParams(params CourtParams) bool {
	return params.SportID != "" &&
		params.Name != "" &&
		isValidLocationType(params.LocationType) &&
		params.PricePerHour >= 0
}

func optionalString(value string) *string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return nil
	}

	return &trimmed
}

func isValidLocationType(locationType string) bool {
	switch locationType {
	case "INDOOR", "OUTDOOR":
		return true
	default:
		return false
	}
}

func isWritableCourtStatus(status string) bool {
	switch status {
	case "ACTIVE", "INACTIVE", "MAINTENANCE":
		return true
	default:
		return false
	}
}

func toCourtResponse(court Court) CourtResponse {
	return CourtResponse{
		ID:      court.ID,
		VenueID: court.VenueID,
		Sport: SportResponse{
			ID:   court.Sport.ID,
			Name: court.Sport.Name,
		},
		Name:         court.Name,
		Description:  court.Description,
		LocationType: court.LocationType,
		SurfaceType:  court.SurfaceType,
		PricePerHour: court.PricePerHour,
		Status:       court.Status,
		CreatedAt:    court.CreatedAt,
		UpdatedAt:    court.UpdatedAt,
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
