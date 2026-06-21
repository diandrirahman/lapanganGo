package auth

import (
	"context"
	"errors"
	"strings"

	"golang.org/x/crypto/bcrypt"
)

var (
	ErrEmailAlreadyUsed            = errors.New("email already used")
	ErrPhoneAlreadyUsed            = errors.New("phone already used")
	ErrInvalidCredential           = errors.New("invalid email or password")
	ErrUnsupportedRegistrationRole = errors.New("unsupported registration role")
	ErrWeakPassword                = errors.New("password must contain at least one uppercase letter, one lowercase letter, one number, and one special character")
)

type Service struct {
	repository *Repository
	token      *TokenService
}

func NewService(repository *Repository, token *TokenService) *Service {
	return &Service{
		repository: repository,
		token:      token,
	}
}

func (s *Service) Register(ctx context.Context, req RegisterRequest) (UserResponse, error) {
	email := normalizeEmail(req.Email)
	role := "CUSTOMER"

	existingUser, err := s.repository.FindByEmail(ctx, email)
	if err != nil && !IsNotFound(err) {
		return UserResponse{}, err
	}
	if existingUser.ID != "" {
		return UserResponse{}, ErrEmailAlreadyUsed
	}

	if !isPasswordStrong(req.Password) {
		return UserResponse{}, ErrWeakPassword
	}

	passwordHash, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
	if err != nil {
		return UserResponse{}, err
	}

	var phone *string
	if strings.TrimSpace(req.Phone) != "" {
		value := strings.TrimSpace(req.Phone)
		phone = &value
	}

	user, err := s.repository.CreateUser(ctx, CreateUserParams{
		Name:         strings.TrimSpace(req.Name),
		Email:        email,
		Phone:        phone,
		PasswordHash: string(passwordHash),
		Role:         role,
	})
	if err != nil {
		if IsUniqueViolation(err) {
			switch UniqueViolationConstraint(err) {
			case "users_email_key":
				return UserResponse{}, ErrEmailAlreadyUsed
			case "users_phone_key":
				return UserResponse{}, ErrPhoneAlreadyUsed
			}
		}
		return UserResponse{}, err
	}

	return toUserResponse(user), nil
}

func (s *Service) Login(ctx context.Context, req LoginRequest) (LoginResponse, error) {
	email := normalizeEmail(req.Email)

	user, err := s.repository.FindByEmail(ctx, email)
	if IsNotFound(err) {
		return LoginResponse{}, ErrInvalidCredential
	}
	if err != nil {
		return LoginResponse{}, err
	}

	err = bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(req.Password))
	if err != nil {
		return LoginResponse{}, ErrInvalidCredential
	}

	userResponse := toUserResponse(user)
	token, err := s.token.Generate(userResponse)
	if err != nil {
		return LoginResponse{}, err
	}

	return LoginResponse{
		Message: "Login successful",
		Token:   token,
		User:    userResponse,
	}, nil
}

func (s *Service) GetUserByEmail(ctx context.Context, email string) (UserResponse, error) {
	user, err := s.repository.FindByEmail(ctx, normalizeEmail(email))
	if err != nil {
		return UserResponse{}, err
	}

	return toUserResponse(user), nil
}

func normalizeEmail(email string) string {
	return strings.ToLower(strings.TrimSpace(email))
}

func toUserResponse(user User) UserResponse {
	return UserResponse{
		ID:        user.ID,
		Name:      user.Name,
		Email:     user.Email,
		Phone:     user.Phone,
		Role:      user.Role,
		Status:    user.Status,
		CreatedAt: user.CreatedAt,
	}
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
