package venues

import (
	"context"
	"errors"
	"strings"
)

var (
	ErrOwnerProfileNotFound = errors.New("owner profile not found")
	ErrVenueNotFound        = errors.New("venue not found")
	ErrVenueAlreadyExists   = errors.New("venue already exists")
	ErrInvalidFacilities    = errors.New("invalid facilities")
	ErrInvalidVenuePayload  = errors.New("invalid venue payload")
	ErrInvalidVenueStatus   = errors.New("invalid venue status")
)

type Service struct {
	repository *Repository
}

func NewService(repository *Repository) *Service {
	return &Service{repository: repository}
}

func (s *Service) CreateVenue(ctx context.Context, userID string, req CreateVenueRequest) (VenueResponse, error) {
	ownerProfile, err := s.getOwnerProfile(ctx, userID)
	if err != nil {
		return VenueResponse{}, err
	}

	facilityIDs := normalizeIDs(req.FacilityIDs)
	facilities, err := s.validateFacilities(ctx, facilityIDs)
	if err != nil {
		return VenueResponse{}, err
	}

	params, err := buildCreateVenueParams(ownerProfile.ID, req)
	if err != nil {
		return VenueResponse{}, err
	}

	venue, err := s.repository.Create(ctx, params, facilityIDs)
	if IsUniqueViolation(err) {
		return VenueResponse{}, ErrVenueAlreadyExists
	}
	if err != nil {
		return VenueResponse{}, err
	}

	return toVenueResponse(venue, facilities), nil
}

func (s *Service) GetPublicVenues(ctx context.Context, req ListPublicVenuesQuery) ([]PublicVenueResponse, error) {
	limit := req.Limit
	if limit <= 0 {
		limit = 10
	}
	page := req.Page
	if page <= 0 {
		page = 1
	}
	offset := (page - 1) * limit

	venues, err := s.repository.ListPublicVenues(ctx, limit, offset)
	if err != nil {
		return nil, err
	}

	responses := make([]PublicVenueResponse, 0, len(venues))
	for _, venue := range venues {
		facilities, err := s.repository.FindFacilitiesByVenueID(ctx, venue.ID)
		if err != nil {
			return nil, err
		}
		responses = append(responses, toPublicVenueResponse(venue, facilities))
	}

	return responses, nil
}

func (s *Service) ListVenues(ctx context.Context, userID string) ([]VenueResponse, error) {
	ownerProfile, err := s.getOwnerProfile(ctx, userID)
	if err != nil {
		return nil, err
	}

	venues, err := s.repository.ListByOwnerProfileID(ctx, ownerProfile.ID)
	if err != nil {
		return nil, err
	}

	responses := make([]VenueResponse, 0, len(venues))
	for _, venue := range venues {
		facilities, err := s.repository.FindFacilitiesByVenueID(ctx, venue.ID)
		if err != nil {
			return nil, err
		}
		responses = append(responses, toVenueResponse(venue, facilities))
	}

	return responses, nil
}

func (s *Service) GetVenue(ctx context.Context, userID, venueID string) (VenueResponse, error) {
	ownerProfile, err := s.getOwnerProfile(ctx, userID)
	if err != nil {
		return VenueResponse{}, err
	}

	venue, err := s.repository.FindByIDAndOwnerProfileID(ctx, venueID, ownerProfile.ID)
	if IsNotFound(err) {
		return VenueResponse{}, ErrVenueNotFound
	}
	if err != nil {
		return VenueResponse{}, err
	}

	facilities, err := s.repository.FindFacilitiesByVenueID(ctx, venue.ID)
	if err != nil {
		return VenueResponse{}, err
	}

	return toVenueResponse(venue, facilities), nil
}

func (s *Service) UpdateVenue(ctx context.Context, userID, venueID string, req UpdateVenueRequest) (VenueResponse, error) {
	ownerProfile, err := s.getOwnerProfile(ctx, userID)
	if err != nil {
		return VenueResponse{}, err
	}

	facilityIDs := normalizeIDs(req.FacilityIDs)
	facilities, err := s.validateFacilities(ctx, facilityIDs)
	if err != nil {
		return VenueResponse{}, err
	}

	params, err := buildUpdateVenueParams(ownerProfile.ID, req)
	if err != nil {
		return VenueResponse{}, err
	}

	venue, err := s.repository.UpdateByIDAndOwnerProfileID(ctx, venueID, params, facilityIDs)
	if IsNotFound(err) {
		return VenueResponse{}, ErrVenueNotFound
	}
	if IsUniqueViolation(err) {
		return VenueResponse{}, ErrVenueAlreadyExists
	}
	if err != nil {
		return VenueResponse{}, err
	}

	return toVenueResponse(venue, facilities), nil
}

