package venues

import (
	"context"
	"errors"
	"net/url"
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

func (s *Service) GetSports(ctx context.Context) ([]SportResponse, error) {
	sports, err := s.repository.GetSports(ctx)
	if err != nil {
		return nil, err
	}
	var res []SportResponse
	for _, sp := range sports {
		res = append(res, SportResponse{
			ID:   sp.ID,
			Name: sp.Name,
		})
	}
	return res, nil
}

func (s *Service) GetFacilities(ctx context.Context) ([]FacilityResponse, error) {
	facilities, err := s.repository.GetFacilities(ctx)
	if err != nil {
		return nil, err
	}
	var res []FacilityResponse
	for _, f := range facilities {
		res = append(res, FacilityResponse{
			ID:   f.ID,
			Name: f.Name,
			Icon: f.Icon,
		})
	}
	return res, nil
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

	return toVenueResponse(venue, facilities, nil), nil
}

func normalizeListPublicVenuesQuery(req ListPublicVenuesQuery) (ListPublicVenuesQuery, int) {
	if req.Limit <= 0 {
		req.Limit = 10
	}
	if req.Page <= 0 {
		req.Page = 1
	}
	req.Q = strings.TrimSpace(req.Q)
	if len(req.Q) < 2 {
		req.Q = ""
	}
	offset := (req.Page - 1) * req.Limit
	return req, offset
}

func (s *Service) GetPublicVenues(ctx context.Context, req ListPublicVenuesQuery) ([]PublicVenueResponse, int, error) {
	req, offset := normalizeListPublicVenuesQuery(req)

	venues, total, err := s.repository.ListPublicVenues(ctx, req, offset)
	if err != nil {
		return nil, 0, err
	}

	venueIDs := make([]string, 0, len(venues))
	for _, venue := range venues {
		venueIDs = append(venueIDs, venue.ID)
	}

	facilitiesMap, err := s.repository.FindFacilitiesByVenueIDs(ctx, venueIDs)
	if err != nil {
		return nil, 0, err
	}

	photosMap, err := s.repository.FindPhotosByVenueIDs(ctx, venueIDs)
	if err != nil {
		return nil, 0, err
	}

	responses := make([]PublicVenueResponse, 0, len(venues))
	for _, venue := range venues {
		facilities := facilitiesMap[venue.ID]
		if facilities == nil {
			facilities = []Facility{}
		}
		photos := photosMap[venue.ID]
		if photos == nil {
			photos = []VenuePhoto{}
		}
		responses = append(responses, toPublicVenueResponse(venue, facilities, photos))
	}

	return responses, total, nil
}

func (s *Service) GetPublicVenue(ctx context.Context, venueID string) (PublicVenueDetailResponse, error) {
	venue, err := s.repository.FindPublicVenueByID(ctx, venueID)
	if IsNotFound(err) {
		return PublicVenueDetailResponse{}, ErrVenueNotFound
	}
	if err != nil {
		return PublicVenueDetailResponse{}, err
	}

	facilities, err := s.repository.FindFacilitiesByVenueID(ctx, venue.ID)
	if err != nil {
		return PublicVenueDetailResponse{}, err
	}

	photos, err := s.repository.GetVenuePhotos(ctx, venue.ID)
	if err != nil {
		return PublicVenueDetailResponse{}, err
	}

	courts, err := s.repository.FindActiveCourtsByVenueID(ctx, venue.ID)
	if err != nil {
		return PublicVenueDetailResponse{}, err
	}

	return PublicVenueDetailResponse{
		PublicVenueResponse: toPublicVenueResponse(venue, facilities, photos),
		Photos:              toVenuePhotoResponses(photos),
		Courts:              toPublicCourtResponses(courts),
	}, nil
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

	venueIDs := make([]string, 0, len(venues))
	for _, venue := range venues {
		venueIDs = append(venueIDs, venue.ID)
	}

	facilitiesMap, err := s.repository.FindFacilitiesByVenueIDs(ctx, venueIDs)
	if err != nil {
		return nil, err
	}

	photosMap, err := s.repository.FindPhotosByVenueIDs(ctx, venueIDs)
	if err != nil {
		return nil, err
	}

	responses := make([]VenueResponse, 0, len(venues))
	for _, venue := range venues {
		facilities := facilitiesMap[venue.ID]
		if facilities == nil {
			facilities = []Facility{}
		}
		photos := photosMap[venue.ID]
		if photos == nil {
			photos = []VenuePhoto{}
		}
		responses = append(responses, toVenueResponse(venue, facilities, photos))
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

	photos, err := s.repository.GetVenuePhotos(ctx, venue.ID)
	if err != nil {
		return VenueResponse{}, err
	}

	return toVenueResponse(venue, facilities, photos), nil
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

	photos, err := s.repository.GetVenuePhotos(ctx, venue.ID)
	if err != nil {
		return VenueResponse{}, err
	}

	return toVenueResponse(venue, facilities, photos), nil
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

	photos, err := s.repository.GetVenuePhotos(ctx, venue.ID)
	if err != nil {
		return VenueResponse{}, err
	}

	return toVenueResponse(venue, facilities, photos), nil
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

func toVenueResponse(venue Venue, facilities []Facility, photos []VenuePhoto) VenueResponse {
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
		PrimaryPhoto:   getPrimaryPhotoURL(photos),
		Photos:         toVenuePhotoResponses(photos),
		Facilities:     toFacilityResponses(facilities),
		CreatedAt:      venue.CreatedAt,
		UpdatedAt:      venue.UpdatedAt,
	}
}

func toPublicVenueResponse(venue Venue, facilities []Facility, photos []VenuePhoto) PublicVenueResponse {
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
		PrimaryPhoto: getPrimaryPhotoURL(photos),
		Facilities:  toFacilityResponses(facilities),
		CreatedAt:   venue.CreatedAt,
		UpdatedAt:   venue.UpdatedAt,
	}
}

func toVenuePhotoResponses(photos []VenuePhoto) []VenuePhotoResponse {
	var responses []VenuePhotoResponse
	for _, p := range photos {
		responses = append(responses, VenuePhotoResponse{
			ID:        p.ID,
			VenueID:   p.VenueID,
			ImageURL:  p.ImageURL,
			AltText:   p.AltText,
			SortOrder: p.SortOrder,
			IsPrimary: p.IsPrimary,
			CreatedAt: p.CreatedAt,
			UpdatedAt: p.UpdatedAt,
		})
	}
	if responses == nil {
		responses = []VenuePhotoResponse{}
	}
	return responses
}

func getPrimaryPhotoURL(photos []VenuePhoto) *string {
	var firstPhoto *string
	for _, p := range photos {
		if firstPhoto == nil {
			firstPhoto = &p.ImageURL
		}
		if p.IsPrimary {
			return &p.ImageURL
		}
	}
	// Fallback to the first photo if none is primary
	return firstPhoto
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

func toPublicCourtResponses(courts []Court) []PublicCourtResponse {
	responses := make([]PublicCourtResponse, 0, len(courts))
	for _, court := range courts {
		responses = append(responses, PublicCourtResponse{
			ID: court.ID,
			Sport: PublicSportResponse{
				ID:   court.Sport.ID,
				Name: court.Sport.Name,
			},
			Name:         court.Name,
			Description:  court.Description,
			LocationType: court.LocationType,
			SurfaceType:  court.SurfaceType,
			PricePerHour: court.PricePerHour,
			CreatedAt:    court.CreatedAt,
			UpdatedAt:    court.UpdatedAt,
		})
	}
	return responses
}

func (s *Service) AddVenuePhoto(ctx context.Context, userID, venueID string, req CreateVenuePhotoRequest) (VenuePhotoResponse, error) {
	ownerProfile, err := s.getOwnerProfile(ctx, userID)
	if err != nil {
		return VenuePhotoResponse{}, err
	}

	venue, err := s.repository.FindByIDAndOwnerProfileID(ctx, venueID, ownerProfile.ID)
	if IsNotFound(err) {
		return VenuePhotoResponse{}, ErrVenueNotFound
	}
	if err != nil {
		return VenuePhotoResponse{}, err
	}

	if err := validateImageURL(req.ImageURL); err != nil {
		return VenuePhotoResponse{}, err
	}

	isPrimary := false
	if req.IsPrimary != nil {
		isPrimary = *req.IsPrimary
	}

	sortOrder := 0
	if req.SortOrder != nil {
		sortOrder = *req.SortOrder
	}

	photo := VenuePhoto{
		VenueID:   venue.ID,
		ImageURL:  req.ImageURL,
		AltText:   req.AltText,
		SortOrder: sortOrder,
		IsPrimary: isPrimary,
	}

	createdPhoto, err := s.repository.AddVenuePhoto(ctx, photo)
	if err != nil {
		return VenuePhotoResponse{}, err
	}

	return VenuePhotoResponse{
		ID:        createdPhoto.ID,
		VenueID:   createdPhoto.VenueID,
		ImageURL:  createdPhoto.ImageURL,
		AltText:   createdPhoto.AltText,
		SortOrder: createdPhoto.SortOrder,
		IsPrimary: createdPhoto.IsPrimary,
		CreatedAt: createdPhoto.CreatedAt,
		UpdatedAt: createdPhoto.UpdatedAt,
	}, nil
}

func (s *Service) UpdateVenuePhoto(ctx context.Context, userID, venueID, photoID string, req UpdateVenuePhotoRequest) (VenuePhotoResponse, error) {
	ownerProfile, err := s.getOwnerProfile(ctx, userID)
	if err != nil {
		return VenuePhotoResponse{}, err
	}

	venue, err := s.repository.FindByIDAndOwnerProfileID(ctx, venueID, ownerProfile.ID)
	if IsNotFound(err) {
		return VenuePhotoResponse{}, ErrVenueNotFound
	}
	if err != nil {
		return VenuePhotoResponse{}, err
	}

	photo, err := s.repository.GetVenuePhotoByID(ctx, photoID)
	if IsNotFound(err) || photo.VenueID != venue.ID {
		return VenuePhotoResponse{}, errors.New("photo not found")
	}
	if err != nil {
		return VenuePhotoResponse{}, err
	}

	if req.AltText != nil {
		photo.AltText = req.AltText
	}
	if req.SortOrder != nil {
		photo.SortOrder = *req.SortOrder
	}
	if req.IsPrimary != nil {
		photo.IsPrimary = *req.IsPrimary
	}

	err = s.repository.UpdateVenuePhoto(ctx, photo)
	if err != nil {
		return VenuePhotoResponse{}, err
	}

	photo, _ = s.repository.GetVenuePhotoByID(ctx, photoID)

	return VenuePhotoResponse{
		ID:        photo.ID,
		VenueID:   photo.VenueID,
		ImageURL:  photo.ImageURL,
		AltText:   photo.AltText,
		SortOrder: photo.SortOrder,
		IsPrimary: photo.IsPrimary,
		CreatedAt: photo.CreatedAt,
		UpdatedAt: photo.UpdatedAt,
	}, nil
}

func (s *Service) DeleteVenuePhoto(ctx context.Context, userID, venueID, photoID string) error {
	ownerProfile, err := s.getOwnerProfile(ctx, userID)
	if err != nil {
		return err
	}

	venue, err := s.repository.FindByIDAndOwnerProfileID(ctx, venueID, ownerProfile.ID)
	if IsNotFound(err) {
		return ErrVenueNotFound
	}
	if err != nil {
		return err
	}

	photo, err := s.repository.GetVenuePhotoByID(ctx, photoID)
	if IsNotFound(err) || photo.VenueID != venue.ID {
		return errors.New("photo not found")
	}
	if err != nil {
		return err
	}

	return s.repository.DeleteVenuePhoto(ctx, photoID)
}

func validateImageURL(u string) error {
	parsed, err := url.Parse(u)
	if err != nil {
		return errors.New("invalid image URL")
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return errors.New("image URL must use http or https scheme")
	}
	return nil
}
