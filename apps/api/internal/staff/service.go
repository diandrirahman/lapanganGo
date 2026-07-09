package staff

import (
	"context"
	"errors"
	"strings"

	"github.com/jackc/pgx/v5"
	"golang.org/x/crypto/bcrypt"
)

var (
	ErrEmailAlreadyUsed   = errors.New("email already used")
	ErrPhoneAlreadyUsed   = errors.New("phone already used")
	ErrWeakPassword       = errors.New("password must contain at least one uppercase letter, one lowercase letter, one number, and one special character")
	ErrStaffNotFound      = errors.New("staff not found")
	ErrInvalidVenueAccess = errors.New("invalid venue ID")
)

type Service struct {
	repository *Repository
}

func NewService(repository *Repository) *Service {
	return &Service{repository: repository}
}

func (s *Service) CreateStaff(ctx context.Context, ownerProfileID, actorUserID string, req CreateStaffRequest) (StaffResponse, error) {
	if !isPasswordStrong(req.Password) {
		return StaffResponse{}, ErrWeakPassword
	}

	passwordHash, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
	if err != nil {
		return StaffResponse{}, err
	}

	var phone *string
	if req.Phone != nil && strings.TrimSpace(*req.Phone) != "" {
		val := strings.TrimSpace(*req.Phone)
		phone = &val
	}

	params := CreateStaffParams{
		OwnerProfileID:  ownerProfileID,
		Name:            strings.TrimSpace(req.Name),
		Email:           strings.ToLower(strings.TrimSpace(req.Email)),
		Phone:           phone,
		PasswordHash:    string(passwordHash),
		Role:            req.Role,
		Permissions:     req.Permissions,
		VenueIDs:        req.VenueIDs,
		CreatedByUserID: actorUserID,
	}

	staff, err := s.repository.CreateStaff(ctx, params)
	if err != nil {
		if IsUniqueViolation(err) {
			// Actually need to check which constraint failed, but typically it's email.
			return StaffResponse{}, ErrEmailAlreadyUsed
		}
		if errors.Is(err, ErrInvalidVenueAccess) || IsForeignKeyViolation(err) {
			return StaffResponse{}, ErrInvalidVenueAccess
		}
		return StaffResponse{}, err
	}

	return staff, nil
}

func (s *Service) ListStaff(ctx context.Context, ownerProfileID string) ([]StaffResponse, error) {
	return s.repository.ListStaffByOwner(ctx, ownerProfileID)
}

func (s *Service) GetStaff(ctx context.Context, ownerProfileID, staffID string) (StaffResponse, error) {
	staff, err := s.repository.GetStaffByID(ctx, ownerProfileID, staffID)
	if err != nil {
		if IsNotFound(err) {
			return StaffResponse{}, ErrStaffNotFound
		}
		return StaffResponse{}, err
	}
	return staff, nil
}

func (s *Service) UpdateStaff(ctx context.Context, ownerProfileID, staffID string, req UpdateStaffRequest) (StaffResponse, error) {
	var phone *string
	if req.Phone != nil && strings.TrimSpace(*req.Phone) != "" {
		val := strings.TrimSpace(*req.Phone)
		phone = &val
	}

	params := UpdateStaffParams{
		ID:             staffID,
		OwnerProfileID: ownerProfileID,
		Name:           strings.TrimSpace(req.Name),
		Phone:          phone,
		Role:           req.Role,
		Permissions:    req.Permissions,
		VenueIDs:       req.VenueIDs,
	}

	staff, err := s.repository.UpdateStaff(ctx, params)
	if err != nil {
		if IsNotFound(err) {
			return StaffResponse{}, ErrStaffNotFound
		}
		if IsUniqueViolation(err) {
			return StaffResponse{}, ErrPhoneAlreadyUsed
		}
		if errors.Is(err, ErrInvalidVenueAccess) || IsForeignKeyViolation(err) {
			return StaffResponse{}, ErrInvalidVenueAccess
		}
		return StaffResponse{}, err
	}
	return staff, nil
}

func (s *Service) UpdateStatus(ctx context.Context, ownerProfileID, staffID string, req UpdateStaffStatusRequest) (StaffResponse, error) {
	staff, err := s.repository.UpdateStatus(ctx, ownerProfileID, staffID, req.Status)
	if err != nil {
		if IsNotFound(err) {
			return StaffResponse{}, ErrStaffNotFound
		}
		return StaffResponse{}, err
	}
	return staff, nil
}

func (s *Service) UpdateVenues(ctx context.Context, ownerProfileID, staffID string, req UpdateStaffVenuesRequest) (StaffResponse, error) {
	staff, err := s.repository.UpdateVenues(ctx, ownerProfileID, staffID, req.VenueIDs)
	if err != nil {
		if err == pgx.ErrNoRows {
			return StaffResponse{}, ErrStaffNotFound
		}
		if errors.Is(err, ErrInvalidVenueAccess) || IsForeignKeyViolation(err) {
			return StaffResponse{}, ErrInvalidVenueAccess
		}
		return StaffResponse{}, err
	}
	return staff, nil
}

func isPasswordStrong(password string) bool {
	if len(password) < 8 {
		return false
	}
	var hasUpper, hasLower, hasNumber, hasSpecial bool
	for _, char := range password {
		switch {
		case char >= 'A' && char <= 'Z':
			hasUpper = true
		case char >= 'a' && char <= 'z':
			hasLower = true
		case char >= '0' && char <= '9':
			hasNumber = true
		default:
			hasSpecial = true
		}
	}
	return hasUpper && hasLower && hasNumber && hasSpecial
}
