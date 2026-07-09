package staff

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"golang.org/x/crypto/bcrypt"
)

var (
	ErrEmailAlreadyUsed   = errors.New("email already used")
	ErrPhoneAlreadyUsed   = errors.New("phone already used")
	ErrWeakPassword       = errors.New("password must contain at least one uppercase letter, one lowercase letter, one number, and one special character")
	ErrStaffNotFound      = errors.New("staff not found")
	ErrInvalidVenueAccess = errors.New("invalid venue ID")
	ErrInvalidToken       = errors.New("invalid or expired token")
)

type Service struct {
	repository *Repository
}

func NewService(repository *Repository) *Service {
	return &Service{repository: repository}
}

func generateSecureToken() (string, string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", "", err
	}
	raw := base64.URLEncoding.EncodeToString(b)
	hash := sha256.Sum256([]byte(raw))
	return raw, base64.URLEncoding.EncodeToString(hash[:]), nil
}

func staffSetupPasswordURL(token, requestBaseURL string) string {
	baseURL := strings.TrimRight(strings.TrimSpace(requestBaseURL), "/")
	if baseURL == "" {
		baseURL = strings.TrimRight(strings.TrimSpace(os.Getenv("FRONTEND_BASE_URL")), "/")
	}
	if baseURL == "" {
		baseURL = "http://localhost:3000"
	}
	return baseURL + "/staff/setup-password?token=" + url.QueryEscape(token)
}

func (s *Service) CreateStaff(ctx context.Context, ownerProfileID, actorUserID, frontendBaseURL string, req CreateStaffRequest) (StaffResponse, error) {
	// Create placeholder hash for new staff
	rawPass, _, _ := generateSecureToken()
	passwordHash, err := bcrypt.GenerateFromPassword([]byte(rawPass), bcrypt.DefaultCost)
	if err != nil {
		return StaffResponse{}, err
	}

	rawToken, hashToken, err := generateSecureToken()
	if err != nil {
		return StaffResponse{}, err
	}
	expiresAt := time.Now().Add(24 * time.Hour)

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
		InviteTokenHash: hashToken,
		InviteExpiresAt: expiresAt,
	}

	staff, err := s.repository.CreateStaff(ctx, params)
	if err != nil {
		if IsUniqueViolation(err) {
			switch UniqueViolationConstraint(err) {
			case "users_email_key":
				return StaffResponse{}, ErrEmailAlreadyUsed
			case "users_phone_key":
				return StaffResponse{}, ErrPhoneAlreadyUsed
			}
		}
		if errors.Is(err, ErrInvalidVenueAccess) || IsForeignKeyViolation(err) {
			return StaffResponse{}, ErrInvalidVenueAccess
		}
		return StaffResponse{}, err
	}

	setupURL := staffSetupPasswordURL(rawToken, frontendBaseURL)
	staff.InviteURL = &setupURL

	return staff, nil
}

func (s *Service) RegenerateInvite(ctx context.Context, ownerProfileID, staffID, actorUserID, frontendBaseURL string) (RegenerateInviteResponse, error) {
	staff, err := s.GetStaff(ctx, ownerProfileID, staffID)
	if err != nil {
		return RegenerateInviteResponse{}, err
	}

	if staff.InvitationStatus == "ACTIVE" {
		return RegenerateInviteResponse{}, errors.New("staff already active")
	}

	rawToken, hashToken, err := generateSecureToken()
	if err != nil {
		return RegenerateInviteResponse{}, err
	}

	err = s.repository.InvalidateStaffInvites(ctx, staff.ID, "SET_PASSWORD")
	if err != nil {
		return RegenerateInviteResponse{}, err
	}

	expiresAt := time.Now().Add(24 * time.Hour)
	invite := StaffInvite{
		StaffMemberID:   staff.ID,
		OwnerProfileID:  ownerProfileID,
		StaffUserID:     staff.UserID,
		TokenHash:       hashToken,
		Purpose:         "SET_PASSWORD",
		ExpiresAt:       expiresAt,
		CreatedByUserID: actorUserID,
	}

	_, err = s.repository.CreateStaffInvite(ctx, invite)
	if err != nil {
		return RegenerateInviteResponse{}, err
	}

	err = s.repository.UpdateStaffInvitationStatus(ctx, staff.ID, "INVITED")
	if err != nil {
		return RegenerateInviteResponse{}, err
	}

	return RegenerateInviteResponse{
		InviteURL: staffSetupPasswordURL(rawToken, frontendBaseURL),
		ExpiresAt: expiresAt,
	}, nil
}

func (s *Service) ResetPasswordToken(ctx context.Context, ownerProfileID, staffID, actorUserID, frontendBaseURL string) (ResetStaffPasswordResponse, error) {
	staff, err := s.GetStaff(ctx, ownerProfileID, staffID)
	if err != nil {
		return ResetStaffPasswordResponse{}, err
	}

	if staff.InvitationStatus != "ACTIVE" {
		return ResetStaffPasswordResponse{}, errors.New("staff is not active")
	}

	rawToken, hashToken, err := generateSecureToken()
	if err != nil {
		return ResetStaffPasswordResponse{}, err
	}

	err = s.repository.InvalidateStaffInvites(ctx, staff.ID, "RESET_PASSWORD")
	if err != nil {
		return ResetStaffPasswordResponse{}, err
	}

	expiresAt := time.Now().Add(24 * time.Hour)
	invite := StaffInvite{
		StaffMemberID:   staff.ID,
		OwnerProfileID:  ownerProfileID,
		StaffUserID:     staff.UserID,
		TokenHash:       hashToken,
		Purpose:         "RESET_PASSWORD",
		ExpiresAt:       expiresAt,
		CreatedByUserID: actorUserID,
	}

	_, err = s.repository.CreateStaffInvite(ctx, invite)
	if err != nil {
		return ResetStaffPasswordResponse{}, err
	}

	return ResetStaffPasswordResponse{
		ResetURL:  staffSetupPasswordURL(rawToken, frontendBaseURL),
		ExpiresAt: expiresAt,
	}, nil
}

func (s *Service) SetupPassword(ctx context.Context, req SetupStaffPasswordRequest) (StaffInvite, error) {
	if !isPasswordStrong(req.Password) {
		return StaffInvite{}, ErrWeakPassword
	}

	hashBytes := sha256.Sum256([]byte(req.Token))
	hashToken := base64.URLEncoding.EncodeToString(hashBytes[:])

	// Try SET_PASSWORD first
	invite, err := s.repository.FindStaffInviteByTokenHash(ctx, hashToken, "SET_PASSWORD")
	if err != nil {
		// Try RESET_PASSWORD
		invite, err = s.repository.FindStaffInviteByTokenHash(ctx, hashToken, "RESET_PASSWORD")
		if err != nil {
			return StaffInvite{}, ErrInvalidToken
		}
	}

	if invite.UsedAt != nil || invite.ExpiresAt.Before(time.Now()) {
		return StaffInvite{}, ErrInvalidToken
	}

	passwordHash, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
	if err != nil {
		return StaffInvite{}, err
	}

	err = s.repository.UpdateUserPassword(ctx, invite.StaffUserID, string(passwordHash))
	if err != nil {
		return StaffInvite{}, err
	}

	err = s.repository.MarkStaffInviteUsed(ctx, invite.ID)
	if err != nil {
		return StaffInvite{}, err
	}

	if invite.Purpose == "SET_PASSWORD" {
		err = s.repository.UpdateStaffInvitationStatus(ctx, invite.StaffMemberID, "ACTIVE")
		if err != nil {
			return StaffInvite{}, err
		}
	}

	return invite, nil
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
