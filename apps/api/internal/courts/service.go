package courts

import (
	"context"
	"errors"
	"strings"
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

func (s *Service) CreateCourt(ctx context.Context, userID, venueID string, req CreateCourtRequest) (CourtResponse, error) {
	ownerProfile, err := s.getOwnerProfile(ctx, userID)
	if err != nil {
		return CourtResponse{}, err
	}

	if _, err := s.getOwnedVenue(ctx, venueID, ownerProfile.ID); err != nil {
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

func (s *Service) ListCourts(ctx context.Context, userID, venueID string) ([]CourtResponse, error) {
	ownerProfile, err := s.getOwnerProfile(ctx, userID)
	if err != nil {
		return nil, err
	}

	if _, err := s.getOwnedVenue(ctx, venueID, ownerProfile.ID); err != nil {
		return nil, err
	}

	courts, err := s.repository.ListByVenueIDAndOwnerProfileID(ctx, venueID, ownerProfile.ID)
	if err != nil {
		return nil, err
	}

	responses := make([]CourtResponse, 0, len(courts))
	for _, court := range courts {
		responses = append(responses, toCourtResponse(court))
	}

	return responses, nil
}

func (s *Service) GetCourt(ctx context.Context, userID, courtID string) (CourtResponse, error) {
	ownerProfile, err := s.getOwnerProfile(ctx, userID)
	if err != nil {
		return CourtResponse{}, err
	}

	court, err := s.repository.FindByIDAndOwnerProfileID(ctx, courtID, ownerProfile.ID)
	if IsNotFound(err) {
		return CourtResponse{}, ErrCourtNotFound
	}
	if err != nil {
		return CourtResponse{}, err
	}

	return toCourtResponse(court), nil
}

func (s *Service) UpdateCourt(ctx context.Context, userID, courtID string, req UpdateCourtRequest) (CourtResponse, error) {
	ownerProfile, err := s.getOwnerProfile(ctx, userID)
	if err != nil {
		return CourtResponse{}, err
	}

	if _, err := s.getSport(ctx, req.SportID); err != nil {
		return CourtResponse{}, err
	}

	params, err := buildUpdateCourtParams(req)
	if err != nil {
		return CourtResponse{}, err
	}

	court, err := s.repository.UpdateByIDAndOwnerProfileID(ctx, courtID, ownerProfile.ID, params)
	if IsNotFound(err) {
		return CourtResponse{}, ErrCourtNotFound
	}
	if IsUniqueViolation(err) {
		return CourtResponse{}, ErrCourtAlreadyExists
	}
	if err != nil {
		return CourtResponse{}, err
	}

	return toCourtResponse(court), nil
}

func (s *Service) UpdateCourtStatus(ctx context.Context, userID, courtID, status string) (CourtResponse, error) {
	ownerProfile, err := s.getOwnerProfile(ctx, userID)
	if err != nil {
		return CourtResponse{}, err
	}

	status = strings.TrimSpace(status)
	if !isWritableCourtStatus(status) {
		return CourtResponse{}, ErrInvalidCourtStatus
	}

	court, err := s.repository.UpdateStatusByIDAndOwnerProfileID(ctx, courtID, ownerProfile.ID, status)
	if IsNotFound(err) {
		return CourtResponse{}, ErrCourtNotFound
	}
	if err != nil {
		return CourtResponse{}, err
	}

	return toCourtResponse(court), nil
}

func (s *Service) getOwnerProfile(ctx context.Context, userID string) (OwnerProfile, error) {
	profile, err := s.repository.FindOwnerProfileByUserID(ctx, userID)
	if IsNotFound(err) {
		return OwnerProfile{}, ErrOwnerProfileNotFound
	}
	if err != nil {
		return OwnerProfile{}, err
	}

	return profile, nil
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