func (s *Service) UpdateVenueStatus(ctx context.Context, userID, venueID, status string) (VenueResponse, error) {
	ownerProfile, err := s.getOwnerProfile(ctx, userID)
	if err != nil {
		return VenueResponse{}, err
	}

	status = strings.TrimSpace(status)
	if !isOwnerWritableStatus(status) {
		return VenueResponse{}, ErrInvalidVenueStatus
	}

	venue, err := s.repository.UpdateStatusByIDAndOwnerProfileID(ctx, venueID, ownerProfile.ID, status)
	if IsNotFound(err) {
		return VenueResponse{}, ErrVenueNotFound
	}
	if err != nil {
		return VenueResponse{}, err
	}

	facilities, err := s.repository.FindFacilitiesByVenueID(ctx, venue.ID)
	if err != nil {
		return VenueResponse{}, err
	}

	return toVenueResponse(venue, facilities), nil
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

func (s *Service) validateFacilities(ctx context.Context, facilityIDs []string) ([]Facility, error) {
	if len(facilityIDs) == 0 {
		return []Facility{}, nil
	}

	facilities, err := s.repository.FindFacilitiesByIDs(ctx, facilityIDs)
	if err != nil {
		return nil, err
	}
	if len(facilities) != len(facilityIDs) {
		return nil, ErrInvalidFacilities
	}

	return facilities, nil
}

func buildCreateVenueParams(ownerProfileID string, req CreateVenueRequest) (VenueParams, error) {
	params := VenueParams{
		OwnerProfileID: ownerProfileID,
		Name:           strings.TrimSpace(req.Name),
		Description:    optionalString(req.Description),
		Address:        strings.TrimSpace(req.Address),
		District:       optionalString(req.District),
		City:           strings.TrimSpace(req.City),
		Province:       optionalString(req.Province),
		PostalCode:     optionalString(req.PostalCode),
		Latitude:       req.Latitude,
		Longitude:      req.Longitude,
	}

	if params.Name == "" || params.Address == "" || params.City == "" {
		return VenueParams{}, ErrInvalidVenuePayload
	}

	return params, nil
}

func buildUpdateVenueParams(ownerProfileID string, req UpdateVenueRequest) (VenueParams, error) {
	params := VenueParams{
		OwnerProfileID: ownerProfileID,
		Name:           strings.TrimSpace(req.Name),
		Description:    optionalString(req.Description),
		Address:        strings.TrimSpace(req.Address),
		District:       optionalString(req.District),
		City:           strings.TrimSpace(req.City),
		Province:       optionalString(req.Province),
		PostalCode:     optionalString(req.PostalCode),
		Latitude:       req.Latitude,
		Longitude:      req.Longitude,
	}

	if params.Name == "" || params.Address == "" || params.City == "" {
		return VenueParams{}, ErrInvalidVenuePayload
	}

	return params, nil
}

func optionalString(value string) *string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return nil
	}

	return &trimmed
}

func normalizeIDs(ids []string) []string {
	seen := make(map[string]bool, len(ids))
	normalized := make([]string, 0, len(ids))
	for _, id := range ids {
		id = strings.TrimSpace(id)
		if id == "" || seen[id] {
			continue
		}

		seen[id] = true
		normalized = append(normalized, id)
	}

	return normalized
}

func isOwnerWritableStatus(status string) bool {
	switch status {
	case "DRAFT", "ACTIVE", "INACTIVE":
		return true
	default:
		return false
	}
}

func toVenueResponse(venue Venue, facilities []Facility) VenueResponse {
	return VenueResponse{
		ID:             venue.ID,
		OwnerProfileID: venue.OwnerProfileID,
		Name:           venue.Name,
		Description:    venue.Description,
		Address:        venue.Address,
		District:       venue.District,
		City:           venue.City,
		Province:       venue.Province,
		PostalCode:     venue.PostalCode,
		Latitude:       venue.Latitude,
		Longitude:      venue.Longitude,
		Status:         venue.Status,
		Facilities:     toFacilityResponses(facilities),
		CreatedAt:      venue.CreatedAt,
		UpdatedAt:      venue.UpdatedAt,
	}
}

func toPublicVenueResponse(venue Venue, facilities []Facility) PublicVenueResponse {
	return PublicVenueResponse{
		ID:          venue.ID,
		Name:        venue.Name,
		Description: venue.Description,
		Address:     venue.Address,
		District:    venue.District,
		City:        venue.City,
		Province:    venue.Province,
		PostalCode:  venue.PostalCode,
		Latitude:    venue.Latitude,
		Longitude:   venue.Longitude,
		Facilities:  toFacilityResponses(facilities),
		CreatedAt:   venue.CreatedAt,
		UpdatedAt:   venue.UpdatedAt,
	}
}

func toFacilityResponses(facilities []Facility) []FacilityResponse {
	responses := make([]FacilityResponse, 0, len(facilities))
	for _, facility := range facilities {
		responses = append(responses, FacilityResponse{
			ID:   facility.ID,
			Name: facility.Name,
			Icon: facility.Icon,
		})
	}

	return responses
}
