package auth

import (
	"context"
	"errors"
	"strings"

	"golang.org/x/crypto/bcrypt"
)

var (
	ErrEmailAlreadyUsed  = errors.New("email already used")
	ErrInvalidCredential = errors.New("invalid email or password")
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

	existingUser, err := s.repository.FindByEmail(ctx, email)
	if err != nil && !IsNotFound(err) {
		return UserResponse{}, err
	}
	if existingUser.ID != "" {
		return UserResponse{}, ErrEmailAlreadyUsed
	}

	passwordHash, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
	if err != nil {
		return UserResponse{}, err
	}

	role := strings.TrimSpace(req.Role)
	if role == "" {
		role = "CUSTOMER"
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
