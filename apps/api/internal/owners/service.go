package owners

import (
	"context"
	"errors"
	"strings"
)

var (
	ErrProfileAlreadyExists = errors.New("owner profile already exists")
	ErrProfileNotFound      = errors.New("owner profile not found")
)

type Service struct {
	repository *Repository
}

func NewService(repository *Repository) *Service {
	return &Service{repository: repository}
}

func (s *Service) CreateProfile(ctx context.Context, userID string, req CreateProfileRequest) (ProfileResponse, error) {
	params := ProfileParams{
		UserID:            userID,
		BusinessName:      strings.TrimSpace(req.BusinessName),
		IdentityNumber:    optionalString(req.IdentityNumber),
		BankName:          optionalString(req.BankName),
		BankAccountNumber: optionalString(req.BankAccountNumber),
		BankAccountName:   optionalString(req.BankAccountName),
	}

	profile, err := s.repository.Create(ctx, params)
	if IsUniqueViolation(err) {
		return ProfileResponse{}, ErrProfileAlreadyExists
	}
	if err != nil {
		return ProfileResponse{}, err
	}

	return toProfileResponse(profile), nil
}

func (s *Service) GetProfile(ctx context.Context, userID string) (ProfileResponse, error) {
	profile, err := s.repository.FindByUserID(ctx, userID)
	if IsNotFound(err) {
		return ProfileResponse{}, ErrProfileNotFound
	}
	if err != nil {
		return ProfileResponse{}, err
	}

	return toProfileResponse(profile), nil
}

func (s *Service) UpdateProfile(ctx context.Context, userID string, req UpdateProfileRequest) (ProfileResponse, error) {
	params := ProfileParams{
		UserID:            userID,
		BusinessName:      strings.TrimSpace(req.BusinessName),
		IdentityNumber:    optionalString(req.IdentityNumber),
		BankName:          optionalString(req.BankName),
		BankAccountNumber: optionalString(req.BankAccountNumber),
		BankAccountName:   optionalString(req.BankAccountName),
	}

	profile, err := s.repository.UpdateByUserID(ctx, params)
	if IsNotFound(err) {
		return ProfileResponse{}, ErrProfileNotFound
	}
	if err != nil {
		return ProfileResponse{}, err
	}

	return toProfileResponse(profile), nil
}

func optionalString(value string) *string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return nil
	}

	return &trimmed
}

func (s *Service) GetMetrics(ctx context.Context, userID string) (OwnerMetricsResponse, error) {
	totalVenues, activeBookings, totalRevenue, err := s.repository.GetMetrics(ctx, userID)
	if err != nil {
		return OwnerMetricsResponse{}, errors.New("failed to retrieve metrics")
	}

	return OwnerMetricsResponse{
		TotalVenues:    totalVenues,
		ActiveBookings: activeBookings,
		TotalRevenue:   totalRevenue,
	}, nil
}

func toProfileResponse(profile Profile) ProfileResponse {
	return ProfileResponse{
		ID:                 profile.ID,
		UserID:             profile.UserID,
		BusinessName:       profile.BusinessName,
		IdentityNumber:     profile.IdentityNumber,
		BankName:           profile.BankName,
		BankAccountNumber:  profile.BankAccountNumber,
		BankAccountName:    profile.BankAccountName,
		VerificationStatus: profile.VerificationStatus,
		VerifiedAt:         profile.VerifiedAt,
		CreatedAt:          profile.CreatedAt,
		UpdatedAt:          profile.UpdatedAt,
	}
}
